# GstPipe

Use the `gst-pipe(.exe)` binary in the `runOnDemand` param in `mediamtx.yml` config.
```
	gst-pipe.exe "rtspsrc location=..."	[ --debug=<1,2> ]]
	gst-pipe.exe "rtspsrc location=rtsp://172.25.25.141:554/video1 name=src user-id=admin user-pw=piscada3138 latency=10 drop-on-latency=true udp-buffer-size=10242888 ! queue2 ! rtph264depay ! h264parse ! queue2 ! rtspclientsink name=sink location=rtsp://localhost:7554/PI-41" --debug=<1,2>
```

# Development

## Ubuntu/Linux - for Linux binary and docker

1. Install all apt-packages needed + libcurl
2. MISSING HERE: SEE DOCKER_FILES FOR DEPENDENCIES
3. Build in gcc by using:

```bash
# build
gcc pipeme.c stats_post.c -o pipeme `pkg-config --cflags --libs gstreamer-1.0` -lcurl
```

## Windows - for Windows binary

### Prerequisites

Install both the MSVC **runtime** an **development** .msi packages from gstreamer from official site: https://gstreamer.freedesktop.org/download/#windows

### Gstreamer dependencies

1. Install gstreamer mentioned above.
2. Ensure env var GSTREAMER_1_0_ROOT_X86_64 exists. It is used in the gst-pipe.vcxproj file.
3. Open Visual Studio (Community 2022) project gst-pipe.vcxproj

### libcurl dependencies

Source: https://stackoverflow.com/a/54680718/8851459

1. Get latest vcpkg zip file from https://github.com/microsoft/vcpkg/releases and extract it to C:\vcpkg\

2. Open Developer Command Prompt for VS 2022 (see Windows Start menu or `%PROGRAMDATA%\Microsoft\Windows\Start Menu\Programs\Visual Studio 2022\Visual Studio Tools\`) and cd to `C:\vcpkg\`

3. Run `bootstrap - vcpkg.bat` 4. Run `vcpkg.exe integrate install` 5. Run `vcpkg.exe install curl`

4. Reopen this project in Visual Studio 2022. 

### Build and run

1. Build solution by (Ctrl+Shift+B) or F7

2. Go to debug folder and test e.g with: to find gst-pipe.exe

3. Start the `mediamtx.exe` in a seperate terminal, and then fire up gst-pipe manually like:
	
	`gst-pipe.exe "rtspsrc location=rtsp://..." --debug=1`

