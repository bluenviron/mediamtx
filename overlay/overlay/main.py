#!/usr/bin/env python3

################################################################################
# SPDX-FileCopyrightText: Copyright (c) 2021 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
################################################################################
import sys
sys.path.append("../")
from common.bus_call import bus_call
from common.is_aarch_64 import is_aarch64
from common.FPS import PERF_DATA
import pyds
import platform
import math
import time
from ctypes import *
import gi
gi.require_version("Gst", "1.0")
from gi.repository import Gst, GLib
import configparser
import datetime
import yaml
import threading
import argparse
import urllib.parse
import psycopg2
import os
from jtop import jtop

# 🗄️ PostgreSQL 환경 변수 설정 예시
# export POSTGRES_HOST=localhost
# export POSTGRES_DB=gpsdb
# export POSTGRES_USER=gpsuser
# export POSTGRES_PASSWORD=gpspassword

# 🗄️ PostgreSQL 전역 연결 변수
postgres_connection = None
postgres_cursor = None

MAX_DISPLAY_LEN = 64
PGIE_CLASS_ID_FACE = 0  # FaceDetect 모델에서는 Face 클래스만 검출
MUXER_OUTPUT_WIDTH = 1920
MUXER_OUTPUT_HEIGHT = 1080
MUXER_BATCH_TIMEOUT_USEC = 4000000
TILED_OUTPUT_WIDTH = 1280
TILED_OUTPUT_HEIGHT = 720
GST_CAPS_FEATURES_NVMM = "memory:NVMM"
OSD_PROCESS_MODE = 0  # CPU 모드 (filled box와 복잡한 렌더링 지원)
OSD_DISPLAY_TEXT = 1  # 텍스트 표시 활성화
pgie_classes_str = ["Face"]  # FaceDetect 모델은 얼굴만 검출

# 🚀 성능 최적화를 위한 추론 간격 설정
INFERENCE_INTERVAL = 1  # 5프레임마다 한 번씩 추론 수행

# 이전 검출 결과 캐시 (각 카메라별)
previous_detections = {}

# Camera name mapping for source_id
camera_id_to_name = {}

# 📊 성능 모니터링을 위한 전역 변수
perf_data = None
frame_drop_counts = {}
latency_measurements = {}
last_frame_timestamps = {}

# 📊 프레임 드랍 및 지연 통계
class PerformanceMonitor:
    def __init__(self, num_cameras):
        self.num_cameras = num_cameras
        self.frame_counts = {i: 0 for i in range(num_cameras)}
        self.dropped_frames = {i: 0 for i in range(num_cameras)}
        self.last_timestamps = {i: None for i in range(num_cameras)}
        self.latencies = {i: [] for i in range(num_cameras)}
        self.start_time = time.time()
        
    def update_frame(self, source_id, frame_number, timestamp=None):
        self.frame_counts[source_id] += 1
        
        # 프레임 드랍 감지
        if self.last_timestamps.get(source_id):
            expected_frame = self.last_timestamps[source_id] + 1
            if frame_number > expected_frame:
                dropped = frame_number - expected_frame
                self.dropped_frames[source_id] += dropped
        
        self.last_timestamps[source_id] = frame_number
        
        # 지연 측정 (타임스탬프가 있는 경우)
        if timestamp:
            current_time = time.time() * 1000  # ms 단위
            latency = current_time - timestamp
            self.latencies[source_id].append(latency)
            # 최근 100개 측정값만 유지
            if len(self.latencies[source_id]) > 100:
                self.latencies[source_id].pop(0)
    
    def get_stats(self):
        current_time = time.time()
        elapsed = current_time - self.start_time
        
        stats = {}
        for source_id in range(self.num_cameras):
            camera_name = camera_id_to_name.get(source_id, f"Camera-{source_id}")
            
            total_frames = self.frame_counts[source_id]
            dropped_frames = self.dropped_frames[source_id]
            fps = total_frames / elapsed if elapsed > 0 else 0
            drop_rate = (dropped_frames / (total_frames + dropped_frames)) * 100 if (total_frames + dropped_frames) > 0 else 0
            
            avg_latency = 0
            if self.latencies[source_id]:
                avg_latency = sum(self.latencies[source_id]) / len(self.latencies[source_id])
            
            stats[camera_name] = {
                'fps': round(fps, 2),
                'total_frames': total_frames,
                'dropped_frames': dropped_frames,
                'drop_rate': round(drop_rate, 2),
                'avg_latency_ms': round(avg_latency, 2)
            }
        
        return stats

# 전역 성능 모니터 인스턴스
performance_monitor = None

# 전역 파이프라인 변수 (TextOverlay 업데이트용)
global_pipeline = None
global_textoverlay_elements = {}  # 카메라별 textoverlay 요소 저장

# Jetson 통계 모니터링
jetson = None

