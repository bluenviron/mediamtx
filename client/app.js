/**
 * Main Application Controller
 * Coordinates MQTT signaling and WebRTC connection
 */
class FPTCameraClient {
    constructor() {
        this.mqttClient = null;
        this.webrtcClient = null;
        this.cameras = [];
        this.currentCamera = null;
        this.connectionState = 'disconnected';
        
        // Event callbacks
        this.onCameraListUpdate = null;
        this.onConnectionStateChange = null;
        this.onStreamReady = null;
        this.onError = null;
    }

    /**
     * Initialize the client
     */
    async initialize() {
        this.log('Initializing FPT Camera Client...');
        
        // Create MQTT client
        this.mqttClient = new MQTTSignalingClient(CONFIG);
        this.setupMQTTCallbacks();
        
        // Create WebRTC client
        this.webrtcClient = new WebRTCClient(CONFIG);
        this.setupWebRTCCallbacks();
        
        // Connect to MQTT
        await this.connectMQTT();
        
        this.log('Client initialized');
    }

    /**
     * Setup MQTT event callbacks
     */
    setupMQTTCallbacks() {
        this.mqttClient.onConnected = () => {
            this.updateConnectionState('mqtt_connected');
        };

        this.mqttClient.onDisconnected = () => {
            this.updateConnectionState('mqtt_disconnected');
        };

        this.mqttClient.onError = (error) => {
            this.logError('MQTT Error:', error);
            if (this.onError) this.onError(error);
        };

        this.mqttClient.onReconnecting = () => {
            this.updateConnectionState('mqtt_reconnecting');
        };
    }

    /**
     * Setup WebRTC event callbacks
     */
    setupWebRTCCallbacks() {
        this.webrtcClient.onIceCandidate = (candidate) => {
            // Send ICE candidate via MQTT
            if (this.currentCamera) {
                this.sendIceCandidate(candidate);
            }
        };

        this.webrtcClient.onTrack = (track, streams) => {
            this.log('Remote track received:', track.kind);
            if (this.onStreamReady) {
                this.onStreamReady(this.webrtcClient.getRemoteStream());
            }
        };

        this.webrtcClient.onDataChannelOpen = () => {
            this.log('Data channel opened');
            this.updateConnectionState('data_channel_open');
        };

        this.webrtcClient.onDataChannelMessage = (message) => {
            this.handleDataChannelMessage(message);
        };

        this.webrtcClient.onConnectionStateChange = (state) => {
            this.log('WebRTC connection state:', state);
            this.updateConnectionState('webrtc_' + state);
        };

        this.webrtcClient.onError = (error) => {
            this.logError('WebRTC Error:', error);
            if (this.onError) this.onError(error);
        };
    }

    /**
     * Connect to MQTT broker
     */
    async connectMQTT() {
        try {
            await this.mqttClient.connect();
            this.log('MQTT connected');
            
            // Subscribe to discovery topic
            await this.mqttClient.subscribe(CONFIG.topics.discovery, (message) => {
                this.handleDiscoveryMessage(message);
            });
            
        } catch (error) {
            this.logError('Failed to connect MQTT:', error);
            throw error;
        }
    }

    /**
     * Handle discovery message (camera list)
     * @param {object} message - Discovery message
     */
    handleDiscoveryMessage(message) {
        this.log('Discovery message received:', message);
        
        if (Array.isArray(message)) {
            this.cameras = message;
        } else if (message.cameras) {
            this.cameras = message.cameras;
        }
        
        if (this.onCameraListUpdate) {
            this.onCameraListUpdate(this.cameras);
        }
    }

    /**
     * Send camera credentials for authentication
     * @param {string} serial - Camera serial
     * @param {string} username - Camera username
     * @param {string} password - Camera password
     * @param {string} ip - Camera IP address (optional)
     */
    async sendCredentials(serial, username, password, ip = '') {
        this.log(`Sending credentials for ${serial}` + (ip ? ` (IP: ${ip})` : ''));
        await this.mqttClient.publishCredentials(serial, username, password, ip);
    }

    /**
     * Connect to a camera via WebRTC
     * @param {string} serial - Camera serial
     */
    async connectToCamera(serial) {
        this.log(`Connecting to camera: ${serial}`);
        
        this.currentCamera = { serial };
        this.updateConnectionState('connecting');
        
        // Initialize WebRTC
        this.webrtcClient.initialize(this.mqttClient.getClientId());
        
        // Subscribe to signaling response topic
        await this.mqttClient.subscribeSignaling(serial, (message) => {
            this.handleSignalingResponse(message);
        });
        
        // Send signaling request
        const request = createSignalingRequest(serial, this.mqttClient.getClientId());
        await this.mqttClient.publishSignaling(serial, request);
        
        this.log('Signaling request sent');
    }

    /**
     * Handle signaling response from camera/NVR
     * @param {object} message - Signaling response
     */
    async handleSignalingResponse(message) {
        this.log('Signaling response:', message);
        
        const response = parseSignalingResponse(message);
        
        if (!response.success) {
            this.logError('Signaling failed:', response.error);
            if (this.onError) this.onError(new Error(response.error));
            return;
        }
        
        switch (response.type) {
            case SignalingType.DENY:
                this.handleDenyResponse(response.data);
                break;
                
            case SignalingType.OFFER:
                await this.handleOfferResponse(response.data);
                break;
                
            case SignalingType.CCU:
                this.handleCCUResponse(response.data);
                break;
                
            case SignalingType.ICE_CANDIDATE:
                await this.handleRemoteIceCandidate(response.data);
                break;
                
            default:
                this.log('Unknown signaling type:', response.type);
        }
    }

