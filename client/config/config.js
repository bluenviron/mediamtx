// Client Configuration
const CONFIG = {
    // Adapter API Settings (for ONVIF discovery)
    api: {
        baseUrl: 'http://localhost:8890',
        endpoints: {
            discover: '/api/discover',
            auth: '/api/auth',
            devices: '/api/devices',
            health: '/api/health'
        },
        timeout: 30000
    },

    // MediaMTX Settings
    mediamtx: {
        // WebRTC/WHEP endpoint
        webrtcUrl: 'http://localhost:8889',
        // API endpoint
        apiUrl: 'http://localhost:9997'
    },

    // MQTT Settings
    mqtt: {
        brokerUrl: 'wss://beta-broker-mqtt.fcam.vn:8084/mqtt',
        username: 'hoangbd7',
        password: 'Hoangbd7',
        clientIdPrefix: 'fpt-client-',
        reconnectPeriod: 5000,
        connectTimeout: 30000,
        keepalive: 60,
        clean: true,
        qos: 1
    },

    // Topic Templates (prefix: ipc/fss)
    topics: {
        // Topic prefix
        prefix: 'ipc/fss',
        
        // Discovery topic để lấy danh sách camera
        discovery: 'ipc/fss/discovery',
        
        // Request/Response signaling topics (template)
        // Actual topic: ipc/fss/<serialno>/request/signaling
        requestSignaling: (serial) => `ipc/fss/${serial}/request/signaling`,
        responseSignaling: (serial) => `ipc/fss/${serial}/response/signaling`,
        
        // Credential topic
        credential: (serial) => `ipc/fss/${serial}/credential`
    },

    // WebRTC Settings
    webrtc: {
        iceServers: [
            { urls: 'stun:stun-connect.fcam.vn:3478' },
            { urls: 'stun:stunp-connect.fcam.vn:3478' },
            { urls: 'stun:stun.l.google.com:19302' },
            {
                urls: 'turn:turn-connect.fcam.vn:3478',
                username: 'turnuser',
                credential: 'camfptvnturn133099'
            }
        ],
        // ICE gathering timeout (ms)
        iceGatheringTimeout: 5000,
        // Data channel config
        dataChannel: {
            label: 'control',
            ordered: true
        }
    },

    // UI Settings
    ui: {
        logMaxLines: 100,
        autoScroll: true
    }
};

// Generate unique client ID
function generateClientId() {
    return CONFIG.mqtt.clientIdPrefix + Math.random().toString(36).substring(2, 15);
}

// Export for module usage
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { CONFIG, generateClientId };
}
