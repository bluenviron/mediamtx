#include <stdio.h>
#include <stdarg.h>
#include <cstring>
#include <sys/mman.h>
#include <iostream>

#include <libcamera/camera_manager.h>
#include <libcamera/camera.h>
#include <libcamera/formats.h>
#include <libcamera/control_ids.h>
#include <libcamera/controls.h>
#include <libcamera/framebuffer_allocator.h>
#include <libcamera/property_ids.h>
#include <linux/videodev2.h>

#include "parameters.h"
#include "camera.h"

using libcamera::CameraManager;
using libcamera::CameraConfiguration;
using libcamera::Camera;
using libcamera::ControlList;
using libcamera::FrameBufferAllocator;
using libcamera::FrameBuffer;
using libcamera::Rectangle;
using libcamera::Request;
using libcamera::Span;
using libcamera::Stream;
using libcamera::StreamRoles;
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
static libcamera::PixelFormat mode_to_pixel_format(sensor_mode_t *mode) {
    static std::vector<std::pair<std::pair<int, bool>, libcamera::PixelFormat>> table = {
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
    parameters_t *params;
    camera_frame_cb frame_cb;
    std::unique_ptr<CameraManager> camera_manager;
    std::shared_ptr<Camera> camera;
    Stream *video_stream;
    std::unique_ptr<FrameBufferAllocator> allocator;
    std::vector<std::unique_ptr<Request>> requests;
};

static int get_v4l2_colorspace(std::optional<libcamera::ColorSpace> const &cs) {
    if (cs == libcamera::ColorSpace::Rec709) {
        return V4L2_COLORSPACE_REC709;
    }
    return V4L2_COLORSPACE_SMPTE170M;
}

bool camera_create(parameters_t *params, camera_frame_cb frame_cb, camera_t **cam) {
    std::unique_ptr<CameraPriv> camp = std::make_unique<CameraPriv>();

    camp->camera_manager = std::make_unique<CameraManager>();
    int ret = camp->camera_manager->start();
    if (ret != 0) {
        set_error("CameraManager.start() failed");
        return false;
    }

    std::vector<std::shared_ptr<libcamera::Camera>> cameras = camp->camera_manager->cameras();
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

    setenv("LIBCAMERA_RPI_TUNING_FILE", params->tuning_file, 1);

    ret = camp->camera->acquire();
    if (ret != 0) {
        set_error("Camera.acquire() failed");
        return false;
    }

    StreamRoles stream_roles = { StreamRole::VideoRecording };
    if (params->mode != NULL) {
        stream_roles.push_back(StreamRole::Raw);
    }

    std::unique_ptr<CameraConfiguration> conf = camp->camera->generateConfiguration(stream_roles);
    if (conf == NULL) {
        set_error("Camera.generateConfiguration() failed");
        return false;
    }

    StreamConfiguration &video_stream_conf = conf->at(0);
    video_stream_conf.pixelFormat = formats::YUV420;
    video_stream_conf.bufferCount = params->buffer_count;
    video_stream_conf.size.width = params->width;
    video_stream_conf.size.height = params->height;
    if (params->width >= 1280 || params->height >= 720) {
        video_stream_conf.colorSpace = libcamera::ColorSpace::Rec709;
    } else {
        video_stream_conf.colorSpace = libcamera::ColorSpace::Smpte170m;
    }

    if (params->mode != NULL) {
        StreamConfiguration &raw_stream_conf = conf->at(1);
        raw_stream_conf.size = libcamera::Size(params->mode->width, params->mode->height);
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

    camp->allocator = std::make_unique<FrameBufferAllocator>(camp->camera);
    res = camp->allocator->allocate(camp->video_stream);
    if (res < 0) {
        set_error("allocate() failed");
        return false;
    }

    for (const std::unique_ptr<FrameBuffer> &buffer : camp->allocator->buffers(camp->video_stream)) {
        std::unique_ptr<Request> request = camp->camera->createRequest((uint64_t)camp.get());
        if (request == NULL) {
            set_error("createRequest() failed");
            return false;
        }

        int res = request->addBuffer(camp->video_stream, buffer.get());
        if (res != 0) {
            set_error("addBuffer() failed");
            return false;
        }

        camp->requests.push_back(std::move(request));
    }

    camp->params = params;
    camp->frame_cb = frame_cb;
    *cam = camp.release();

    return true;
}

static void on_request_complete(Request *request) {
    if (request->status() == Request::RequestCancelled) {
        return;
    }

    CameraPriv *camp = (CameraPriv *)request->cookie();

    FrameBuffer *buffer = request->buffers().begin()->second;

    int size = 0;
    for (const FrameBuffer::Plane &plane : buffer->planes()) {
        size += plane.length;
    }

    camp->frame_cb(buffer->planes()[0].fd.get(), size, buffer->metadata().timestamp / 1000);

    request->reuse(Request::ReuseFlag::ReuseBuffers);
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

bool camera_start(camera_t *cam) {
    CameraPriv *camp = (CameraPriv *)cam;

    ControlList ctrls = ControlList(controls::controls);

    ctrls.set(controls::Brightness, camp->params->brightness);
    ctrls.set(controls::Contrast, camp->params->contrast);
    ctrls.set(controls::Saturation, camp->params->saturation);
    ctrls.set(controls::Sharpness, camp->params->sharpness);

    int exposure_mode;
    if (strcmp(camp->params->exposure, "short") == 0) {
        exposure_mode = controls::ExposureShort;
    } else if (strcmp(camp->params->exposure, "long") == 0) {
        exposure_mode = controls::ExposureLong;
    } else if (strcmp(camp->params->exposure, "custom") == 0) {
        exposure_mode = controls::ExposureCustom;
    } else {
        exposure_mode = controls::ExposureNormal;
    }
    ctrls.set(controls::AeExposureMode, exposure_mode);

    int awb_mode;
    if (strcmp(camp->params->awb, "incandescent") == 0) {
        awb_mode = controls::AwbIncandescent;
    } else if (strcmp(camp->params->awb, "tungsten") == 0) {
        awb_mode = controls::AwbTungsten;
    } else if (strcmp(camp->params->awb, "fluorescent") == 0) {
        awb_mode = controls::AwbFluorescent;
    } else if (strcmp(camp->params->awb, "indoor") == 0) {
        awb_mode = controls::AwbIndoor;
    } else if (strcmp(camp->params->awb, "daylight") == 0) {
        awb_mode = controls::AwbDaylight;
    } else if (strcmp(camp->params->awb, "cloudy") == 0) {
        awb_mode = controls::AwbCloudy;
    } else if (strcmp(camp->params->awb, "custom") == 0) {
        awb_mode = controls::AwbCustom;
    } else {
        awb_mode = controls::AwbAuto;
    }
    ctrls.set(controls::AwbMode, awb_mode);

    int denoise_mode;
    if (strcmp(camp->params->denoise, "off") == 0) {
        denoise_mode = controls::draft::NoiseReductionModeOff;
    } else if (strcmp(camp->params->denoise, "cdn_off") == 0) {
        denoise_mode = controls::draft::NoiseReductionModeMinimal;
    } if (strcmp(camp->params->denoise, "cdn_hq") == 0) {
        denoise_mode = controls::draft::NoiseReductionModeHighQuality;
    } else {
        denoise_mode = controls::draft::NoiseReductionModeFast;
    }
    ctrls.set(controls::draft::NoiseReductionMode, denoise_mode);

    if (camp->params->shutter != 0) {
        ctrls.set(controls::ExposureTime, camp->params->shutter);
    }

    int metering_mode;
    if (strcmp(camp->params->metering, "spot") == 0) {
        metering_mode = controls::MeteringSpot;
    } else if (strcmp(camp->params->metering, "matrix") == 0) {
        metering_mode = controls::MeteringMatrix;
    } else if (strcmp(camp->params->metering, "custom") == 0) {
        metering_mode = controls::MeteringCustom;
    } else {
        metering_mode = controls::MeteringCentreWeighted;
    }
    ctrls.set(controls::AeMeteringMode, metering_mode);

    if (camp->params->gain > 0) {
        ctrls.set(controls::AnalogueGain, camp->params->gain);
    }

    ctrls.set(controls::ExposureValue, camp->params->ev);

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
        ctrls.set(controls::ScalerCrop, crop);
    }

    int64_t frame_time = 1000000 / camp->params->fps;
    ctrls.set(controls::FrameDurationLimits, Span<const int64_t, 2>({ frame_time, frame_time }));

    int res = camp->camera->start(&ctrls);
    if (res != 0) {
        set_error("Camera.start() failed");
        return false;
    }

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
