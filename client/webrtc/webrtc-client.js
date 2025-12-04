/**
 * WebRTC Client for Camera Streaming
 */
class WebRTCClient {
    constructor(config) {
        this.config = config;
        this.peerConnection = null;
        this.dataChannel = null;
        this.localStream = null;
        this.remoteStream = null;
        this.iceCandidates = [];
        this.iceGatheringComplete = false;
        
        // State
        this.clientId = null;
        this.currentSerial = null;
        this.currentBrand = null;
        
        // Event callbacks
        this.onIceCandidate = null;
        this.onIceGatheringComplete = null;
        this.onTrack = null;
        this.onDataChannelOpen = null;
        this.onDataChannelMessage = null;
        this.onDataChannelClose = null;
        this.onConnectionStateChange = null;
        this.onError = null;
    }

    /**
     * Initialize WebRTC peer connection
     * @param {string} clientId - Unique client identifier
     */
    initialize(clientId) {
        this.clientId = clientId;
        this.createPeerConnection();
    }

    /**
     * Create RTCPeerConnection with configured ICE servers
     */
    createPeerConnection() {
        const rtcConfig = {
            iceServers: this.config.webrtc.iceServers,
            iceCandidatePoolSize: 10
        };

        this.log('Creating PeerConnection with config:', rtcConfig);
        this.peerConnection = new RTCPeerConnection(rtcConfig);
        this.setupPeerConnectionEvents();
    }

    /**
     * Setup event handlers for peer connection
     */
    setupPeerConnectionEvents() {
        // ICE candidate event
        this.peerConnection.onicecandidate = (event) => {
            if (event.candidate) {
                this.log('ICE candidate:', event.candidate.candidate);
                this.iceCandidates.push(event.candidate);
                
                if (this.onIceCandidate) {
                    this.onIceCandidate(event.candidate);
                }
            } else {
                this.log('ICE gathering complete');
                this.iceGatheringComplete = true;
                
                if (this.onIceGatheringComplete) {
                    this.onIceGatheringComplete(this.iceCandidates);
                }
            }
        };

        // ICE gathering state change
        this.peerConnection.onicegatheringstatechange = () => {
            this.log('ICE gathering state:', this.peerConnection.iceGatheringState);
        };

        // ICE connection state change
        this.peerConnection.oniceconnectionstatechange = () => {
            this.log('ICE connection state:', this.peerConnection.iceConnectionState);
            
            if (this.onConnectionStateChange) {
                this.onConnectionStateChange(this.peerConnection.iceConnectionState);
            }

            if (this.peerConnection.iceConnectionState === 'failed') {
                this.logError('ICE connection failed');
                if (this.onError) this.onError(new Error('ICE connection failed'));
            }
        };

        // Connection state change
        this.peerConnection.onconnectionstatechange = () => {
            this.log('Connection state:', this.peerConnection.connectionState);
        };

        // Track event (remote media)
        this.peerConnection.ontrack = (event) => {
            this.log('Received remote track:', event.track.kind);
            
            if (!this.remoteStream) {
                this.remoteStream = new MediaStream();
            }
            this.remoteStream.addTrack(event.track);
            
            if (this.onTrack) {
                this.onTrack(event.track, event.streams);
            }
        };

        // Data channel event (when remote creates data channel)
        this.peerConnection.ondatachannel = (event) => {
            this.log('Received data channel:', event.channel.label);
            this.setupDataChannel(event.channel);
        };
    }

    /**
     * Create and setup data channel
     * @param {string} label - Data channel label
     * @returns {RTCDataChannel}
     */
    createDataChannel(label = null) {
        const channelLabel = label || this.config.webrtc.dataChannel.label;
        const options = {
            ordered: this.config.webrtc.dataChannel.ordered
        };

        this.log('Creating data channel:', channelLabel);
        this.dataChannel = this.peerConnection.createDataChannel(channelLabel, options);
        this.setupDataChannel(this.dataChannel);
        
        return this.dataChannel;
    }

    /**
     * Setup data channel event handlers
     * @param {RTCDataChannel} channel - Data channel to setup
     */
    setupDataChannel(channel) {
        this.dataChannel = channel;

        channel.onopen = () => {
            this.log('Data channel opened');
            if (this.onDataChannelOpen) this.onDataChannelOpen();
        };

        channel.onclose = () => {
            this.log('Data channel closed');
            if (this.onDataChannelClose) this.onDataChannelClose();
        };

        channel.onmessage = (event) => {
            this.log('Data channel message:', event.data);
            
            let data;
            try {
                data = JSON.parse(event.data);
            } catch {
                data = event.data;
            }
            
            if (this.onDataChannelMessage) {
                this.onDataChannelMessage(data);
            }
        };

        channel.onerror = (error) => {
            this.logError('Data channel error:', error);
            if (this.onError) this.onError(error);
        };
    }

    /**
     * Create SDP Offer
     * @param {object} options - Offer options
     * @returns {Promise<RTCSessionDescriptionInit>}
     */
    async createOffer(options = {}) {
        try {
            // Add transceivers for receiving video and audio
            if (!options.dataChannelOnly) {
                this.peerConnection.addTransceiver('video', { direction: 'recvonly' });
                this.peerConnection.addTransceiver('audio', { direction: 'recvonly' });
            }

            // Create data channel before offer
            if (options.createDataChannel !== false) {
                this.createDataChannel();
            }

            const offer = await this.peerConnection.createOffer({
                offerToReceiveVideo: !options.dataChannelOnly,
                offerToReceiveAudio: !options.dataChannelOnly
            });

            this.log('Created offer:', offer.sdp);
            
            await this.peerConnection.setLocalDescription(offer);
            this.log('Set local description (offer)');

            return offer;
        } catch (error) {
            this.logError('Failed to create offer:', error);
            throw error;
        }
    }

