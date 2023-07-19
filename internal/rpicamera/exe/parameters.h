#ifndef __PARAMETERS_H__
#define __PARAMETERS_H__

#include <stdint.h>
#include <stdbool.h>

#include "window.h"
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
    char *exposure;
    char *awb;
    char *denoise;
    unsigned int shutter;
    char *metering;
    float gain;
    float ev;
    window_t *roi;
    bool hdr;
    char *tuning_file;
    sensor_mode_t *mode;
    float fps;
    unsigned int idr_period;
    unsigned int bitrate;
    unsigned int profile;
    unsigned int level;
    char *af_mode;
    char *af_range;
    char *af_speed;
    float lens_position;
    window_t *af_window;
    bool text_overlay_enable;
    char *text_overlay;

    // private
    unsigned int buffer_count;
    unsigned int capture_buffer_count;
} parameters_t;

#ifdef __cplusplus
extern "C" {
#endif

const char *parameters_get_error();
bool parameters_unserialize(parameters_t *params, const uint8_t *buf, size_t buf_size);
void parameters_destroy(parameters_t *params);

#ifdef __cplusplus
}
#endif

#endif
