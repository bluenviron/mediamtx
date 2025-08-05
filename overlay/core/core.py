import argparse
import sys
import time
from gps.gps import initialize_gps_manager, GPSConfig, close_gps_manager
from .conf import load_config, load_env_config
from pipeline.pipeline import create_pipeline

def main():
    parser = argparse.ArgumentParser(description='MediaMTX Overlay Application')
    parser.add_argument("configpath", 
                  help="path to a config file. The default is overlay.yml.")

    if len(sys.argv)==1:
        parser.print_help(sys.stderr)
        sys.exit(1)
      
    args = parser.parse_args()
    config = load_config(args.configpath)

    try:
        db_config = load_env_config()
        print(f"Database configuration loaded: {db_config['host']}:{db_config['port']}")
    except Exception as e:
        print(f"Error loading environment configuration: {e}")
        return 1

    try:
        gps_config = GPSConfig(
            database_host=db_config['host'],
            database_port=db_config['port'],
            database_user=db_config['user'],
            database_password=db_config['password'],
            database_name=db_config['database'],
        )
        
        gps_manager = initialize_gps_manager(gps_config)
        print("GPS manager initialized successfully")
        
    except Exception as e:
        print(f"Error initializing GPS manager: {e}")
        raise

    # Start GStreamer pipelines for each camera
    pipelines = []
    if "paths" in config:
        try:
            for camera_name, camera_config in config["paths"].items():
                if isinstance(camera_config, dict) and "source" in camera_config and "output" in camera_config:
                    # Parse output URL to get host and port
                    output_url = camera_config["output"]
                    if output_url.startswith("udp://"):
                        # Extract host and port from udp://host:port format
                        host_port = output_url[6:]  # Remove "udp://"
                        if ":" in host_port:
                            host, port_str = host_port.split(":", 1)
                            port = int(port_str)
                        else:
                            host = host_port
                            port = 5000
                    else:
                        # Default values if not UDP format
                        host = "127.0.0.1"
                        port = 5000
                    
                    print(f"Starting overlay for {camera_name}: {camera_config['source']} -> {host}:{port}")
                    pipeline = create_pipeline(
                        input_uri=camera_config["source"],
                        output_host=host,
                        output_port=port
                    )
                    pipeline.start()
                    pipelines.append(pipeline)
                    print(f"GStreamer overlay started for {camera_name}")
            
            if pipelines:
                print(f"Started {len(pipelines)} pipeline(s)")
                
                # Keep running for overlays
                try:
                    while True:
                        time.sleep(1)
                except KeyboardInterrupt:
                    print("\nStopping overlays...")
                    for pipeline in pipelines:
                        pipeline.stop()
                    print("All overlays stopped.")
            else:
                print("No overlays configured")
        except Exception as e:
            print(f"Error starting overlays: {e}")
    
    try:
        close_gps_manager()
        print("GPS manager closed")
    except Exception as e:
        print(f"Error closing GPS manager: {e}")

    return 0