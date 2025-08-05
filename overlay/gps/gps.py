"""
GPS data management module for overlay systems.

This module provides functionality to retrieve GPS data from PostgreSQL database
and make it available to other Python modules.
"""

import psycopg2
import psycopg2.extras
from dataclasses import dataclass
from datetime import datetime
from typing import Optional, Dict, Any
import threading
import time
import logging

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)


@dataclass
class GPSData:
    """GPS data structure."""
    timestamp: datetime
    latitude: float
    longitude: float
    status: str

    def format_coordinate(self, coord: float) -> str:
        """Format coordinate to DD.DDDDDD format."""
        return f"{coord:.6f}"

    def get_formatted_latitude(self) -> str:
        """Get formatted latitude."""
        return self.format_coordinate(self.latitude)

    def get_formatted_longitude(self) -> str:
        """Get formatted longitude."""
        return self.format_coordinate(self.longitude)


class GPSConfig:
    """GPS manager configuration."""
    
    def __init__(self, 
                 database_host: str = "localhost",
                 database_port: int = 5432,
                 database_user: str = "postgres",
                 database_password: str = "",
                 database_name: str = "gps_db",
                 update_interval: float = 1.0,
                 max_connections: int = 10):
        self.database_host = database_host
        self.database_port = database_port
        self.database_user = database_user
        self.database_password = database_password
        self.database_name = database_name
        self.update_interval = update_interval
        self.max_connections = max_connections


class GPSManager:
    """GPS data manager for PostgreSQL database."""
    
    def __init__(self, config: GPSConfig):
        self.config = config
        self.connection = None
        self.current_data: Optional[GPSData] = None
        self.last_update = None
        self._lock = threading.RLock()
        self._stop_event = threading.Event()
        self._update_thread = None
        
        # Initialize connection
        self._connect()
        
        # Start background updates
        self._start_updates()
    
    def _connect(self) -> None:
        """Establish database connection."""
        try:
            self.connection = psycopg2.connect(
                host=self.config.database_host,
                port=self.config.database_port,
                user=self.config.database_user,
                password=self.config.database_password,
                database=self.config.database_name
            )
            logger.info("Successfully connected to GPS database")
        except Exception as e:
            logger.error(f"Failed to connect to database: {e}")
            raise
    
    def _start_updates(self) -> None:
        """Start background GPS data updates."""
        self._update_thread = threading.Thread(target=self._update_loop, daemon=True)
        self._update_thread.start()
        logger.info("Started GPS data update thread")
    
    def _update_loop(self) -> None:
        """Background update loop."""
        while not self._stop_event.is_set():
            try:
                self._update_gps_data()
                time.sleep(self.config.update_interval)
            except Exception as e:
                logger.error(f"Error in GPS update loop: {e}")
                time.sleep(5)  # Wait before retry
    
    def _update_gps_data(self) -> None:
        """Fetch latest GPS data from database."""
        if not self.connection or self.connection.closed:
            self._connect()
            return
        
        query = """
            SELECT timestamp, latitude, longitude, status 
            FROM gps_data 
            WHERE latitude IS NOT NULL AND longitude IS NOT NULL
            ORDER BY timestamp DESC 
            LIMIT 1
        """
        
        try:
            with self.connection.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cursor:
                cursor.execute(query)
                row = cursor.fetchone()
                
                if row:
                    with self._lock:
                        self.current_data = GPSData(
                            timestamp=row['timestamp'],
                            latitude=float(row['latitude']),
                            longitude=float(row['longitude']),
                            status=row['status']
                        )
                        self.last_update = datetime.now()
                        logger.debug(f"Updated GPS data: {self.current_data}")
                else:
                    logger.warning("No GPS data available in database")
                    
        except Exception as e:
            logger.error(f"Error fetching GPS data: {e}")
    
    def get_current_gps(self) -> GPSData:
        """Get current GPS data."""
        with self._lock:
            if self.current_data is None:
                # Return default data if no GPS data available
                return GPSData(
                    timestamp=datetime.now(),
                    latitude=0.0,
                    longitude=0.0,
                    status="V"  # Void (no data)
                )
            return self.current_data
    
    def get_gps_dict(self) -> Dict[str, Any]:
        """Get current GPS data as dictionary."""
        data = self.get_current_gps()
        return {
            'timestamp': data.timestamp,
            'latitude': data.latitude,
            'longitude': data.longitude,
            'status': data.status,
            'formatted_latitude': data.get_formatted_latitude(),
            'formatted_longitude': data.get_formatted_longitude()
        }
    
    def close(self) -> None:
        """Close GPS manager and cleanup resources."""
        self._stop_event.set()
        if self._update_thread:
            self._update_thread.join(timeout=5)
        
        if self.connection and not self.connection.closed:
            self.connection.close()
            logger.info("GPS manager closed")


class MockGPSManager:
    """Mock GPS manager for testing purposes."""
    
    def __init__(self):
        self.data = GPSData(
            timestamp=datetime.now(),
            latitude=37.5665,
            longitude=126.9780,
            status="A"
        )
    
    def get_current_gps(self) -> GPSData:
        """Get mock GPS data."""
        return self.data
    
    def get_gps_dict(self) -> Dict[str, Any]:
        """Get mock GPS data as dictionary."""
        return {
            'timestamp': self.data.timestamp,
            'latitude': self.data.latitude,
            'longitude': self.data.longitude,
            'status': self.data.status,
            'formatted_latitude': self.data.get_formatted_latitude(),
            'formatted_longitude': self.data.get_formatted_longitude()
        }
    
    def close(self) -> None:
        """Close mock manager."""
        pass


# Global GPS manager instance
_gps_manager: Optional[GPSManager] = None


def initialize_gps_manager(config: GPSConfig) -> GPSManager:
    """Initialize global GPS manager."""
    global _gps_manager
    if _gps_manager is not None:
        _gps_manager.close()
    
    _gps_manager = GPSManager(config)
    return _gps_manager


def get_gps_manager() -> Optional[GPSManager]:
    """Get global GPS manager instance."""
    return _gps_manager


def get_current_gps() -> GPSData:
    """Get current GPS data from global manager."""
    if _gps_manager is None:
        raise RuntimeError("GPS manager not initialized. Call initialize_gps_manager() first.")
    return _gps_manager.get_current_gps()


def get_gps_dict() -> Dict[str, Any]:
    """Get current GPS data as dictionary from global manager."""
    if _gps_manager is None:
        raise RuntimeError("GPS manager not initialized. Call initialize_gps_manager() first.")
    return _gps_manager.get_gps_dict()


def close_gps_manager() -> None:
    """Close global GPS manager."""
    global _gps_manager
    if _gps_manager is not None:
        _gps_manager.close()
        _gps_manager = None


# Convenience functions for easy access
def get_latitude() -> float:
    """Get current latitude."""
    return get_current_gps().latitude


def get_longitude() -> float:
    """Get current longitude."""
    return get_current_gps().longitude


def get_gps_status() -> str:
    """Get current GPS status."""
    return get_current_gps().status


def get_formatted_coordinates() -> tuple:
    """Get formatted latitude and longitude."""
    data = get_current_gps()
    return (data.get_formatted_latitude(), data.get_formatted_longitude()) 