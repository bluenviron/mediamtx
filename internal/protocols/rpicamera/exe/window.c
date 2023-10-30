#include <stdlib.h>
#include <string.h>

#include "window.h"

bool window_load(const char *encoded, window_t *window) {
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

    window->x = vals[0];
    window->y = vals[1];
    window->width = vals[2];
    window->height = vals[3];

    return true;
}
