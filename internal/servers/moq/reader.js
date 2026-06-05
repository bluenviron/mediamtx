"use strict";

/**
 * @callback OnError
 * @param {string} err - error.
 */

/**
 * @callback OnSubscribed
 * @param {boolean} hasAudio - whether an audio track was subscribed.
 */

/**
 * @callback OnAudioMuted
 * @param {boolean} muted - whether the audio is muted.
 */

/**
 * @typedef Conf
 * @type {object}
 * @property {string} fingerprintUrl - URL to fetch the server certificate fingerprint.
 * @property {string} url - WebTransport URL to connect to.
 * @property {string} user - username.
 * @property {string} pass - password.
 * @property {string} token - token.
 * @property {HTMLElement} videoElement - element where the video canvas will be appended.
 * @property {OnError} onError - called when there's an error.
 * @property {OnSubscribed} onSubscribed - called when track subscription is successful.
 * @property {OnAudioMuted} onAudioMuted - called when audio is muted or unmuted.
 */

/** Media-over-QUIC reader. */
class MediaMTXMoQReader {
  static #RETRY_PAUSE = 2000;

  static #MOQT_VERSION = "moqt-18";

  static #SETUP_TYPE = 0x2f00n;
  static #MSG_SUBSCRIBE = 0x03n;
  static #MSG_SUBSCRIBE_OK = 0x04n;
  static #MSG_REQUEST_ERROR = 0x05n;
  static #SUBGROUP_TYPE_MASK = 0x90n;
  static #SUBGROUP_TYPE_BITS = 0x10n;
  static #SUBGROUP_PROPS_BIT = 0x01n;
  static #PARAM_AUTH_TOKEN = 0x03n;
  static #USE_VALUE = 3n;

  static #NAMESPACE = "stream";
  static #VIDEO_REQUEST_ID = BigInt(10);
  static #AUDIO_REQUEST_ID = BigInt(11);

  static #MAX_VIDEO_REORDERED_SUBGROUPS = 30;
  static #MAX_VIDEO_FRAMES_IN_DECODER = 10;

  static #MAX_AUDIO_REORDERED_SUBGROUPS = 50;
  static #MAX_AUDIO_FRAMES_IN_DECODER = 10;
  static #AUDIO_START_LATENCY_SECS = 0.0;
  static #AUDIO_MAX_LATENCY_SECS = 0.5;

  #conf;
  #state = "running";
  #restartTimeout = null;
  #wt = null;
  #fingerprint = null;
  #catalog = null;
  #uniStreamsQueue = [];
  #uniStreamsListeners = [];
  #videoTrack = null;
  #videoParams = null;
  #videoCanvas = null;
  #videoDecoder = null;
  #videoReorderer = null;
  #audioTrack = null;
  #audioCtx = null;
  #audioDecoder = null;
  #audioReorderer = null;

  /**
   * Create a MediaMTXMoQReader.
   * @param {Conf} conf - configuration.
   */
  constructor(conf) {
    this.#conf = conf;
    this.#start();
  }

