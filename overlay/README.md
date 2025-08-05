# MediaMTX Overlay System

GPS 데이터를 비디오 스트림에 오버레이하여 UDP로 전송하는 시스템입니다.

## Features

- PostgreSQL 데이터베이스에서 GPS 데이터 실시간 조회
- GStreamer를 사용한 비디오 스트림 처리
- GPS 정보를 텍스트로 오버레이
- UDP를 통한 실시간 스트림 전송
- 설정 파일을 통한 유연한 구성

## Requirements

- Python 3.8+
- GStreamer 1.0
- PostgreSQL
- Jetson Xavier NX (권장)

## Installation

1. 의존성 설치:
```bash
pip install -r requirements.txt
```

2. 시스템 GStreamer 패키지 설치:
```bash
sudo apt-get update
sudo apt-get install gstreamer1.0-tools gstreamer1.0-plugins-base gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad gstreamer1.0-plugins-ugly gstreamer1.0-libav \
    gstreamer1.0-x gstreamer1.0-alsa gstreamer1.0-gl gstreamer1.0-gtk3 \
    gstreamer1.0-qt5 gstreamer1.0-pulseaudio
```

## Configuration

### 1. 환경 변수 설정 (.env 파일)

```bash
# Database configuration
POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=your_password
POSTGRES_DB=gps_db
```

### 2. 설정 파일 (overlay.yml)

```yaml
paths:
  cam121:
    source: rtsp://admin:q1w2e3r4@192.168.1.121/streaming/channels/101
    output: udp://238.0.0.1:10121

  cam122:
    source: rtsp://admin:q1w2e3r4@192.168.1.122/streaming/channels/101
    output: udp://238.0.0.1:10122
```

## Usage

### 기본 실행

```bash
python -m core.core overlay.yml
```

### 설정 파일 없이 실행

```bash
python -m core.core /path/to/your/config.yml
```

### Overlay 모듈 직접 사용

```python
from overlay.overlay import create_overlay

# Overlay 인스턴스 생성
overlay = create_overlay(
    input_uri="rtsp://admin:q1w2e3r4@192.168.1.121/streaming/channels/101",
    output_host="238.0.0.1",
    output_port=10121
)

# 시작
overlay.start()

# 상태 확인
status = overlay.get_status()
print(status)

# 정지
overlay.stop()
```

## GPS Data Format

오버레이되는 GPS 정보는 다음과 같은 형식으로 표시됩니다:

```
GPS: 37.566500, 126.978000
Status: Active
Time: 14:30:25
```

## GStreamer Pipeline

시스템은 다음과 같은 GStreamer 파이프라인을 사용합니다:

```
uridecodebin -> videoconvert -> videoscale -> textoverlay -> x264enc -> h264parse -> rtph264pay -> udpsink
```

## Troubleshooting

### GStreamer 오류
- GStreamer 패키지가 올바르게 설치되었는지 확인
- `gst-inspect-1.0` 명령어로 플러그인 확인

### GPS 데이터 없음
- PostgreSQL 데이터베이스 연결 확인
- GPS 데이터가 데이터베이스에 저장되고 있는지 확인

### 비디오 스트림 문제
- 입력 URI가 올바른지 확인
- 네트워크 연결 상태 확인

## Development

### 프로젝트 구조

```
mediamtx/overlay/
├── core/
│   ├── core.py      # 메인 애플리케이션
│   └── conf.py      # 설정 관리
├── gps/
│   └── gps.py       # GPS 데이터 관리
├── overlay/
│   └── overlay.py   # GStreamer 오버레이
├── requirements.txt  # Python 의존성
├── overlay.yml      # 설정 파일
└── README.md        # 이 파일
```

### 새로운 기능 추가

1. `overlay/overlay.py`에서 새로운 오버레이 타입 추가
2. `core/conf.py`에서 설정 로딩 로직 수정
3. `overlay.yml`에서 새로운 설정 옵션 추가

## License

이 프로젝트는 MIT 라이선스 하에 배포됩니다. 