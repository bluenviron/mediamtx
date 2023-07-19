#include <stdio.h>
#include <stdarg.h>
#include <cstring>
#include <sys/mman.h>
#include <iostream>
#include <mutex>
#include <sys/ioctl.h>
#include <fcntl.h>
#include <unistd.h>

#include <libcamera/camera_manager.h>
#include <libcamera/camera.h>
#include <libcamera/formats.h>
#include <libcamera/control_ids.h>
#include <libcamera/controls.h>
#include <libcamera/framebuffer_allocator.h>
#include <libcamera/property_ids.h>
#include <linux/videodev2.h>

#include "camera.h"

using libcamera::CameraManager;
using libcamera::CameraConfiguration;
using libcamera::Camera;
using libcamera::ColorSpace;
using libcamera::ControlList;
using libcamera::FrameBufferAllocator;
using libcamera::FrameBuffer;
using libcamera::PixelFormat;
using libcamera::Rectangle;
using libcamera::Request;
using libcamera::Size;
using libcamera::Span;
using libcamera::Stream;
using libcamera::StreamRole;
using libcamera::StreamConfiguration;
using libcamera::Transform;

namespace controls = libcamera::controls;
namespace formats = libcamera::formats;
namespace properties = libcamera::properties;

static char errbuf[256];

static void set_error(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vsnprintf(errbuf, 256, format, args);
}

const char *camera_get_error() {
    return errbuf;
}

// https://github.com/raspberrypi/libcamera-apps/blob/dd97618a25523c2c4aa58f87af5f23e49aa6069c/core/libcamera_app.cpp#L42
static PixelFormat mode_to_pixel_format(sensor_mode_t *mode) {
    static std::vector<std::pair<std::pair<int, bool>, PixelFormat>> table = {
        { {8, false}, formats::SBGGR8 },
        { {8, true}, formats::SBGGR8 },
        { {10, false}, formats::SBGGR10 },
        { {10, true}, formats::SBGGR10_CSI2P },
        { {12, false}, formats::SBGGR12 },
        { {12, true}, formats::SBGGR12_CSI2P },
    };

    auto it = std::find_if(table.begin(), table.end(), [&mode] (auto &m) {
        return mode->bit_depth == m.first.first && mode->packed == m.first.second; });
    if (it != table.end()) {
        return it->second;
    }

    return formats::SBGGR12_CSI2P;
}

struct CameraPriv {
    const parameters_t *params;
    camera_frame_cb frame_cb;
    std::unique_ptr<CameraManager> camera_manager;
    std::shared_ptr<Camera> camera;
    Stream *video_stream;
    std::unique_ptr<FrameBufferAllocator> allocator;
    std::vector<std::unique_ptr<Request>> requests;
    std::mutex ctrls_mutex;
    std::unique_ptr<ControlList> ctrls;
    std::map<FrameBuffer *, uint8_t *> mapped_buffers;
};

static int get_v4l2_colorspace(std::optional<ColorSpace> const &cs) {
    if (cs == ColorSpace::Rec709) {
        return V4L2_COLORSPACE_REC709;
    }
    return V4L2_COLORSPACE_SMPTE170M;
}

// https://github.com/raspberrypi/libcamera-apps/blob/a5b5506a132056ac48ba22bc581cc394456da339/core/libcamera_app.cpp#L824
static uint8_t *map_buffer(FrameBuffer *buffer) {
    size_t buffer_size = 0;

    for (unsigned i = 0; i < buffer->planes().size(); i++) {
        const FrameBuffer::Plane &plane = buffer->planes()[i];
        buffer_size += plane.length;

        if (i == buffer->planes().size() - 1 || plane.fd.get() != buffer->planes()[i + 1].fd.get()) {
            return (uint8_t *)mmap(NULL, buffer_size, PROT_READ | PROT_WRITE, MAP_SHARED, plane.fd.get(), 0);
        }
    }

    return NULL;
}

// https://github.com/raspberrypi/libcamera-apps/blob/a6267d51949d0602eedf60f3ddf8c6685f652812/core/options.cpp#L101
static void set_hdr(bool hdr) {
    bool ok = false;
    for (int i = 0; i < 4 && !ok; i++)
    {
        std::string dev("/dev/v4l-subdev");
        dev += (char)('0' + i);
        int fd = open(dev.c_str(), O_RDWR, 0);
        if (fd < 0)
            continue;

        v4l2_control ctrl { V4L2_CID_WIDE_DYNAMIC_RANGE, hdr };
        ok = !ioctl(fd, VIDIOC_S_CTRL, &ctrl);
        close(fd);
    }
}

