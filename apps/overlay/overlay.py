#!/usr/bin/env python3
"""
RTSP Stream Recorder with Timestamp and GPS Overlay
Reads RTSP stream, adds red timestamp and GPS coordinates overlay at bottom, saves as MP4
Uses ffprobe for accurate resolution and FPS detection
"""

import cv2
import subprocess
import json
import datetime
import sys
import argparse
import os
from pathlib import Path
from gps import GPSReader


def get_stream_info(rtsp_url):
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
            print(f"Error running ffprobe: {result.stderr}")
            return None
            
        data = json.loads(result.stdout)
        
        if not data.get('streams'):
            print("No video streams found")
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
            
        print(f"Stream info: {width}x{height} @ {fps:.2f} FPS")
        
        return {
            'width': width,
            'height': height,
            'fps': fps,
            'codec': stream.get('codec_name', 'unknown')
        }
        
    except subprocess.TimeoutExpired:
        print("ffprobe timeout - stream may not be available")
        return None
    except json.JSONDecodeError:
        print("Failed to parse ffprobe output")
        return None
    except Exception as e:
        print(f"Error getting stream info: {e}")
        return None


def create_udp_streamer(udp_address, width, height, fps):
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
        
        print(f"Starting ffmpeg UDP streamer: {' '.join(ffmpeg_cmd)}")
        
        process = subprocess.Popen(
            ffmpeg_cmd,
            stdin=subprocess.PIPE,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE
        )
        
        return process
        
    except Exception as e:
        print(f"Failed to create UDP streamer: {e}")
        return None


def add_overlay_info(frame, timestamp_str, gps_data=None, ship_name=None):
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


