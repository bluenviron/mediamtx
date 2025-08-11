import argparse
import sys
import time
from gps.gps import initialize_gps_manager, GPSConfig, close_gps_manager
from .conf import load_config, load_env_config
from pipeline.pipeline import create_pipeline
import logging
from colorama import init, Fore, Back, Style

# Initialize colorama for cross-platform colored output
init(autoreset=True)

class ColoredFormatter(logging.Formatter):
    """Custom formatter with colors for different log levels."""
    
    COLORS = {
        'DEBUG': Fore.CYAN,
        'INFO': Fore.GREEN,
        'WARNING': Fore.YELLOW,
        'ERROR': Fore.RED,
        'CRITICAL': Fore.RED + Back.WHITE + Style.BRIGHT,
    }
    
    def format(self, record):
        # Get the original format
        log_message = super().format(record)
        
        # Add color based on log level
        level_name = record.levelname
        if level_name in self.COLORS:
            # Color only the level name part
            colored_level = f"{self.COLORS[level_name]}[{level_name}]{Style.RESET_ALL}"
            log_message = log_message.replace(f"[{level_name}]", colored_level)
        
        return log_message

# Create a custom logger with colored output
def setup_colored_logging():
    """Setup colored logging configuration."""
    logger = logging.getLogger()
    logger.setLevel(logging.INFO)
    
    # Remove existing handlers
    for handler in logger.handlers[:]:
        logger.removeHandler(handler)
    
    # Create console handler with colored formatter
    console_handler = logging.StreamHandler()
    formatter = ColoredFormatter(
        fmt='[%(levelname)s] %(name)s: %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )
    console_handler.setFormatter(formatter)
    logger.addHandler(console_handler)

# Setup colored logging
setup_colored_logging()
logger = logging.getLogger(__name__)

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
        logger.info(f"Database configuration loaded: {db_config['host']}:{db_config['port']}")
    except Exception as e:
        logger.error(f"Error loading environment configuration: {e}")
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
        logger.info("GPS manager initialized successfully")
        
    except Exception as e:
        logger.error(f"Error initializing GPS manager: {e}")
        raise

    # Start streaming pipelines for each camera
    pipelines = []
    if "paths" in config:
        from pipeline.pipeline import create_pipelines_from_config
        
        try:
            pipelines = create_pipelines_from_config(config, gps_manager)
            
            if not pipelines:
                logger.error("No valid pipelines found in configuration")
                return 1
            
            logger.info(f"Starting {len(pipelines)} streaming pipelines...")
            
            # Start all pipelines
            for i, pipeline in enumerate(pipelines):
                if not pipeline.start():
                    logger.error(f"Failed to start pipeline {i}")
                    return 1
            
            logger.info("All pipelines started successfully")
            
            # Keep running until interrupted
            try:
                while True:
                    time.sleep(1)
            except KeyboardInterrupt:
                logger.info("Shutting down pipelines...")
                for pipeline in pipelines:
                    pipeline.stop()
                logger.info("All pipelines stopped")
                
        except Exception as e:
            logger.error(f"Error during pipeline execution: {e}")
            # Stop all pipelines on error
            for pipeline in pipelines:
                pipeline.stop()
            return 1
    
    try:
        close_gps_manager()
        logger.info("GPS manager closed")
    except Exception as e:
        logger.error(f"Error closing GPS manager: {e}")

    return 0