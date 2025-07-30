#!/bin/bash

# GPS 시뮬레이터 스크립트
# 매 1초마다 GPS 원본 파일에서 한 줄씩 읽어서 gps.txt에 덮어쓰기

# 파일 경로 설정
GPS_ORIGINAL_FILE="/gps.original.txt"
GPS_OUTPUT_FILE="/gps.txt"

# 원본 파일이 존재하는지 확인
if [ ! -f "$GPS_ORIGINAL_FILE" ]; then
    echo "오류: GPS 원본 파일을 찾을 수 없습니다: $GPS_ORIGINAL_FILE"
    exit 1
fi

# 전체 라인 수 확인
TOTAL_LINES=$(wc -l < "$GPS_ORIGINAL_FILE")
echo "GPS 시뮬레이터 시작 - 총 $TOTAL_LINES 라인"
echo "원본 파일: $GPS_ORIGINAL_FILE"
echo "출력 파일: $GPS_OUTPUT_FILE"
echo "매 1초마다 GPS 데이터를 업데이트합니다..."
echo "중단하려면 Ctrl+C를 누르세요."

# 현재 라인 번호 (1부터 시작)
current_line=1

# SIGINT (Ctrl+C) 신호 처리
trap 'echo -e "\n\nGPS 시뮬레이터가 중단되었습니다."; exit 0' SIGINT

# 무한 루프로 GPS 데이터 출력
while true; do
    # 현재 라인의 GPS 데이터 추출
    gps_data=$(sed -n "${current_line}p" "$GPS_ORIGINAL_FILE")
    
    # GPS 데이터가 비어있지 않으면 출력 파일에 쓰기
    if [ -n "$gps_data" ]; then
        echo "$gps_data" > "$GPS_OUTPUT_FILE"
        echo "[$current_line/$TOTAL_LINES] $gps_data"
    fi
    
    # 다음 라인으로 이동
    current_line=$((current_line + 1))
    
    # 마지막 라인에 도달하면 처음부터 다시 시작
    if [ $current_line -gt $TOTAL_LINES ]; then
        current_line=1
        echo "=== 모든 GPS 데이터를 출력했습니다. 처음부터 다시 시작합니다. ==="
    fi
    
    # 1초 대기
    sleep 1
done 