def stream_with_overlay(rtsp_url, udp_address, ship_name=None, gps_file_path=None, duration_seconds=None):
    """
    Main function to stream RTSP with timestamp, ship name and GPS overlay to UDP
    """
    print(f"Getting stream information for: {rtsp_url}")
    
    # Initialize GPS reader with custom path if provided
    if gps_file_path:
        gps_reader = GPSReader(gps_file_path)
    else:
        gps_reader = GPSReader()
    
    print(f"GPS file path: {os.path.abspath(gps_reader.gps_file_path)}")
    if ship_name:
        print(f"Ship name: {ship_name}")
    else:
        print("Ship name: Not provided")
    
    # Get stream info using ffprobe
    stream_info = get_stream_info(rtsp_url)
    if not stream_info:
        print("Failed to get stream information. Trying with default settings...")
        stream_info = {'width': 1920, 'height': 1080, 'fps': 25.0}
    
    # Open RTSP stream
    print(f"Opening RTSP stream: {rtsp_url}")
    cap = cv2.VideoCapture(rtsp_url)
    
    if not cap.isOpened():
        print(f"Error: Could not open RTSP stream: {rtsp_url}")
        return False
    
    # Set buffer size to reduce latency
    cap.set(cv2.CAP_PROP_BUFFERSIZE, 1)
    
    # Get actual frame dimensions (in case ffprobe was wrong)
    ret, test_frame = cap.read()
    if not ret:
        print("Error: Could not read frame from stream")
        cap.release()
        return False
    
    actual_height, actual_width = test_frame.shape[:2]
    print(f"Actual frame size: {actual_width}x{actual_height}")
    
    # Create UDP streamer
    print(f"Starting UDP stream to: {udp_address}")
    streamer = create_udp_streamer(udp_address, actual_width, actual_height, stream_info['fps'])
    if not streamer:
        cap.release()
        return False
    
    frame_count = 0
    start_time = datetime.datetime.now()
    last_timestamp_str = start_time.strftime("%Y-%m-%d %H:%M:%S")
    current_gps_data = gps_reader.read_gps_data()  # Initial GPS read
    
    print("Streaming started. Press Ctrl+C to interrupt.")
    
    try:
        # Process the test frame first
        timestamp_str = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        frame_with_overlay = add_overlay_info(test_frame.copy(), timestamp_str, current_gps_data, ship_name)
        try:
            streamer.stdin.write(frame_with_overlay.tobytes())
            streamer.stdin.flush()
        except BrokenPipeError:
            print("Warning: UDP stream connection broken")
        frame_count += 1
        
        while True:
            ret, frame = cap.read()
            if not ret:
                print("Warning: Failed to read frame, attempting to reconnect...")
                cap.release()
                cap = cv2.VideoCapture(rtsp_url)
                cap.set(cv2.CAP_PROP_BUFFERSIZE, 1)
                continue
            
            # Get current timestamp
            current_time = datetime.datetime.now()
            timestamp_str = current_time.strftime("%Y-%m-%d %H:%M:%S")
            
            # Update GPS data when timestamp changes (second changes)
            if timestamp_str != last_timestamp_str:
                current_gps_data = gps_reader.read_gps_data()
                last_timestamp_str = timestamp_str
            
            # Add timestamp, ship name and GPS overlay
            frame_with_overlay = add_overlay_info(frame.copy(), timestamp_str, current_gps_data, ship_name)
            
            # Send frame to UDP stream
            try:
                streamer.stdin.write(frame_with_overlay.tobytes())
                streamer.stdin.flush()
            except BrokenPipeError:
                print("Warning: UDP stream connection broken, attempting to restart...")
                # Try to restart the streamer
                streamer.terminate()
                streamer = create_udp_streamer(udp_address, actual_width, actual_height, stream_info['fps'])
                if not streamer:
                    print("Failed to restart UDP streamer, stopping...")
                    break
            frame_count += 1
            
            # Add a small delay to prevent CPU overload
            import time
            time.sleep(0.001)
                
            # Check duration limit
            if duration_seconds and (datetime.datetime.now() - start_time).total_seconds() > duration_seconds:
                print(f"Streaming duration limit ({duration_seconds}s) reached")
                break
                
            # Print progress every 100 frames
            if frame_count % 100 == 0:
                elapsed = (datetime.datetime.now() - start_time).total_seconds()
                print(f"Streamed {frame_count} frames in {elapsed:.1f} seconds")
    
    except KeyboardInterrupt:
        print("\nStreaming interrupted by user")
    
    except Exception as e:
        print(f"Error during streaming: {e}")
    
    finally:
        # Cleanup
        cap.release()
        if streamer:
            streamer.stdin.close()
            streamer.terminate()
            streamer.wait()
        
        elapsed = (datetime.datetime.now() - start_time).total_seconds()
        print(f"Streaming completed: {frame_count} frames in {elapsed:.1f} seconds")
        print(f"UDP stream sent to: {udp_address}")
    
    return True


def main():
    parser = argparse.ArgumentParser(description='Stream RTSP with timestamp, ship name and GPS overlay to UDP')
    parser.add_argument('--url', '-u', 
                        required=True,
                        help='RTSP stream URL')
    parser.add_argument('--output', '-o',
                        required=True,
                        help='UDP output address')
    parser.add_argument('--ship-name', '-s',
                        required=True,
                        help='Ship name to display in overlay')
    parser.add_argument('--gps-file', '-g',
                        required=True,
                        help='GPS file path')
    parser.add_argument('--duration', '-d', type=int,
                        help='Streaming duration in seconds (optional)')
    
    args = parser.parse_args()
    
    print("=== RTSP to UDP Streamer with Ship Name & GPS Overlay ===")
    print(f"Input URL: {args.url}")
    print(f"UDP output: {args.output}")
    print(f"Ship name: {args.ship_name}")
    
    # Check GPS file exists
    if not os.path.exists(args.gps_file):
        print(f"Error: GPS file not found: {args.gps_file}")
        return 1
    
    print(f"GPS file: {args.gps_file}")
    if args.duration:
        print(f"Duration: {args.duration} seconds")
    print("=" * 55)
    
    success = stream_with_overlay(args.url, args.output, args.ship_name, args.gps_file, args.duration)
    
    if success:
        print("Streaming completed successfully!")
        return 0
    else:
        print("Streaming failed!")
        return 1


if __name__ == "__main__":
    sys.exit(main()) 