def get_gps_info():
    """PostgreSQL에서 최신 GPS 정보를 가져옵니다."""
    global postgres_connection, postgres_cursor
    
    # 연결이 없거나 끊어진 경우 재연결
    if postgres_connection is None or postgres_cursor is None:
        try:
            postgres_connection = psycopg2.connect(
                host=os.getenv('POSTGRES_HOST', 'localhost'),
                database=os.getenv('POSTGRES_DB', 'gpsdb'),
                user=os.getenv('POSTGRES_USER', 'gpsuser'),
                password=os.getenv('POSTGRES_PASSWORD', 'gpspassword'),
                port=5432
            )
            postgres_cursor = postgres_connection.cursor()
            print("✅ PostgreSQL connection successful")
        except (Exception, psycopg2.Error) as error:
            print(f"Error while connecting to PostgreSQL: {error}")
            return "GPS DB Error"

    try:
        # 연결 상태 확인 및 재연결 시도
        postgres_connection.rollback()  # 이전 트랜잭션 정리
        
        query = """
            SELECT timestamp, latitude, longitude, status
            FROM gps_data
            WHERE latitude IS NOT NULL AND longitude IS NOT NULL
            ORDER BY timestamp DESC
            LIMIT 1
        """
        postgres_cursor.execute(query)
        record = postgres_cursor.fetchone()
        
        if record:
            timestamp, lat, lon, status = record
            # 소수점 6자리까지 포맷
            return f"Lat: {lat:.6f}, Lon: {lon:.6f}"
        else:
            return "No GPS data"

    except (Exception, psycopg2.Error) as error:
        print(f"Error while executing GPS query: {error}")
        # 연결 오류인 경우 연결 초기화
        try:
            if postgres_cursor:
                postgres_cursor.close()
                postgres_cursor = None
            if postgres_connection:
                postgres_connection.close()
                postgres_connection = None
        except:
            pass
        return "GPS DB Error"


def cleanup_postgres_connection():
    """PostgreSQL 연결을 정리합니다."""
    global postgres_connection, postgres_cursor
    
    try:
        if postgres_cursor:
            postgres_cursor.close()
            postgres_cursor = None
            print("✅ PostgreSQL cursor closed")
        
        if postgres_connection:
            postgres_connection.close()
            postgres_connection = None
            print("✅ PostgreSQL connection closed")
            
    except Exception as e:
        print(f"Error closing PostgreSQL connection: {e}")


def get_python_processes_info():
    """Jetson에서 Python 프로세스 정보를 가져옵니다."""
    global jetson
    
    try:
        if jetson is None:
            return "Jetson stats not available"
        
        python_processes = []
        
        # processes[][9]에서 Python을 포함하는 프로세스 찾기
        for process in jetson.processes:
            if len(process) > 9 and 'python' in str(process[9]).lower():
                # 프로세스 정보 추출 (PID, 이름, CPU%, 메모리 등)
                pid = process[0] if len(process) > 0 else "N/A"

                current_pid = os.getpid()
                if pid != current_pid:
                    continue

                cpu = process[6] if len(process) > 6 else 0
                mem = process[7] / 1024 if len(process) > 7 else 0
                gpu = process[8] / 1024 if len(process) > 8 else 0
                
                python_processes.append(f"{pid}, CPU{int(cpu)}%, MEM{int(mem)}MB, GPU{int(gpu)}MB")
        
        if python_processes:
            return " | ".join(python_processes[:3])  # 최대 3개 프로세스만 표시
        else:
            return "No Python processes"
            
    except Exception as e:
        print(f"Error getting Python processes info: {e}")
        return "Process info error"

def get_current_time():
    """현재 시간을 포맷된 문자열로 반환합니다."""
    return datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")

def update_textoverlay():
    """모든 textoverlay 요소의 텍스트를 업데이트합니다."""
    global global_textoverlay_elements
    
    current_time = get_current_time()
    gps_info = get_gps_info()
    python_processes_info = get_python_processes_info()
    display_text = f"Time: {current_time}\nGPS: {gps_info}\nPython: {python_processes_info}"
    
    # 모든 textoverlay 요소 업데이트
    for camera_name, textoverlay_element in global_textoverlay_elements.items():
        try:
            textoverlay_element.set_property("text", display_text)
        except Exception as e:
            print(f"Error updating textoverlay for {camera_name}: {e}")
    
    return True  # 콜백 계속 실행

