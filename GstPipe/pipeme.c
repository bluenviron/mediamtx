#include <gst/gst.h>
#include <glib.h>
#include <stdio.h>
#include <string.h>
#include <curl/curl.h>
#include <time.h>
#include <stdarg.h>

#include "stats_post.h"

// Global variables
char rtsp_location[256];
char hostname[256];
char camera_path[256];

// Initial state to send either sink or src element rtpsource stats
int src_is_active = 0;

typedef struct
{
	GstElement* jitter_buffer;
	gpointer session;
	guint32 ssrc;
} JitterBufferData;

static gboolean bus_callback(GstBus* bus, GstMessage* message, gpointer data);
static void src_manager(GstElement* element, GstElement* manager, gpointer user_data);
static void sink_manager(GstElement* element, GstElement* manager, gpointer user_data);
static gboolean print_jitter_stats(JitterBufferData* data);
static void new_jitter_buffer(GstElement* element, GstElement* jitter_buffer, gpointer session, guint32 ssrc);
static void on_ssrc_active_src(GstElement* rtp_bin, guint session_id, guint32 ssrc);
static void get_stats_from_sink_session(gpointer session, guint32 ssrc);
static void on_ssrc_sender_active_sink(GstElement* rtp_bin, guint session_id, guint32 ssrc);
static void get_stats_from_src_session(gpointer session, guint32 ssrc);
static void get_stats_from_rtpsession(gpointer session);
static void extract_rtsp_info_from_sink(GstElement* sink_element);

static void log_message(const char* format, va_list args);
static void g_log(const char* format, ...);
static void dbg_log(const char* format, ...);



void extract_rtsp_info_from_sink(GstElement* sink_element)
{
	gchar* location = NULL;
	g_object_get(sink_element, "location", &location, NULL);

	if (location)
	{
		strncpy_s(rtsp_location, sizeof(rtsp_location), location, sizeof(rtsp_location) - 1);
		rtsp_location[sizeof(rtsp_location) - 1] = '\0';

		const char* hostname_start = strstr(rtsp_location, "//") + 2;
		const char* hostname_end = strchr(hostname_start, '/');
		if (hostname_end == NULL)
		{
			hostname_end = hostname_start + strlen(hostname_start); // In case there's no path after the hostname
		}

		const char* port_start = strchr(hostname_start, ':');
		if (port_start != NULL && port_start < hostname_end)
		{
			hostname_end = port_start;
		}

		size_t hostname_length = hostname_end - hostname_start;
		strncpy_s(hostname, sizeof(hostname), hostname_start, hostname_length);
		hostname[hostname_length] = '\0';

		const char* camera_path_start = strchr(hostname_start, '/') + 1;
		if (camera_path_start != NULL)
		{
			strcpy_s(camera_path, sizeof(camera_path), camera_path_start);
		}
		else
		{
			camera_path[0] = '\0'; // No path available
		}

		g_free(location);
	}
}

int DEBUG_LEVEL = 0;

static void log_message(const char* format, va_list args)
{
	// Add timestamp before print out
	time_t t = time(NULL);
	struct tm tm = *localtime(&t);
	printf("%d/%02d/%02d %02d:%02d:%02d DBG [GST_PIPE] [%s] ", tm.tm_year + 1900, tm.tm_mon + 1, tm.tm_mday, tm.tm_hour, tm.tm_min, tm.tm_sec, camera_path);

	// Print the rest of the message
	vprintf(format, args);
}

static void g_log(const char* format, ...)
{
	va_list args;
	va_start(args, format);
	log_message(format, args);
	va_end(args);
}

static void dbg_log(const char* format, ...)
{
	if (DEBUG_LEVEL == 1)
	{
		va_list args;
		va_start(args, format);
		log_message(format, args);
		va_end(args);
	}
}

