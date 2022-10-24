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

char errbuf[256];

static void set_error(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vsnprintf(errbuf, 256, format, args);
}

const char *camera_get_error() {
    return errbuf;
}

struct CameraPriv {
    parameters_t *params;
    camera_frame_cb frame_cb;
    std::unique_ptr<CameraManager> camera_manager;
    std::shared_ptr<Camera> camera;
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

    ret = camp->camera->acquire();
    if (ret != 0) {
        set_error("Camera.acquire() failed");
        return false;
    }

    StreamRoles stream_roles = { StreamRole::VideoRecording };
    std::unique_ptr<CameraConfiguration> conf = camp->camera->generateConfiguration(stream_roles);
    if (conf == NULL) {
        set_error("Camera.generateConfiguration() failed");
        return false;
    }

    StreamConfiguration &stream_conf = conf->at(0);
    stream_conf.pixelFormat = formats::YUV420;
    stream_conf.bufferCount = params->buffer_count;
    stream_conf.size.width = params->width;
    stream_conf.size.height = params->height;
    if (params->width >= 1280 || params->height >= 720) {
        stream_conf.colorSpace = libcamera::ColorSpace::Rec709;
    } else {
        stream_conf.colorSpace = libcamera::ColorSpace::Smpte170m;
    }

    conf->transform = Transform::Identity;
    if (params->h_flip) {
        conf->transform = Transform::HFlip * conf->transform;
    }
    if (params->v_flip) {
        conf->transform = Transform::VFlip * conf->transform;
    }

    setenv("LIBCAMERA_RPI_TUNING_FILE", params->tuning_file, 1);

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

    Stream *stream = stream_conf.stream();

    camp->allocator = std::make_unique<FrameBufferAllocator>(camp->camera);
    res = camp->allocator->allocate(stream);
    if (res < 0) {
        set_error("allocate() failed");
        return false;
    }

    for (const std::unique_ptr<FrameBuffer> &buffer : camp->allocator->buffers(stream)) {
        std::unique_ptr<Request> request = camp->camera->createRequest((uint64_t)camp.get());
        if (request == NULL) {
            set_error("createRequest() failed");
            return false;
        }

        int res = request->addBuffer(stream, buffer.get());
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

int camera_get_stride(camera_t *cam) {
    CameraPriv *camp = (CameraPriv *)cam;
    return (*camp->camera->streams().begin())->configuration().stride;
}

int camera_get_colorspace(camera_t *cam) {
    CameraPriv *camp = (CameraPriv *)cam;
    return get_v4l2_colorspace((*camp->camera->streams().begin())->configuration().colorSpace);
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

    if (strlen(camp->params->roi) != 0) {
        float vals[4];
        int i = 0;
        char *token = strtok((char *)camp->params->roi, ",");
        while (token != NULL) {
            vals[i++] = atof(token);
            token = strtok(NULL, ",");
        }

        Rectangle sensor_area = camp->camera->properties().get(libcamera::properties::ScalerCropMaximum);
        Rectangle crop(
            vals[0] * sensor_area.width,
            vals[1] * sensor_area.height,
            vals[2] * sensor_area.width,
            vals[3] * sensor_area.height);
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