# pgie_src_pad_buffer_probe will extract metadata received on OSD sink pad
# and update params for drawing rectangle, object information etc.
def pgie_src_pad_buffer_probe(pad, info, u_data):
    global performance_monitor, perf_data
    frame_number = 0
    num_rects = 0
    gst_buffer = info.get_buffer()
    if not gst_buffer:
        print("Unable to get GstBuffer ")
        return

    # Retrieve batch metadata from the gst_buffer
    batch_meta = pyds.gst_buffer_get_nvds_batch_meta(hash(gst_buffer))
    l_frame = batch_meta.frame_meta_list
    while l_frame is not None:
        try:
            frame_meta = pyds.NvDsFrameMeta.cast(l_frame.data)
        except StopIteration:
            break

        frame_number = frame_meta.frame_num
        source_id = frame_meta.source_id
        camera_name = camera_id_to_name.get(source_id, f"Unknown-{source_id}")
        
        # 📊 성능 모니터링 업데이트
        if performance_monitor:
            # NTP 타임스탬프 가져오기 (지연 측정용)
            timestamp_ms = None
            if hasattr(frame_meta, 'ntp_timestamp') and frame_meta.ntp_timestamp > 0:
                timestamp_ms = frame_meta.ntp_timestamp / 1000000  # ns를 ms로 변환
            
            performance_monitor.update_frame(source_id, frame_number, timestamp_ms)
        
        # FPS 업데이트
        if perf_data:
            perf_data.update_fps(f"stream{source_id}")
        
        # 🚀 추론 간격 체크 - 5프레임마다 실제 추론, 나머지는 캐시 사용
        is_inference_frame = (frame_number % INFERENCE_INTERVAL) == 0
        
        # 🔴 객체 메타데이터 처리 추가 - 빨간색 outline box + blur 효과
        num_rects = frame_meta.num_obj_meta
        l_obj = frame_meta.obj_meta_list
        objects_detected = 0
        
        # 추론 프레임인 경우: 새로운 검출 결과 처리 및 캐시 업데이트
        if is_inference_frame:
            current_detections = []
            
            while l_obj is not None:
                try:
                    obj_meta = pyds.NvDsObjectMeta.cast(l_obj.data)
                    objects_detected += 1
                    
                    # 검출 결과를 캐시에 저장
                    detection_info = {
                        'left': obj_meta.rect_params.left,
                        'top': obj_meta.rect_params.top,
                        'width': obj_meta.rect_params.width,
                        'height': obj_meta.rect_params.height,
                        'confidence': obj_meta.confidence
                    }
                    current_detections.append(detection_info)
                    
                    # ⚫ 검은색 filled box 설정 + blur 효과
                    obj_meta.rect_params.border_color.set(0.0, 0.0, 0.0, 1.0)  # 검은색 테두리
                    obj_meta.rect_params.border_width = 2
                    obj_meta.rect_params.has_bg_color = 1  # 배경색 활성화
                    obj_meta.rect_params.bg_color.set(0.0, 0.0, 0.0, 1.0)  # 검은색 배경 
                    
                    # Blur 효과 적용
                    try:
                        if hasattr(obj_meta.rect_params, 'blur_objs_inside_bbox'):
                            obj_meta.rect_params.blur_objs_inside_bbox = 1
                        obj_meta.obj_label = f"Face-BLUR [NEW]"  # 새 검출 표시
                        if len(obj_meta.misc_obj_info) > 0:
                            obj_meta.misc_obj_info[0] = 1
                    except Exception as blur_error:
                        print(f"Blur setting error: {blur_error}")
                        # Blur 실패 시 검은색 배경으로 대체
                        obj_meta.rect_params.has_bg_color = 1
                        obj_meta.rect_params.bg_color.set(0.0, 0.0, 0.0, 1.0)

                except StopIteration:
                    break
                
                try:
                    l_obj = l_obj.next
                except StopIteration:
                    break
                    
            # 캐시 업데이트
            previous_detections[source_id] = current_detections
            
        else:
            # 🔄 캐시된 결과 사용 - 추론 스킵된 프레임
            cached_detections = previous_detections.get(source_id, [])
            objects_detected = len(cached_detections)
            
            # 캐시된 검출 결과를 현재 프레임에 적용
            if cached_detections:
                # 기존 객체 메타데이터 제거 (새로운 검출 결과가 없는 경우)
                while l_obj is not None:
                    try:
                        obj_meta = pyds.NvDsObjectMeta.cast(l_obj.data)
                        l_obj = l_obj.next
                    except StopIteration:
                        break
                
                # 캐시된 검출 결과로 새 객체 메타데이터 생성
                for detection in cached_detections:
                    try:
                        # 새 객체 메타데이터 생성
                        obj_meta = pyds.allocate_nvds_obj_meta()
                        
                        # 검출 정보 설정
                        obj_meta.rect_params.left = detection['left']
                        obj_meta.rect_params.top = detection['top']
                        obj_meta.rect_params.width = detection['width']
                        obj_meta.rect_params.height = detection['height']
                        obj_meta.confidence = detection['confidence']
                        
                        # ⚫ 검은색 filled box 설정 (캐시된 결과)
                        obj_meta.rect_params.border_color.set(0.0, 0.0, 0.0, 1.0)  # 검은색 테두리
                        obj_meta.rect_params.border_width = 2
                        obj_meta.rect_params.has_bg_color = 1  # 배경색 활성화
                        obj_meta.rect_params.bg_color.set(0.0, 0.0, 0.0, 1.0)  # 검은색 배경
                        
                        # Blur 효과 적용
                        try:
                            if hasattr(obj_meta.rect_params, 'blur_objs_inside_bbox'):
                                obj_meta.rect_params.blur_objs_inside_bbox = 1
                            obj_meta.obj_label = f"Face-BLUR [CACHED]"  # 캐시된 검출 표시
                            if len(obj_meta.misc_obj_info) > 0:
                                obj_meta.misc_obj_info[0] = 1
                        except Exception as blur_error:
                            print(f"Blur setting error (cached): {blur_error}")
                            obj_meta.rect_params.has_bg_color = 1
                            obj_meta.rect_params.bg_color.set(0.0, 0.0, 0.0, 1.0)
                        
                        # 프레임 메타데이터에 객체 추가
                        pyds.nvds_add_obj_meta_to_frame(frame_meta, obj_meta, None)
                        
                    except Exception as e:
                        print(f"Error creating cached object meta: {e}")
        
        # 📊 로그 출력 - 추론/캐시 상태 표시
        if frame_number % 12 == 0:
            status = "🔍 INFERENCE" if is_inference_frame else "📋 CACHED"
            print(f"Camera: {camera_name}, Frame: {frame_number}, {status}, Objects: {objects_detected}")
        
        if ts_from_rtsp:
            ts = frame_meta.ntp_timestamp/1000000000
            print(f"RTSP Timestamp: {datetime.datetime.utcfromtimestamp(ts).strftime('%Y-%m-%d %H:%M:%S')}")

        try:
            l_frame = l_frame.next
        except StopIteration:
            break

    return Gst.PadProbeReturn.OK


