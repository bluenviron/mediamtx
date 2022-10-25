#ifndef __SENSOR_MODE_H__
#define __SENSOR_MODE_H__

#include <stdbool.h>

typedef struct {
    int width;
    int height;
    int bit_depth;
    bool packed;
} sensor_mode_t;

bool sensor_mode_load(const char *encoded, sensor_mode_t **mode);

#endif
