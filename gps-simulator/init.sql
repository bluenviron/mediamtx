-- GPS 데이터 테이블 생성
CREATE TABLE IF NOT EXISTS gps_data (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    raw_nmea TEXT NOT NULL,
    time_utc TIME,
    status CHAR(1),
    latitude DECIMAL(10, 8),
    latitude_direction CHAR(1),
    longitude DECIMAL(11, 8),
    longitude_direction CHAR(1),
    speed_knots DECIMAL(8, 3),
    course_degrees DECIMAL(6, 2),
    date_ddmmyy DATE,
    magnetic_variation DECIMAL(6, 2),
    magnetic_variation_direction CHAR(1),
    checksum VARCHAR(4)
);

-- 인덱스 생성
CREATE INDEX IF NOT EXISTS idx_gps_timestamp ON gps_data(timestamp);
CREATE INDEX IF NOT EXISTS idx_gps_date ON gps_data(date_ddmmyy);
CREATE INDEX IF NOT EXISTS idx_gps_location ON gps_data(latitude, longitude);

-- 테이블 설명
COMMENT ON TABLE gps_data IS 'GPS NMEA GPRMC data storage';
COMMENT ON COLUMN gps_data.raw_nmea IS 'Original NMEA sentence';
COMMENT ON COLUMN gps_data.time_utc IS 'UTC time from NMEA';
COMMENT ON COLUMN gps_data.status IS 'A=Active, V=Void';
COMMENT ON COLUMN gps_data.latitude IS 'Latitude in decimal degrees';
COMMENT ON COLUMN gps_data.longitude IS 'Longitude in decimal degrees';
COMMENT ON COLUMN gps_data.speed_knots IS 'Speed over ground in knots';
COMMENT ON COLUMN gps_data.course_degrees IS 'Course over ground in degrees';