def perf_callback():
    """📊 성능 통계를 주기적으로 출력하는 콜백"""
    global performance_monitor, perf_data
    
    # FPS 통계 출력
    if perf_data:
        perf_data.perf_print_callback()
    
    # 상세 성능 통계 출력
    if performance_monitor:
        stats = performance_monitor.get_stats()
        print("\n🔥 ===== 상세 성능 통계 =====")
        for camera_name, stat in stats.items():
            print(f"📹 {camera_name}:")
            print(f"   FPS: {stat['fps']}")
            print(f"   총 프레임: {stat['total_frames']}")
            print(f"   드랍된 프레임: {stat['dropped_frames']}")
            print(f"   드랍률: {stat['drop_rate']}%")
            if stat['avg_latency_ms'] > 0:
                print(f"   평균 지연: {stat['avg_latency_ms']}ms")
        print("================================\n")
    
    return True  # 콜백 계속 실행


def setup_latency_optimization(source_bin, camera_name):
    """지연 최적화를 위한 소스 설정"""
    def source_child_added(child_proxy, obj, name, user_data):
        print(f"Source child added: {name} for {camera_name}")
        
        if name.find("source") != -1:
            # RTSP 소스 지연 최적화
            if obj.find_property("latency") is not None:
                obj.set_property("latency", 200)  # 200ms 버퍼링
                print(f"  ✅ Set latency=200ms for {camera_name}")
            
            # 지연 시 프레임 드랍 활성화
            if obj.find_property("drop-on-latency") is not None:
                obj.set_property("drop-on-latency", True)
                print(f"  ✅ Enabled drop-on-latency for {camera_name}")
            
            # NTP 동기화 설정 (타임스탬프 기반 지연 측정)
            if ts_from_rtsp:
                pyds.configure_source_for_ntp_sync(hash(obj))
                print(f"  ✅ Configured NTP sync for {camera_name}")
    
    # uridecodebin의 child-added 신호에 연결
    source_element = source_bin.get_by_name(f"uri-decode-bin-{camera_name}")
    if source_element:
        source_element.connect("child-added", source_child_added, None)


def cb_newpad(decodebin, decoder_src_pad, data):
    print("In cb_newpad\n")
    caps = decoder_src_pad.get_current_caps()
    gststruct = caps.get_structure(0)
    gstname = gststruct.get_name()
    source_bin = data
    features = caps.get_features(0)

    # Need to check if the pad created by the decodebin is for video and not
    # audio.
    print("gstname=", gstname)
    if gstname.find("video") != -1:
        # Link the decodebin pad only if decodebin has picked nvidia
        # decoder plugin nvdec_*. We do this by checking if the pad caps contain
        # NVMM memory features.
        print("features=", features)
        if features.contains("memory:NVMM"):
            # Get the source bin ghost pad
            bin_ghost_pad = source_bin.get_static_pad("src")
            if not bin_ghost_pad.set_target(decoder_src_pad):
                sys.stderr.write(
                    "Failed to link decoder src pad to source bin ghost pad\n"
                )
        else:
            sys.stderr.write(
                " Error: Decodebin did not pick nvidia decoder plugin.\n")


