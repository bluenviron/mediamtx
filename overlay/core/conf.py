import yaml
import os
from dotenv import load_dotenv

def load_config(config_path):
    with open(config_path, 'r') as file:
        return yaml.safe_load(file)

def load_env_config():
    """Load database configuration from .env file with priority order:
    1. System environment variables (highest priority)
    2. Current working directory .env
    3. Script directory .env
    4. Default values (lowest priority)
    """
    current_dir_env = os.path.join(os.getcwd(), '.env')
    script_dir_env = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '.env')
    
    # Current working directory .env (medium priority)
    if os.path.exists(current_dir_env):
        load_dotenv(current_dir_env, override=False)
        print(f"Loaded .env from current working directory: {current_dir_env}")
    
    # Load .env files in order (lower priority files are loaded first)
    # Script directory .env (lowest priority)
    if os.path.exists(script_dir_env):
        load_dotenv(script_dir_env, override=False)
        print(f"Loaded .env from script directory: {script_dir_env}")
    
    # System environment variables have highest priority (already loaded)
    
    # Get database configuration from environment variables
    db_config = {
        'host': os.getenv('POSTGRES_HOST', 'localhost'),
        'port': int(os.getenv('POSTGRES_PORT', '5432')),
        'user': os.getenv('POSTGRES_USER', 'postgres'),
        'password': os.getenv('POSTGRES_PASSWORD', ''),
        'database': os.getenv('POSTGRES_DB', 'gps_db'),
    }
    
    return db_config 