int main(int argc, char* argv[])
{
	if (argc == 1)
	{
		printf("Pipeline string cannot be empty\n");
		return 1;
	}

	// Check for debug mode
	if (argc == 3)
	{
		gchar* dbg_str = g_strjoinv(" ", &argv[2]);
		if (g_str_has_prefix(dbg_str, "--debug="))
		{
			const char* debug_value_str = dbg_str + strlen("--debug=");
			DEBUG_LEVEL = atoi(debug_value_str);
			printf("Debug mode enabled with --debug=<1,2> flag, level: %d\n", DEBUG_LEVEL);
		}
		else
		{
			printf("Invalid third argument: %s\n", argv[2]);
			return 1;
		}
		g_free(dbg_str);
	}

	gst_init(&argc, &argv);

	GMainLoop* main_loop = g_main_loop_new(NULL, FALSE);
	gchar* pipeline_string = g_strjoinv(" ", &argv[1]);

	GError* error = NULL;
	GstElement* pipeline = gst_parse_launch(pipeline_string, &error);
	if (!pipeline)
	{
		g_log("Parse error: %s\n", error->message);
		g_error_free(error);
		return 2;
	}

	GstElement* src_element = gst_bin_get_by_name(GST_BIN(pipeline), "src");
	if (!src_element)
	{
		g_log("Element 'src' not found\n");
		return 2;
	}

	g_signal_connect(src_element, "new-manager", G_CALLBACK(src_manager), NULL);

	GstElement* sink_element = gst_bin_get_by_name(GST_BIN(pipeline), "sink");
	if (!sink_element)
	{
		g_log("Element 'sink' not found\n");
		return 2;
	}

	// Extract RTSP information from the sink element
	extract_rtsp_info_from_sink(sink_element);

	dbg_log("RTSP Location: %s\n", rtsp_location);
	dbg_log("Hostname: %s\n", hostname);
	dbg_log("Camera Path: %s\n", camera_path);

	g_signal_connect(sink_element, "new-manager", G_CALLBACK(sink_manager), NULL);

	GstBus* bus = gst_element_get_bus(pipeline);
	gst_bus_add_watch(bus, bus_callback, main_loop);
	gst_object_unref(bus);

	gst_element_set_state(pipeline, GST_STATE_PLAYING);
	g_main_loop_run(main_loop);

	gst_element_set_state(pipeline, GST_STATE_NULL);
	gst_object_unref(pipeline);
	g_main_loop_unref(main_loop);
	g_free(pipeline_string);

	return 0;
}



static gboolean bus_callback(GstBus* bus, GstMessage* message, gpointer data)
{
	GMainLoop* main_loop = (GMainLoop*)data;

	switch (GST_MESSAGE_TYPE(message))
	{
	case GST_MESSAGE_EOS:
		g_main_loop_quit(main_loop);
		break;
	case GST_MESSAGE_ERROR:
	{
		GError* err;
		gchar* debug_info;
		gst_message_parse_error(message, &err, &debug_info);
		g_log("ERROR: %s\n", err->message);
		if (debug_info)
		{
			g_log("DEBUG: %s\n", debug_info);
			g_free(debug_info);
		}
		g_error_free(err);
		g_main_loop_quit(main_loop);
		break;
	}
	default:
		// Print the message details here
		if (gst_message_get_structure(message) && DEBUG_LEVEL == 2)
		{
			gchar* message_details = gst_structure_to_string(gst_message_get_structure(message));
			g_log("[LVL=2] %s\n", message_details);
			g_free(message_details);
		}
		break;
	}
	return TRUE;
}

static void src_manager(GstElement* element, GstElement* manager, gpointer user_data)
{
	dbg_log("New src mngr detected: %p\n", manager);

	g_signal_connect(manager, "new-jitterbuffer", G_CALLBACK(new_jitter_buffer), NULL);
	g_signal_connect(manager, "on-ssrc-active", G_CALLBACK(on_ssrc_active_src), NULL);
}

static void sink_manager(GstElement* element, GstElement* manager, gpointer user_data)
{
	dbg_log("New sink mngr detected: %p\n", manager);
	g_signal_connect(manager, "on-sender-ssrc-active", G_CALLBACK(on_ssrc_sender_active_sink), NULL);
}

