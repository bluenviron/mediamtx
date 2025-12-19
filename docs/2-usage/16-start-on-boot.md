# Start on boot

## Linux

On most Linux distributions (including Ubuntu and Debian, but not OpenWrt), _systemd_ is in charge of managing services and starting them on boot.

Move the server executable and configuration in global folders:

```sh
sudo mv mediamtx /usr/local/bin/
sudo mv mediamtx.yml /usr/local/etc/
```

Create a _systemd_ service:

```sh
sudo tee /etc/systemd/system/mediamtx.service >/dev/null << EOF
[Unit]
After=network-online.target
Wants=network-online.target
[Service]
ExecStart=/usr/local/bin/mediamtx /usr/local/etc/mediamtx.yml
[Install]
WantedBy=multi-user.target
EOF
```

Enable a _wait-online_ service to make sure that _MediaMTX_ is started after network has been properly initialized:

```sh
sudo systemctl enable systemd-networkd-wait-online.service
```

If SELinux is enabled (for instance in case of RedHat, Rocky, CentOS++), add correct security context:

```sh
semanage fcontext -a -t bin_t /usr/local/bin/mediamtx
restorecon -Fv /usr/local/bin/mediamtx
```

Enable and start the service:

```sh
sudo systemctl daemon-reload
sudo systemctl enable mediamtx
sudo systemctl start mediamtx
```

## OpenWrt

Move the server executable and configuration in global folders:

```sh
mv mediamtx /usr/bin/
mkdir -p /usr/etc && mv mediamtx.yml /usr/etc/
```

Create a procd service:

```sh
tee /etc/init.d/mediamtx >/dev/null << EOF
#!/bin/sh /etc/rc.common
USE_PROCD=1
START=95
STOP=01
start_service() {
    procd_open_instance
    procd_set_param command /usr/bin/mediamtx
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
```

Enable and start the service:

```sh
chmod +x /etc/init.d/mediamtx
/etc/init.d/mediamtx enable
/etc/init.d/mediamtx start
```

Read the server logs:

```sh
logread
```

## Windows

Download the [WinSW v2 executable](https://github.com/winsw/winsw/releases) and place it into the same folder of `mediamtx.exe`.

In the same folder, create a file named `WinSW-x64.xml` with this content:

```xml
<service>
  <id>mediamtx</id>
  <name>mediamtx</name>
  <description></description>
  <executable>%BASE%/mediamtx.exe</executable>
</service>
```

Open a terminal, navigate to the folder and run:

```
WinSW-x64 install
```

The server is now installed as a system service and will start at boot time.
