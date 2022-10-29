#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdarg.h>
#include <stdio.h>

#include <linux/videodev2.h>

#include "parameters.h"

static char errbuf[256];

static void set_error(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vsnprintf(errbuf, 256, format, args);
}

const char *parameters_get_error() {
    return errbuf;
}

bool parameters_load(parameters_t *params) {
    params->camera_id = atoi(getenv("CAMERA_ID"));
    params->width = atoi(getenv("WIDTH"));
    params->height = atoi(getenv("HEIGHT"));
    params->h_flip = (strcmp(getenv("H_FLIP"), "1") == 0);
    params->v_flip = (strcmp(getenv("V_FLIP"), "1") == 0);
    params->brightness = atof(getenv("BRIGHTNESS"));
    params->contrast = atof(getenv("CONTRAST"));
    params->saturation = atof(getenv("SATURATION"));
    params->sharpness = atof(getenv("SHARPNESS"));
    params->exposure = getenv("EXPOSURE");
    params->awb = getenv("AWB");
    params->denoise = getenv("DENOISE");
    params->shutter = atoi(getenv("SHUTTER"));
    params->metering = getenv("METERING");
    params->gain = atof(getenv("GAIN"));
    params->ev = atof(getenv("EV"));

    if (strlen(getenv("ROI")) != 0) {
        bool ok = roi_load(getenv("ROI"), &params->roi);
        if (!ok) {
            set_error("invalid ROI");
            return false;
        }
    } else {
        params->roi = NULL;
    }

    params->tuning_file = getenv("TUNING_FILE");

    if (strlen(getenv("MODE")) != 0) {
        bool ok = sensor_mode_load(getenv("MODE"), &params->mode);
        if (!ok) {
            set_error("invalid sensor mode");
            return false;
        }
    } else {
        params->mode = NULL;
    }

    params->fps = atoi(getenv("FPS"));
    params->idr_period = atoi(getenv("IDR_PERIOD"));
    params->bitrate = atoi(getenv("BITRATE"));

    const char *profile = getenv("PROFILE");
    if (strcmp(profile, "baseline") == 0) {
        params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_BASELINE;
    } else if (strcmp(profile, "main") == 0) {
        params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_MAIN;
    } else {
        params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_HIGH;
    }

    const char *level = getenv("LEVEL");
    if (strcmp(level, "4.0") == 0) {
        params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_0;
    } else if (strcmp(level, "4.1") == 0) {
        params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_1;
    } else {
        params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_2;
    }

    params->buffer_count = 3;
    params->capture_buffer_count = params->buffer_count * 2;

    return true;
}