bool camera_create(const parameters_t *params, camera_frame_cb frame_cb, camera_t **cam) {
    set_hdr(params->hdr);

    // We make sure to set the environment variable before libcamera init
    setenv("LIBCAMERA_RPI_TUNING_FILE", params->tuning_file, 1);

    std::unique_ptr<CameraPriv> camp = std::make_unique<CameraPriv>();

    camp->camera_manager = std::make_unique<CameraManager>();
    int ret = camp->camera_manager->start();
    if (ret != 0) {
        set_error("CameraManager.start() failed");
        return false;
    }

    std::vector<std::shared_ptr<Camera>> cameras = camp->camera_manager->cameras();
    auto rem = std::remove_if(cameras.begin(), cameras.end(),
        [](auto &cam) { return cam->id().find("/usb") != std::string::npos; });
    cameras.erase(rem, cameras.end());
    if (params->camera_id >= cameras.size()){
        set_error("selected camera is not available");
        return false;
    }

    camp->camera = camp->camera_manager->get(cameras[params->camera_id]->id());
    if (camp->camera == NULL) {
        set_error("CameraManager.get() failed");
        return false;
    }

    ret = camp->camera->acquire();
    if (ret != 0) {
        set_error("Camera.acquire() failed");
        return false;
    }

    std::vector<libcamera::StreamRole> stream_roles = { StreamRole::VideoRecording };
    if (params->mode != NULL) {
        stream_roles.push_back(StreamRole::Raw);
    }

    std::unique_ptr<CameraConfiguration> conf = camp->camera->generateConfiguration(stream_roles);
    if (conf == NULL) {
        set_error("Camera.generateConfiguration() failed");
        return false;
    }

    StreamConfiguration &video_stream_conf = conf->at(0);
    video_stream_conf.size = libcamera::Size(params->width, params->height);
    video_stream_conf.pixelFormat = formats::YUV420;
    video_stream_conf.bufferCount = params->buffer_count;
    if (params->width >= 1280 || params->height >= 720) {
        video_stream_conf.colorSpace = ColorSpace::Rec709;
    } else {
        video_stream_conf.colorSpace = ColorSpace::Smpte170m;
    }

    if (params->mode != NULL) {
        StreamConfiguration &raw_stream_conf = conf->at(1);
        raw_stream_conf.size = Size(params->mode->width, params->mode->height);
        raw_stream_conf.pixelFormat = mode_to_pixel_format(params->mode);
        raw_stream_conf.bufferCount = video_stream_conf.bufferCount;
    }

    conf->transform = Transform::Identity;
    if (params->h_flip) {
        conf->transform = Transform::HFlip * conf->transform;
    }
    if (params->v_flip) {
        conf->transform = Transform::VFlip * conf->transform;
    }

    CameraConfiguration::Status vstatus = conf->validate();
    if (vstatus == CameraConfiguration::Invalid) {
        set_error("StreamConfiguration.validate() failed");
        return false;
    }

    int res = camp->camera->configure(conf.get());
    if (res != 0) {
        set_error("Camera.configure() failed");
        return false;
    }

    camp->video_stream = video_stream_conf.stream();

    for (unsigned int i = 0; i < params->buffer_count; i++) {
        std::unique_ptr<Request> request = camp->camera->createRequest((uint64_t)camp.get());
        if (request == NULL) {
            set_error("createRequest() failed");
            return false;
        }
        camp->requests.push_back(std::move(request));
    }

    camp->allocator = std::make_unique<FrameBufferAllocator>(camp->camera);
    for (StreamConfiguration &stream_conf : *conf) {
        Stream *stream = stream_conf.stream();

        res = camp->allocator->allocate(stream);
        if (res < 0) {
            set_error("allocate() failed");
            return false;
        }

        int i = 0;
        for (const std::unique_ptr<FrameBuffer> &buffer : camp->allocator->buffers(stream)) {
            // map buffer of the video stream only
            if (stream == video_stream_conf.stream()) {
                camp->mapped_buffers[buffer.get()] = map_buffer(buffer.get());
            }

            res = camp->requests.at(i++)->addBuffer(stream, buffer.get());
            if (res != 0) {
                set_error("addBuffer() failed");
                return false;
            }
        }
    }

    camp->params = params;
    camp->frame_cb = frame_cb;
    *cam = camp.release();

    return true;
}

