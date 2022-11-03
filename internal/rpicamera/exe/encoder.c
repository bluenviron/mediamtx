#include <stdbool.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>
#include <stdint.h>
#include <fcntl.h>
#include <unistd.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/ioctl.h>
#include <errno.h>
#include <poll.h>
#include <pthread.h>

#include <linux/videodev2.h>

#include "parameters.h"
#include "encoder.h"

#define DEVICE              "/dev/video11"
#define POLL_TIMEOUT_MS     200

static char errbuf[256];

static void set_error(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vsnprintf(errbuf, 256, format, args);
}

const char *encoder_get_error() {
    return errbuf;
}

typedef struct {
    parameters_t *params;
    int fd;
    void **capture_buffers;
    int cur_buffer;
    encoder_output_cb output_cb;
    pthread_t output_thread;
    bool ts_initialized;
    uint64_t start_ts;
} encoder_priv_t;

static void *output_thread(void *userdata) {
    encoder_priv_t *encp = (encoder_priv_t *)userdata;

    while (true) {
        struct pollfd p = { encp->fd, POLLIN, 0 };
        int res = poll(&p, 1, POLL_TIMEOUT_MS);
        if (res == -1) {
            fprintf(stderr, "output_thread(): poll() failed\n");
            exit(1);
        }

        if (p.revents & POLLIN) {
            struct v4l2_buffer buf = {0};
            struct v4l2_plane planes[VIDEO_MAX_PLANES] = {0};
            buf.type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE;
            buf.memory = V4L2_MEMORY_DMABUF;
            buf.length = 1;
            buf.m.planes = planes;
            int res = ioctl(encp->fd, VIDIOC_DQBUF, &buf);
            if (res != 0) {
                fprintf(stderr, "output_thread(): ioctl(VIDIOC_DQBUF) failed\n");
                exit(1);
            }

            memset(&buf, 0, sizeof(buf));
            memset(planes, 0, sizeof(planes));
            buf.type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE;
            buf.memory = V4L2_MEMORY_MMAP;
            buf.length = 1;
            buf.m.planes = planes;
            res = ioctl(encp->fd, VIDIOC_DQBUF, &buf);
            if (res == 0) {
                uint64_t ts = ((uint64_t)buf.timestamp.tv_sec * (uint64_t)1000000) + (uint64_t)buf.timestamp.tv_usec;

                if (!encp->ts_initialized) {
                    encp->ts_initialized = true;
                    encp->start_ts = ts;
                }

                ts -= encp->start_ts;

                const uint8_t *bufmem = (const uint8_t *)encp->capture_buffers[buf.index];
                int bufsize = buf.m.planes[0].bytesused;
                encp->output_cb(ts, bufmem, bufsize);

                int index = buf.index;
                int length = buf.m.planes[0].length;

                struct v4l2_buffer buf = {0};
                struct v4l2_plane planes[VIDEO_MAX_PLANES] = {0};
                buf.type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE;
                buf.memory = V4L2_MEMORY_MMAP;
                buf.index = index;
                buf.length = 1;
                buf.m.planes = planes;
                buf.m.planes[0].bytesused = 0;
                buf.m.planes[0].length = length;
                int res = ioctl(encp->fd, VIDIOC_QBUF, &buf);
                if (res < 0) {
                    fprintf(stderr, "output_thread(): ioctl(VIDIOC_QBUF) failed\n");
                    exit(1);
                }
            }
        }
    }

    return NULL;
}