def decodebin_child_added(child_proxy, Object, name, user_data):
    print("Decodebin child added:", name, "\n")
    if name.find("decodebin") != -1:
        Object.connect("child-added", decodebin_child_added, user_data)

    if ts_from_rtsp:
        if name.find("source") != -1:
            pyds.configure_source_for_ntp_sync(hash(Object))


def create_source_bin(index, uri):
    print(f"Creating source bin for index {index}")

    # Create a source GstBin to abstract this bin's content from the rest of the
    # pipeline
    bin_name = "source-bin-%02d" % index
    print(bin_name)
    nbin = Gst.Bin.new(bin_name)
    if not nbin:
        sys.stderr.write(" Unable to create source bin \n")

    # Source element for reading from the uri.
    # We will use decodebin and let it figure out the container format of the
    # stream and the codec and plug the appropriate demux and decode plugins.
    uri_decode_bin = Gst.ElementFactory.make("uridecodebin", f"uri-decode-bin-{index}")
    if not uri_decode_bin:
        sys.stderr.write(" Unable to create uri decode bin \n")
    # We set the input uri to the source element
    uri_decode_bin.set_property("uri", uri)
    # Connect to the "pad-added" signal of the decodebin which generates a
    # callback once a new pad for raw data has beed created by the decodebin
    uri_decode_bin.connect("pad-added", cb_newpad, nbin)
    uri_decode_bin.connect("child-added", decodebin_child_added, nbin)

    # We need to create a ghost pad for the source bin which will act as a proxy
    # for the video decoder src pad. The ghost pad will not have a target right
    # now. Once the decode bin creates the video decoder and generates the
    # cb_newpad callback, we will set the ghost pad target to the video decoder
    # src pad.
    Gst.Bin.add(nbin, uri_decode_bin)
    bin_pad = nbin.add_pad(
        Gst.GhostPad.new_no_target(
            "src", Gst.PadDirection.SRC))
    if not bin_pad:
        sys.stderr.write(" Failed to add ghost pad in source bin \n")
        return None
    return nbin


def parse_udp_output(output_url):
    """UDP output URL 파싱 (예: udp://238.0.0.1:10121)"""
    try:
        parsed = urllib.parse.urlparse(output_url)
        if parsed.scheme != 'udp':
            raise ValueError(f"Unsupported scheme: {parsed.scheme}")
        
        host = parsed.hostname or "238.0.0.1"
        port = parsed.port or 12345
        
        return host, port
    except Exception as e:
        sys.stderr.write(f"Error parsing UDP output URL '{output_url}': {e}\n")
        return "238.0.0.1", 12345