static gboolean print_jitter_stats(JitterBufferData* data)
{
	// Access the elements in the structure
	GstElement* jitter_buffer = data->jitter_buffer;
	gpointer session = data->session;
	guint32 ssrc = data->ssrc;

	dbg_log("JitterBuffer: %p, Session: %p, SSRC: %u\n", jitter_buffer, session, ssrc);

	GValue stats_value = G_VALUE_INIT;
	g_object_get_property(G_OBJECT(jitter_buffer), "stats", &stats_value);
	if (G_VALUE_HOLDS(&stats_value, GST_TYPE_STRUCTURE))
	{
		const GstStructure* gstStats = gst_value_get_structure(&stats_value);

		guint64 numLost, numLate, numDuplicates, rtxCount, rtxSuccessCount, avgJitter, rtxRtt;
		gdouble rtxPerPacket;

		gst_structure_get_uint64(gstStats, "num-lost", &numLost);
		gst_structure_get_uint64(gstStats, "num-late", &numLate);
		gst_structure_get_uint64(gstStats, "num-duplicates", &numDuplicates);
		gst_structure_get_uint64(gstStats, "avg-jitter", &avgJitter);
		gst_structure_get_uint64(gstStats, "rtx-count", &rtxCount);
		gst_structure_get_uint64(gstStats, "rtx-success-count", &rtxSuccessCount);
		gst_structure_get_double(gstStats, "rtx-per-packet", &rtxPerPacket);
		gst_structure_get_uint64(gstStats, "rtx-rtt", &rtxRtt);

		dbg_log("  Num Lost:  %lu\n", numLost);
		dbg_log("  Num Late:  %lu\n", numLate);
		dbg_log("  Num Duplicates:  %lu\n", numDuplicates);
		dbg_log("  Avg Jitter: (in ns)  %lu\n", avgJitter);
		dbg_log("  RTX Count:  %lu\n", rtxCount);
		dbg_log("  RTX Success Count:  %lu\n", rtxSuccessCount);
		dbg_log("  RTX Per Packet: %f\n", rtxPerPacket);
		dbg_log("  RTX RTT: %lu\n", rtxRtt);

		// Example usage for JITTER_BUFFER
		PostFields postFields;
		postFields.statType = JITTER_BUFFER;
		postFields.stats.jitterBufferStats = (JitterBufferStats){ numLost, numLate, numDuplicates, avgJitter, rtxCount, rtxSuccessCount, rtxPerPacket, rtxRtt };
		sendPostRequest(postFields, "jitterbuffer", hostname, camera_path);

		g_value_unset(&stats_value);
		return TRUE;
	}
	else
	{
		g_log("Error: stats is not of type GstStructure\n");
		g_value_unset(&stats_value);
		return FALSE;
	}
}

static void new_jitter_buffer(GstElement* element, GstElement* jitter_buffer, gpointer session, guint32 ssrc)
{
	dbg_log("New jitterBuffer detected: %p\n", jitter_buffer);

	JitterBufferData* data = g_new(JitterBufferData, 1);
	data->jitter_buffer = jitter_buffer;
	data->session = session;
	data->ssrc = ssrc;

	g_timeout_add(5000, (GSourceFunc)print_jitter_stats, data);
}

static void on_ssrc_sender_active_sink(GstElement* rtp_bin, guint session_id, guint32 ssrc)
{
	if (src_is_active)
	{
		dbg_log("SSRC SENDER (SINK) is active. Ignoring RTPSource (SRC) stats\n");
		return;
	}

	dbg_log("On SSRC SENDER (SINK) active: sessionID: %u, ssrc: %u\n", session_id, ssrc);

	// https://gstreamer.freedesktop.org/documentation/rtpmanager/RTPSession.html?gi-language=c#RTPSession
	GstElement* session;
	g_signal_emit_by_name(rtp_bin, "get-internal-session", session_id, &session);

	if (session)
	{
		get_stats_from_sink_session(session, ssrc);
	}
	else
	{
		g_log("Error: session is nil\n");
	}
}

static void on_ssrc_active_src(GstElement* rtp_bin, guint session_id, guint32 ssrc)
{
	dbg_log("On SSRC active (SRC): sessionID: %u, ssrc: %u\n", session_id, ssrc);

	// It is active. This will ignore using rtpsource sink.
	src_is_active = 1;

	// https://gstreamer.freedesktop.org/documentation/rtpmanager/RTPSession.html?gi-language=c#RTPSession
	GstElement* session;
	g_signal_emit_by_name(rtp_bin, "get-internal-session", session_id, &session);

	if (session)
	{
		// Get stats grom RtpSession:stats
		get_stats_from_rtpsession(session);
		get_stats_from_src_session(session, ssrc);
	}
	else
	{
		g_log("Error: session is nil\n");
	}
}

static void get_stats_from_sink_session(gpointer session, guint32 ssrc)
{
	GstElement* sink;
	g_signal_emit_by_name(session, "get-source-by-ssrc", ssrc, &sink);

	if (sink)
	{

		GValue stats_value = G_VALUE_INIT;
		g_object_get_property(G_OBJECT(sink), "stats", &stats_value);
		if (G_VALUE_HOLDS(&stats_value, GST_TYPE_STRUCTURE))
		{
			const GstStructure* stats = gst_value_get_structure(&stats_value);

			// G_TYPE_UINT64
			guint64 bitrate;

			gst_structure_get_uint64(stats, "bitrate", &bitrate);

			if (bitrate == 0)
			{
				dbg_log("No bitrate received\n");
				g_value_unset(&stats_value);
				return;
			}

			dbg_log("Camera Path: %s\n", camera_path);
			dbg_log(" ** RTPSource [SINK] stats: %s\n", camera_path);
			dbg_log("  bitrate: %lu\n", bitrate);

			PostFields postFields;
			postFields.statType = RTP_SOURCE;
			postFields.stats.rtpSourceStats = (RtpSourceStats){ 0, 0, bitrate, 0 };
			sendPostRequest(postFields, "rtpsource", hostname, camera_path);
		}
		else
		{
			g_log("Error: stats is not of type GstStructure\n");
		}
		g_value_unset(&stats_value);
	}
	else
	{
		g_log("Error: source is nil\n");
	}
}

