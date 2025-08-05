#!/bin/bash

# GPS 데이터 조회 유틸리티 스크립트

# PostgreSQL 연결 정보
POSTGRES_HOST="${POSTGRES_HOST:-localhost}"
POSTGRES_DB="${POSTGRES_DB:-gpsdb}"
POSTGRES_USER="${POSTGRES_USER:-gpsuser}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-gpspassword}"

export PGPASSWORD="$POSTGRES_PASSWORD"

echo "GPS Data Query Utility"
echo "======================"

case "${1:-help}" in
    "count")
        echo "Total GPS records:"
        psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT COUNT(*) FROM gps_data;"
        ;;
    "latest")
        echo "Latest 10 GPS records:"
        psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT id, timestamp, latitude, longitude, speed_knots, course_degrees FROM gps_data ORDER BY timestamp DESC LIMIT 10;"
        ;;
    "hourly")
        echo "GPS records count by hour (last 24 hours):"
        psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT DATE_TRUNC('hour', timestamp) as hour, COUNT(*) as records FROM gps_data WHERE timestamp >= NOW() - INTERVAL '24 hours' GROUP BY hour ORDER BY hour DESC;"
        ;;
    "status")
        echo "GPS data status summary:"
        psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT status, COUNT(*) as count FROM gps_data GROUP BY status;"
        ;;
    "clean")
        echo "Cleaning old GPS data (older than 7 days)..."
        psql -h "$POSTGRES_HOST" -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "DELETE FROM gps_data WHERE timestamp < NOW() - INTERVAL '7 days';"
        echo "Cleanup completed."
        ;;
    "help"|*)
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  count     - Show total number of GPS records"
        echo "  latest    - Show latest 10 GPS records"
        echo "  hourly    - Show GPS records count by hour (last 24 hours)"
        echo "  status    - Show GPS data status summary"
        echo "  clean     - Remove GPS data older than 7 days"
        echo "  help      - Show this help message"
        ;;
esac