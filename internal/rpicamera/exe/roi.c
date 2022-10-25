#include <stdlib.h>
#include <string.h>

#include "roi.h"

bool roi_load(const char *encoded, roi_t **mode) {
    float vals[4];
    int i = 0;
    char *token = strtok((char *)encoded, ",");
    while (token != NULL) {
        vals[i] = atof(token);
        if (vals[i] < 0 || vals[i] > 1) {
            return false;
        }

        i++;
        token = strtok(NULL, ",");
    }

    if (i != 4) {
        return false;
    }

    *mode = malloc(sizeof(roi_t));
    (*mode)->x = vals[0];
    (*mode)->y = vals[1];
    (*mode)->width = vals[2];
    (*mode)->height = vals[3];
    return true;
}