static void get_stats_from_rtpsession(gpointer session)
{
	dbg_log("Getting stats from RTPSession...\n");

	GValue stats_value = G_VALUE_INIT;
	g_object_get_property(G_OBJECT(session), "stats", &stats_value);

	if (G_VALUE_HOLDS(&stats_value, GST_TYPE_STRUCTURE))
	{
		const GstStructure* stats = gst_value_get_structure(&stats_value);

		// "rtx-drop-count" G_TYPE_UINT The number of retransmission events dropped (due to bandwidth constraints)
		// "sent-nack-count" G_TYPE_UINT Number of NACKs sent
		// "recv-nack-count" G_TYPE_UINT Number of NACKs received

		// G_TYPE_UINT
		guint rtx_drop_count, sent_nack_count, recv_nack_count;

		gst_structure_get_uint(stats, "rtx-drop-count", &rtx_drop_count);
		gst_structure_get_uint(stats, "sent-nack-count", &sent_nack_count);
		gst_structure_get_uint(stats, "recv-nack-count", &recv_nack_count);

		if (rtx_drop_count == 0 || sent_nack_count == 0 || recv_nack_count == 0)
		{
			dbg_log("No packets received\n");
			g_value_unset(&stats_value);
			return;
		}

		dbg_log(" ** RTPSession stats: %s\n", camera_path);
		dbg_log("  rtx-drop-count: %u\n", rtx_drop_count);
		dbg_log("  sent-nack-count: %u\n", sent_nack_count);
		dbg_log("  recv-nack-count: %u\n", recv_nack_count);

		// Example usage for RTP_SESSION
		PostFields postFields;
		postFields.statType = RTP_SESSION;
		postFields.stats.rtpSessionStats = (RtpSessionStats){ rtx_drop_count, sent_nack_count, recv_nack_count };
		sendPostRequest(postFields, "rtpsession", hostname, camera_path);
	}
	else
	{
		g_log("Error: stats is not of type GstStructure\n");
	}
	g_value_unset(&stats_value);
}

static void get_stats_from_src_session(gpointer session, guint32 ssrc)
{
	GstElement* source;
	g_signal_emit_by_name(session, "get-source-by-ssrc", ssrc, &source);

	if (source)
	{
		GValue stats_value = G_VALUE_INIT;
		g_object_get_property(G_OBJECT(source), "stats", &stats_value);
		if (G_VALUE_HOLDS(&stats_value, GST_TYPE_STRUCTURE))
		{
			const GstStructure* stats = gst_value_get_structure(&stats_value);

			// G_TYPE_INT
			gint packets_lost;

			// G_TYPE_UINT
			guint jitter;

			// G_TYPE_UINT64
			guint64 bitrate, packets_received;

			gst_structure_get_int(stats, "packets-lost", &packets_lost);
			gst_structure_get_uint64(stats, "bitrate", &bitrate);
			gst_structure_get_uint64(stats, "packets-received", &packets_received);
			gst_structure_get_uint(stats, "jitter", &jitter);

			if (packets_received == 0)
			{
				dbg_log("No packets received\n");
				g_value_unset(&stats_value);
				return;
			}

			dbg_log(" ** RTPSource (SRC) stats: %s\n", camera_path);
			dbg_log("  packets-lost: %d\n", packets_lost);
			dbg_log("  packets-received: %lu\n", packets_received);
			dbg_log("  bitrate: %lu\n", bitrate);
			dbg_log("  jitter: %u\n", jitter);

			PostFields postFields;
			postFields.statType = RTP_SOURCE;
			postFields.stats.rtpSourceStats = (RtpSourceStats){ packets_lost, packets_received, bitrate, jitter };
			sendPostRequest(postFields, "rtpsource", hostname, camera_path);
		}
		else
		{
			g_log("Error: stats is not of type GstStructure\n");
		}
		g_value_unset(&stats_value);
	}
	else
	{
		g_log("Error: source is nil\n");
	}
}
