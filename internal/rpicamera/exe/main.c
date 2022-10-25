#include <stdio.h>
#include <stdbool.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdlib.h>
#include <fcntl.h>
#include <unistd.h>
#include <signal.h>
#include <string.h>
#include <pthread.h>

#include "parameters.h"
#include "camera.h"
#include "encoder.h"

int pipe_fd;
pthread_mutex_t pipe_mutex;
parameters_t params;
camera_t *cam;
encoder_t *enc;

static void pipe_write_error(int fd, const char *format, ...) {
    char buf[256];
    buf[0] = 'e';
    va_list args;
    va_start(args, format);
    vsnprintf(&buf[1], 255, format, args);
    int n = strlen(buf);
    write(fd, &n, 4);
    write(fd, buf, n);
}

static void pipe_write_ready(int fd) {
    char buf[] = {'r'};
    int n = 1;
    write(fd, &n, 4);
    write(fd, buf, n);
}

static void pipe_write_buf(int fd, uint64_t ts, const uint8_t *buf, int n) {
    char head[] = {'b'};
    n += 1 + sizeof(uint64_t);
    write(fd, &n, 4);
    write(fd, head, 1);
    write(fd, &ts, sizeof(uint64_t));
    write(fd, buf, n - 1 - sizeof(uint64_t));
}

static void on_frame(int buffer_fd, uint64_t size, uint64_t timestamp) {
    encoder_encode(enc, buffer_fd, size, timestamp);
}

static void on_encoder_output(uint64_t ts, const uint8_t *buf, uint64_t size) {
    pthread_mutex_lock(&pipe_mutex);
    pipe_write_buf(pipe_fd, ts, buf, size);
    pthread_mutex_unlock(&pipe_mutex);
}

static bool init_siglistener(sigset_t *set) {
    sigemptyset(set);

    int res = sigaddset(set, SIGKILL);
    if (res == -1) {
        return false;
    }

    return true;
}

int main() {
    pipe_fd = atoi(getenv("PIPE_FD"));

    pthread_mutex_init(&pipe_mutex, NULL);
    pthread_mutex_lock(&pipe_mutex);

    bool ok = parameters_load(&params);
    if (!ok) {
        pipe_write_error(pipe_fd, "parameters_load(): %s", parameters_get_error());
        return 5;
    }

    ok = camera_create(
        &params,
        on_frame,
        &cam);
    if (!ok) {
        pipe_write_error(pipe_fd, "camera_create(): %s", camera_get_error());
        return 5;
    }

    ok = encoder_create(
        &params,
        camera_get_mode_stride(cam),
        camera_get_mode_colorspace(cam),
        on_encoder_output,
        &enc);
    if (!ok) {
        pipe_write_error(pipe_fd, "encoder_create(): %s", encoder_get_error());
        return 5;
    }

    ok = camera_start(cam);
    if (!ok) {
        pipe_write_error(pipe_fd, "camera_start(): %s", camera_get_error());
        return 5;
    }

    sigset_t set;
    ok = init_siglistener(&set);
    if (!ok) {
        pipe_write_error(pipe_fd, "init_siglistener() failed");
        return 5;
    }

    pipe_write_ready(pipe_fd);
    pthread_mutex_unlock(&pipe_mutex);

    int sig;
    sigwait(&set, &sig);

    return 0;
}