bool encoder_create(parameters_t *params, int stride, int colorspace, encoder_output_cb output_cb, encoder_t **enc) {
    *enc = malloc(sizeof(encoder_priv_t));
    encoder_priv_t *encp = (encoder_priv_t *)(*enc);

    encp->fd = open(DEVICE, O_RDWR, 0);
    if (encp->fd < 0) {
        set_error("unable to open device");
        return false;
    }

    struct v4l2_control ctrl = {0};
    ctrl.id = V4L2_CID_MPEG_VIDEO_BITRATE;
    ctrl.value = params->bitrate;
    int res = ioctl(encp->fd, VIDIOC_S_CTRL, &ctrl);
    if (res != 0) {
        set_error("unable to set bitrate");
        close(encp->fd);
        return false;
    }

    ctrl.id = V4L2_CID_MPEG_VIDEO_H264_PROFILE;
    ctrl.value = params->profile;
    res = ioctl(encp->fd, VIDIOC_S_CTRL, &ctrl);
    if (res != 0) {
        set_error("unable to set profile");
        close(encp->fd);
        return false;
    }

    ctrl.id = V4L2_CID_MPEG_VIDEO_H264_LEVEL;
    ctrl.value = params->level;
    res = ioctl(encp->fd, VIDIOC_S_CTRL, &ctrl);
    if (res != 0) {
        set_error("unable to set level");
        close(encp->fd);
        return false;
    }

    ctrl.id = V4L2_CID_MPEG_VIDEO_H264_I_PERIOD;
    ctrl.value = params->idr_period;
    res = ioctl(encp->fd, VIDIOC_S_CTRL, &ctrl);
    if (res != 0) {
        set_error("unable to set IDR period");
        close(encp->fd);
        return false;
    }

    ctrl.id = V4L2_CID_MPEG_VIDEO_REPEAT_SEQ_HEADER;
    ctrl.value = 0;
    res = ioctl(encp->fd, VIDIOC_S_CTRL, &ctrl);
    if (res != 0) {
        set_error("unable to set REPEAT_SEQ_HEADER");
        close(encp->fd);
        return false;
    }

    struct v4l2_format fmt = {0};
    fmt.type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE;
    fmt.fmt.pix_mp.width = params->width;
    fmt.fmt.pix_mp.height = params->height;
    fmt.fmt.pix_mp.pixelformat = V4L2_PIX_FMT_YUV420;
    fmt.fmt.pix_mp.plane_fmt[0].bytesperline = stride;
    fmt.fmt.pix_mp.field = V4L2_FIELD_ANY;
    fmt.fmt.pix_mp.colorspace = colorspace;
    fmt.fmt.pix_mp.num_planes = 1;
    res = ioctl(encp->fd, VIDIOC_S_FMT, &fmt);
    if (res != 0) {
        set_error("unable to set output format");
        close(encp->fd);
        return false;
    }

    memset(&fmt, 0, sizeof(fmt));
    fmt.type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE;
    fmt.fmt.pix_mp.width = params->width;
    fmt.fmt.pix_mp.height = params->height;
    fmt.fmt.pix_mp.pixelformat = V4L2_PIX_FMT_H264;
    fmt.fmt.pix_mp.field = V4L2_FIELD_ANY;
    fmt.fmt.pix_mp.colorspace = V4L2_COLORSPACE_DEFAULT;
    fmt.fmt.pix_mp.num_planes = 1;
    fmt.fmt.pix_mp.plane_fmt[0].bytesperline = 0;
    fmt.fmt.pix_mp.plane_fmt[0].sizeimage = 512 << 10;
    res = ioctl(encp->fd, VIDIOC_S_FMT, &fmt);
    if (res != 0) {
        set_error("unable to set capture format");
        close(encp->fd);
        return false;
    }

    struct v4l2_streamparm parm = {0};
    parm.type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE;
    parm.parm.output.timeperframe.numerator = 1;
    parm.parm.output.timeperframe.denominator = params->fps;
    res = ioctl(encp->fd, VIDIOC_S_PARM, &parm);
    if (res != 0) {
        set_error("unable to set fps");
        close(encp->fd);
        return false;
    }

    struct v4l2_requestbuffers reqbufs = {0};
    reqbufs.count = params->buffer_count;
    reqbufs.type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE;
    reqbufs.memory = V4L2_MEMORY_DMABUF;
    res = ioctl(encp->fd, VIDIOC_REQBUFS, &reqbufs);
    if (res != 0) {
        set_error("unable to set output buffers");
        close(encp->fd);
        return false;
    }

    memset(&reqbufs, 0, sizeof(reqbufs));
    reqbufs.count = params->capture_buffer_count;
    reqbufs.type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE;
    reqbufs.memory = V4L2_MEMORY_MMAP;
    res = ioctl(encp->fd, VIDIOC_REQBUFS, &reqbufs);
    if (res != 0) {
        set_error("unable to set capture buffers");
        close(encp->fd);
        return false;
    }

    encp->capture_buffers = malloc(sizeof(void *) * reqbufs.count);

    for (unsigned int i = 0; i < reqbufs.count; i++) {
        struct v4l2_plane planes[VIDEO_MAX_PLANES];

        struct v4l2_buffer buffer = {0};
        buffer.type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE;
        buffer.memory = V4L2_MEMORY_MMAP;
        buffer.index = i;
        buffer.length = 1;
        buffer.m.planes = planes;
        int res = ioctl(encp->fd, VIDIOC_QUERYBUF, &buffer);
        if (res != 0) {
            set_error("unable to query buffer");
            free(encp->capture_buffers);
            close(encp->fd);
            return false;
        }

        encp->capture_buffers[i] = mmap(
            0,
            buffer.m.planes[0].length,
            PROT_READ | PROT_WRITE, MAP_SHARED,
            encp->fd,
            buffer.m.planes[0].m.mem_offset);
        if (encp->capture_buffers[i] == MAP_FAILED) {
            set_error("mmap() failed");
            free(encp->capture_buffers);
            close(encp->fd);
            return false;
        }

        res = ioctl(encp->fd, VIDIOC_QBUF, &buffer);
        if (res != 0) {
            set_error("ioctl(VIDIOC_QBUF) failed");
            free(encp->capture_buffers);
            close(encp->fd);
            return false;
        }
    }

    enum v4l2_buf_type type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE;
    res = ioctl(encp->fd, VIDIOC_STREAMON, &type);
    if (res != 0) {
        set_error("unable to activate output stream");
        free(encp->capture_buffers);
        close(encp->fd);
        return false;
    }

    type = V4L2_BUF_TYPE_VIDEO_CAPTURE_MPLANE;
    res = ioctl(encp->fd, VIDIOC_STREAMON, &type);
    if (res != 0) {
        set_error("unable to activate capture stream");
        free(encp->capture_buffers);
        close(encp->fd);
        return false;
    }

    encp->params = params;
    encp->cur_buffer = 0;
    encp->output_cb = output_cb;
    encp->ts_initialized = false;

    pthread_create(&encp->output_thread, NULL, output_thread, encp);

    return true;
}

void encoder_encode(encoder_t *enc, int buffer_fd, size_t size, int64_t timestamp_us) {
    encoder_priv_t *encp = (encoder_priv_t *)enc;

    int index = encp->cur_buffer++;
    encp->cur_buffer %= encp->params->buffer_count;

    struct v4l2_buffer buf = {0};
    struct v4l2_plane planes[VIDEO_MAX_PLANES] = {0};
    buf.type = V4L2_BUF_TYPE_VIDEO_OUTPUT_MPLANE;
    buf.index = index;
    buf.field = V4L2_FIELD_NONE;
    buf.memory = V4L2_MEMORY_DMABUF;
    buf.length = 1;
    buf.timestamp.tv_sec = timestamp_us / 1000000;
    buf.timestamp.tv_usec = timestamp_us % 1000000;
    buf.m.planes = planes;
    buf.m.planes[0].m.fd = buffer_fd;
    buf.m.planes[0].bytesused = size;
    buf.m.planes[0].length = size;
    int res = ioctl(encp->fd, VIDIOC_QBUF, &buf);
    if (res != 0) {
        fprintf(stderr, "encoder_encode(): ioctl(VIDIOC_QBUF) failed\n");
        // it happens when the raspberry is under pressure. do not exit.
    }
}
