#ifndef __TEXT_H__
#define __TEXT_H__

#include <stdint.h>
#include <stdbool.h>

#include "parameters.h"

typedef void text_t;

const char *text_get_error();
bool text_create(const parameters_t *params, text_t **text);
void text_draw(text_t *text, uint8_t *buf, int stride, int height);

#endif