    /**
     * Handle deny response (max clients reached)
     * @param {object} data - Deny response data
     */
    handleDenyResponse(data) {
        this.logError('Connection denied:', data);
        this.updateConnectionState('denied');
        
        const error = new Error(`Connection denied. Max: ${data.ClientMax}, Current: ${data.CurrentClientsTotal}`);
        if (this.onError) this.onError(error);
    }

    /**
     * Handle offer response from camera
     * @param {object} data - Offer data with SDP
     */
    async handleOfferResponse(data) {
        this.log('Received offer from camera');
        
        try {
            // Set remote description (offer)
            await this.webrtcClient.setRemoteDescription(data.Sdp, 'offer');
            
            // Create answer
            const answer = await this.webrtcClient.createAnswer();
            
            // Wait for ICE gathering
            await this.webrtcClient.waitForIceGathering();
            
            // Get final SDP with candidates
            const finalSdp = this.webrtcClient.getLocalDescriptionWithCandidates();
            
            // Send answer via MQTT
            const answerMessage = createAnswerMessage(
                this.currentCamera.serial,
                this.mqttClient.getClientId(),
                finalSdp
            );
            
            await this.mqttClient.publishSignaling(
                this.currentCamera.serial,
                answerMessage
            );
            
            this.log('Answer sent');
            this.updateConnectionState('answer_sent');
            
        } catch (error) {
            this.logError('Failed to handle offer:', error);
            if (this.onError) this.onError(error);
        }
    }

    /**
     * Handle CCU (concurrent user count) response
     * @param {object} data - CCU data
     */
    handleCCUResponse(data) {
        this.log('CCU update:', data.CurrentClientsTotal);
    }

    /**
     * Handle remote ICE candidate
     * @param {object} data - ICE candidate data
     */
    async handleRemoteIceCandidate(data) {
        if (data.Candidate) {
            await this.webrtcClient.addIceCandidate(data.Candidate);
        }
    }

    /**
     * Send ICE candidate via MQTT
     * @param {RTCIceCandidate} candidate - ICE candidate
     */
    async sendIceCandidate(candidate) {
        if (!this.currentCamera) return;
        
        const message = createIceCandidateMessage(
            this.currentCamera.serial,
            this.mqttClient.getClientId(),
            candidate
        );
        
        await this.mqttClient.publishSignaling(
            this.currentCamera.serial,
            message
        );
    }

    /**
     * Handle data channel message
     * @param {object} message - Data channel message
     */
    handleDataChannelMessage(message) {
        this.log('Data channel message:', message);
        
        if (message.Command === DataChannelCommand.STREAM) {
            this.log('Stream response:', message.Result);
        } else if (message.Command === DataChannelCommand.ONVIF_STATUS) {
            this.log('ONVIF status:', message.Content);
        }
    }

    /**
     * Request stream from NVR
     * @param {number} channelMask - Channel bitmask
     * @param {number} resolutionMask - Resolution bitmask
     */
    requestStream(channelMask, resolutionMask) {
        if (!this.currentCamera) {
            this.logError('No camera connected');
            return;
        }
        
        this.webrtcClient.requestStream(
            this.currentCamera.serial,
            channelMask,
            resolutionMask
        );
    }

    /**
     * Request ONVIF status from NVR
     */
    requestOnvifStatus() {
        if (!this.currentCamera) {
            this.logError('No camera connected');
            return;
        }
        
        this.webrtcClient.requestOnvifStatus(this.currentCamera.serial);
    }

    /**
     * Disconnect from current camera
     */
    disconnect() {
        if (this.currentCamera) {
            // Unsubscribe from signaling topic
            const topic = CONFIG.topics.responseSignaling(this.currentCamera.serial);
            this.mqttClient.unsubscribe(topic);
        }
        
        this.webrtcClient.close();
        this.currentCamera = null;
        this.updateConnectionState('disconnected');
        
        this.log('Disconnected');
    }

    /**
     * Update connection state
     * @param {string} state - New state
     */
    updateConnectionState(state) {
        this.connectionState = state;
        this.log('Connection state:', state);
        
        if (this.onConnectionStateChange) {
            this.onConnectionStateChange(state);
        }
    }

    /**
     * Get current connection state
     * @returns {string}
     */
    getConnectionState() {
        return this.connectionState;
    }

    /**
     * Get remote stream
     * @returns {MediaStream}
     */
    getRemoteStream() {
        return this.webrtcClient.getRemoteStream();
    }

    /**
     * Get camera list
     * @returns {Array}
     */
    getCameras() {
        return this.cameras;
    }

    /**
     * Log helper
     */
    log(...args) {
        console.log('[App]', ...args);
        if (window.addLog) window.addLog('App', args.join(' '));
    }

    /**
     * Error log helper
     */
    logError(...args) {
        console.error('[App]', ...args);
        if (window.addLog) window.addLog('App ERROR', args.join(' '));
    }
}

// Export for module usage
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { FPTCameraClient };
}