static int buffer_size(const std::vector<FrameBuffer::Plane> &planes) {
    int size = 0;
    for (const FrameBuffer::Plane &plane : planes) {
        size += plane.length;
    }
    return size;
}

static void on_request_complete(Request *request) {
    if (request->status() == Request::RequestCancelled) {
        return;
    }

    CameraPriv *camp = (CameraPriv *)request->cookie();

    FrameBuffer *buffer = request->buffers().at(camp->video_stream);

    camp->frame_cb(
        camp->mapped_buffers.at(buffer),
        camp->video_stream->configuration().stride,
        camp->video_stream->configuration().size.height,
        buffer->planes()[0].fd.get(),
        buffer_size(buffer->planes()),
        buffer->metadata().timestamp / 1000);

    request->reuse(Request::ReuseFlag::ReuseBuffers);

    {
        std::lock_guard<std::mutex> lock(camp->ctrls_mutex);
        request->controls() = *camp->ctrls;
        camp->ctrls->clear();
    }

    camp->camera->queueRequest(request);
}

int camera_get_mode_stride(camera_t *cam) {
    CameraPriv *camp = (CameraPriv *)cam;
    return camp->video_stream->configuration().stride;
}

int camera_get_mode_colorspace(camera_t *cam) {
    CameraPriv *camp = (CameraPriv *)cam;
    return get_v4l2_colorspace(camp->video_stream->configuration().colorSpace);
}

static void fill_dynamic_controls(ControlList *ctrls, const parameters_t *params) {
    ctrls->set(controls::Brightness, params->brightness);
    ctrls->set(controls::Contrast, params->contrast);
    ctrls->set(controls::Saturation, params->saturation);
    ctrls->set(controls::Sharpness, params->sharpness);

    int exposure_mode;
    if (strcmp(params->exposure, "short") == 0) {
        exposure_mode = controls::ExposureShort;
    } else if (strcmp(params->exposure, "long") == 0) {
        exposure_mode = controls::ExposureLong;
    } else if (strcmp(params->exposure, "custom") == 0) {
        exposure_mode = controls::ExposureCustom;
    } else {
        exposure_mode = controls::ExposureNormal;
    }
    ctrls->set(controls::AeExposureMode, exposure_mode);

    int awb_mode;
    if (strcmp(params->awb, "incandescent") == 0) {
        awb_mode = controls::AwbIncandescent;
    } else if (strcmp(params->awb, "tungsten") == 0) {
        awb_mode = controls::AwbTungsten;
    } else if (strcmp(params->awb, "fluorescent") == 0) {
        awb_mode = controls::AwbFluorescent;
    } else if (strcmp(params->awb, "indoor") == 0) {
        awb_mode = controls::AwbIndoor;
    } else if (strcmp(params->awb, "daylight") == 0) {
        awb_mode = controls::AwbDaylight;
    } else if (strcmp(params->awb, "cloudy") == 0) {
        awb_mode = controls::AwbCloudy;
    } else if (strcmp(params->awb, "custom") == 0) {
        awb_mode = controls::AwbCustom;
    } else {
        awb_mode = controls::AwbAuto;
    }
    ctrls->set(controls::AwbMode, awb_mode);

    int denoise_mode;
    if (strcmp(params->denoise, "cdn_off") == 0) {
        denoise_mode = controls::draft::NoiseReductionModeMinimal;
    } else if (strcmp(params->denoise, "cdn_hq") == 0) {
        denoise_mode = controls::draft::NoiseReductionModeHighQuality;
    } else if (strcmp(params->denoise, "cdn_fast") == 0) {
        denoise_mode = controls::draft::NoiseReductionModeFast;
    } else {
        denoise_mode = controls::draft::NoiseReductionModeOff;
    }
    ctrls->set(controls::draft::NoiseReductionMode, denoise_mode);

    ctrls->set(controls::ExposureTime, params->shutter);

    int metering_mode;
    if (strcmp(params->metering, "spot") == 0) {
        metering_mode = controls::MeteringSpot;
    } else if (strcmp(params->metering, "matrix") == 0) {
        metering_mode = controls::MeteringMatrix;
    } else if (strcmp(params->metering, "custom") == 0) {
        metering_mode = controls::MeteringCustom;
    } else {
        metering_mode = controls::MeteringCentreWeighted;
    }
    ctrls->set(controls::AeMeteringMode, metering_mode);

    ctrls->set(controls::AnalogueGain, params->gain);

    ctrls->set(controls::ExposureValue, params->ev);

    int64_t frame_time = (int64_t)(((float)1000000) / params->fps);
    ctrls->set(controls::FrameDurationLimits, Span<const int64_t, 2>({ frame_time, frame_time }));
}

