#ifndef __WINDOW_H__
#define __WINDOW_H__

#include <stdbool.h>

typedef struct {
    float x;
    float y;
    float width;
    float height;
} window_t;

bool window_load(const char *encoded, window_t *window);

#endif
