CFLAGS = \
	-Ofast \
	-Werror \
	-Wall \
	-Wextra \
	-Wno-unused-parameter \
	-Wno-unused-result

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
	$$(pkg-config --libs libcamera)

OBJS = \
	camera.o \
	encoder.o \
	main.o \
	parameters.o \
	pipe.o \
	window.o \
	sensor_mode.o

all: exe

%.o: %.c
	$(CC) $(CFLAGS) -c $< -o $@

%.o: %.cpp
	$(CXX) $(CXXFLAGS) -c $< -o $@

exe: $(OBJS)
	$(CXX) $^ $(LDFLAGS) -o $@
	patchelf --replace-needed libcamera.so.0.0.2 libcamera.so.x.x.x $@
	patchelf --replace-needed libcamera-base.so.0.0.2 libcamera-base.so.x.x.x $@
