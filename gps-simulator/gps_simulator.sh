#!/bin/bash

# GPS 시뮬레이터 스크립트
# 매 1초마다 GPS 원본 파일에서 한 줄씩 읽어서 PostgreSQL 데이터베이스에 삽입

# 파일 경로 설정
GPS_ORIGINAL_FILE="/gps.original.txt"

# PostgreSQL 연결 정보 (환경변수에서 가져오기)
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_DB="${POSTGRES_DB:-gpsdb}"
POSTGRES_USER="${POSTGRES_USER:-gpsuser}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-gpspassword}"

# 원본 파일이 존재하는지 확인
if [ ! -f "$GPS_ORIGINAL_FILE" ]; then
    echo "Error: GPS original file not found: $GPS_ORIGINAL_FILE"
    exit 1
fi

# PostgreSQL 연결 테스트
echo "Testing PostgreSQL connection..."
export PGPASSWORD="$POSTGRES_PASSWORD"
until psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c '\q' 2>/dev/null; do
    echo "Waiting for PostgreSQL to be ready..."
    sleep 2
done
echo "PostgreSQL connection established successfully."

# NMEA GPRMC 데이터 파싱 함수
parse_gprmc() {
    local nmea_sentence="$1"
    
    # GPRMC 형식: $GPRMC,time,status,lat,lat_dir,lon,lon_dir,speed,course,date,mag_var,mag_var_dir*checksum
    IFS=',' read -ra FIELDS <<< "$nmea_sentence"
    
    if [ "${FIELDS[0]}" != "\$GPRMC" ]; then
        return 1
    fi
    
    # 체크섬 분리
    local date_and_checksum="${FIELDS[9]}"
    local checksum=""
    if [[ "$date_and_checksum" == *"*"* ]]; then
        IFS='*' read -ra DATE_CHECKSUM <<< "$date_and_checksum"
        FIELDS[9]="${DATE_CHECKSUM[0]}"
        checksum="${DATE_CHECKSUM[1]}"
    fi
    
    # 위도/경도를 십진수로 변환
    local lat_decimal=""
    local lon_decimal=""
    
    if [ -n "${FIELDS[3]}" ] && [ -n "${FIELDS[5]}" ]; then
        # DDMM.MMMM 형식을 DD.DDDDDD로 변환
        local lat_deg=$(echo "${FIELDS[3]}" | cut -c1-2)
        local lat_min=$(echo "${FIELDS[3]}" | cut -c3-)
        lat_decimal=$(echo "scale=8; $lat_deg + $lat_min/60" | bc -l)
        
        if [ "${FIELDS[4]}" = "S" ]; then
            lat_decimal="-$lat_decimal"
        fi
        
        local lon_deg=$(echo "${FIELDS[5]}" | cut -c1-3)
        local lon_min=$(echo "${FIELDS[5]}" | cut -c4-)
        lon_decimal=$(echo "scale=8; $lon_deg + $lon_min/60" | bc -l)
        
        if [ "${FIELDS[6]}" = "W" ]; then
            lon_decimal="-$lon_decimal"
        fi
    fi
    
    # PostgreSQL에 데이터 삽입
    local sql="INSERT INTO gps_data (raw_nmea, time_utc, status, latitude, latitude_direction, longitude, longitude_direction, speed_knots, course_degrees, date_ddmmyy, checksum) VALUES ("
    sql+="'$nmea_sentence', "
    sql+="'${FIELDS[1]}', "
    sql+="'${FIELDS[2]}', "
    sql+="${lat_decimal:-NULL}, "
    sql+="'${FIELDS[4]}', "
    sql+="${lon_decimal:-NULL}, "
    sql+="'${FIELDS[6]}', "
    sql+="${FIELDS[7]:-NULL}, "
    sql+="${FIELDS[8]:-NULL}, "
    sql+="'${FIELDS[9]}', "
    sql+="'$checksum');"
    
    psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "$sql" -q 2>/dev/null
    return $?
}

# 전체 라인 수 확인
TOTAL_LINES=$(wc -l < "$GPS_ORIGINAL_FILE")
echo "GPS Simulator starting - Total $TOTAL_LINES lines"
echo "Original file: $GPS_ORIGINAL_FILE"
echo "Database: PostgreSQL ($POSTGRES_HOST:$POSTGRES_DB)"
echo "Updating GPS data every 1 second..."
echo "Press Ctrl+C to stop."

# 현재 라인 번호 (1부터 시작)
current_line=1

# SIGINT (Ctrl+C) 신호 처리
trap 'echo -e "\n\nGPS simulator stopped."; exit 0' SIGINT

# 성공/실패 카운터
success_count=0
error_count=0

# 무한 루프로 GPS 데이터 처리
while true; do
    # 현재 라인의 GPS 데이터 추출
    gps_data=$(sed -n "${current_line}p" "$GPS_ORIGINAL_FILE")
    
    # GPS 데이터가 비어있지 않으면 데이터베이스에 삽입
    if [ -n "$gps_data" ]; then
        if parse_gprmc "$gps_data"; then
            success_count=$((success_count + 1))
            # 100번에 한 번만 로그 출력
            if [ $((current_line % 100)) -eq 0 ]; then
                echo "[$current_line/$TOTAL_LINES] Inserted: $gps_data (Success: $success_count, Errors: $error_count)"
            fi
        else
            error_count=$((error_count + 1))
            echo "Error inserting data at line $current_line: $gps_data"
        fi
    fi
    
    # 다음 라인으로 이동
    current_line=$((current_line + 1))
    
    # 마지막 라인에 도달하면 처음부터 다시 시작
    if [ $current_line -gt $TOTAL_LINES ]; then
        current_line=1
        echo "=== All GPS data processed. Restarting from beginning. (Success: $success_count, Errors: $error_count) ==="
        success_count=0
        error_count=0
    fi
    
    # 0.7초 대기
    sleep 0.7
done 