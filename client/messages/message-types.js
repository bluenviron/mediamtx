// Message Types for MQTT Communication

/**
 * Signaling Message Types
 */
const SignalingType = {
    REQUEST: 'request',      // Initial request from client
    OFFER: 'offer',          // SDP Offer from camera/NVR
    ANSWER: 'answer',        // SDP Answer from client
    DENY: 'deny',            // Connection denied (max clients reached)
    CCU: 'ccu',              // Concurrent user count update
    ICE_CANDIDATE: 'ice-candidate'  // ICE candidate exchange
};

/**
 * Data Channel Commands
 */
const DataChannelCommand = {
    STREAM: 'Stream',
    ONVIF_STATUS: 'OnvifStatus'
};

/**
 * Result Codes
 */
const ResultCode = {
    SUCCESS: 100,
    FAIL: 103,
    STREAM_SUCCESS: 0
};

/**
 * Create Signaling Request Message
 * @param {string} serial - Camera/NVR serial number
 * @param {string} clientId - Unique client identifier
 * @returns {object} - Signaling request message
 */
function createSignalingRequest(serial, clientId) {
    return {
        Method: 'ACT',
        MessageType: 'Signaling',
        Serial: serial,
        Data: {
            Type: SignalingType.REQUEST,
            ClientId: clientId
        },
        Timestamp: Math.floor(Date.now() / 1000)
    };
}

/**
 * Create SDP Answer Message
 * @param {string} serial - Camera/NVR serial number
 * @param {string} clientId - Unique client identifier
 * @param {string} sdp - SDP Answer string
 * @returns {object} - SDP Answer message
 */
function createAnswerMessage(serial, clientId, sdp) {
    return {
        Method: 'ACT',
        MessageType: 'Signaling',
        Serial: serial,
        Data: {
            Type: SignalingType.ANSWER,
            Sdp: sdp,
            ClientId: clientId
        },
        Timestamp: Math.floor(Date.now() / 1000),
        Result: {
            Ret: ResultCode.SUCCESS,
            Message: 'Success'
        }
    };
}

/**
 * Create ICE Candidate Message
 * @param {string} serial - Camera/NVR serial number
 * @param {string} clientId - Unique client identifier
 * @param {RTCIceCandidate} candidate - ICE candidate
 * @returns {object} - ICE candidate message
 */
function createIceCandidateMessage(serial, clientId, candidate) {
    return {
        Method: 'ACT',
        MessageType: 'Signaling',
        Serial: serial,
        Data: {
            Type: SignalingType.ICE_CANDIDATE,
            ClientId: clientId,
            Candidate: {
                candidate: candidate.candidate,
                sdpMid: candidate.sdpMid,
                sdpMLineIndex: candidate.sdpMLineIndex
            }
        },
        Timestamp: Math.floor(Date.now() / 1000)
    };
}

/**
 * Create Credential Message
 * @param {string} serial - Camera/NVR serial number
 * @param {string} username - Camera username
 * @param {string} password - Camera password
 * @param {string} ip - Camera IP address (optional, will be discovered via ONVIF if not provided)
 * @returns {object} - Credential message
 */
function createCredentialMessage(serial, username, password, ip = '') {
    const data = {
        Username: username,
        Password: password
    };
    
    // Only include IP if provided
    if (ip) {
        data.IP = ip;
    }
    
    return {
        Method: 'ACT',
        MessageType: 'Credential',
        Serial: serial,
        Data: data,
        Timestamp: Math.floor(Date.now() / 1000)
    };
}

/**
 * Create Stream Request for Data Channel
 * @param {string} nvrSerial - NVR serial number
 * @param {number} channelMask - 64-bit bitmask for channel enable (bit0=CH1...bit15=CH16)
 * @param {number} resolutionMask - 64-bit bitmask for stream type (0=sub, 1=main)
 * @returns {object} - Stream request message
 */
function createStreamRequest(nvrSerial, channelMask, resolutionMask) {
    return {
        Id: nvrSerial,
        Command: DataChannelCommand.STREAM,
        Type: 'Request',
        Content: {
            ChannelMask: channelMask,
            ResolutionMask: resolutionMask
        }
    };
}

/**
 * Create ONVIF Status Request for Data Channel
 * @param {string} nvrSerial - NVR serial number
 * @returns {object} - ONVIF status request message
 */
function createOnvifStatusRequest(nvrSerial) {
    return {
        Id: nvrSerial,
        Command: DataChannelCommand.ONVIF_STATUS,
        Type: 'Request',
        Content: {}
    };
}

/**
 * Parse Signaling Response
 * @param {object} message - Received message
 * @returns {object} - Parsed response with type and data
 */
function parseSignalingResponse(message) {
    const result = {
        success: false,
        type: null,
        data: null,
        error: null
    };

    try {
        if (message.Result && message.Result.Ret !== ResultCode.SUCCESS) {
            result.error = message.Result.Message || 'Unknown error';
            return result;
        }

        if (message.Data) {
            result.type = message.Data.Type;
            result.data = message.Data;
            result.success = true;
        }
    } catch (e) {
        result.error = e.message;
    }

    return result;
}

// Export for module usage
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        SignalingType,
        DataChannelCommand,
        ResultCode,
        createSignalingRequest,
        createAnswerMessage,
        createIceCandidateMessage,
        createCredentialMessage,
        createStreamRequest,
        createOnvifStatusRequest,
        parseSignalingResponse
    };
}