  /**
   * Unmute the reader.
   */
  unmute() {
    if (this.#audioCtx !== null) {
      this.#audioCtx.resume().then(() => {
        if (this.#conf.onAudioMuted !== undefined) {
          this.#conf.onAudioMuted(false);
        }
      });
    }
  }

  #start() {
    this.#fetchFingerprint()
      .then(() => this.#connect())
      .then(() => this.#setup())
      .then(() => this.#subscribeCatalog())
      .then(() => this.#subscribeAllTracks())
      .then(() => this.#drainDataStreams())
      .catch((err) => this.#handleError(err.message));
  }

  /** @param {string} err */
  #handleError(err) {
    if (this.#state === "running") {
      this.#state = "restarting";

      if (this.#wt !== null) {
        this.#wt.close();
        this.#wt = null;
      }

      if (this.#videoDecoder !== null) {
        try {
          this.#videoDecoder.close();
        } catch (e) {}
        this.#videoDecoder = null;
      }

      if (this.#videoCanvas !== null) {
        this.#videoCanvas.remove();
        this.#videoCanvas = null;
      }

      if (this.#audioDecoder !== null) {
        try {
          this.#audioDecoder.close();
        } catch (e) {}
        this.#audioDecoder = null;
      }

      if (this.#audioCtx !== null) {
        this.#audioCtx.close();
        this.#audioCtx = null;
      }

      this.#uniStreamsQueue = [];
      for (const w of this.#uniStreamsListeners) {
        w.reject(new Error("restarting"));
      }
      this.#uniStreamsListeners = [];
      this.#videoTrack = null;
      this.#videoParams = null;
      this.#videoReorderer = null;
      this.#audioTrack = null;
      this.#audioReorderer = null;

      this.#restartTimeout = window.setTimeout(
        () => this.#restart(),
        MediaMTXMoQReader.#RETRY_PAUSE,
      );

      if (this.#conf.onAudioMuted !== undefined) {
        this.#conf.onAudioMuted(false);
      }

      if (this.#conf.onError !== undefined) {
        this.#conf.onError(`${err}, retrying in some seconds`);
      }
    }
  }

  #restart() {
    this.#restartTimeout = null;
    this.#state = "running";
    this.#start();
  }

  async #fetchFingerprint() {
    const hex = await fetch(this.#conf.fingerprintUrl, {
      headers: this.#authHeader(),
    }).then((r) => r.text());
    this.#fingerprint = new Uint8Array(hex.length / 2);
    for (let i = 0; i < this.#fingerprint.length; i++)
      this.#fingerprint[i] = parseInt(hex.slice(2 * i, 2 * i + 2), 16);
  }

  #authHeader() {
    if (this.#conf.user !== undefined && this.#conf.user !== "") {
      const credentials = btoa(`${this.#conf.user}:${this.#conf.pass}`);
      return { Authorization: `Basic ${credentials}` };
    }
    if (this.#conf.token !== undefined && this.#conf.token !== "") {
      return { Authorization: `Bearer ${this.#conf.token}` };
    }
    return {};
  }

  #encodeAuthParams() {
    let tokenValue = null;
    if (this.#conf.user !== undefined && this.#conf.user !== "") {
      const credentials = btoa(`${this.#conf.user}:${this.#conf.pass}`);
      tokenValue = new TextEncoder().encode(`Basic ${credentials}`);
    } else if (this.#conf.token !== undefined && this.#conf.token !== "") {
      tokenValue = new TextEncoder().encode(`Bearer ${this.#conf.token}`);
    }

    if (tokenValue === null) {
      return MediaMTXMoQReader.#encodeVarint(0);
    }

    const tokenStruct = MediaMTXMoQReader.#concat(
      MediaMTXMoQReader.#encodeVarint(MediaMTXMoQReader.#USE_VALUE),
      MediaMTXMoQReader.#encodeVarint(0), // Token Type (out-of-band)
      tokenValue,
    );

    const param = MediaMTXMoQReader.#concat(
      MediaMTXMoQReader.#encodeVarint(MediaMTXMoQReader.#PARAM_AUTH_TOKEN),
      MediaMTXMoQReader.#encodeVarint(tokenStruct.length),
      tokenStruct,
    );
    return MediaMTXMoQReader.#concat(MediaMTXMoQReader.#encodeVarint(1), param);
  }

  async #connect() {
    this.#wt = new WebTransport(this.#conf.url, {
      serverCertificateHashes: [
        { algorithm: "sha-256", value: this.#fingerprint.buffer },
      ],
      protocols: [MediaMTXMoQReader.#MOQT_VERSION],
    });
    await this.#wt.ready;
    console.log("connected");

    this.#wt.closed
      .then(() => this.#handleError("connection closed"))
      .catch((err) => this.#handleError(err.message));

    this.#acceptUniStreams().catch((err) => this.#handleError(err.message));
  }

  async #acceptUniStreams() {
    const reader = this.#wt.incomingUnidirectionalStreams.getReader();
    for (;;) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }
      if (this.#uniStreamsListeners.length > 0) {
        this.#uniStreamsListeners.shift().resolve(value);
      } else {
        this.#uniStreamsQueue.push(value);
      }
    }
  }

  #nextUni() {
    return new Promise((resolve, reject) => {
      if (this.#uniStreamsQueue.length > 0) {
        resolve(this.#uniStreamsQueue.shift());
      } else {
        this.#uniStreamsListeners.push({ resolve, reject });
      }
    });
  }

  async #setup() {
    const tx = await this.#wt.createUnidirectionalStream();
    const w = tx.getWriter();
    await w.write(
      MediaMTXMoQReader.#encodeVarint(MediaMTXMoQReader.#SETUP_TYPE),
    );
    await w.write(new Uint8Array([0x00, 0x00]));
    w.releaseLock();

    const rx = new MediaMTXMoQReader.#StreamReader(await this.#nextUni());
    const t = await rx.readVarint();
    if (t !== MediaMTXMoQReader.#SETUP_TYPE) {
      throw new Error("unexpected setup type 0x" + t.toString(16));
    }
    await rx.readBytes(await rx.readU16());
    console.log("setup ok");
  }

  async #subscribeCatalog() {
    const bidi = await this.#wt.createBidirectionalStream();
    const w = bidi.writable.getWriter();
    const r = new MediaMTXMoQReader.#StreamReader(bidi.readable);

    await w.write(
      MediaMTXMoQReader.#encodeMsg(
        MediaMTXMoQReader.#MSG_SUBSCRIBE,
        MediaMTXMoQReader.#concat(
          MediaMTXMoQReader.#encodeVarint(0),
          MediaMTXMoQReader.#encodeNamespace(MediaMTXMoQReader.#NAMESPACE),
          MediaMTXMoQReader.#encodeString(".catalog"),
          this.#encodeAuthParams(),
        ),
      ),
    );
    w.releaseLock();

    const t = await r.readVarint();
    switch (t) {
      case MediaMTXMoQReader.#MSG_REQUEST_ERROR: {
        const errPayload = await r.readBytes(await r.readU16());
        throw new Error(MediaMTXMoQReader.#requestErrorReason(errPayload));
      }
      case MediaMTXMoQReader.#MSG_SUBSCRIBE_OK:
        break;
      default:
        throw new Error("expected SUBSCRIBE_OK, got 0x" + t.toString(16));
    }
    await r.readBytes(await r.readU16());

    const { data } = await this.#readSubGroup(await this.#nextUni());
    this.#catalog = JSON.parse(new TextDecoder().decode(data));

    console.log("catalog:", this.#catalog);
  }

  static #readVarintFromBytes(bytes, offset) {
    const b = bytes[offset];
    if ((b & 0x80) === 0) return { value: BigInt(b), size: 1 };
    if ((b & 0xc0) === 0x80)
      return { value: BigInt(((b & 0x3f) << 8) | bytes[offset + 1]), size: 2 };
    if ((b & 0xe0) === 0xc0)
      return {
        value: BigInt(
          ((b & 0x1f) << 16) | (bytes[offset + 1] << 8) | bytes[offset + 2],
        ),
        size: 3,
      };
    if ((b & 0xf0) === 0xe0)
      return {
        value: BigInt(
          ((b & 0x0f) << 24) |
            (bytes[offset + 1] << 16) |
            (bytes[offset + 2] << 8) |
            bytes[offset + 3],
        ),
        size: 4,
      };
    if ((b & 0xf8) === 0xf0)
      return {
        value:
          (BigInt(b & 0x07) << 32n) |
          (BigInt(bytes[offset + 1]) << 24n) |
          (BigInt(bytes[offset + 2]) << 16n) |
          (BigInt(bytes[offset + 3]) << 8n) |
          BigInt(bytes[offset + 4]),
        size: 5,
      };
    if ((b & 0xfc) === 0xf8)
      return {
        value:
          (BigInt(b & 0x03) << 40n) |
          (BigInt(bytes[offset + 1]) << 32n) |
          (BigInt(bytes[offset + 2]) << 24n) |
          (BigInt(bytes[offset + 3]) << 16n) |
          (BigInt(bytes[offset + 4]) << 8n) |
          BigInt(bytes[offset + 5]),
        size: 6,
      };
    if ((b & 0xfe) === 0xfc)
      return {
        value:
          (BigInt(b & 0x01) << 48n) |
          (BigInt(bytes[offset + 1]) << 40n) |
          (BigInt(bytes[offset + 2]) << 32n) |
          (BigInt(bytes[offset + 3]) << 24n) |
          (BigInt(bytes[offset + 4]) << 16n) |
          (BigInt(bytes[offset + 5]) << 8n) |
          BigInt(bytes[offset + 6]),
        size: 7,
      };
    if (b === 0xfe)
      return {
        value:
          (BigInt(bytes[offset + 1]) << 48n) |
          (BigInt(bytes[offset + 2]) << 40n) |
          (BigInt(bytes[offset + 3]) << 32n) |
          (BigInt(bytes[offset + 4]) << 24n) |
          (BigInt(bytes[offset + 5]) << 16n) |
          (BigInt(bytes[offset + 6]) << 8n) |
          BigInt(bytes[offset + 7]),
        size: 8,
      };
    return {
      value:
        (BigInt(bytes[offset + 1]) << 56n) |
        (BigInt(bytes[offset + 2]) << 48n) |
        (BigInt(bytes[offset + 3]) << 40n) |
        (BigInt(bytes[offset + 4]) << 32n) |
        (BigInt(bytes[offset + 5]) << 24n) |
        (BigInt(bytes[offset + 6]) << 16n) |
        (BigInt(bytes[offset + 7]) << 8n) |
        BigInt(bytes[offset + 8]),
      size: 9,
    };
  }

  static #requestErrorReason(payload) {
    let off = 0;
    let v = MediaMTXMoQReader.#readVarintFromBytes(payload, off);
    off += v.size;
    v = MediaMTXMoQReader.#readVarintFromBytes(payload, off);
    off += v.size;
    v = MediaMTXMoQReader.#readVarintFromBytes(payload, off);
    off += v.size;
    return new TextDecoder().decode(payload.slice(off, off + Number(v.value)));
  }

  async #readSubGroup(readable) {
    const r = new MediaMTXMoQReader.#StreamReader(readable);
    const streamType = await r.readVarint();
    if (
      (streamType & MediaMTXMoQReader.#SUBGROUP_TYPE_MASK) !==
      MediaMTXMoQReader.#SUBGROUP_TYPE_BITS
    ) {
      throw new Error("not a subgroup stream");
    }
    const withProps =
      (streamType & MediaMTXMoQReader.#SUBGROUP_PROPS_BIT) !== 0n;

    const trackAlias = await r.readVarint();
    const groupId = await r.readVarint();

    await r.readVarint(); // idDelta
    if (withProps) {
      const propsLen = await r.readVarint();
      if (propsLen > 0n) await r.readBytes(Number(propsLen));
    }
    const len = await r.readVarint();
    if (len === 0n) {
      throw new Error("first object has zero length");
    }
    const data = await r.readBytes(Number(len));

    await r.readVarint(); // idDelta
    if (withProps) {
      const propsLen = await r.readVarint();
      if (propsLen > 0n) await r.readBytes(Number(propsLen));
    }
    const endLen = await r.readVarint();
    if (endLen !== 0n) {
      throw new Error("end chunk has non-zero length");
    }

    return { data, trackAlias, groupId };
  }

  async #subscribeAllTracks() {
    const promises = [];

    for (let i = 0; i < this.#catalog.tracks.length; i++) {
      const track = this.#catalog.tracks[i];

      if (/^(avc3|hev1|av01|vp09|vp8)/.test(track.codec)) {
        if (this.#videoTrack === null) {
          this.#videoTrack = track;
          promises.push(
            this.#subscribeTrack(MediaMTXMoQReader.#VIDEO_REQUEST_ID, track),
          );
        }
      } else if (/^(opus|mp4a|flac|pcm)/.test(track.codec)) {
        if (this.#audioTrack === null) {
          this.#audioTrack = track;
          promises.push(
            this.#subscribeTrack(MediaMTXMoQReader.#AUDIO_REQUEST_ID, track),
          );
        }
      } else {
        throw new Error("unsupported codec: " + track.codec);
      }
    }

    await Promise.all(promises);

    if (this.#conf.onSubscribed !== undefined) {
      this.#conf.onSubscribed(this.#audioTrack !== null);
    }
  }

  async #subscribeTrack(requestId, track) {
    const bidi = await this.#wt.createBidirectionalStream();
    const w = bidi.writable.getWriter();
    const r = new MediaMTXMoQReader.#StreamReader(bidi.readable);

    await w.write(
      MediaMTXMoQReader.#encodeMsg(
        MediaMTXMoQReader.#MSG_SUBSCRIBE,
        MediaMTXMoQReader.#concat(
          MediaMTXMoQReader.#encodeVarint(requestId),
          MediaMTXMoQReader.#encodeNamespace(MediaMTXMoQReader.#NAMESPACE),
          MediaMTXMoQReader.#encodeString(track.name),
          this.#encodeAuthParams(),
        ),
      ),
    );
    w.releaseLock();

    const t = await r.readVarint();
    if (t !== MediaMTXMoQReader.#MSG_SUBSCRIBE_OK) {
      throw new Error("expected SUBSCRIBE_OK, got 0x" + t.toString(16));
    }
    await r.readBytes(await r.readU16());

    if (requestId === MediaMTXMoQReader.#VIDEO_REQUEST_ID) {
      this.#videoCanvas = document.createElement("canvas");
      this.#conf.videoElement.appendChild(this.#videoCanvas);

      const ctxGL = this.#videoCanvas.getContext("webgl2", {
        desynchronized: true,
        powerPreference: "high-performance",
      });

      let ctx2D = null;
      if (ctxGL === null) {
        console.log("video renderer: 2D canvas");

        ctx2D = this.#videoCanvas.getContext("2d");
      } else {
        console.log("video renderer: WebGL2");

        const vert = ctxGL.createShader(ctxGL.VERTEX_SHADER);
        ctxGL.shaderSource(
          vert,
          `#version 300 es
          in vec2 a_pos;
          out vec2 v_uv;
          void main() {
            v_uv = vec2(a_pos.x * 0.5 + 0.5, 0.5 - a_pos.y * 0.5);
            gl_Position = vec4(a_pos, 0, 1);
          }
        `,
        );
        ctxGL.compileShader(vert);

        const frag = ctxGL.createShader(ctxGL.FRAGMENT_SHADER);
        ctxGL.shaderSource(
          frag,
          `#version 300 es
          precision mediump float;
          uniform sampler2D u_tex;
          in vec2 v_uv;
          out vec4 color;
          void main() { color = texture(u_tex, v_uv); }
        `,
        );
        ctxGL.compileShader(frag);

        const prog = ctxGL.createProgram();
        ctxGL.attachShader(prog, vert);
        ctxGL.attachShader(prog, frag);
        ctxGL.linkProgram(prog);
        ctxGL.useProgram(prog);

        const buf = ctxGL.createBuffer();
        ctxGL.bindBuffer(ctxGL.ARRAY_BUFFER, buf);
        ctxGL.bufferData(
          ctxGL.ARRAY_BUFFER,
          new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]),
          ctxGL.STATIC_DRAW,
        );
        const loc = ctxGL.getAttribLocation(prog, "a_pos");
        ctxGL.enableVertexAttribArray(loc);
        ctxGL.vertexAttribPointer(loc, 2, ctxGL.FLOAT, false, 0, 0);

        const tex = ctxGL.createTexture();
        ctxGL.bindTexture(ctxGL.TEXTURE_2D, tex);
        ctxGL.texParameteri(
          ctxGL.TEXTURE_2D,
          ctxGL.TEXTURE_MIN_FILTER,
          ctxGL.LINEAR,
        );
        ctxGL.texParameteri(
          ctxGL.TEXTURE_2D,
          ctxGL.TEXTURE_MAG_FILTER,
          ctxGL.LINEAR,
        );
        ctxGL.texParameteri(
          ctxGL.TEXTURE_2D,
          ctxGL.TEXTURE_WRAP_S,
          ctxGL.CLAMP_TO_EDGE,
        );
        ctxGL.texParameteri(
          ctxGL.TEXTURE_2D,
          ctxGL.TEXTURE_WRAP_T,
          ctxGL.CLAMP_TO_EDGE,
        );
        ctxGL.uniform1i(ctxGL.getUniformLocation(prog, "u_tex"), 0);
      }

      this.#videoDecoder = new VideoDecoder({
        output: (frame) => {
          if (
            this.#videoCanvas.width !== frame.displayWidth ||
            this.#videoCanvas.height !== frame.displayHeight
          ) {
            this.#videoCanvas.width = frame.displayWidth;
            this.#videoCanvas.height = frame.displayHeight;

            if (ctxGL !== null) {
              ctxGL.viewport(
                0,
                0,
                this.#videoCanvas.width,
                this.#videoCanvas.height,
              );
            }
          }

          if (ctxGL !== null) {
            ctxGL.texImage2D(
              ctxGL.TEXTURE_2D,
              0,
              ctxGL.RGBA,
              ctxGL.RGBA,
              ctxGL.UNSIGNED_BYTE,
              frame,
            );
            ctxGL.drawArrays(ctxGL.TRIANGLE_STRIP, 0, 4);
          } else {
            ctx2D.drawImage(frame, 0, 0);
          }

          frame.close();
        },
        error: (err) => console.error(err.message),
      });

      const config = {
        codec: track.codec,
        optimizeForLatency: true,
      };

      const supported = await VideoDecoder.isConfigSupported(config);
      if (!supported.supported) {
        throw new Error(
          "the browser you are using does not support video codec " +
            track.codec,
        );
      }

      this.#videoDecoder.configure(config);

      this.#videoReorderer = new MediaMTXMoQReader.#Reorderer(
        MediaMTXMoQReader.#MAX_VIDEO_REORDERED_SUBGROUPS,
      );
    } else {
      this.#audioCtx = new AudioContext();

      if (
        this.#audioCtx.state === "suspended" &&
        this.#conf.onAudioMuted !== undefined
      ) {
        this.#conf.onAudioMuted(true);
      }

      let playbackTime = null;

      this.#audioDecoder = new AudioDecoder({
        output: async (data) => {
          try {
            if (this.#audioCtx.state === "running") {
              const buf = this.#audioCtx.createBuffer(
                data.numberOfChannels,
                data.numberOfFrames,
                data.sampleRate,
              );

              for (let ch = 0; ch < data.numberOfChannels; ch++) {
                data.copyTo(buf.getChannelData(ch), {
                  planeIndex: ch,
                  format: "f32-planar",
                });
              }

              const src = this.#audioCtx.createBufferSource();
              src.buffer = buf;
              src.connect(this.#audioCtx.destination);

              if (
                playbackTime === null ||
                playbackTime < this.#audioCtx.currentTime ||
                playbackTime >
                  this.#audioCtx.currentTime +
                    MediaMTXMoQReader.#AUDIO_MAX_LATENCY_SECS
              ) {
                if (playbackTime !== null) {
                  console.info(
                    "audio desync detected (currentTime=" +
                      this.#audioCtx.currentTime.toFixed(3) +
                      " playbackTime=" +
                      playbackTime.toFixed(3) +
                      ") - resetting playback time",
                  );
                }
                playbackTime =
                  this.#audioCtx.currentTime +
                  MediaMTXMoQReader.#AUDIO_START_LATENCY_SECS;
              }

              src.start(playbackTime);

              playbackTime += buf.duration;
            }
          } finally {
            data.close();
          }
        },
        error: (err) => console.error(err.message),
      });

      const config = {
        codec: track.codec,
        sampleRate: track.samplerate,
        numberOfChannels: track.channels,
      };
      if (track.initData) {
        config.description = MediaMTXMoQReader.#base64ToBuffer(track.initData);
      }

      this.#audioDecoder.configure(config);

      this.#audioReorderer = new MediaMTXMoQReader.#Reorderer(
        MediaMTXMoQReader.#MAX_AUDIO_REORDERED_SUBGROUPS,
      );
    }

    console.log("subscribed track " + track.name + " (" + track.codec + ")");
  }

  async #drainDataStreams() {
    for (;;) {
      const stream = await this.#nextUni();
      this.#onDataTrack(stream).catch((err) => this.#handleError(err.message));
    }
  }

  async #onDataTrack(readable) {
    const { data, trackAlias, groupId } = await this.#readSubGroup(readable);
    if (data.length === 0) {
      return;
    }

    if (trackAlias === MediaMTXMoQReader.#VIDEO_REQUEST_ID) {
      const sgs = this.#videoReorderer.push(data, groupId);

      for (const sg of sgs) {
        this.#decodeVideo(sg.data, sg.groupId);
      }
    } else {
      const sgs = this.#audioReorderer.push(data, groupId);

      for (const sg of sgs) {
        this.#decodeAudio(sg.data, sg.groupId);
      }
    }
  }

  #decodeVideo(data, groupId) {
    // this happens when the screen is off
    if (
      this.#videoDecoder.decodeQueueSize >=
      MediaMTXMoQReader.#MAX_VIDEO_FRAMES_IN_DECODER
    ) {
      console.log("skipping video frame, decode queue is full");
      return;
    }

    if (/^(avc3)/.test(this.#videoTrack.codec)) {
      let sps = null;
      let pps = null;
      for (const nalu of MediaMTXMoQReader.#splitAVCC(data)) {
        const naluType = nalu[0] & 0x1f;
        if (naluType === 7) {
          sps = nalu;
        } else if (naluType === 8) {
          pps = nalu;
        }
      }

      if (
        sps !== null &&
        pps !== null &&
        (this.#videoParams === null ||
          !MediaMTXMoQReader.#bytesEqual(this.#videoParams.sps, sps) ||
          !MediaMTXMoQReader.#bytesEqual(this.#videoParams.pps, pps))
      ) {
        this.#videoParams = { sps, pps };
        this.#videoDecoder.configure({
          codec: this.#videoTrack.codec,
          optimizeForLatency: true,
          description: MediaMTXMoQReader.#makeAvcC(sps, pps),
        });
        console.log("video params updated");
      }
    } else if (/^(hev1)/.test(this.#videoTrack.codec)) {
      let vps = null;
      let sps = null;
      let pps = null;
      for (const nalu of MediaMTXMoQReader.#splitAVCC(data)) {
        const naluType = (nalu[0] >> 1) & 0x3f;
        if (naluType === 32) {
          vps = nalu;
        } else if (naluType === 33) {
          sps = nalu;
        } else if (naluType === 34) {
          pps = nalu;
        }
      }

      if (
        vps !== null &&
        sps !== null &&
        pps !== null &&
        (this.#videoParams === null ||
          !MediaMTXMoQReader.#bytesEqual(this.#videoParams.vps, vps) ||
          !MediaMTXMoQReader.#bytesEqual(this.#videoParams.sps, sps) ||
          !MediaMTXMoQReader.#bytesEqual(this.#videoParams.pps, pps))
      ) {
        this.#videoParams = { vps, sps, pps };
        this.#videoDecoder.configure({
          codec: this.#videoTrack.codec,
          optimizeForLatency: true,
          description: MediaMTXMoQReader.#makeHvcC(vps, sps, pps),
        });
        console.log("video params updated");
      }
    }

    const timestamp = performance.now() * 1000;
    this.#videoDecoder.decode(
      new EncodedVideoChunk({ type: "key", timestamp, data }),
    );
  }

  #decodeAudio(data, groupId) {
    // this happens when the screen is off
    if (
      this.#audioDecoder.decodeQueueSize >=
      MediaMTXMoQReader.#MAX_AUDIO_FRAMES_IN_DECODER
    ) {
      console.log("skipping audio frame, decode queue is full");
      return;
    }

    const timestamp = Number(groupId);
    this.#audioDecoder.decode(
      new EncodedAudioChunk({ type: "key", timestamp, data }),
    );
  }

  static #makeAvcC(sps, pps) {
    const avcC = new Uint8Array(11 + sps.length + pps.length);
    let off = 0;
    avcC[off++] = 0x01; // configurationVersion
    avcC[off++] = sps[1]; // AVCProfileIndication
    avcC[off++] = sps[2]; // profile_compatibility
    avcC[off++] = sps[3]; // AVCLevelIndication
    avcC[off++] = 0xff; // reserved + lengthSizeMinusOne=3
    avcC[off++] = 0xe1; // reserved + numSPS=1
    avcC[off++] = (sps.length >> 8) & 0xff;
    avcC[off++] = sps.length & 0xff;
    avcC.set(sps, off);
    off += sps.length;
    avcC[off++] = 0x01; // numPPS=1
    avcC[off++] = (pps.length >> 8) & 0xff;
    avcC[off++] = pps.length & 0xff;
    avcC.set(pps, off);
    return avcC;
  }

  static #makeHvcC(vps, sps, pps) {
    // Extract profile/level from SPS NALU:
    //   bytes 0-1: NALU header
    //   byte 2: sps_vps_id(4) | sps_max_sub_layers_minus1(3) | sps_temporal_id_nesting_flag(1)
    //   bytes 3-14: profile_tier_level general portion (12 bytes)
    const numTemporalLayers = ((sps[2] >> 1) & 0x7) + 1;
    const temporalIdNested = sps[2] & 0x1;
    const arrays = [
      { type: 32, nalu: vps }, // VPS
      { type: 33, nalu: sps }, // SPS
      { type: 34, nalu: pps }, // PPS
    ];
    const totalSize = 23 + arrays.reduce((n, a) => n + 5 + a.nalu.length, 0);
    const hvcC = new Uint8Array(totalSize);
    let off = 0;
    hvcC[off++] = 0x01; // configurationVersion
    hvcC[off++] = sps[3]; // general_profile_space(2)|general_tier_flag(1)|general_profile_idc(5)
    hvcC[off++] = sps[4]; // general_profile_compatibility_flags
    hvcC[off++] = sps[5];
    hvcC[off++] = sps[6];
    hvcC[off++] = sps[7];
    hvcC[off++] = sps[8]; // general_constraint_indicator_flags
    hvcC[off++] = sps[9];
    hvcC[off++] = sps[10];
    hvcC[off++] = sps[11];
    hvcC[off++] = sps[12];
    hvcC[off++] = sps[13];
    hvcC[off++] = sps[14]; // general_level_idc
    hvcC[off++] = 0xf0; // reserved(4) | min_spatial_segmentation_idc[11:8]
    hvcC[off++] = 0x00; // min_spatial_segmentation_idc[7:0]
    hvcC[off++] = 0xfc; // reserved(6) | parallelismType(2)
    hvcC[off++] = 0xfd; // reserved(6) | chromaFormat=1 (4:2:0)
    hvcC[off++] = 0xf8; // reserved(5) | bitDepthLumaMinus8=0
    hvcC[off++] = 0xf8; // reserved(5) | bitDepthChromaMinus8=0
    hvcC[off++] = 0x00; // avgFrameRate
    hvcC[off++] = 0x00;
    // constantFrameRate(2)=0 | numTemporalLayers(3) | temporalIdNested(1) | lengthSizeMinusOne(2)=3
    hvcC[off++] =
      ((numTemporalLayers & 0x7) << 3) | ((temporalIdNested & 0x1) << 2) | 0x03;
    hvcC[off++] = arrays.length; // numOfArrays
    for (const { type, nalu } of arrays) {
      hvcC[off++] = 0x80 | (type & 0x3f); // array_completeness=1 | reserved | NAL_unit_type
      hvcC[off++] = 0x00; // numNalus high
      hvcC[off++] = 0x01; // numNalus low = 1
      hvcC[off++] = (nalu.length >> 8) & 0xff;
      hvcC[off++] = nalu.length & 0xff;
      hvcC.set(nalu, off);
      off += nalu.length;
    }
    return hvcC;
  }

  static #encodeVarint(value) {
    const v = BigInt(value);
    if (v < 128n) {
      return new Uint8Array([Number(v)]);
    }
    if (v < 16384n) {
      return new Uint8Array([0x80 | Number(v >> 8n), Number(v & 0xffn)]);
    }
    if (v < 2097152n) {
      return new Uint8Array([
        0xc0 | Number(v >> 16n),
        Number((v >> 8n) & 0xffn),
        Number(v & 0xffn),
      ]);
    }
    if (v < 268435456n) {
      return new Uint8Array([
        0xe0 | Number(v >> 24n),
        Number((v >> 16n) & 0xffn),
        Number((v >> 8n) & 0xffn),
        Number(v & 0xffn),
      ]);
    }
    if (v < 34359738368n) {
      return new Uint8Array([
        0xf0 | Number(v >> 32n),
        Number((v >> 24n) & 0xffn),
        Number((v >> 16n) & 0xffn),
        Number((v >> 8n) & 0xffn),
        Number(v & 0xffn),
      ]);
    }
    if (v < 4398046511104n) {
      return new Uint8Array([
        0xf8 | Number(v >> 40n),
        Number((v >> 32n) & 0xffn),
        Number((v >> 24n) & 0xffn),
        Number((v >> 16n) & 0xffn),
        Number((v >> 8n) & 0xffn),
        Number(v & 0xffn),
      ]);
    }
    if (v < 562949953421312n) {
      return new Uint8Array([
        0xfc | Number(v >> 48n),
        Number((v >> 40n) & 0xffn),
        Number((v >> 32n) & 0xffn),
        Number((v >> 24n) & 0xffn),
        Number((v >> 16n) & 0xffn),
        Number((v >> 8n) & 0xffn),
        Number(v & 0xffn),
      ]);
    }
    if (v < 72057594037927936n) {
      return new Uint8Array([
        0xfe,
        Number((v >> 48n) & 0xffn),
        Number((v >> 40n) & 0xffn),
        Number((v >> 32n) & 0xffn),
        Number((v >> 24n) & 0xffn),
        Number((v >> 16n) & 0xffn),
        Number((v >> 8n) & 0xffn),
        Number(v & 0xffn),
      ]);
    }
    return new Uint8Array([
      0xff,
      Number((v >> 56n) & 0xffn),
      Number((v >> 48n) & 0xffn),
      Number((v >> 40n) & 0xffn),
      Number((v >> 32n) & 0xffn),
      Number((v >> 24n) & 0xffn),
      Number((v >> 16n) & 0xffn),
      Number((v >> 8n) & 0xffn),
      Number(v & 0xffn),
    ]);
  }

  static #concat(...arrays) {
    const total = arrays.reduce((n, a) => n + a.length, 0);
    const out = new Uint8Array(total);
    let pos = 0;
    for (const a of arrays) {
      out.set(a, pos);
      pos += a.length;
    }
    return out;
  }

  static #encodeString(s) {
    const b = new TextEncoder().encode(s);
    return MediaMTXMoQReader.#concat(
      MediaMTXMoQReader.#encodeVarint(b.length),
      b,
    );
  }

  static #encodeNamespace(parts) {
    let b = MediaMTXMoQReader.#encodeVarint(parts.length);
    for (const p of parts)
      b = MediaMTXMoQReader.#concat(b, MediaMTXMoQReader.#encodeString(p));
    return b;
  }

  static #encodeMsg(type, payload) {
    return MediaMTXMoQReader.#concat(
      MediaMTXMoQReader.#encodeVarint(type),
      new Uint8Array([(payload.length >> 8) & 0xff, payload.length & 0xff]),
      payload,
    );
  }

  static #splitAVCC(data) {
    const nalus = [];
    let i = 0;
    while (i + 4 <= data.length) {
      const len =
        (data[i] << 24) |
        (data[i + 1] << 16) |
        (data[i + 2] << 8) |
        data[i + 3];
      i += 4;
      if (len <= 0 || i + len > data.length) break;
      nalus.push(data.slice(i, i + len));
      i += len;
    }
    return nalus;
  }

  static #base64ToBuffer(b64) {
    const bin = atob(b64);
    const buf = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) buf[i] = bin.charCodeAt(i);
    return buf;
  }

  static #bytesEqual(a, b) {
    if (a.length !== b.length) {
      return false;
    }
    for (let i = 0; i < a.length; i++) {
      if (a[i] !== b[i]) {
        return false;
      }
    }
    return true;
  }

  static #Reorderer = class {
    #maxReorderered;
    #curGroupId = null;
    #pending = new Map();

    constructor(maxReorderered) {
      this.#maxReorderered = maxReorderered;
    }

    push(data, groupId) {
      if (this.#curGroupId === null) {
        this.#curGroupId = groupId;
        return [{ data, groupId }];
      } else if (groupId <= this.#curGroupId) {
        console.log("skipping out-of-order subgroup");
      } else if (
        groupId === this.#curGroupId + 1n &&
        this.#pending.size === 0
      ) {
        this.#curGroupId = groupId;
        return [{ data, groupId }];
      } else {
        this.#pending.set(groupId, data);

        const diff = groupId - this.#curGroupId;

        let countInRange = 0n;
        for (const id of this.#pending.keys()) {
          if (id <= groupId) {
            countInRange++;
          }
        }

        if (countInRange === diff) {
          return this.#flushUpTo(groupId);
        } else if (this.#pending.size > this.#maxReorderered) {
          console.log("too many reordered subgroups, flushing");
          return this.#flushUpTo(groupId);
        }
      }
      return [];
    }

    // Flushes pending items from curGroupId+1 through maxGroupId (skipping
    // gaps), advances curGroupId, then drains any further consecutive items
    // already in pending so they are not stranded.
    #flushUpTo(maxGroupId) {
      const out = [];
      for (let i = this.#curGroupId + 1n; i <= maxGroupId; i++) {
        const d = this.#pending.get(i);
        if (d !== undefined) {
          out.push({ data: d, groupId: i });
          this.#pending.delete(i);
        }
      }
      this.#curGroupId = maxGroupId;
      for (;;) {
        const next = this.#pending.get(this.#curGroupId + 1n);
        if (next === undefined) break;
        this.#curGroupId += 1n;
        out.push({ data: next, groupId: this.#curGroupId });
        this.#pending.delete(this.#curGroupId);
      }
      return out;
    }
  };

  static #StreamReader = class {
    #reader = null;
    #buf = new Uint8Array(0);

    constructor(src) {
      this.#reader = src.getReader();
    }

    async #fill() {
      if (!this.#reader) {
        throw new Error("stream ended");
      }
      const { value, done } = await this.#reader.read();
      if (done) {
        throw new Error("stream ended");
      }
      const next = new Uint8Array(this.#buf.length + value.length);
      next.set(this.#buf);
      next.set(value, this.#buf.length);
      this.#buf = next;
    }

    async readBytes(n) {
      while (this.#buf.length < n) await this.#fill();
      const out = this.#buf.slice(0, n);
      this.#buf = this.#buf.slice(n);
      return out;
    }

    async readVarint() {
      while (this.#buf.length < 1) await this.#fill();
      const b = this.#buf[0];
      let size;
      if ((b & 0x80) === 0) {
        size = 1;
      } else if ((b & 0xc0) === 0x80) {
        size = 2;
      } else if ((b & 0xe0) === 0xc0) {
        size = 3;
      } else if ((b & 0xf0) === 0xe0) {
        size = 4;
      } else if ((b & 0xf8) === 0xf0) {
        size = 5;
      } else if ((b & 0xfc) === 0xf8) {
        size = 6;
      } else if ((b & 0xfe) === 0xfc) {
        size = 7;
      } else if (b === 0xfe) {
        size = 8;
      } else {
        size = 9; // b === 0xff
      }
      const d = await this.readBytes(size);
      switch (size) {
        case 1:
          return BigInt(d[0]);
        case 2:
          return BigInt(((d[0] & 0x3f) << 8) | d[1]);
        case 3:
          return BigInt(((d[0] & 0x1f) << 16) | (d[1] << 8) | d[2]);
        case 4:
          return BigInt(
            ((d[0] & 0x0f) << 24) | (d[1] << 16) | (d[2] << 8) | d[3],
          );
        case 5:
          return (
            (BigInt(d[0] & 0x07) << 32n) |
            (BigInt(d[1]) << 24n) |
            (BigInt(d[2]) << 16n) |
            (BigInt(d[3]) << 8n) |
            BigInt(d[4])
          );
        case 6:
          return (
            (BigInt(d[0] & 0x03) << 40n) |
            (BigInt(d[1]) << 32n) |
            (BigInt(d[2]) << 24n) |
            (BigInt(d[3]) << 16n) |
            (BigInt(d[4]) << 8n) |
            BigInt(d[5])
          );
        case 7:
          return (
            (BigInt(d[0] & 0x01) << 48n) |
            (BigInt(d[1]) << 40n) |
            (BigInt(d[2]) << 32n) |
            (BigInt(d[3]) << 24n) |
            (BigInt(d[4]) << 16n) |
            (BigInt(d[5]) << 8n) |
            BigInt(d[6])
          );
        case 8:
          return (
            (BigInt(d[1]) << 48n) |
            (BigInt(d[2]) << 40n) |
            (BigInt(d[3]) << 32n) |
            (BigInt(d[4]) << 24n) |
            (BigInt(d[5]) << 16n) |
            (BigInt(d[6]) << 8n) |
            BigInt(d[7])
          );
        default: // 9
          return (
            (BigInt(d[1]) << 56n) |
            (BigInt(d[2]) << 48n) |
            (BigInt(d[3]) << 40n) |
            (BigInt(d[4]) << 32n) |
            (BigInt(d[5]) << 24n) |
            (BigInt(d[6]) << 16n) |
            (BigInt(d[7]) << 8n) |
            BigInt(d[8])
          );
      }
    }

    async readU16() {
      const d = await this.readBytes(2);
      return (d[0] << 8) | d[1];
    }
  };
}

window.MediaMTXMoQReader = MediaMTXMoQReader;
