/**
 * MQTT Client Wrapper for WebRTC Signaling
 */
class MQTTSignalingClient {
    constructor(config) {
        this.config = config;
        this.client = null;
        this.clientId = generateClientId();
        this.subscriptions = new Map();
        this.messageHandlers = new Map();
        this.connected = false;
        this.reconnecting = false;
        
        // Event callbacks
        this.onConnected = null;
        this.onDisconnected = null;
        this.onError = null;
        this.onMessage = null;
        this.onReconnecting = null;
    }

    /**
     * Connect to MQTT broker
     * @returns {Promise<void>}
     */
    async connect() {
        return new Promise((resolve, reject) => {
            const options = {
                clientId: this.clientId,
                username: this.config.mqtt.username,
                password: this.config.mqtt.password,
                reconnectPeriod: this.config.mqtt.reconnectPeriod,
                connectTimeout: this.config.mqtt.connectTimeout,
                keepalive: this.config.mqtt.keepalive,
                clean: this.config.mqtt.clean
            };

            this.log('Connecting to MQTT broker...', this.config.mqtt.brokerUrl);
            
            try {
                this.client = mqtt.connect(this.config.mqtt.brokerUrl, options);

                this.client.on('connect', () => {
                    this.connected = true;
                    this.reconnecting = false;
                    this.log('Connected to MQTT broker');
                    
                    // Resubscribe to all topics after reconnect
                    this.resubscribeAll();
                    
                    if (this.onConnected) this.onConnected();
                    resolve();
                });

                this.client.on('reconnect', () => {
                    this.reconnecting = true;
                    this.log('Reconnecting to MQTT broker...');
                    if (this.onReconnecting) this.onReconnecting();
                });

                this.client.on('close', () => {
                    this.connected = false;
                    this.log('Disconnected from MQTT broker');
                    if (this.onDisconnected) this.onDisconnected();
                });

                this.client.on('error', (error) => {
                    this.logError('MQTT error:', error);
                    if (this.onError) this.onError(error);
                    if (!this.connected) reject(error);
                });

                this.client.on('message', (topic, message) => {
                    this.handleMessage(topic, message);
                });

            } catch (error) {
                this.logError('Failed to connect:', error);
                reject(error);
            }
        });
    }

    /**
     * Disconnect from MQTT broker
     */
    disconnect() {
        if (this.client) {
            this.client.end();
            this.connected = false;
            this.log('Disconnected');
        }
    }

    /**
     * Subscribe to a topic
     * @param {string} topic - Topic to subscribe
     * @param {function} handler - Message handler callback
     * @returns {Promise<void>}
     */
    async subscribe(topic, handler) {
        return new Promise((resolve, reject) => {
            if (!this.connected) {
                reject(new Error('Not connected to MQTT broker'));
                return;
            }

            this.client.subscribe(topic, { qos: this.config.mqtt.qos }, (error) => {
                if (error) {
                    this.logError(`Failed to subscribe to ${topic}:`, error);
                    reject(error);
                } else {
                    this.subscriptions.set(topic, handler);
                    this.log(`Subscribed to: ${topic}`);
                    resolve();
                }
            });
        });
    }

    /**
     * Unsubscribe from a topic
     * @param {string} topic - Topic to unsubscribe
     * @returns {Promise<void>}
     */
    async unsubscribe(topic) {
        return new Promise((resolve, reject) => {
            if (!this.connected) {
                reject(new Error('Not connected to MQTT broker'));
                return;
            }

            this.client.unsubscribe(topic, (error) => {
                if (error) {
                    this.logError(`Failed to unsubscribe from ${topic}:`, error);
                    reject(error);
                } else {
                    this.subscriptions.delete(topic);
                    this.log(`Unsubscribed from: ${topic}`);
                    resolve();
                }
            });
        });
    }

    /**
     * Publish a message to a topic
     * @param {string} topic - Topic to publish to
     * @param {object|string} message - Message to publish
     * @param {object} options - Publish options
     * @returns {Promise<void>}
     */
    async publish(topic, message, options = {}) {
        return new Promise((resolve, reject) => {
            if (!this.connected) {
                reject(new Error('Not connected to MQTT broker'));
                return;
            }

            const payload = typeof message === 'string' ? message : JSON.stringify(message);
            const pubOptions = {
                qos: options.qos || this.config.mqtt.qos,
                retain: options.retain || false
            };

            this.client.publish(topic, payload, pubOptions, (error) => {
                if (error) {
                    this.logError(`Failed to publish to ${topic}:`, error);
                    reject(error);
                } else {
                    this.log(`Published to: ${topic}`);
                    resolve();
                }
            });
        });
    }

    /**
     * Handle incoming messages
     * @param {string} topic - Topic the message was received on
     * @param {Buffer} message - Raw message buffer
     */
    handleMessage(topic, message) {
        try {
            const payload = message.toString();
            let parsed;
            
            try {
                parsed = JSON.parse(payload);
            } catch {
                parsed = payload;
            }

            this.log(`Message on ${topic}:`, parsed);

            // Call topic-specific handler
            const handler = this.subscriptions.get(topic);
            if (handler) {
                handler(parsed, topic);
            }

            // Call global message handler
            if (this.onMessage) {
                this.onMessage(parsed, topic);
            }

        } catch (error) {
            this.logError('Error handling message:', error);
        }
    }

    /**
     * Resubscribe to all topics (after reconnect)
     */
    resubscribeAll() {
        for (const [topic, handler] of this.subscriptions) {
            this.client.subscribe(topic, { qos: this.config.mqtt.qos }, (error) => {
                if (error) {
                    this.logError(`Failed to resubscribe to ${topic}:`, error);
                } else {
                    this.log(`Resubscribed to: ${topic}`);
                }
            });
        }
    }

    /**
     * Subscribe to signaling response topic for a camera
     * @param {string} serial - Camera serial number
     * @param {function} handler - Response handler
     */
    async subscribeSignaling(serial, handler) {
        const topic = this.config.topics.responseSignaling(serial);
        await this.subscribe(topic, handler);
    }

    /**
     * Publish signaling request
     * @param {string} serial - Camera serial number
     * @param {object} message - Signaling message
     */
    async publishSignaling(serial, message) {
        const topic = this.config.topics.requestSignaling(serial);
        await this.publish(topic, message);
    }

    /**
     * Publish camera credentials
     * @param {string} serial - Camera serial number
     * @param {string} username - Camera username
     * @param {string} password - Camera password
     * @param {string} ip - Camera IP address (optional)
     */
    async publishCredentials(serial, username, password, ip = '') {
        const topic = this.config.topics.credential(serial);
        const message = createCredentialMessage(serial, username, password, ip);
        await this.publish(topic, message);
    }

    /**
     * Log helper
     */
    log(...args) {
        console.log('[MQTT]', ...args);
        if (window.addLog) window.addLog('MQTT', args.join(' '));
    }

    /**
     * Error log helper
     */
    logError(...args) {
        console.error('[MQTT]', ...args);
        if (window.addLog) window.addLog('MQTT ERROR', args.join(' '));
    }

    /**
     * Get client ID
     */
    getClientId() {
        return this.clientId;
    }

    /**
     * Check if connected
     */
    isConnected() {
        return this.connected;
    }
}

// Export for module usage
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { MQTTSignalingClient };
}
