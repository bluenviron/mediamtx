#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <fcntl.h>
#include <unistd.h>

#include "pipe.h"

void pipe_write_error(int fd, const char *format, ...) {
    char buf[256];
    buf[0] = 'e';
    va_list args;
    va_start(args, format);
    vsnprintf(&buf[1], 255, format, args);
    uint32_t n = strlen(buf);
    write(fd, &n, 4);
    write(fd, buf, n);
}

void pipe_write_ready(int fd) {
    char buf[] = {'r'};
    uint32_t n = 1;
    write(fd, &n, 4);
    write(fd, buf, n);
}

void pipe_write_buf(int fd, uint64_t ts, const uint8_t *buf, uint32_t n) {
    char head[] = {'b'};
    n += 1 + sizeof(uint64_t);
    write(fd, &n, 4);
    write(fd, head, 1);
    write(fd, &ts, sizeof(uint64_t));
    write(fd, buf, n - 1 - sizeof(uint64_t));
}

uint32_t pipe_read(int fd, uint8_t **pbuf) {
    uint32_t n;
    read(fd, &n, 4);

    *pbuf = malloc(n);
    read(fd, *pbuf, n);
    return n;
}
