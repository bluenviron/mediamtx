Overlay 개발 계획을 세우기 위해 먼저 `.cursor/rules/overlay.mdc` 파일의 내용을 확인해보겠습니다.
Read file: .cursor/rules/overlay.mdc

GPS 스키마 정보도 확인하고 현재 코드베이스의 구조를 파악해서 개발 계획을 세우기 위해 몇 가지 파일을 확인해보겠습니다.
Read file: apps/gps-simulator/init.sql
Search files...
Search files...

이제 OpenCV와 관련된 부분을 확인하고 전체적인 개발 계획을 세워보겠습니다.
Search files...
Search files...
Ran tool

## Overlay 개발 계획

`.cursor/rules/overlay.mdc` 파일을 바탕으로 다음과 같은 단계별 개발 계획을 제안합니다:

### 1. 아키텍처 설계 (1단계)

**새로운 컴포넌트 구조:**
```
internal/overlay/
├── overlay.go          # 메인 오버레이 엔진
├── gps_client.go       # PostgreSQL GPS 데이터 클라이언트
├── text_renderer.go    # OpenCV 텍스트 렌더링
└── config.go          # 오버레이 설정
```

**통합 지점:**
- `internal/stream/stream.go`: 비디오 스트림 미들웨어로 오버레이 적용
- `internal/formatprocessor/`: H.264/H.265 프레임 처리 시점에서 오버레이 삽입

### 2. 데이터베이스 연동 (2단계)

**GPS 클라이언트 구현:**
```go
// internal/overlay/gps_client.go
type GPSClient struct {
    db *sql.DB
    lastUpdate time.Time
    currentData *GPSData
}

type GPSData struct {
    Timestamp time.Time
    Latitude  float64
    Longitude float64
}
```

**기능:**
- 1초마다 최신 GPS 데이터 조회
- 메모리 캐싱으로 성능 최적화
- 데이터베이스 연결 풀 관리

### 3. OpenCV CUDA 오버레이 엔진 (3단계)

**OpenCV CUDA 통합:**
```go
// internal/overlay/text_renderer.go
type TextRenderer struct {
    font     *opencv.Font
    gpuMat   *opencv.GpuMat
    stream   *opencv.Stream
}

func (tr *TextRenderer) RenderOverlay(frame []byte, gpsData *GPSData, shipName string) ([]byte, error) {
    // GPU 메모리로 프레임 업로드
    // CUDA로 텍스트 오버레이 처리
    // CPU 메모리로 결과 다운로드
}
```

**성능 최적화:**
- GPU 메모리 재사용
- CUDA 스트림을 통한 비동기 처리
- 배치 처리로 지연시간 최소화

### 4. 스트림 미들웨어 통합 (4단계)

**Stream 레벨 통합:**
```go
// internal/stream/stream.go 수정
type Stream struct {
    // ... 기존 필드들
    overlayEngine *overlay.Engine
}

func (s *Stream) WriteRTPPacket(media *description.Media, format format.Format, pkt *rtp.Packet, ntp time.Time, pts int64) {
    // 기존 처리 로직
    
    // H.264/H.265 비디오 프레임인 경우 오버레이 적용
    if needsOverlay(media, format) {
        processedFrame := s.overlayEngine.ProcessFrame(frameData)
        // 오버레이된 프레임으로 교체
    }
    
    // 기존 스트림 처리 계속
}
```

### 5. 설정 및 초기화 (5단계)

**설정 파일 확장:**
```yaml
# mediamtx.yml에 추가
overlay:
  enabled: true
  updateInterval: 1s
  fontPath: "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
  fontSize: 24
  textColor: "255,255,255"
  backgroundColor: "0,0,0,128"
  position: "top-left"
  
database:
  host: "${DB_HOST}"
  port: "${DB_PORT}"
  user: "${DB_USER}"
  password: "${DB_PASSWORD}"
  dbname: "${DB_NAME}"
```

**PathManager 연동:**
```go
// internal/core/path_manager.go 수정
type pathManager struct {
    // ... 기존 필드들
    overlayEngine *overlay.Engine
}

func (pm *pathManager) createPath(...) {
    pa := &path{
        // ... 기존 필드들
        shipName: pm.shipName, // 이미 존재하는 필드 활용
    }
}
```

### 6. 개발 우선순위

**Phase 1 (핵심 기능):**
1. GPS Manager 구현
2. 기본 OpenCV Scaffold 구현
3. 설정 시스템 구현
4. Stream 미들웨어 통합

**Phase 2 (최적화):**
1. CUDA GPU 가속 적용
2. 메모리 풀링 및 성능 최적화
3. 1초 정확한 타이밍 동기화
4. 오류 처리 및 복원 메커니즘

**Phase 3 (고도화):**
1. 오버레이 스타일 커스터마이제이션
2. 다중 해상도 지원
3. 성능 모니터링 및 메트릭
4. 테스트 커버리지 확장

### 7. 기술적 고려사항

**메모리 관리:**
- GPU 메모리 누수 방지
- 프레임 버퍼 재사용
- 가비지 컬렉션 최적화

**동기화:**
- GPS 데이터 업데이트와 프레임 렌더링 동기화
- 1초 정확한 타이밍 보장
- 멀티 스트림 간 일관성

**성능:**
- 지연시간 1ms 이하 목표 [[memory:4716479]]
- GPU 사용률 최적화
- CPU-GPU 메모리 전송 최소화

이 계획을 통해 모든 RTSP 입력에 대해 실시간으로 GPS 정보와 선박명을 오버레이하는 시스템을 구축할 수 있습니다.