    /**
     * Create SDP Answer (when receiving offer from camera)
     * @returns {Promise<RTCSessionDescriptionInit>}
     */
    async createAnswer() {
        try {
            const answer = await this.peerConnection.createAnswer();
            this.log('Created answer:', answer.sdp);
            
            await this.peerConnection.setLocalDescription(answer);
            this.log('Set local description (answer)');

            return answer;
        } catch (error) {
            this.logError('Failed to create answer:', error);
            throw error;
        }
    }

    /**
     * Set remote description (Offer from camera or Answer from camera)
     * @param {string} sdp - SDP string
     * @param {string} type - 'offer' or 'answer'
     */
    async setRemoteDescription(sdp, type) {
        try {
            const description = new RTCSessionDescription({
                type: type,
                sdp: sdp
            });

            await this.peerConnection.setRemoteDescription(description);
            this.log(`Set remote description (${type})`);
        } catch (error) {
            this.logError('Failed to set remote description:', error);
            throw error;
        }
    }

    /**
     * Add ICE candidate
     * @param {object} candidate - ICE candidate object
     */
    async addIceCandidate(candidate) {
        try {
            if (candidate && candidate.candidate) {
                const iceCandidate = new RTCIceCandidate(candidate);
                await this.peerConnection.addIceCandidate(iceCandidate);
                this.log('Added ICE candidate');
            }
        } catch (error) {
            this.logError('Failed to add ICE candidate:', error);
            throw error;
        }
    }

    /**
     * Wait for ICE gathering to complete
     * @param {number} timeout - Timeout in milliseconds
     * @returns {Promise<RTCIceCandidate[]>}
     */
    async waitForIceGathering(timeout = null) {
        const gatherTimeout = timeout || this.config.webrtc.iceGatheringTimeout;
        
        return new Promise((resolve, reject) => {
            if (this.iceGatheringComplete) {
                resolve(this.iceCandidates);
                return;
            }

            const timeoutId = setTimeout(() => {
                this.log('ICE gathering timeout, proceeding with collected candidates');
                resolve(this.iceCandidates);
            }, gatherTimeout);

            const originalCallback = this.onIceGatheringComplete;
            this.onIceGatheringComplete = (candidates) => {
                clearTimeout(timeoutId);
                if (originalCallback) originalCallback(candidates);
                resolve(candidates);
            };
        });
    }

    /**
     * Get local description with gathered ICE candidates
     * @returns {string}
     */
    getLocalDescriptionWithCandidates() {
        if (this.peerConnection.localDescription) {
            return this.peerConnection.localDescription.sdp;
        }
        return null;
    }

    /**
     * Send message via data channel
     * @param {object|string} message - Message to send
     */
    sendDataChannelMessage(message) {
        if (this.dataChannel && this.dataChannel.readyState === 'open') {
            const payload = typeof message === 'string' ? message : JSON.stringify(message);
            this.dataChannel.send(payload);
            this.log('Sent data channel message:', payload);
        } else {
            this.logError('Data channel not open');
        }
    }

    /**
     * Request stream via data channel
     * @param {string} nvrSerial - NVR serial number
     * @param {number} channelMask - Channel bitmask
     * @param {number} resolutionMask - Resolution bitmask
     */
    requestStream(nvrSerial, channelMask, resolutionMask) {
        const message = createStreamRequest(nvrSerial, channelMask, resolutionMask);
        this.sendDataChannelMessage(message);
    }

    /**
     * Request ONVIF status via data channel
     * @param {string} nvrSerial - NVR serial number
     */
    requestOnvifStatus(nvrSerial) {
        const message = createOnvifStatusRequest(nvrSerial);
        this.sendDataChannelMessage(message);
    }

    /**
     * Get remote stream
     * @returns {MediaStream}
     */
    getRemoteStream() {
        return this.remoteStream;
    }

    /**
     * Get connection state
     * @returns {string}
     */
    getConnectionState() {
        return this.peerConnection ? this.peerConnection.connectionState : 'closed';
    }

    /**
     * Get ICE connection state
     * @returns {string}
     */
    getIceConnectionState() {
        return this.peerConnection ? this.peerConnection.iceConnectionState : 'closed';
    }

    /**
     * Close the connection
     */
    close() {
        if (this.dataChannel) {
            this.dataChannel.close();
            this.dataChannel = null;
        }

        if (this.peerConnection) {
            this.peerConnection.close();
            this.peerConnection = null;
        }

        if (this.remoteStream) {
            this.remoteStream.getTracks().forEach(track => track.stop());
            this.remoteStream = null;
        }

        this.iceCandidates = [];
        this.iceGatheringComplete = false;
        
        this.log('Connection closed');
    }

    /**
     * Reset for new connection
     */
    reset() {
        this.close();
        this.createPeerConnection();
    }

    /**
     * Log helper
     */
    log(...args) {
        console.log('[WebRTC]', ...args);
        if (window.addLog) window.addLog('WebRTC', args.join(' '));
    }

    /**
     * Error log helper
     */
    logError(...args) {
        console.error('[WebRTC]', ...args);
        if (window.addLog) window.addLog('WebRTC ERROR', args.join(' '));
    }
}

// Export for module usage
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { WebRTCClient };
}
