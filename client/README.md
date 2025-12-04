# FPT Camera WebRTC Client

Client application for connecting to FPT cameras via WebRTC with MQTT signaling.

## Architecture

```
┌─────────────┐       MQTT        ┌─────────────┐       Local        ┌─────────────┐
│   Client    │◄─────────────────►│   Server    │◄──────────────────►│  MediaMTX   │
│  (Browser)  │                   │  (MQTT)     │                    │  (WebRTC)   │
└─────────────┘                   └─────────────┘                    └─────────────┘
      │                                  │                                  │
      │ 1. Create Offer SDP              │                                  │
      │ 2. Gather ICE Candidates         │                                  │
      │ 3. Publish Offer ──────────────► │                                  │
      │                                  │ 4. Relay to MediaMTX ───────────►│
      │                                  │                                  │
      │                                  │◄───────── 5. Create Answer ──────│
      │◄───────── 6. Publish Answer ─────│◄───────── 6. Send Answer ────────│
      │                                  │                                  │
      │ 7. Set Remote Description        │                                  │
      │ 8. WebRTC Connection Established │                                  │
      └──────────────────────────────────┼──────────────────────────────────┘
                                   Media Stream
```

## Flow

1. **Client tạo Offer SDP** - Client bắt đầu quá trình WebRTC
2. **Client thực hiện Gathering ICE** - Thu thập ICE Candidates
3. **Client gửi Offer + ICE qua MQTT → Server** - Signaling qua MQTT
4. **Server relay tới MediaMTX** - Server chuyển thông tin
5. **MediaMTX tạo Answer SDP** - MediaMTX xử lý và sinh Answer
6. **MediaMTX gửi Answer về Server** - Thông tin đáp ứng
7. **Server publish Answer qua MQTT → Client** - Client nhận signaling
8. **Client thiết lập WebRTC** - setRemoteDescription và kết nối

## Topics MQTT

| Topic Pattern | Description |
|--------------|-------------|
| `ipc/discovery` | Danh sách cameras |
| `ipc/<brand>/<serial>/credential` | Username/password camera |
| `ipc/<brand>/<serial>/request/signaling` | Client → Server signaling |
| `ipc/<brand>/<serial>/response/signaling` | Server → Client signaling |

## Message Types

### Signaling Request
```json
{
  "Method": "ACT",
  "MessageType": "Signaling",
  "Serial": "c05o24110000021",
  "Data": {
    "Type": "request",
    "ClientId": "client-123"
  },
  "Timestamp": 1589861099
}
```

### Signaling Offer (from Camera)
```json
{
  "Method": "ACT",
  "MessageType": "Signaling",
  "Serial": "c05o24110000021",
  "Data": {
    "Type": "offer",
    "ClientId": "client-123",
    "Sdp": "v=0\r\n...",
    "IceServers": ["stun.l.google.com", "turn-test-1.fcam.vn"]
  },
  "Result": { "Ret": 100, "Message": "Success" },
  "Timestamp": 1589861099
}
```

### Signaling Answer (from Client)
```json
{
  "Method": "ACT",
  "MessageType": "Signaling",
  "Serial": "c05o24110000021",
  "Data": {
    "Type": "answer",
    "Sdp": "v=0\r\n...",
    "ClientId": "client-123"
  },
  "Timestamp": 1589861099,
  "Result": { "Ret": 100, "Message": "Success" }
}
```

### Data Channel - Stream Request
```json
{
  "Id": "nvrtnqd07",
  "Command": "Stream",
  "Type": "Request",
  "Content": {
    "ChannelMask": 3,
    "ResolutionMask": 2
  }
}
```

- **ChannelMask**: 64-bit bitmask for channels (bit0=CH1...bit15=CH16)
- **ResolutionMask**: 64-bit bitmask for resolution (0=sub-stream, 1=main-stream)

## Files Structure

```
client/
├── index.html              # Main HTML UI
├── app.js                  # Application controller
├── config/
│   └── config.js           # Configuration (MQTT, WebRTC, Topics)
├── messages/
│   └── message-types.js    # Message definitions and helpers
├── mqtt/
│   └── mqtt-client.js      # MQTT client wrapper
└── webrtc/
    └── webrtc-client.js    # WebRTC client wrapper
```

## Configuration

Edit `config/config.js` to update:

```javascript
const CONFIG = {
    mqtt: {
        brokerUrl: 'wss://beta-broker-mqtt.fcam.vn:8084/mqtt',
        username: 'hoangbd7',
        password: 'Hoangbd7'
    },
    webrtc: {
        iceServers: [
            { urls: 'stun:stun-connect.fcam.vn:3478' },
            {
                urls: 'turn:turn-connect.fcam.vn:3478',
                username: 'turnuser',
                credential: 'camfptvnturn133099'
            }
        ]
    }
};
```

## Usage

### Option 1: Open directly in browser
Simply open `index.html` in a modern browser (Chrome, Firefox, Edge).

### Option 2: Run with local server
```bash
# Using Python
python -m http.server 8080

# Using Node.js
npx serve .

# Using PHP
php -S localhost:8080
```

Then open http://localhost:8080

### Steps to Connect

1. Click **Connect MQTT** to connect to the MQTT broker
2. Enter camera **Brand** and **Serial** number
3. (Optional) Enter camera **Username/Password** and click **Send Credentials**
4. Click **Start WebRTC** to begin signaling
5. Once connected, use **Data Channel** controls to request streams

## Requirements

- Modern browser with WebRTC support
- Network access to:
  - MQTT broker: `wss://beta-broker-mqtt.fcam.vn:8084/mqtt`
  - STUN servers: `stun-connect.fcam.vn:3478`
  - TURN servers: `turn-connect.fcam.vn:3478`

## Troubleshooting

### MQTT Connection Failed
- Check network connectivity
- Verify MQTT credentials
- Check if broker URL is correct

### WebRTC Connection Failed
- Check ICE server configuration
- Verify TURN credentials
- Check firewall settings

### No Video Stream
- Verify camera is online
- Check data channel Stream request
- Verify channel mask and resolution settings

## License

MIT License
