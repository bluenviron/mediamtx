#!/usr/bin/env python3
"""
RTSP Stream Pipeline with Timestamp and GPS Overlay
Reads RTSP stream, adds red timestamp and GPS coordinates overlay at bottom, streams to UDP
Uses ffprobe for accurate resolution and FPS detection
"""

import cv2
import subprocess
import json
import datetime
import sys
import os
import time
import threading
from pathlib import Path
from typing import Optional, Dict, Any, List
from gps.gps import GPSManager, GPSConfig
import logging

logging.basicConfig(
    level=logging.INFO,
    format='[%(levelname)s] %(name)s: %(message)s',
)
logger = logging.getLogger(__name__)


def get_stream_info(rtsp_url: str) -> Optional[Dict[str, Any]]:
    """
    Use ffprobe to get accurate stream information (resolution, fps)
    """
    try:
        cmd = [
            'ffprobe',
            '-v', 'quiet',
            '-print_format', 'json',
            '-show_streams',
            '-select_streams', 'v:0',  # Select first video stream
            rtsp_url
        ]
        
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=10)
        
        if result.returncode != 0:
            logger.error(f"Error running ffprobe: {result.stderr}")
            return None
            
        data = json.loads(result.stdout)
        
        if not data.get('streams'):
            logger.error("No video streams found")
            return None
            
        stream = data['streams'][0]
        
        # Get resolution
        width = int(stream.get('width', 0))
        height = int(stream.get('height', 0))
        
        # Get FPS - try multiple fields
        fps = None
        if 'r_frame_rate' in stream:
            fps_str = stream['r_frame_rate']
            if '/' in fps_str:
                num, den = fps_str.split('/')
                fps = float(num) / float(den) if float(den) != 0 else None
        
        if not fps and 'avg_frame_rate' in stream:
            fps_str = stream['avg_frame_rate']
            if '/' in fps_str:
                num, den = fps_str.split('/')
                fps = float(num) / float(den) if float(den) != 0 else None
                
        if not fps:
            fps = 25.0  # Default fallback
            
        logger.info(f"Stream info: {width}x{height} @ {fps:.2f} FPS")
        
        return {
            'width': width,
            'height': height,
            'fps': fps,
            'codec': stream.get('codec_name', 'unknown')
        }
        
    except subprocess.TimeoutExpired:
        logger.warning("ffprobe timeout - stream may not be available")
        return None
    except json.JSONDecodeError:
        logger.error("Failed to parse ffprobe output")
        return None
    except Exception as e:
        logger.error(f"Error getting stream info: {e}")
        return None


def create_udp_streamer(udp_address: str, width: int, height: int, fps: float) -> Optional[subprocess.Popen]:
    """
    Create ffmpeg subprocess for UDP streaming
    """
    try:
        # Add packet size parameter for better UDP streaming
        if '?' in udp_address:
            udp_url = f"{udp_address}&pkt_size=1316"
        else:
            udp_url = f"{udp_address}?pkt_size=1316"
        
        # FFmpeg command for UDP streaming (compatible with mediamtx)
        ffmpeg_cmd = [
            'ffmpeg',
            '-y',  # Overwrite output
            '-f', 'rawvideo',
            '-vcodec', 'rawvideo',
            '-s', f'{width}x{height}',
            '-pix_fmt', 'bgr24',
            '-r', str(fps),
            '-i', '-',  # Input from stdin
            '-c:v', 'libx264',
            '-pix_fmt', 'yuv420p',  # Explicit pixel format for compatibility
            '-preset', 'ultrafast',
            '-b:v', '600k',  # Set bitrate for consistent quality
            '-f', 'mpegts',
            udp_url
        ]
        
        logger.info(f"Starting ffmpeg UDP streamer: {' '.join(ffmpeg_cmd)}")
        
        process = subprocess.Popen(
            ffmpeg_cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE
        )
        
        return process
        
    except Exception as e:
        logger.error(f"Failed to create UDP streamer: {e}")
        return None