def create_output_branch(camera_name, source_id, output_url):
    """각 카메라별 UDP 출력 브랜치 생성 (deepstream-udp-simple.py 참조)"""
    print(f"Creating UDP output branch for {camera_name} (source_id: {source_id})")
    
    # Parse UDP output URL
    udp_host, udp_port = parse_udp_output(output_url)
    print(f"UDP output: {udp_host}:{udp_port}")
    
    # Create elements for this output branch
    nvvidconv = Gst.ElementFactory.make("nvvideoconvert", f"nvvidconv-{camera_name}")
    nvosd = Gst.ElementFactory.make("nvdsosd", f"nvosd-{camera_name}")
    nvvidconv_postosd = Gst.ElementFactory.make("nvvideoconvert", f"nvvidconv_postosd-{camera_name}")
    
    # 🕐 텍스트 오버레이 요소 추가
    textoverlay = Gst.ElementFactory.make("textoverlay", f"textoverlay-{camera_name}")
    nvvidconv_text = Gst.ElementFactory.make("nvvideoconvert", f"nvvidconv-text-{camera_name}")
    
    caps = Gst.ElementFactory.make("capsfilter", f"caps-{camera_name}")
    
    # Create encoder
    if codec == "H264":
        encoder = Gst.ElementFactory.make("nvv4l2h264enc", f"encoder-{camera_name}")
    elif codec == "H265":
        encoder = Gst.ElementFactory.make("nvv4l2h265enc", f"encoder-{camera_name}")
    
    # Create H264/H265 parser
    if codec == "H264":
        parser = Gst.ElementFactory.make("h264parse", f"parser-{camera_name}")
    elif codec == "H265":
        parser = Gst.ElementFactory.make("h265parse", f"parser-{camera_name}")
    
    # Create MPEG-TS muxer (like deepstream-udp-simple.py)
    mux = Gst.ElementFactory.make("mpegtsmux", f"mux-{camera_name}")
    
    # Create UDP sink
    udp_sink = Gst.ElementFactory.make("udpsink", f"udpsink-{camera_name}")

    # Check if all elements were created
    elements = [nvvidconv, nvosd, nvvidconv_postosd, textoverlay, nvvidconv_text, caps, encoder, parser, mux, udp_sink]
    for element in elements:
        if not element:
            sys.stderr.write(f"Failed to create element for {camera_name}\n")
            return None

    # Set properties
    caps.set_property("caps", Gst.Caps.from_string("video/x-raw(memory:NVMM), format=I420"))
    
    # 🎨 OSD 속성 설정 - 객체 표시 및 blur 활성화 (텍스트는 textoverlay로 처리)
    nvosd.set_property("process-mode", OSD_PROCESS_MODE)  # 0: CPU 모드 (filled box 지원)
    nvosd.set_property("display-text", 0)  # 0: 텍스트 표시 비활성화 (textoverlay 사용)
    
    # 🕐 TextOverlay 속성 설정
    textoverlay.set_property("font-desc", "Sans, 24")
    textoverlay.set_property("valignment", "bottom")
    textoverlay.set_property("halignment", "center")
    textoverlay.set_property("text", "Loading...")  # 초기 텍스트
    
    # 전역 딕셔너리에 textoverlay 요소 저장
    global_textoverlay_elements[camera_name] = textoverlay
    
    # Blur 관련 속성 설정 (가능한 경우)
    try:
        # DeepStream blur 모드 활성화 시도
        if hasattr(nvosd, 'set_property'):
            # blur 기능 활성화 (DeepStream 버전에 따라 다를 수 있음)
            try:
                nvosd.set_property("enable-blur", True)
            except:
                pass
            try:
                nvosd.set_property("blur-objects", True)  
            except:
                pass
    except Exception as e:
        print(f"OSD blur properties not available: {e}")
    
    encoder.set_property("bitrate", bitrate)
    if is_aarch64():
        encoder.set_property("preset-level", 1)
        encoder.set_property("insert-sps-pps", 1)

    # Set mux properties
    mux.set_property("alignment", 1)
    
    # Set UDP sink properties
    udp_sink.set_property("host", udp_host)
    udp_sink.set_property("port", udp_port)
    udp_sink.set_property("auto-multicast", True)

    return {
        'elements': elements,
        'nvvidconv': nvvidconv,
        'nvosd': nvosd,
        'nvvidconv_postosd': nvvidconv_postosd,
        'textoverlay': textoverlay,
        'nvvidconv_text': nvvidconv_text,
        'caps': caps,
        'encoder': encoder,
        'parser': parser,
        'mux': mux,
        'udp_sink': udp_sink,
        'udp_host': udp_host,
        'udp_port': udp_port
    }


