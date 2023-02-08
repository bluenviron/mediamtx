#ifndef __CAMERA_H__
#define __CAMERA_H__

typedef void camera_t;

typedef void (*camera_frame_cb)(int buffer_fd, uint64_t size, uint64_t timestamp);

#ifdef __cplusplus
extern "C" {
#endif

const char *camera_get_error();
bool camera_create(const parameters_t *params, camera_frame_cb frame_cb, camera_t **cam);
int camera_get_mode_stride(camera_t *cam);
int camera_get_mode_colorspace(camera_t *cam);
bool camera_start(camera_t *cam);
void camera_reload_params(camera_t *cam, const parameters_t *params);

#ifdef __cplusplus
}
#endif

#endif
