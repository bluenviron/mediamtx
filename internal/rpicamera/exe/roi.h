#ifndef __ROI_H__
#define __ROI_H__

#include <stdbool.h>

typedef struct {
    float x;
    float y;
    float width;
    float height;
} roi_t;

bool roi_load(const char *encoded, roi_t **mode);

#endif