def create_unified_pipeline(camera_configs):
    """모든 카메라를 하나의 파이프라인으로 통합"""
    global performance_monitor, perf_data, global_pipeline, global_textoverlay_elements
    
    print("Creating unified pipeline for all cameras")
    num_cameras = len(camera_configs)
    
    # 📊 성능 모니터링 초기화
    performance_monitor = PerformanceMonitor(num_cameras)
    perf_data = PERF_DATA(num_cameras)
    print(f"📊 Performance monitoring initialized for {num_cameras} cameras")
    
    # Create main pipeline
    pipeline = Gst.Pipeline("unified-pipeline")
    if not pipeline:
        sys.stderr.write("Unable to create unified pipeline\n")
        return None, None
    
    # 전역 파이프라인 변수 설정
    global_pipeline = pipeline

    # Create unified streammux
    streammux = Gst.ElementFactory.make("nvstreammux", "unified-streammux")
    if not streammux:
        sys.stderr.write("Unable to create unified NvStreamMux\n")
        return None, None

    # Set streammux properties for batch processing
    streammux.set_property("width", MUXER_OUTPUT_WIDTH)
    streammux.set_property("height", MUXER_OUTPUT_HEIGHT)
    streammux.set_property("batched-push-timeout", MUXER_BATCH_TIMEOUT_USEC)
    streammux.set_property("batch-size", 1)
    
    if ts_from_rtsp:
        streammux.set_property("attach-sys-ts", 0)

    # Create single inference engine for all cameras
    if gie == "nvinfer":
        pgie = Gst.ElementFactory.make("nvinfer", "unified-pgie")
    else:
        pgie = Gst.ElementFactory.make("nvinferserver", "unified-pgie")
    
    if not pgie:
        sys.stderr.write("Unable to create unified pgie\n")
        return None, None

    # Set pgie properties for batch processing
    if gie == "nvinfer":
        pgie.set_property("config-file-path", "facedetect_pgie_config.txt")
    pgie.set_property("batch-size", 1)
    
    # 🚀 핵심 최적화: 5프레임마다 한 번씩만 추론 수행
    pgie.set_property("interval", INFERENCE_INTERVAL)
    print(f"🚀 GPU 최적화: {INFERENCE_INTERVAL}프레임마다 추론 수행 (GPU 사용량 {100//INFERENCE_INTERVAL}% 절약)")

    # Create nvstreamdemux to separate streams after inference
    streamdemux = Gst.ElementFactory.make("nvstreamdemux", "stream-demux")
    if not streamdemux:
        sys.stderr.write("Unable to create nvstreamdemux\n")
        return None, None

    # Add main elements to pipeline
    pipeline.add(streammux)
    pipeline.add(pgie)
    pipeline.add(streamdemux)

    # Create source bins and connect to streammux
    source_bins = []
    
    for idx, (camera_name, camera_config) in enumerate(camera_configs.items()):
        source_uri = camera_config.get('source')
        if not source_uri:
            sys.stderr.write(f"No source specified for camera {camera_name}\n")
            continue

        # Store camera name mapping
        camera_id_to_name[idx] = camera_name
        
        # Create source bin
        source_bin = create_source_bin(idx, source_uri)
        if not source_bin:
            continue

        pipeline.add(source_bin)
        source_bins.append((camera_name, source_bin, idx))

        # 🚀 지연 최적화 설정 적용
        setup_latency_optimization(source_bin, camera_name)

        # Link source to streammux
        sinkpad = streammux.get_request_pad(f"sink_{idx}")
        if sinkpad:
            srcpad = source_bin.get_static_pad("src")
            if srcpad:
                srcpad.link(sinkpad)
            else:
                sys.stderr.write(f"Failed to get src pad for {camera_name}\n")
        else:
            sys.stderr.write(f"Failed to get sink pad for {camera_name}\n")

    # Link main processing chain
    streammux.link(pgie)
    pgie.link(streamdemux)

    # Create output branches for each camera
    output_branches = []
    
    for idx, (camera_name, _, source_id) in enumerate(source_bins):
        # Get camera config for output URL
        camera_config = camera_configs.get(camera_name)
        if not camera_config:
            sys.stderr.write(f"No config found for camera {camera_name}\n")
            continue
            
        output_url = camera_config.get('output')
        if not output_url:
            sys.stderr.write(f"No output URL specified for camera {camera_name}\n")
            continue

        # Create output branch
        branch = create_output_branch(camera_name, source_id, output_url)
        if not branch:
            continue

        # Add branch elements to pipeline
        for element in branch['elements']:
            pipeline.add(element)

        # Link elements within the branch (with textoverlay)
        branch['nvvidconv'].link(branch['nvosd'])
        branch['nvosd'].link(branch['nvvidconv_postosd'])
        branch['nvvidconv_postosd'].link(branch['textoverlay'])
        branch['textoverlay'].link(branch['nvvidconv_text'])
        branch['nvvidconv_text'].link(branch['caps'])
        branch['caps'].link(branch['encoder'])
        branch['encoder'].link(branch['parser'])
        branch['parser'].link(branch['mux'])
        branch['mux'].link(branch['udp_sink'])

        # Link demux to output branch
        demux_srcpad = streamdemux.get_request_pad(f"src_{source_id}")
        if demux_srcpad:
            branch_sinkpad = branch['nvvidconv'].get_static_pad("sink")
            if branch_sinkpad:
                demux_srcpad.link(branch_sinkpad)
            else:
                sys.stderr.write(f"Failed to get sink pad for branch {camera_name}\n")
        else:
            sys.stderr.write(f"Failed to get demux src pad for {camera_name}\n")

        output_branches.append((camera_name, branch))

    # Add probe to pgie src pad
    pgie_src_pad = pgie.get_static_pad("src")
    if pgie_src_pad:
        pgie_src_pad.add_probe(Gst.PadProbeType.BUFFER, pgie_src_pad_buffer_probe, 0)

    print(f"Unified pipeline created with {len(source_bins)} cameras")
    return pipeline, output_branches


# 🔧 파이프라인 상태를 추적하는 전역 변수 추가
pipeline_stopped = False

def run_unified_pipeline(pipeline):
    """통합 파이프라인 실행"""
    global pipeline_stopped
    print("Starting unified pipeline")
    
    # Create event loop for this pipeline
    loop = GLib.MainLoop()
    bus = pipeline.get_bus()
    bus.add_signal_watch()
    bus.connect("message", bus_call, loop)

    # 📊 성능 모니터링 타이머 설정 (5초마다 실행)
    GLib.timeout_add_seconds(5, perf_callback)
    print("📊 Performance monitoring timer started (5 seconds interval)")
    
    # 🕐 TextOverlay 업데이트 타이머 설정 (1초마다 실행)
    GLib.timeout_add_seconds(1, update_textoverlay)
    print("🕐 TextOverlay update timer started (1 second interval)")

    # Start pipeline
    print("Setting pipeline to PLAYING state...")
    ret = pipeline.set_state(Gst.State.PLAYING)
    if ret == Gst.StateChangeReturn.FAILURE:
        print("ERROR: Unable to set the pipeline to the playing state")
        return
    
    try:
        loop.run()
    except BaseException as e:
        print(f"Unified pipeline stopped: {e}")
    finally:
        # 🔧 안전한 파이프라인 정리
        if not pipeline_stopped:
            print("Stopping pipeline safely...")
            pipeline.set_state(Gst.State.PAUSED)
            pipeline.get_state(Gst.CLOCK_TIME_NONE)  # 상태 전환 대기
            pipeline.set_state(Gst.State.READY)
            pipeline.get_state(Gst.CLOCK_TIME_NONE)  # 상태 전환 대기
            pipeline.set_state(Gst.State.NULL)
            pipeline.get_state(Gst.CLOCK_TIME_NONE)  # 상태 전환 대기
            pipeline_stopped = True
            print("Pipeline stopped safely")


