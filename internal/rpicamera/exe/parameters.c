#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <stdarg.h>
#include <stdio.h>

#include <linux/videodev2.h>

#include "base64.h"
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

bool parameters_unserialize(parameters_t *params, const uint8_t *buf, size_t buf_size) {
    memset(params, 0, sizeof(parameters_t));

    char *tmp = malloc(buf_size + 1);
    memcpy(tmp, buf, buf_size);
    tmp[buf_size] = 0x00;

    while (true)  {
        char *entry = strsep(&tmp, " ");
        if (entry == NULL) {
            break;
        }

        char *key = strsep(&entry, ":");
        char *val = strsep(&entry, ":");

        if (strcmp(key, "CameraID") == 0) {
            params->camera_id = atoi(val);
        } else if (strcmp(key, "Width") == 0) {
            params->width = atoi(val);
        } else if (strcmp(key, "Height") == 0) {
            params->height = atoi(val);
        } else if (strcmp(key, "HFlip") == 0) {
            params->h_flip = (strcmp(val, "1") == 0);
        } else if (strcmp(key, "VFlip") == 0) {
            params->v_flip = (strcmp(val, "1") == 0);
        } else if (strcmp(key, "Brightness") == 0) {
            params->brightness = atof(val);
        } else if (strcmp(key, "Contrast") == 0) {
           params->contrast = atof(val);
        } else if (strcmp(key, "Saturation") == 0) {
            params->saturation = atof(val);
        } else if (strcmp(key, "Sharpness") == 0) {
            params->sharpness = atof(val);
        } else if (strcmp(key, "Exposure") == 0) {
            params->exposure = base64_decode(val);
        } else if (strcmp(key, "AWB") == 0) {
            params->awb = base64_decode(val);
        } else if (strcmp(key, "Denoise") == 0) {
            params->denoise = base64_decode(val);
        } else if (strcmp(key, "Shutter") == 0) {
            params->shutter = atoi(val);
        } else if (strcmp(key, "Metering") == 0) {
            params->metering = base64_decode(val);
        } else if (strcmp(key, "Gain") == 0) {
            params->gain = atof(val);
        } else if (strcmp(key, "EV") == 0) {
            params->ev = atof(val);
        } else if (strcmp(key, "ROI") == 0) {
            char *decoded_val = base64_decode(val);
            if (strlen(decoded_val) != 0) {
                params->roi = malloc(sizeof(window_t));
                bool ok = window_load(decoded_val, params->roi);
                if (!ok) {
                    set_error("invalid ROI");
                    free(decoded_val);
                    goto failed;
                }
            }
            free(decoded_val);
        } else if (strcmp(key, "HDR") == 0) {
            params->hdr = (strcmp(val, "1") == 0);
        } else if (strcmp(key, "TuningFile") == 0) {
            params->tuning_file = base64_decode(val);
        } else if (strcmp(key, "Mode") == 0) {
            char *decoded_val = base64_decode(val);
            if (strlen(decoded_val) != 0) {
                params->mode = malloc(sizeof(sensor_mode_t));
                bool ok = sensor_mode_load(decoded_val, params->mode);
                if (!ok) {
                    set_error("invalid sensor mode");
                    free(decoded_val);
                    goto failed;
                }
            }
            free(decoded_val);
        } else if (strcmp(key, "FPS") == 0) {
            params->fps = atof(val);
        } else if (strcmp(key, "IDRPeriod") == 0) {
            params->idr_period = atoi(val);
        } else if (strcmp(key, "Bitrate") == 0) {
            params->bitrate = atoi(val);
        } else if (strcmp(key, "Profile") == 0) {
            char *decoded_val = base64_decode(val);
            if (strcmp(decoded_val, "baseline") == 0) {
                params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_BASELINE;
            } else if (strcmp(decoded_val, "main") == 0) {
                params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_MAIN;
            } else {
                params->profile = V4L2_MPEG_VIDEO_H264_PROFILE_HIGH;
            }
            free(decoded_val);
        } else if (strcmp(key, "Level") == 0) {
            char *decoded_val = base64_decode(val);
            if (strcmp(decoded_val, "4.0") == 0) {
                params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_0;
            } else if (strcmp(decoded_val, "4.1") == 0) {
                params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_1;
            } else {
                params->level = V4L2_MPEG_VIDEO_H264_LEVEL_4_2;
            }
            free(decoded_val);
        } else if (strcmp(key, "AfMode") == 0) {
            params->af_mode = base64_decode(val);
        } else if (strcmp(key, "AfRange") == 0) {
            params->af_range = base64_decode(val);
        } else if (strcmp(key, "AfSpeed") == 0) {
            params->af_speed = base64_decode(val);
        } else if (strcmp(key, "LensPosition") == 0) {
            params->lens_position = atof(val);
        } else if (strcmp(key, "AfWindow") == 0) {
            char *decoded_val = base64_decode(val);
            if (strlen(decoded_val) != 0) {
                params->af_window = malloc(sizeof(window_t));
                bool ok = window_load(decoded_val, params->af_window);
                if (!ok) {
                    set_error("invalid AfWindow");
                    free(decoded_val);
                    goto failed;
                }
            }
            free(decoded_val);
        } else if (strcmp(key, "TextOverlayEnable") == 0) {
            params->text_overlay_enable = (strcmp(val, "1") == 0);
        } else if (strcmp(key, "TextOverlay") == 0) {
            params->text_overlay = base64_decode(val);
        }
    }

    free(tmp);

    params->buffer_count = 6;
    params->capture_buffer_count = params->buffer_count * 2;

    return true;

failed:
    free(tmp);
    parameters_destroy(params);

    return false;
}

void parameters_destroy(parameters_t *params) {
    if (params->exposure != NULL) {
        free(params->exposure);
    }
    if (params->awb != NULL) {
        free(params->awb);
    }
    if (params->denoise != NULL) {
        free(params->denoise);
    }
    if (params->metering != NULL) {
        free(params->metering);
    }
    if (params->roi != NULL) {
        free(params->roi);
    }
    if (params->tuning_file != NULL) {
        free(params->tuning_file);
    }
    if (params->mode != NULL) {
        free(params->mode);
    }
    if (params->af_mode != NULL) {
        free(params->af_mode);
    }
    if (params->af_range != NULL) {
        free(params->af_range);
    }
    if (params->af_speed != NULL) {
        free(params->af_speed);
    }
    if (params->af_window != NULL) {
        free(params->af_window);
    }
    if (params->text_overlay != NULL) {
        free(params->text_overlay);
    }
}