bool camera_start(camera_t *cam) {
    CameraPriv *camp = (CameraPriv *)cam;

    camp->ctrls = std::make_unique<ControlList>(controls::controls);

    fill_dynamic_controls(camp->ctrls.get(), camp->params);

    if (camp->camera->controls().count(&controls::AfMode) > 0) {
        int af_mode;
        if (strcmp(camp->params->af_mode, "manual") == 0) {
            af_mode = controls::AfModeManual;
        } else if (strcmp(camp->params->af_mode, "continuous") == 0) {
            af_mode = controls::AfModeContinuous;
        } else {
            af_mode = controls::AfModeAuto;
        }
        camp->ctrls->set(controls::AfMode, af_mode);

        if (af_mode == controls::AfModeManual) {
            camp->ctrls->set(controls::LensPosition, camp->params->lens_position);
        }
    }

    if (camp->camera->controls().count(&controls::AfRange) > 0) {
        int af_range;
        if (strcmp(camp->params->af_range, "macro") == 0) {
            af_range = controls::AfRangeMacro;
        } else if (strcmp(camp->params->af_range, "full") == 0) {
            af_range = controls::AfRangeFull;
        } else {
            af_range = controls::AfRangeNormal;
        }
        camp->ctrls->set(controls::AfRange, af_range);
    }

    if (camp->camera->controls().count(&controls::AfSpeed) > 0) {
        int af_speed;
        if (strcmp(camp->params->af_range, "fast") == 0) {
            af_speed = controls::AfSpeedFast;
        } else {
            af_speed = controls::AfSpeedNormal;
        }
        camp->ctrls->set(controls::AfSpeed, af_speed);
    }

    if (camp->params->roi != NULL) {
        std::optional<Rectangle> opt = camp->camera->properties().get(properties::ScalerCropMaximum);
        Rectangle sensor_area;
        try {
            sensor_area = opt.value();
        } catch(const std::bad_optional_access& exc) {
            set_error("get(ScalerCropMaximum) failed");
            return false;
        }

        Rectangle crop(
            camp->params->roi->x * sensor_area.width,
            camp->params->roi->y * sensor_area.height,
            camp->params->roi->width * sensor_area.width,
            camp->params->roi->height * sensor_area.height);
        crop.translateBy(sensor_area.topLeft());
        camp->ctrls->set(controls::ScalerCrop, crop);
    }

    if (camp->params->af_window != NULL) {
        std::optional<Rectangle> opt = camp->camera->properties().get(properties::ScalerCropMaximum);
        Rectangle sensor_area;
        try {
            sensor_area = opt.value();
        } catch(const std::bad_optional_access& exc) {
            set_error("get(ScalerCropMaximum) failed");
            return false;
        }

        Rectangle afwindows_rectangle[1];

        afwindows_rectangle[0] = Rectangle(
            camp->params->af_window->x * sensor_area.width,
            camp->params->af_window->y * sensor_area.height,
            camp->params->af_window->width * sensor_area.width,
            camp->params->af_window->height * sensor_area.height);

        afwindows_rectangle[0].translateBy(sensor_area.topLeft());
        camp->ctrls->set(controls::AfMetering, controls::AfMeteringWindows);
        camp->ctrls->set(controls::AfWindows, afwindows_rectangle);
    }

    int res = camp->camera->start(camp->ctrls.get());
    if (res != 0) {
        set_error("Camera.start() failed");
        return false;
    }

    camp->ctrls->clear();

    camp->camera->requestCompleted.connect(on_request_complete);

    for (std::unique_ptr<Request> &request : camp->requests) {
        int res = camp->camera->queueRequest(request.get());
        if (res != 0) {
            set_error("Camera.queueRequest() failed");
            return false;
        }
    }

    return true;
}

void camera_reload_params(camera_t *cam, const parameters_t *params) {
    CameraPriv *camp = (CameraPriv *)cam;

    std::lock_guard<std::mutex> lock(camp->ctrls_mutex);
    fill_dynamic_controls(camp->ctrls.get(), params);
}