def main(config_file):
    global pipeline_stopped, jetson
    pipeline_stopped = False
    
    # Initialize Jetson stats
    try:
        jetson = jtop()
        jetson.start()
        print("✅ Jetson stats initialized successfully")
    except Exception as e:
        print(f"⚠️ Warning: Could not initialize Jetson stats: {e}")
        jetson = None
    
    # Load YAML configuration
    try:
        with open(config_file, 'r') as file:
            config = yaml.safe_load(file)
    except Exception as e:
        sys.stderr.write(f"Error loading YAML config: {e}\n")
        return 1

    if not config:
        sys.stderr.write("Empty or invalid YAML configuration\n")
        return 1

    # Standard GStreamer initialization
    Gst.init(None)

    # 🔧 GStreamer 로그 레벨 조정 - ONVIF 메타데이터 경고 줄이기
    Gst.debug_set_default_threshold(Gst.DebugLevel.ERROR)  # ERROR 레벨만 표시
    # 또는 특정 카테고리만 조정:
    # Gst.debug_set_threshold_for_name("uridecodebin", Gst.DebugLevel.ERROR)

    # Create unified pipeline
    pipeline, output_branches = create_unified_pipeline(config)
    if not pipeline or not output_branches:
        sys.stderr.write("Failed to create unified pipeline\n")
        return 1

    # Print UDP output information
    print(f"\n*** DeepStream: UDP Multicast Streaming Started ***")
    print("UDP Output streams:")
    for camera_name, branch in output_branches:
        udp_host = branch['udp_host']
        udp_port = branch['udp_port']
        print(f"  - Camera {camera_name}: udp://{udp_host}:{udp_port}")
    print()
    print(f"GPU Efficiency: All {len(output_branches)} cameras processed in single batch inference")
    print("Press Ctrl+C to stop...")

    # Start unified pipeline in separate thread
    pipeline_thread = threading.Thread(target=run_unified_pipeline, args=(pipeline,))
    pipeline_thread.daemon = True
    pipeline_thread.start()

    try:
        # Keep main thread alive
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\nShutting down...")
    except BaseException as e:
        print(f"Error: {e}")

    # 🔧 메인 스레드에서는 파이프라인 정리를 하지 않음 (중복 방지)
    if not pipeline_stopped:
        print("Waiting for pipeline thread to finish...")
        pipeline_thread.join(timeout=5.0)
    
    # Clean up Jetson stats
    if jetson:
        try:
            jetson.close()
            print("✅ Jetson stats connection closed")
        except Exception as e:
            print(f"Error closing Jetson stats: {e}")
    
    # Clean up PostgreSQL connection
    cleanup_postgres_connection()
    
    return 0


def parse_args():
    parser = argparse.ArgumentParser(description='DeepStream UDP Multicast StreamMux Application')
    parser.add_argument("-c", "--config", required=True,
                  help="Path to YAML configuration file")
    parser.add_argument("-g", "--gie", default="nvinfer",
                  help="choose GPU inference engine type nvinfer or nvinferserver , default=nvinfer", choices=['nvinfer','nvinferserver'])
    parser.add_argument("--codec", default="H264",
                  help="UDP Streaming Codec H264/H265 , default=H264", choices=['H264','H265'])
    parser.add_argument("-b", "--bitrate", default=10000000,
                  help="Set the encoding bitrate ", type=int)
    parser.add_argument("--rtsp-ts", action="store_true", default=False, dest='rtsp_ts', 
                  help="Attach NTP timestamp from RTSP source")
    
    # Check input arguments
    if len(sys.argv)==1:
        parser.print_help(sys.stderr)
        sys.exit(1)
    
    args = parser.parse_args()
    global codec
    global bitrate
    global gie
    global ts_from_rtsp
    gie = args.gie
    codec = args.codec
    bitrate = args.bitrate
    ts_from_rtsp = args.rtsp_ts
    return args.config

if __name__ == '__main__':
    config_file = parse_args()
    sys.exit(main(config_file))
