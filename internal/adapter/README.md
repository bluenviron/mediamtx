# FPT Camera WebRTC Adapter

This adapter bridges between FPT Camera's custom WebRTC signaling protocol (via MQTT) and MediaMTX's WHEP/WHIP protocol.

## Architecture

```
┌─────────────┐       MQTT        ┌─────────────────┐     WHEP/WHIP    ┌──────────┐
│   Client    │◄─────────────────►│     Adapter     │◄────────────────►│ MediaMTX │
│  (Mobile)   │                   │    (Bridge)     │                  │ (Server) │
└─────────────┘                   └─────────────────┘                  └──────────┘
      │                                    │                                 │
      │                                    │                                 │
      ▼                                    ▼                                 ▼
  FPT Payload                         Translation                      WHEP/WHIP
  - Request                           - Session Mgmt                   Standard
  - Offer                             - SDP Relay                      Protocol
  - Answer                            - ICE Handling
  - ICE Candidate
```

## Flow

### Signaling Flow (Client → Camera via Adapter)

1. **Client sends Request** → MQTT `ipc/<brand>/<serial>/request/signaling`
2. **Adapter receives Request** → Creates session, requests offer from MediaMTX
3. **MediaMTX returns Offer** → Via WHEP endpoint
4. **Adapter sends Offer** → MQTT `ipc/<brand>/<serial>/response/signaling`
5. **Client sends Answer** → MQTT `ipc/<brand>/<serial>/request/signaling`
6. **Adapter relays Answer** → To MediaMTX via WHEP
7. **Connection established** → Media flows via WebRTC

### FPT Camera Payload Format

**Request:**
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

**Offer Response:**
```json
{
  "Method": "ACT",
  "MessageType": "Signaling",
  "Serial": "c05o24110000021",
  "Data": {
    "Type": "offer",
    "ClientId": "client-123",
    "Sdp": "v=0\r\n...",
    "IceServers": ["stun-connect.fcam.vn", "turn-connect.fcam.vn"]
  },
  "Result": { "Ret": 100, "Message": "Success" },
  "Timestamp": 1589861099
}
```

**Answer:**
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
  "Timestamp": 1589861099
}
```

## Project Structure

```
internal/adapter/
├── doc.go          # Package documentation
├── types.go        # Message types and structures
├── mqtt.go         # MQTT client wrapper
├── whep.go         # WHEP client for MediaMTX
├── adapter.go      # Main adapter logic
└── config.go       # Configuration from environment

cmd/adapter/
└── main.go         # Entry point
```

## Building

```bash
# Download dependencies
go mod download

# Build the adapter
go build -o bin/fpt-adapter ./cmd/adapter

# Or using make
make -f adapter.mk build-adapter
```

## Running

### Using environment variables:

```bash
# Copy example config
cp .env.example .env

# Edit .env with your settings
vim .env

# Run with env file
export $(cat .env | grep -v '^#' | xargs) && ./bin/fpt-adapter
```

### Using command line:

```bash
# Show help
./bin/fpt-adapter -help

# Show version
./bin/fpt-adapter -version

# Show example env
./bin/fpt-adapter -env-example
```

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| FPT_MQTT_BROKER | MQTT broker URL | wss://beta-broker-mqtt.fcam.vn:8084/mqtt |
| FPT_MQTT_USER | MQTT username | - |
| FPT_MQTT_PASS | MQTT password | - |
| MEDIAMTX_WHEP_URL | MediaMTX WHEP endpoint | http://localhost:8889 |
| WEBRTC_STUN_SERVERS | STUN servers (comma-separated) | stun:stun-connect.fcam.vn:3478 |
| TURN_SERVER_URL | TURN server URL | turn:turn-connect.fcam.vn:3478 |
| TURN_USERNAME | TURN username | turnuser |
| TURN_PASSWORD | TURN password | - |
| ADAPTER_MAX_SESSIONS | Maximum concurrent sessions | 100 |
| ADAPTER_SESSION_TIMEOUT | Session timeout in minutes | 30 |

## MQTT Topics

| Topic | Direction | Description |
|-------|-----------|-------------|
| `ipc/discovery` | Subscribe | Camera discovery list |
| `ipc/<brand>/<serial>/request/signaling` | Subscribe | Client signaling requests |
| `ipc/<brand>/<serial>/response/signaling` | Publish | Signaling responses to client |
| `ipc/<brand>/<serial>/credential` | Subscribe | Camera credentials from client |

## Integration with MediaMTX

The adapter communicates with MediaMTX using the WHEP (WebRTC-HTTP Egress Protocol) endpoint:

1. **Stream Path**: Uses `<brand>/<serial>` as the stream path
2. **WHEP Endpoint**: `http://localhost:8889/<brand>/<serial>/whep`
3. **SDP Exchange**: Translates between FPT format and standard SDP

### MediaMTX Configuration

Ensure MediaMTX is configured with:

```yaml
# mediamtx.yml
webrtc: yes
webrtcAddress: :8889

paths:
  all:
    # Allow all paths for dynamic camera streams
```

## Testing

```bash
# Run adapter tests
go test -v ./internal/adapter/...

# Or using make
make -f adapter.mk test-adapter
```

## License

MIT License
