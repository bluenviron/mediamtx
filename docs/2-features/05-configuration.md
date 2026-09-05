# Configuration

_MediaMTX_ can be configured through parameters that are listed and commented in the [configuration file](../5-references/1-configuration-file.md) (`mediamtx.yml`).

## Change the configuration

There are several ways to change configuration parameters:

1. Edit the configuration file.

   When installing the server as a [standalone binary](../1-kickoff/2-install.md#standalone-binary), the configuration file is included into the release bundle.

   When installing the server as a [Docker container](../1-kickoff/2-install.md#docker-container), you need to download the configuration file in a local folder, mount the folder and point `MediaMTX` to the file in the folder:

   ```sh
   mkdir config
   wget https://raw.githubusercontent.com/bluenviron/mediamtx/{version_tag}/mediamtx.yml -O config/mediamtx.yml
   docker run --rm -it \
   -p 8554:8554 \
   -p 1935:1935 \
   -p 8888:8888 \
   -p 8889:8889 \
   -p 8892:8892 \
   -p 8890:8890/udp \
   -p 8189:8189/udp \
   -p 8892:8892/udp \
   -p 8893:8893/udp \
   -v "$PWD/config:/config" \
   -w /config \
   bluenviron/mediamtx:1 ./mediamtx.yml
   ```

   You can edit the configuration file when the server is running (hot reloading). Changes are detected and applied without disconnecting existing clients, whenever it's possible.

2. Use environment variables, in the format `MTX_PARAMNAME`, where `PARAMNAME` is the uppercase name of a parameter. For instance, the `rtspAddress` parameter can be overridden in the following way:

   ```sh
   MTX_RTSPADDRESS="127.0.0.1:8554" ./mediamtx
   ```

   Parameters that have array as value can be overridden by setting a comma-separated list. For example:

   ```sh
   MTX_RTSPTRANSPORTS="tcp,udp"
   ```

   Parameters in maps can be overridden by using underscores, in the following way:

   ```sh
   MTX_PATHS_TEST_SOURCE=rtsp://myurl ./mediamtx
   ```

   Parameters in lists can be overridden in the same way as parameters in maps, using their position like an additional key. This is particularly useful if you want to use internal users but define credentials through environment variables:

   ```sh
   MTX_AUTHINTERNALUSERS_0_USER=username
   MTX_AUTHINTERNALUSERS_0_PASS=password
   ```

   This method is particularly useful when using Docker; any configuration parameter can be changed by passing environment variables with the `-e` flag:

   ```sh
   docker run --rm -it \
   --network=host \
   -e MTX_PATHS_TEST_SOURCE=rtsp://myurl \
   bluenviron/mediamtx:1
   ```

3. Use the [Control API](21-control-api.md).

## Validate the configuration

You can validate a configuration file without starting the server:

```sh
./mediamtx --validate-conf=/path/to/mediamtx.yml
```

## Encrypt the configuration

The configuration file can be entirely encrypted for security purposes by using the `crypto_secretbox` function of the NaCL library. An online tool for performing this operation is [available here](https://play.golang.org/p/rX29jwObNe4).

After performing the encryption, put the base64-encoded result into the configuration file, and launch the server with the `MTX_CONFKEY` variable:

```sh
MTX_CONFKEY=mykey ./mediamtx
```
