FROM alpine:latest

# 필요한 패키지 설치 (bash, coreutils, postgresql-client, bc)
RUN apk add --no-cache bash coreutils postgresql-client bc

# 작업 디렉토리 설정
WORKDIR /app

# GPS 시뮬레이터 스크립트 복사
COPY gps_simulator.sh /app/gps_simulator.sh

# 스크립트에 실행 권한 부여
RUN chmod +x /app/gps_simulator.sh

# 컨테이너 시작 시 GPS 시뮬레이터 실행
CMD ["/app/gps_simulator.sh"] 