def add_overlay_info(frame, timestamp_str: str, gps_data=None, ship_name: Optional[str] = None):
    """
    Add red timestamp, ship name and GPS overlay at the bottom of the frame
    """
    height, width = frame.shape[:2]
    
    # Font settings
    font = cv2.FONT_HERSHEY_SIMPLEX
    font_scale = 0.8
    color = (0, 0, 255)  # Red color in BGR
    thickness = 2
    line_spacing = 35
    
    # Prepare overlay text lines
    overlay_lines = [timestamp_str]
    
    # Add ship name if available
    if ship_name:
        ship_line = f"Ship: {ship_name}"
        overlay_lines.append(ship_line)
    
    # Add GPS information if available
    if gps_data:
        gps_line = f"GPS: {gps_data['formatted_position']}"
        overlay_lines.append(gps_line)
    else:
        overlay_lines.append("GPS: No signal")
    
    # Position overlays at top left
    for i, line in enumerate(overlay_lines):
        # Get text size for each line
        (text_width, text_height), baseline = cv2.getTextSize(line, font, font_scale, thickness)
        
        # Calculate position - top left corner
        x = 20  # 20 pixels from left edge
        y = 30 + (i * line_spacing) + text_height  # 30 pixels from top edge
        
        # Add black background rectangle for better readability
        cv2.rectangle(frame, 
                      (x - 10, y - text_height - 5), 
                      (x + text_width + 10, y + baseline + 5), 
                      (0, 0, 0), 
                      cv2.FILLED)
        
        # Add text
        cv2.putText(frame, line, (x, y), font, font_scale, color, thickness)
    
    return frame


