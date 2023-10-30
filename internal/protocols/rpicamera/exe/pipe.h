#ifndef __PIPE_H__
#define __PIPE_H__

#include <stdbool.h>
#include <stdint.h>

void pipe_write_error(int fd, const char *format, ...);
void pipe_write_ready(int fd);
void pipe_write_buf(int fd, uint64_t ts, const uint8_t *buf, uint32_t n);
uint32_t pipe_read(int fd, uint8_t **pbuf);

#endif
