#ifndef __ENCODER_H__
#define __ENCODER_H__

typedef void encoder_t;

typedef void (*encoder_output_cb)(uint64_t ts, const uint8_t *buf, uint64_t size);

const char *encoder_get_error();
bool encoder_create(parameters_t *params, int stride, int colorspace, encoder_output_cb output_cb, encoder_t **enc);
void encoder_encode(encoder_t *enc, int buffer_fd, size_t size, int64_t timestamp_us);

#endif
