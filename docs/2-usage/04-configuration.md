# Configuration

All the configuration parameters are listed and commented in the [configuration file](/docs/references/configuration-file) (`mediamtx.yml`).

There are several ways to change the configuration:

1. By editing the configuration file, that is
   - included into the release bundle
   - available in the root folder of the Docker image (`/mediamtx.yml`); it can be overridden in this way:

     ```sh
     docker run --rm -it --network=host -v "$PWD/mediamtx.yml:/mediamtx.yml:ro" bluenviron/mediamtx
     ```

   The configuration can be changed dynamically when the server is running (hot reloading) by writing to the configuration file. Changes are detected and applied without disconnecting existing clients, whenever it's possible.

2. By overriding configuration parameters with environment variables, in the format `MTX_PARAMNAME`, where `PARAMNAME` is the uppercase name of a parameter. For instance, the `rtspAddress` parameter can be overridden in the following way:

   ```
   MTX_RTSPADDRESS="127.0.0.1:8554" ./mediamtx
   ```

   Parameters that have array as value can be overridden by setting a comma-separated list. For example:

   ```
   MTX_RTSPTRANSPORTS="tcp,udp"
   ```

   Parameters in maps can be overridden by using underscores, in the following way:

   ```
   MTX_PATHS_TEST_SOURCE=rtsp://myurl ./mediamtx
   ```

   This method is particularly useful when using Docker; any configuration parameter can be changed by passing environment variables with the `-e` flag:

   ```
   docker run --rm -it --network=host -e MTX_PATHS_TEST_SOURCE=rtsp://myurl bluenviron/mediamtx
   ```

3. By using the [Control API](control-api).
