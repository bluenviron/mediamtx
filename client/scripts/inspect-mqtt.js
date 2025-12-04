#!/usr/bin/env node
// Simple MQTT inspector: subscribe to a topic and print raw payloads (pretty JSON when possible)
// Usage: node inspect-mqtt.js <brokerUrl> <username> <password> <topic>
// Example: node inspect-mqtt.js wss://beta-broker-mqtt.fcam.vn:8084/mqtt myuser mypass "ipc/fss/#"

const mqtt = require('mqtt');

function tryParseJSON(s) {
  try {
    return JSON.parse(s);
  } catch (e) {
    return null;
  }
}

const args = process.argv.slice(2);
if (args.length < 4) {
  console.error('Usage: node inspect-mqtt.js <brokerUrl> <username> <password> <topic>');
  process.exit(1);
}

const [brokerUrl, username, password, topic] = args;

const options = {
  username,
  password,
  reconnectPeriod: 5000,
  connectTimeout: 30000,
};

console.log(`Connecting to ${brokerUrl} ...`);
const client = mqtt.connect(brokerUrl, options);

client.on('connect', () => {
  console.log('Connected. Subscribing to', topic);
  client.subscribe(topic, { qos: 1 }, (err) => {
    if (err) console.error('Subscribe error', err);
  });
});

client.on('message', (t, payloadBuffer) => {
  const payload = payloadBuffer.toString();
  const parsed = tryParseJSON(payload);
  console.log('---');
  console.log('Topic:', t);
  if (parsed !== null) {
    try {
      console.log('JSON payload:');
      console.log(JSON.stringify(parsed, null, 2));
    } catch (e) {
      console.log('Could not stringify JSON:', e.message);
      console.log('Raw payload:', payload);
    }
  } else {
    console.log('Raw payload (non-JSON):');
    console.log(payload);
  }
});

client.on('error', (err) => {
  console.error('MQTT error:', err.message || err);
});

process.on('SIGINT', () => {
  console.log('\nDisconnecting...');
  client.end(() => process.exit(0));
});
