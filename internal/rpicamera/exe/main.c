#include <stdio.h>
#include <stdbool.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>

#include "parameters.h"
#include "pipe.h"
#include "camera.h"
#include "encoder.h"

int pipe_video_fd;
pthread_mutex_t pipe_video_mutex;
encoder_t *enc;

static void on_frame(int buffer_fd, uint64_t size, uint64_t timestamp) {
    encoder_encode(enc, buffer_fd, size, timestamp);
}

static void on_encoder_output(uint64_t ts, const uint8_t *buf, uint64_t size) {
    pthread_mutex_lock(&pipe_video_mutex);
    pipe_write_buf(pipe_video_fd, ts, buf, size);
    pthread_mutex_unlock(&pipe_video_mutex);
}

int main() {
    int pipe_conf_fd = atoi(getenv("PIPE_CONF_FD"));
    pipe_video_fd = atoi(getenv("PIPE_VIDEO_FD"));

    uint8_t *buf;
    uint32_t n = pipe_read(pipe_conf_fd, &buf);

    parameters_t params;
    bool ok = parameters_unserialize(&params, &buf[1], n-1);
    free(buf);
    if (!ok) {
        pipe_write_error(pipe_video_fd, "parameters_unserialize(): %s", parameters_get_error());
        return 5;
    }

    pthread_mutex_init(&pipe_video_mutex, NULL);
    pthread_mutex_lock(&pipe_video_mutex);

    camera_t *cam;
    ok = camera_create(
        &params,
        on_frame,
        &cam);
    if (!ok) {
        pipe_write_error(pipe_video_fd, "camera_create(): %s", camera_get_error());
        return 5;
    }

    ok = encoder_create(
        &params,
        camera_get_mode_stride(cam),
        camera_get_mode_colorspace(cam),
        on_encoder_output,
        &enc);
    if (!ok) {
        pipe_write_error(pipe_video_fd, "encoder_create(): %s", encoder_get_error());
        return 5;
    }

    ok = camera_start(cam);
    if (!ok) {
        pipe_write_error(pipe_video_fd, "camera_start(): %s", camera_get_error());
        return 5;
    }

    pipe_write_ready(pipe_video_fd);
    pthread_mutex_unlock(&pipe_video_mutex);

    while (true) {
        uint8_t *buf;
        uint32_t n = pipe_read(pipe_conf_fd, &buf);

        switch (buf[0]) {
        case 'e':
            return 0;

        case 'c':
            {
                parameters_t params;
                bool ok = parameters_unserialize(&params, &buf[1], n-1);
                free(buf);
                if (!ok) {
                    printf("skipping reloading parameters since they are invalid: %s\n", parameters_get_error());
                    continue;
                }
                camera_reload_params(cam, &params);
                parameters_destroy(&params);
            }
        }
    }

    return 0;
}
