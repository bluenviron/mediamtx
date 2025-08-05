#!/usr/bin/env python3

import sys
import gi
gi.require_version("Gst", "1.0")
from gi.repository import Gst, GLib
import threading
import time

class Pipeline:
    def __init__(self, input_uri, output_host, output_port):
        self.input_uri = input_uri
        self.output_host = output_host
        self.output_port = output_port
        self.pipeline = None
        self.loop = None
        self.running = False
        self.thread = None
        
    def create_pipeline(self):
        """Create GStreamer pipeline for pipeline processing"""
        # Initialize GStreamer
        Gst.init(None)
        
        # Create pipeline
        self.pipeline = Gst.Pipeline.new("pipeline")
        if not self.pipeline:
            sys.stderr.write("Unable to create pipeline\n")
            return False
        
        # Create muxer
        mux = Gst.ElementFactory.make("mpegtsmux", "mux")
        if not mux:
            sys.stderr.write("Unable to create mpegtsmux\n")
            return False
        
        mux.set_property("alignment", 1)
        
        # Create UDP sink
        udp_sink = Gst.ElementFactory.make("udpsink", "udp-sink")
        if not udp_sink:
            sys.stderr.write("Unable to create udpsink\n")
            return False
        
        udp_sink.set_property("host", self.output_host)
        udp_sink.set_property("port", self.output_port)
        udp_sink.set_property("auto-multicast", True)
        
        # Add mux and sink to pipeline
        self.pipeline.add(mux)
        self.pipeline.add(udp_sink)
        
        # Link mux to udp_sink
        if not mux.link(udp_sink):
            sys.stderr.write("Failed to link mux to udp_sink\n")
            return False
        
        # Create RTSP source elements
        rtsp_src = Gst.ElementFactory.make("rtspsrc", "rtsp-src")
        if not rtsp_src:
            sys.stderr.write("Unable to create rtspsrc\n")
            return False
        
        rtsp_src.set_property("location", self.input_uri)
        rtsp_src.set_property("latency", 200)
        
        # Depayloader and parser
        depay = Gst.ElementFactory.make("rtph264depay", "depay")
        h264parse = Gst.ElementFactory.make("h264parse", "h264parse")
        
        if not depay or not h264parse:
            sys.stderr.write("Unable to create depay or h264parse\n")
            return False
        
        self.pipeline.add(rtsp_src)
        self.pipeline.add(depay)
        self.pipeline.add(h264parse)
        
        # Link depay and parser
        if not depay.link(h264parse):
            sys.stderr.write("Failed to link depay to h264parse\n")
            return False
        
        # Use existing H.264 pipeline without re-encoding (fps, width, height from source)
        if not h264parse.link(mux):
            sys.stderr.write("Failed to link h264parse to mux\n")
            return False
        
        # Connect pad-added signal for rtspsrc
        def on_pad_added(src, pad):
            print(f"Pad added: {pad.get_name()}")
            if "video" in pad.get_name() or pad.get_name().startswith("recv_rtp_src_0"):
                sink_pad = depay.get_static_pad("sink")
                if not sink_pad.is_linked():
                    pad.link(sink_pad)
        
        rtsp_src.connect("pad-added", on_pad_added)
        
        # Create event loop and bus
        self.loop = GLib.MainLoop()
        bus = self.pipeline.get_bus()
        bus.add_signal_watch()
        
        def bus_call(bus, message, loop):
            t = message.type
            if t == Gst.MessageType.EOS:
                print("End-of-pipeline")
                if self.running:
                    loop.quit()
            elif t == Gst.MessageType.ERROR:
                err, debug = message.parse_error()
                print(f"Error: {err}: {debug}")
                if self.running:
                    loop.quit()
            elif t == Gst.MessageType.WARNING:
                warn, debug = message.parse_warning()
                print(f"Warning: {warn}: {debug}")
            return True
        
        bus.connect("message", bus_call, self.loop)
        
        return True
    
    def start(self):
        """Start the pipeline processing"""
        if not self.create_pipeline():
            return False
        
        print(f"Starting pipeline processing")
        print(f"Input: {self.input_uri}")
        print(f"Output: udp://{self.output_host}:{self.output_port}")
        print("Video: passthrough (fps, width, height from source)")
        
        # Start pipeline
        ret = self.pipeline.set_state(Gst.State.PLAYING)
        if ret == Gst.StateChangeReturn.FAILURE:
            sys.stderr.write("Failed to start pipeline\n")      
            return False
        
        self.running = True
        
        # Run in separate thread
        self.thread = threading.Thread(target=self._run_loop)
        self.thread.daemon = True
        self.thread.start()
        
        return True
    
    def _run_loop(self):
        """Run the GStreamer main loop in a separate thread"""
        try:
            self.loop.run()
        except Exception as e:
            print(f"Error in pipeline loop: {e}")
    
    def stop(self):
        """Stop the pipeline processing"""
        print("Stopping pipeline...")
        self.running = False
        
        if self.loop:
            self.loop.quit()
        
        if self.pipeline:
            self.pipeline.set_state(Gst.State.NULL)
        
        if self.thread and self.thread.is_alive():
            self.thread.join(timeout=5)
        
        print("Pipeline stopped.")

def create_pipeline(input_uri, output_host, output_port):
    """Create and return an pipeline instance"""
    return Pipeline(input_uri, output_host, output_port)
