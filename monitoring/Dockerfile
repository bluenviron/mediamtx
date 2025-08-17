FROM python:3.9-slim

# 시스템 패키지 업데이트 및 필요한 도구 설치
RUN apt-get update && apt-get install -y \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# 작업 디렉토리 설정
WORKDIR /app

# Python 의존성 파일 복사 및 설치
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# 애플리케이션 파일 복사
COPY *.py .

# 포트 노출 (기본값: 8000, 서비스 파일에서는 16379 사용)
EXPOSE 8000

# 헬스체크 추가 (jtop 연결 확인)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD python3 -c "from jtop import jtop; j=jtop(); j.start(); print('OK' if j.ok() else 'FAIL'); j.close()" || exit 1

# 애플리케이션 실행
CMD ["python3", "jetson_stats_prometheus_collector.py", "--port", "8000", "--log-level", "DEBUG"] 