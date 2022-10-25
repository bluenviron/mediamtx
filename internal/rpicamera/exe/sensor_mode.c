#include <stdio.h>
#include <ctype.h>
#include <stdlib.h>

#include "sensor_mode.h"

bool sensor_mode_load(const char *encoded, sensor_mode_t **mode) {
    *mode = malloc(sizeof(sensor_mode_t));

    char p;
    int n = sscanf(encoded, "%u:%u:%u:%c", &((*mode)->width), &((*mode)->height), &((*mode)->bit_depth), &p);
    if (n < 2) {
        free(*mode);
        return false;
    }

    if (n < 4) {
        (*mode)->packed = true;
    } else if (toupper(p) == 'P') {
        (*mode)->packed = true;
    } else if (toupper(p) == 'U') {
        (*mode)->packed = false;
    }

    if (n < 3) {
        (*mode)->bit_depth = 12;
    }

    return true;
}
