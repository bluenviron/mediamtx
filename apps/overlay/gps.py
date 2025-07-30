#!/usr/bin/env python3
"""
GPS File Reader and Parser
Handles reading and parsing GPS data from GPRMC format files
"""

import os


class GPSReader:
    """GPS file reader and parser"""
    
    def __init__(self, gps_file_path="./gps.txt"):
        self.gps_file_path = gps_file_path
        self.last_gps_data = None
    
    def read_gps_data(self):
        """Read and parse GPS data from file"""
        try:
            if not os.path.exists(self.gps_file_path):
                return None
                
            with open(self.gps_file_path, 'r', encoding='utf-8') as f:
                content = f.read().strip()
                if content:
                    # Process each line
                    for line in content.split('\n'):
                        line = line.strip()
                        if line.startswith('$GPRMC'):
                            gps_data = self.parse_gprmc(line)
                            if gps_data:
                                self.last_gps_data = gps_data
                                return gps_data
                    
        except Exception as e:
            print(f"Error reading GPS file: {e}")
        
        return self.last_gps_data  # Return last known GPS data if file read fails
    
    def parse_gprmc(self, gprmc_line):
        """Parse GPRMC sentence and return GPS data"""
        try:
            # Remove $ and split by comma
            parts = gprmc_line[1:].split(',')
            
            if len(parts) >= 12 and parts[0] == 'GPRMC':
                utc_time = parts[1]
                status = parts[2]
                latitude = parts[3]
                lat_dir = parts[4]
                longitude = parts[5]
                lon_dir = parts[6]
                speed = parts[7]
                course = parts[8]
                date = parts[9]
                
                # Only return data if GPS status is valid
                if status == 'A' and latitude and longitude:
                    return {
                        'latitude': latitude,
                        'lat_dir': lat_dir,
                        'longitude': longitude,
                        'lon_dir': lon_dir,
                        'speed': speed,
                        'course': course,
                        'utc_time': utc_time,
                        'date': date,
                        'status': status,
                        'lat_decimal': self.to_decimal_degrees(latitude, lat_dir),
                        'lon_decimal': self.to_decimal_degrees(longitude, lon_dir),
                        'formatted_position': self.format_coordinates(latitude, lat_dir, longitude, lon_dir)
                    }
        except Exception as e:
            print(f"Error parsing GPRMC: {e}")
        
        return None
    
    def to_decimal_degrees(self, coord_str, direction):
        """Convert DDMM.MMMM format to decimal degrees"""
        if not coord_str:
            return 0.0
        
        try:
            if '.' in coord_str:
                decimal_pos = coord_str.index('.')
                if decimal_pos >= 4:  # DDDMM.MMMM (longitude)
                    degrees = float(coord_str[:decimal_pos-2])
                    minutes = float(coord_str[decimal_pos-2:])
                else:  # DDMM.MMMM (latitude)
                    degrees = float(coord_str[:decimal_pos-2])
                    minutes = float(coord_str[decimal_pos-2:])
                
                decimal = degrees + (minutes / 60.0)
                
                # Apply direction
                if direction in ['S', 'W']:
                    decimal = -decimal
                
                return decimal
        except:
            pass
        
        return 0.0
    
    def format_coordinates(self, latitude, lat_dir, longitude, lon_dir):
        """Format coordinates for display"""
        try:
            if '.' in latitude:
                lat_decimal_pos = latitude.index('.')
                if lat_decimal_pos >= 4:  # Handle longitude format in latitude
                    lat_degrees = latitude[:lat_decimal_pos-2]
                    lat_minutes = latitude[lat_decimal_pos-2:]
                else:
                    lat_degrees = latitude[:lat_decimal_pos-2]
                    lat_minutes = latitude[lat_decimal_pos-2:]
            else:
                return f"{latitude} {lat_dir}, {longitude} {lon_dir}"
            
            if '.' in longitude:
                lon_decimal_pos = longitude.index('.')
                if lon_decimal_pos >= 4:  # DDDMM.MMMM (longitude)
                    lon_degrees = longitude[:lon_decimal_pos-2]
                    lon_minutes = longitude[lon_decimal_pos-2:]
                else:
                    lon_degrees = longitude[:lon_decimal_pos-2]
                    lon_minutes = longitude[lon_decimal_pos-2:]
            else:
                return f"{latitude} {lat_dir}, {longitude} {lon_dir}"
            
            return f"{lat_degrees}\" {lat_minutes}' {lat_dir}, {lon_degrees}\" {lon_minutes}' {lon_dir}"
        
        except Exception:
            return f"{latitude} {lat_dir}, {longitude} {lon_dir}" 