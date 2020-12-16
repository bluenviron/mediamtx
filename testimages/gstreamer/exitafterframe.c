
#include <gst/gst.h>

GType gst_exitafterframe_get_type ();

#define GST_TYPE_EXITAFTERFRAME (gst_exitafterframe_get_type())
#define GST_EXITAFTERFRAME(obj) (G_TYPE_CHECK_INSTANCE_CAST((obj),GST_TYPE_EXITAFTERFRAME,GstExitAfterFrame))

typedef struct
{
  GstElement element;
  GstPad *srcpad;
  GstPad *sinkpad;

} GstExitAfterFrame;

typedef struct
{
  GstElementClass parent_class;

} GstExitAfterFrameClass;

#define gst_exitafterframe_parent_class parent_class
G_DEFINE_TYPE (GstExitAfterFrame, gst_exitafterframe, GST_TYPE_ELEMENT);

static GstStaticPadTemplate sink_factory = GST_STATIC_PAD_TEMPLATE(
    "sink",
    GST_PAD_SINK,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS("video/x-raw")
);

static GstStaticPadTemplate src_factory = GST_STATIC_PAD_TEMPLATE(
    "src",
    GST_PAD_SRC,
    GST_PAD_ALWAYS,
    GST_STATIC_CAPS("video/x-raw")
);

static GstFlowReturn
gst_exitafterframe_chain (GstPad * pad, GstObject * parent, GstBuffer * buf)
{
  GstExitAfterFrame *filter = GST_EXITAFTERFRAME (parent);
  exit(0);
  return gst_pad_push (filter->srcpad, buf);
}

static void
gst_exitafterframe_class_init(GstExitAfterFrameClass* klass) {
    GstElementClass* element_class = (GstElementClass*)klass;

    gst_element_class_set_details_simple(
        element_class,
        "Plugin",
        "FIXME:Generic",
        "FIXME:Generic Template Element",
        "AUTHOR_NAME AUTHOR_EMAIL"
    );

    gst_element_class_add_pad_template(element_class,
        gst_static_pad_template_get(&src_factory));
    gst_element_class_add_pad_template(element_class,
        gst_static_pad_template_get(&sink_factory));
}

static void
gst_exitafterframe_init (GstExitAfterFrame* filter)
{
  GstElement* element = GST_ELEMENT(filter);

  filter->sinkpad = gst_pad_new_from_static_template(&sink_factory, "sink");
  gst_pad_set_chain_function(filter->sinkpad, gst_exitafterframe_chain);
  GST_PAD_SET_PROXY_CAPS(filter->sinkpad);
  gst_element_add_pad(element, filter->sinkpad);

  filter->srcpad = gst_pad_new_from_static_template(&src_factory, "src");
  GST_PAD_SET_PROXY_CAPS(filter->srcpad);
  gst_element_add_pad(element, filter->srcpad);
}

static gboolean
plugin_init (GstPlugin * plugin)
{
  return gst_element_register (plugin, "exitafterframe", GST_RANK_NONE,
          GST_TYPE_EXITAFTERFRAME);
}

#define PACKAGE "exitafterframe"

GST_PLUGIN_DEFINE (GST_VERSION_MAJOR, GST_VERSION_MINOR, exitafterframe,
    "exitafterframe", plugin_init, "1.0", "LGPL", "exitafterframe",
    "http://example.com")
