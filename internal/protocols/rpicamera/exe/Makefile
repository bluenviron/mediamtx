CFLAGS = \
	-Ofast \
	-Werror \
	-Wall \
	-Wextra \
	-Wno-unused-parameter \
	-Wno-unused-result \
	$$(pkg-config --cflags freetype2)

CXXFLAGS = \
	-Ofast \
	-Werror \
	-Wall \
	-Wextra \
	-Wno-unused-parameter \
	-Wno-unused-result \
	-std=c++17 \
	$$(pkg-config --cflags libcamera)

LDFLAGS = \
	-s \
	-pthread \
	$$(pkg-config --libs freetype2) \
	$$(pkg-config --libs libcamera)

OBJS = \
	base64.o \
	camera.o \
	encoder.o \
	main.o \
	parameters.o \
	pipe.o \
	sensor_mode.o \
	text.o \
	window.o

all: exe

text_font.h: text_font.ttf
	xxd --include $< > text_font.h

%.o: %.c text_font.h
	$(CC) $(CFLAGS) -c $< -o $@

%.o: %.cpp
	$(CXX) $(CXXFLAGS) -c $< -o $@

exe: $(OBJS)
	$(CXX) $^ $(LDFLAGS) -o $@
