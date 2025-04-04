######################################
FROM ubuntu:20.04 AS build

RUN apt update && apt install -y --no-install-recommends \
    pkg-config \
    gcc \
    libgstreamer-plugins-base1.0-dev \
    && rm -rf /var/lib/apt/lists/*

COPY exitafterframe.c /s/
RUN cd /s \
    && gcc \
    exitafterframe.c \
    -o libexitafterframe.so \
    -Ofast \
    -s \
    -Werror \
    -Wall \
    -Wextra \
    -Wno-unused-parameter \
    -fPIC \
    -shared \
    -Wl,--no-undefined \
    $(pkg-config --cflags --libs gstreamer-1.0) \
    && mv libexitafterframe.so /usr/lib/x86_64-linux-gnu/gstreamer-1.0/ \
    && rm -rf /s

######################################
FROM ubuntu:20.04

RUN apt update && apt install -y --no-install-recommends \
    gstreamer1.0-tools \
    gstreamer1.0-plugins-good \
    gstreamer1.0-plugins-bad \
    gstreamer1.0-rtsp \
    gstreamer1.0-libav \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /usr/lib/x86_64-linux-gnu/gstreamer-1.0/libexitafterframe.so /usr/lib/x86_64-linux-gnu/gstreamer-1.0/

COPY emptyvideo.mkv /

COPY start.sh /
RUN chmod +x /start.sh

ENTRYPOINT [ "/start.sh" ]