class StreamPipeline:
    """
    RTSP to UDP streaming pipeline with overlay
    """
    
    def __init__(self, rtsp_url: str, udp_address: str, ship_name: Optional[str] = None, 
                 gps_manager: Optional[GPSManager] = None, duration_seconds: Optional[int] = None):
        self.rtsp_url = rtsp_url
        self.udp_address = udp_address
        self.ship_name = ship_name
        self.gps_manager = gps_manager
        self.duration_seconds = duration_seconds
        
        self.cap = None
        self.streamer = None
        self.frame_count = 0
        self.start_time = None
        self.last_timestamp_str = None
        self.current_gps_data = None
        self._stop_event = threading.Event()
        self._stream_thread = None
        
    def start(self) -> bool:
        """
        Start the streaming pipeline
        """
        logger.info(f"Getting stream information for: {self.rtsp_url}")
        
        if self.gps_manager:
            logger.info("GPS manager available")
        else:
            logger.info("GPS manager not available")
            
        if self.ship_name:
            logger.info(f"Ship name: {self.ship_name}")
        else:
            logger.info("Ship name: Not provided")
        
        # Get stream info using ffprobe
        stream_info = get_stream_info(self.rtsp_url)
        if not stream_info:
            logger.warning("Failed to get stream information. Trying with default settings...")
            stream_info = {'width': 1920, 'height': 1080, 'fps': 25.0}
        
        # Open RTSP stream
        logger.info(f"Opening RTSP stream: {self.rtsp_url}")
        self.cap = cv2.VideoCapture(self.rtsp_url)
        
        if not self.cap.isOpened():
            logger.error(f"Error: Could not open RTSP stream: {self.rtsp_url}")
            return False
        
        # Set buffer size to reduce latency
        self.cap.set(cv2.CAP_PROP_BUFFERSIZE, 1)
        
        # Get actual frame dimensions (in case ffprobe was wrong)
        ret, test_frame = self.cap.read()
        if not ret:
            logger.error("Error: Could not read frame from stream")
            self.cap.release()
            return False
        
        actual_height, actual_width = test_frame.shape[:2]
        logger.info(f"Actual frame size: {actual_width}x{actual_height}")
        
        # Create UDP streamer
        logger.info(f"Starting UDP stream to: {self.udp_address}")
        self.streamer = create_udp_streamer(self.udp_address, actual_width, actual_height, stream_info['fps'])
        if not self.streamer:
            self.cap.release()
            return False
        
        # Initialize timing and GPS data
        self.start_time = datetime.datetime.now()
        self.last_timestamp_str = self.start_time.strftime("%Y-%m-%d %H:%M:%S")
        if self.gps_manager:
            self.current_gps_data = self._get_gps_data()
        
        # Start streaming thread
        self._stream_thread = threading.Thread(target=self._stream_loop, daemon=True)
        self._stream_thread.start()
        
        logger.info("Streaming started. Use stop() method to interrupt.")
        return True
    
    def _get_gps_data(self) -> Optional[Dict[str, Any]]:
        """
        Get current GPS data from GPS manager
        """
        if not self.gps_manager:
            return None
            
        try:
            gps_data = self.gps_manager.get_current_gps()
            # Check for both "A" (Active) and "VALID" status values
            if gps_data and (gps_data.status == "A" or gps_data.status == "V"):
                return {
                    'formatted_position': f"{gps_data.get_formatted_latitude()}, {gps_data.get_formatted_longitude()}"
                }
        except Exception as e:
            logger.error(f"Error getting GPS data: {e}")
        
        return None
    
    def _stream_loop(self):
        """
        Main streaming loop
        """
        try:
            # Process the test frame first
            ret, frame = self.cap.read()
            if ret:
                timestamp_str = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
                frame_with_overlay = add_overlay_info(frame.copy(), timestamp_str, self.current_gps_data, self.ship_name)
                try:
                    self.streamer.stdin.write(frame_with_overlay.tobytes())
                    self.streamer.stdin.flush()
                except BrokenPipeError:
                    logger.warning("Warning: UDP stream connection broken")
                self.frame_count += 1
            
            while not self._stop_event.is_set():
                ret, frame = self.cap.read()
                if not ret:
                    logger.warning("Warning: Failed to read frame, attempting to reconnect...")
                    self.cap.release()
                    self.cap = cv2.VideoCapture(self.rtsp_url)
                    self.cap.set(cv2.CAP_PROP_BUFFERSIZE, 1)
                    continue
                
                # Get current timestamp
                current_time = datetime.datetime.now()
                timestamp_str = current_time.strftime("%Y-%m-%d %H:%M:%S")
                
                # Update GPS data when timestamp changes (second changes)
                if timestamp_str != self.last_timestamp_str:
                    self.current_gps_data = self._get_gps_data()
                    self.last_timestamp_str = timestamp_str
                
                # Add timestamp, ship name and GPS overlay
                frame_with_overlay = add_overlay_info(frame.copy(), timestamp_str, self.current_gps_data, self.ship_name)
                
                # Send frame to UDP stream
                try:
                    self.streamer.stdin.write(frame_with_overlay.tobytes())
                    self.streamer.stdin.flush()
                except BrokenPipeError:
                    logger.warning("Warning: UDP stream connection broken, attempting to restart...")
                    # Try to restart the streamer
                    self.streamer.terminate()
                    self.streamer = create_udp_streamer(self.udp_address, frame.shape[1], frame.shape[0], 25.0)
                    if not self.streamer:
                        logger.error("Failed to restart UDP streamer, stopping...")
                        break
                self.frame_count += 1
                
                # Add a small delay to prevent CPU overload
                time.sleep(0.001)
                    
                # Check duration limit
                if self.duration_seconds and (datetime.datetime.now() - self.start_time).total_seconds() > self.duration_seconds:
                    logger.info(f"Streaming duration limit ({self.duration_seconds}s) reached")
                    break
                    
                # Print progress every 100 frames
                if self.frame_count % 100 == 0:
                    elapsed = (datetime.datetime.now() - self.start_time).total_seconds()
                    logger.info(f"Streamed {self.frame_count} frames in {elapsed:.1f} seconds")
        
        except Exception as e:
            logger.error(f"Error during streaming: {e}")
    
    def stop(self):
        """
        Stop the streaming pipeline
        """
        logger.info("Stopping streaming pipeline...")
        self._stop_event.set()
        
        if self._stream_thread:
            self._stream_thread.join(timeout=5)
        
        # Cleanup
        if self.cap:
            self.cap.release()
        if self.streamer:
            self.streamer.stdin.close()
            self.streamer.terminate()
            self.streamer.wait()
        
        if self.start_time:
            elapsed = (datetime.datetime.now() - self.start_time).total_seconds()
            logger.info(f"Streaming completed: {self.frame_count} frames in {elapsed:.1f} seconds")
            logger.info(f"UDP stream sent to: {self.udp_address}")


def create_pipeline(rtsp_url: str, udp_address: str, ship_name: Optional[str] = None, 
                   gps_manager: Optional[GPSManager] = None, duration_seconds: Optional[int] = None) -> StreamPipeline:
    """
    Create a new streaming pipeline
    """
    return StreamPipeline(rtsp_url, udp_address, ship_name, gps_manager, duration_seconds)


def create_pipelines_from_config(config: Dict[str, Any], gps_manager: Optional[GPSManager] = None) -> List[StreamPipeline]:
    """
    Create multiple pipelines from configuration
    """
    pipelines = []
    
    if "paths" in config:
        for path_name, path_config in config["paths"].items():
            if "source" in path_config and "destination" in path_config:
                rtsp_url = path_config["source"]
                udp_address = path_config["destination"]
                ship_name = path_config.get("ship_name")
                duration = path_config.get("duration")
                
                pipeline = create_pipeline(rtsp_url, udp_address, ship_name, gps_manager, duration)
                pipelines.append(pipeline)
                logger.info(f"Created pipeline for {path_name}: {rtsp_url} -> {udp_address}")
    
    return pipelines
