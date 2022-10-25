#ifndef __PARAMETERS_H__
#define __PARAMETERS_H__

#include <stdbool.h>

#include "roi.h"
#include "sensor_mode.h"

typedef struct {
    unsigned int camera_id;
    unsigned int width;
    unsigned int height;
    bool h_flip;
    bool v_flip;
    float brightness;
    float contrast;
    float saturation;
    float sharpness;
    const char *exposure;
    const char *awb;
    const char *denoise;
    unsigned int shutter;
    const char *metering;
    float gain;
    float ev;
    roi_t *roi;
    const char *tuning_file;
    sensor_mode_t *mode;
    unsigned int fps;
    unsigned int idr_period;
    unsigned int bitrate;
    unsigned int profile;
    unsigned int level;

    // private
    unsigned int buffer_count;
    unsigned int capture_buffer_count;
} parameters_t;

#ifdef __cplusplus
extern "C" {
#endif

const char *parameters_get_error();
bool parameters_load(parameters_t *params);

#ifdef __cplusplus
}
#endif

#endif
