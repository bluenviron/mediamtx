#include <time.h>

#include <ft2build.h>
#include FT_FREETYPE_H

#include "text_font.h"
#include "text.h"

static char errbuf[256];

static void set_error(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vsnprintf(errbuf, 256, format, args);
}

const char *text_get_error() {
    return errbuf;
}

typedef struct {
    bool disabled;
    char *text_overlay;
    FT_Library library;
    FT_Face face;
} text_priv_t;

bool text_create(const parameters_t *params, text_t **text) {
    *text = malloc(sizeof(text_priv_t));
    text_priv_t *textp = (text_priv_t *)(*text);
    memset(textp, 0, sizeof(text_priv_t));

    textp->disabled = params->text_overlay_disable;
    textp->text_overlay = strdup(params->text_overlay);

    if (!textp->disabled) {
        int error = FT_Init_FreeType(&textp->library);
        if (error) {
            set_error("FT_Init_FreeType() failed");
            goto failed;
        }

        error = FT_New_Memory_Face(
            textp->library,
            IBMPlexMono_Medium,
            sizeof(IBMPlexMono_Medium),
            0,
            &textp->face);
        if (error) {
            set_error("FT_New_Memory_Face() failed");
            goto failed;
        }

        error = FT_Set_Pixel_Sizes(
            textp->face,
            25,
            25);
        if (error) {
            set_error("FT_Set_Pixel_Sizes() failed");
            goto failed;
        }
    }

    return true;

failed:
    free(textp);

    return false;
}

static void draw_bitmap(uint8_t *buf, int stride, int height, const FT_Bitmap *bitmap, int x, int y) {
    uint8_t *Y = buf;
    // uint8_t *U = Y + stride * height;
    // uint8_t *V = U + (stride / 2) * (height / 2);

    for (unsigned int src_y = 0; src_y < bitmap->rows; src_y++) {
        for (unsigned int src_x = 0; src_x < bitmap->width; src_x++) {
            uint8_t v = bitmap->buffer[src_y*bitmap->pitch + src_x];

            if (v != 0) {
                unsigned int dest_x = x + src_x;
                unsigned int dest_y = y + src_y;
                Y[dest_y*stride + dest_x] = v;
            }
        }
    }
}

void text_draw(text_t *text, uint8_t *buf, int stride, int height) {
    text_priv_t *textp = (text_priv_t *)text;

    if (textp->disabled) {
        return;
    }

    time_t timer = time(NULL);
    struct tm *tm_info = localtime(&timer);
    char buffer[255];
    memset(buffer, 0, sizeof(buffer));
    strftime(buffer, 255, textp->text_overlay, tm_info);

    int x = 10;
    int y = 35;

    for (const char *ptr = buffer; *ptr != 0x00; ptr++) {
        int error = FT_Load_Char(textp->face, *ptr, FT_LOAD_RENDER);
        if (error) {
            continue;
        }

        draw_bitmap(
            buf,
            stride,
            height,
            &textp->face->glyph->bitmap,
            x + textp->face->glyph->bitmap_left,
            y - textp->face->glyph->bitmap_top);

        x += textp->face->glyph->advance.x >> 6;
    }
}
