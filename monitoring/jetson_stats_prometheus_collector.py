#!/usr/bin/python3
# -*- coding: utf-8 -*-

# MIT License
#
# Copyright (c) 2021 Stefan von Cavallar
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
# 
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
# 
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

import time
import atexit
import argparse
import logging
import sys
from jtop import jtop, JtopException
from prometheus_client.core import InfoMetricFamily, GaugeMetricFamily, REGISTRY, CounterMetricFamily
from prometheus_client import start_http_server
from logging_prometheus import ExportingLogHandler

# Configure logging to output to stdout with immediate flush and Prometheus metrics
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.StreamHandler(sys.stdout)
    ]
)
logger = logging.getLogger(__name__)

# Add Prometheus handler for logging metrics
prometheus_handler = ExportingLogHandler()
logger.addHandler(prometheus_handler)

# Force stdout to flush immediately
sys.stdout.reconfigure(line_buffering=True)

class CustomCollector(object):
    def __init__(self):
        atexit.register(self.cleanup)
        logger.info("Initializing Jetson stats collector...")
        try:
            self._jetson = jtop()
            logger.info("jtop instance created successfully")
            self._jetson.start()
            logger.info("jtop started successfully")
        except JtopException as e:
            logger.error(f"Failed to initialize jtop: {e}")
            self._jetson = None
        except Exception as e:
            logger.error(f"Unexpected error during jtop initialization: {e}")
            self._jetson = None

    def cleanup(self):
        logger.info("Closing jetson-stats connection...")
        if self._jetson:
            self._jetson.close()

    def collect(self):
        if not self._jetson:
            logger.warning("jtop not initialized, skipping collection")
            return
            
        try:
            if self._jetson.ok():
                logger.debug("jtop is ready, collecting metrics...")
                
                # Board Information
                yield from self._collect_board_info()
                
                # System metrics
                yield from self._collect_uptime()
                yield from self._collect_nvpmodel()
                
                # CPU metrics
                yield from self._collect_cpu_metrics()
                
                # GPU metrics  
                yield from self._collect_gpu_metrics()
                
                # Memory metrics
                yield from self._collect_memory_metrics()
                
                # Storage metrics
                yield from self._collect_disk_metrics()
                
                # Temperature metrics
                yield from self._collect_temperature_metrics()
                
                # Power metrics
                yield from self._collect_power_metrics()
                
                # Fan metrics
                yield from self._collect_fan_metrics()
                
                # Engine metrics
                yield from self._collect_engine_metrics()
                
                logger.debug("Metrics collection completed successfully")
            else:
                logger.warning("jtop is not ready - may still be initializing or encountering errors")
        except JtopException as e:
            logger.error(f"JtopException during collection: {e}")
        except Exception as e:
            logger.error(f"Unexpected error during collection: {e}")

    def _collect_board_info(self):
        """Board hardware, platform, and libraries information"""
        try:
            if hasattr(self._jetson, 'board') and self._jetson.board:
                logger.debug("Collecting board info")
                
                # Platform info
                if 'platform' in self._jetson.board:
                    platform = self._jetson.board['platform']
                    i = InfoMetricFamily('jetson_board_platform', 'Board platform information')
                    i.add_metric([], {
                        'machine': platform.get('Machine', ''),
                        'system': platform.get('System', ''),
                        'distribution': platform.get('Distribution', ''),
                        'release': platform.get('Release', ''),
                        'python': platform.get('Python', '')
                    })
                    yield i

                # Hardware info  
                if 'hardware' in self._jetson.board:
                    hardware = self._jetson.board['hardware']
                    i = InfoMetricFamily('jetson_board_hardware', 'Board hardware information')
                    i.add_metric([], {
                        'model': hardware.get('Model', ''),
                        'part_number': hardware.get('699-level Part Number', ''),
                        'p_number': hardware.get('P-Number', ''),
                        'module': hardware.get('Module', ''),
                        'soc': hardware.get('SoC', ''),
                        'cuda_arch_bin': hardware.get('CUDA Arch BIN', ''),
                        'codename': hardware.get('Codename', ''),
                        'serial_number': hardware.get('Serial Number', ''),
                        'l4t': hardware.get('L4T', ''),
                        'jetpack': hardware.get('Jetpack', '')
                    })
                    yield i

                # Libraries info
                if 'libraries' in self._jetson.board:
                    libraries = self._jetson.board['libraries']
                    i = InfoMetricFamily('jetson_board_libraries', 'Board libraries information')
                    i.add_metric([], {
                        'cuda': libraries.get('CUDA', ''),
                        'opencv': libraries.get('OpenCV', ''),
                        'opencv_cuda': str(libraries.get('OpenCV-Cuda', False)),
                        'cudnn': libraries.get('cuDNN', ''),
                        'tensorrt': libraries.get('TensorRT', ''),
                        'vpi': libraries.get('VPI', ''),
                        'vulkan': libraries.get('Vulkan', '')
                    })
                    yield i
            else:
                logger.debug("No board info available")
        except Exception as e:
            logger.error(f"Error collecting board info: {e}")

    def _collect_uptime(self):
        """System uptime metrics"""
        try:
            if hasattr(self._jetson, 'uptime') and self._jetson.uptime:
                logger.debug("Collecting uptime")
                g = GaugeMetricFamily('jetson_uptime_seconds', 'System uptime in seconds')
                total_seconds = self._jetson.uptime.total_seconds()
                g.add_metric([], total_seconds)
                yield g
            else:
                logger.debug("No uptime info available")
        except Exception as e:
            logger.error(f"Error collecting uptime: {e}")

    def _collect_nvpmodel(self):
        """NV power model information"""
        try:
            if hasattr(self._jetson, 'nvpmodel'):
                logger.debug("Collecting nvpmodel")
                i = InfoMetricFamily('jetson_nvpmodel', 'Current NV power model')
                i.add_metric([], {'mode': str(self._jetson.nvpmodel)})
                yield i
            else:
                logger.debug("No nvpmodel info available")
        except Exception as e:
            logger.error(f"Error collecting nvpmodel: {e}")

    def _collect_cpu_metrics(self):
        """CPU usage, frequency, and core information"""
        try:
            if hasattr(self._jetson, 'cpu') and self._jetson.cpu:
                logger.debug("Collecting CPU metrics")
            
            # Total CPU usage
            if 'total' in self._jetson.cpu:
                total = self._jetson.cpu['total']
                g = GaugeMetricFamily('jetson_cpu_usage_percent', 'CPU usage percentage', labels=['type'])
                g.add_metric(['user'], total.get('user', 0))
                g.add_metric(['nice'], total.get('nice', 0))
                g.add_metric(['system'], total.get('system', 0))
                g.add_metric(['idle'], total.get('idle', 0))
                yield g

            # Per-core metrics
            if 'cpu' in self._jetson.cpu:
                # Core usage
                g_usage = GaugeMetricFamily('jetson_cpu_core_usage_percent', 'Per-core CPU usage percentage', labels=['core', 'type'])
                g_freq = GaugeMetricFamily('jetson_cpu_core_frequency_hz', 'Per-core CPU frequency in Hz', labels=['core', 'type'])
                g_online = GaugeMetricFamily('jetson_cpu_core_online', 'CPU core online status', labels=['core'])
                
                for i, core in enumerate(self._jetson.cpu['cpu']):
                    core_name = f'cpu{i}'
                    
                    # Online status
                    g_online.add_metric([core_name], 1 if core.get('online', False) else 0)
                    
                    if core.get('online', False):
                        # Usage metrics
                        g_usage.add_metric([core_name, 'user'], core.get('user', 0))
                        g_usage.add_metric([core_name, 'nice'], core.get('nice', 0))
                        g_usage.add_metric([core_name, 'system'], core.get('system', 0))
                        g_usage.add_metric([core_name, 'idle'], core.get('idle', 0))
                        
                        # Frequency metrics
                        if 'freq' in core:
                            freq = core['freq']
                            g_freq.add_metric([core_name, 'current'], freq.get('cur', 0))
                            g_freq.add_metric([core_name, 'min'], freq.get('min', 0))
                            g_freq.add_metric([core_name, 'max'], freq.get('max', 0))
                
                yield g_usage
                yield g_freq
                yield g_online
            else:
                logger.debug("No CPU info available")
        except Exception as e:
            logger.error(f"Error collecting CPU metrics: {e}")

    def _collect_gpu_metrics(self):
        """GPU usage, frequency, and status metrics"""
        try:
            if hasattr(self._jetson, 'gpu') and self._jetson.gpu:
                logger.debug("Collecting GPU metrics")
            for gpu_name, gpu_data in self._jetson.gpu.items():
                
                # GPU load
                if 'status' in gpu_data and 'load' in gpu_data['status']:
                    g = GaugeMetricFamily('jetson_gpu_usage_percent', 'GPU usage percentage', labels=['gpu'])
                    g.add_metric([gpu_name], gpu_data['status']['load'])
                    yield g
                
                # GPU frequency
                if 'freq' in gpu_data:
                    freq = gpu_data['freq']
                    g = GaugeMetricFamily('jetson_gpu_frequency_hz', 'GPU frequency in Hz', labels=['gpu', 'type'])
                    g.add_metric([gpu_name, 'current'], freq.get('cur', 0))
                    g.add_metric([gpu_name, 'min'], freq.get('min', 0))
                    g.add_metric([gpu_name, 'max'], freq.get('max', 0))
                    yield g
                
                # GPU status info
                if 'status' in gpu_data:
                    status = gpu_data['status']
                    g = GaugeMetricFamily('jetson_gpu_status', 'GPU status flags', labels=['gpu', 'status'])
                    g.add_metric([gpu_name, 'railgate'], 1 if status.get('railgate', False) else 0)
                    g.add_metric([gpu_name, 'tpc_pg_mask'], 1 if status.get('tpc_pg_mask', False) else 0)
                    g.add_metric([gpu_name, '3d_scaling'], 1 if status.get('3d_scaling', False) else 0)
                    yield g
            else:
                logger.debug("No GPU info available")
        except Exception as e:
            logger.error(f"Error collecting GPU metrics: {e}")

    def _collect_memory_metrics(self):
        """Memory (RAM, SWAP, EMC) metrics"""
        try:
            if hasattr(self._jetson, 'memory') and self._jetson.memory:
                logger.debug("Collecting memory metrics")
            
            # RAM metrics
            if 'RAM' in self._jetson.memory:
                ram = self._jetson.memory['RAM']
                g = GaugeMetricFamily('jetson_memory_ram_bytes', 'RAM memory usage in bytes', labels=['type'])
                g.add_metric(['total'], ram.get('tot', 0) * 1024)  # Convert KB to bytes
                g.add_metric(['used'], ram.get('used', 0) * 1024)
                g.add_metric(['free'], ram.get('free', 0) * 1024)
                g.add_metric(['buffers'], ram.get('buffers', 0) * 1024)
                g.add_metric(['cached'], ram.get('cached', 0) * 1024)
                g.add_metric(['shared'], ram.get('shared', 0) * 1024)
                yield g

            # SWAP metrics
            if 'SWAP' in self._jetson.memory:
                swap = self._jetson.memory['SWAP']
                g = GaugeMetricFamily('jetson_memory_swap_bytes', 'SWAP memory usage in bytes', labels=['type'])
                g.add_metric(['total'], swap.get('tot', 0) * 1024)
                g.add_metric(['used'], swap.get('used', 0) * 1024)
                g.add_metric(['cached'], swap.get('cached', 0) * 1024)
                yield g
                
                # Individual SWAP devices
                if 'table' in swap:
                    g_dev = GaugeMetricFamily('jetson_memory_swap_device_bytes', 'SWAP device usage in bytes', labels=['device', 'type'])
                    for device, info in swap['table'].items():
                        g_dev.add_metric([device, 'size'], info.get('size', 0) * 1024)
                        g_dev.add_metric([device, 'used'], info.get('used', 0) * 1024)
                    yield g_dev

            # EMC metrics
            if 'EMC' in self._jetson.memory:
                emc = self._jetson.memory['EMC']
                g = GaugeMetricFamily('jetson_memory_emc_frequency_hz', 'EMC frequency in Hz', labels=['type'])
                g.add_metric(['current'], emc.get('cur', 0))
                g.add_metric(['min'], emc.get('min', 0))
                g.add_metric(['max'], emc.get('max', 0))
                yield g
                
                g_status = GaugeMetricFamily('jetson_memory_emc_online', 'EMC online status')
                g_status.add_metric([], 1 if emc.get('online', False) else 0)
                yield g_status
            else:
                logger.debug("No memory info available")
        except Exception as e:
            logger.error(f"Error collecting memory metrics: {e}")

    def _collect_disk_metrics(self):
        """Disk usage metrics"""
        if hasattr(self._jetson, 'disk') and self._jetson.disk:
            g = GaugeMetricFamily('jetson_disk_usage_gb', 'Disk usage in GB', labels=['type'])
            g.add_metric(['total'], self._jetson.disk.get('total', 0))
            g.add_metric(['used'], self._jetson.disk.get('used', 0))
            g.add_metric(['available'], self._jetson.disk.get('available', 0))
            g.add_metric(['available_no_root'], self._jetson.disk.get('available_no_root', 0))
            yield g

    def _collect_temperature_metrics(self):
        """Temperature sensor metrics"""
        if hasattr(self._jetson, 'temperature') and self._jetson.temperature:
            g = GaugeMetricFamily('jetson_temperature_celsius', 'Temperature in Celsius', labels=['sensor'])
            for sensor, data in self._jetson.temperature.items():
                if isinstance(data, dict) and 'temp' in data:
                    g.add_metric([sensor], data['temp'])
            yield g
            
            # Temperature sensor online status
            g_online = GaugeMetricFamily('jetson_temperature_sensor_online', 'Temperature sensor online status', labels=['sensor'])
            for sensor, data in self._jetson.temperature.items():
                if isinstance(data, dict):
                    g_online.add_metric([sensor], 1 if data.get('online', False) else 0)
            yield g_online

    def _collect_power_metrics(self):
        """Power consumption metrics"""
        if hasattr(self._jetson, 'power') and self._jetson.power:
            
            # Rail power metrics
            if 'rail' in self._jetson.power:
                g_power = GaugeMetricFamily('jetson_power_rail_mw', 'Rail power consumption in milliwatts', labels=['rail'])
                g_voltage = GaugeMetricFamily('jetson_power_rail_voltage_mv', 'Rail voltage in millivolts', labels=['rail'])
                g_current = GaugeMetricFamily('jetson_power_rail_current_ma', 'Rail current in milliamps', labels=['rail'])
                
                for rail_name, rail_data in self._jetson.power['rail'].items():
                    g_power.add_metric([rail_name], rail_data.get('power', 0))
                    g_voltage.add_metric([rail_name], rail_data.get('volt', 0))
                    g_current.add_metric([rail_name], rail_data.get('curr', 0))
                
                yield g_power
                yield g_voltage
                yield g_current

            # Total power metrics
            if 'tot' in self._jetson.power:
                tot = self._jetson.power['tot']
                g = GaugeMetricFamily('jetson_power_total_mw', 'Total power consumption in milliwatts', labels=['type'])
                g.add_metric(['current'], tot.get('power', 0))
                g.add_metric(['average'], tot.get('avg', 0))
                yield g

    def _collect_fan_metrics(self):
        """Fan metrics"""
        if hasattr(self._jetson, 'fan') and self._jetson.fan:
            for fan_name, fan_data in self._jetson.fan.items():
                
                # Fan speed percentage
                if 'speed' in fan_data and fan_data['speed']:
                    g = GaugeMetricFamily('jetson_fan_speed_percent', 'Fan speed percentage', labels=['fan'])
                    # speed is a list, take the first value
                    g.add_metric([fan_name], fan_data['speed'][0] if fan_data['speed'] else 0)
                    yield g
                
                # Fan RPM
                if 'rpm' in fan_data and fan_data['rpm']:
                    g = GaugeMetricFamily('jetson_fan_rpm', 'Fan RPM', labels=['fan'])
                    g.add_metric([fan_name], fan_data['rpm'][0] if fan_data['rpm'] else 0)
                    yield g
                
                # Fan profile info
                if 'profile' in fan_data:
                    i = InfoMetricFamily('jetson_fan_profile', 'Fan profile information', labels=['fan'])
                    i.add_metric([fan_name], {
                        'profile': fan_data.get('profile', ''),
                        'governor': fan_data.get('governor', ''),
                        'control': fan_data.get('control', '')
                    })
                    yield i

    def _collect_engine_metrics(self):
        """Hardware engine metrics"""
        if hasattr(self._jetson, 'engine') and self._jetson.engine:
            g_online = GaugeMetricFamily('jetson_engine_online', 'Engine online status', labels=['engine', 'component'])
            g_freq = GaugeMetricFamily('jetson_engine_frequency_hz', 'Engine frequency in Hz', labels=['engine', 'component', 'type'])
            
            for engine_name, engine_data in self._jetson.engine.items():
                for component_name, component_data in engine_data.items():
                    if isinstance(component_data, dict):
                        # Online status
                        g_online.add_metric([engine_name, component_name], 
                                          1 if component_data.get('online', False) else 0)
                        
                        # Frequency metrics
                        if 'cur' in component_data:
                            g_freq.add_metric([engine_name, component_name, 'current'], 
                                            component_data['cur'])
                        if 'min' in component_data:
                            g_freq.add_metric([engine_name, component_name, 'min'], 
                                            component_data['min'])
                        if 'max' in component_data:
                            g_freq.add_metric([engine_name, component_name, 'max'], 
                                            component_data['max'])
            
            yield g_online
            yield g_freq


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Jetson stats Prometheus collector')
    parser.add_argument('--port', type=int, default=8000, help='Metrics collector port number')
    parser.add_argument('--log-level', default='INFO', choices=['DEBUG', 'INFO', 'WARNING', 'ERROR'], 
                        help='Log level')

    args = parser.parse_args()
    
    # Set log level from argument
    logger.setLevel(getattr(logging, args.log_level))

    start_http_server(args.port)
    REGISTRY.register(CustomCollector())
    
    logger.info(f"Jetson stats Prometheus collector started on port {args.port}")
    logger.info("Press Ctrl+C to stop the collector")
    
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        logger.info("Shutting down collector...")
        sys.exit(0)
