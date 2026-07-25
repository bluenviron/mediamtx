"use strict";

/**
 * @callback OnError
 * @param {string} err - error.
 */

/**
 * @callback OnConnected
 */

/**
 * @typedef Conf
 * @type {object}
 * @property {string} fingerprintUrl - URL to fetch the server certificate fingerprint.
 * @property {string} url - WebTransport URL to connect to.
 * @property {string} user - username.
 * @property {string} pass - password.
 * @property {string} token - token.
 * @property {MediaStream} stream - media stream to publish.
 * @property {string} videoCodec - video codec string (e.g., "avc3.640028").
 * @property {number} videoBitrate - video bitrate in kbps.
 * @property {number} videoFramerate - video framerate.
 * @property {number} videoKeyframeInterval - video keyframe interval in seconds.
 * @property {number} videoWidth - video width.
 * @property {number} videoHeight - video height.
 * @property {string} audioCodec - audio codec string (e.g., "opus").
 * @property {number} audioBitrate - audio bitrate in kbps.
 * @property {OnError} onError - called when there's an error.
 * @property {OnConnected} onConnected - called when publishing has started.
 */

/** Media-over-QUIC publisher. */
class MediaMTXMoQPublisher {
  static #RETRY_PAUSE = 2000;

  static #MOQT_VERSION = "moqt-19";

  static #SETUP_TYPE = 0x2f00n;
  static #MSG_PUBLISH = 0x1dn;
  static #MSG_REQUEST_OK = 0x07n;
  static #MSG_REQUEST_ERROR = 0x05n;
  static #SUBGROUP_TYPE = 0x30n;
  static #SUBGROUP_TYPE_WITH_PROPS = 0x31n;
  static #PARAM_AUTH_TOKEN = 0x03n;
  static #USE_VALUE = 3n;

  static #NAMESPACE = "stream";

  #conf;
  #state = "running";
  #restartTimeout = null;
  #wt = null;
  #fingerprint = null;
  #uniStreamsQueue = [];
  #uniStreamsListeners = [];
  #videoEncoder = null;
  #audioEncoder = null;

  /**
   * Create a MediaMTXMoQPublisher.
   * @param {Conf} conf - configuration.
   */
  constructor(conf) {
    this.#conf = conf;
    this.#start();
  }

  /**
   * Close the publisher and all its resources.
   */
  close() {
    this.#state = "closed";
    this.#cleanup();
    if (this.#restartTimeout !== null) {
      clearTimeout(this.#restartTimeout);
    }
  }

  #cleanup() {
    if (this.#wt !== null) {
      this.#wt.close();
      this.#wt = null;
    }

    if (this.#videoEncoder !== null) {
      try {
        this.#videoEncoder.close();
      } catch (e) {}
      this.#videoEncoder = null;
    }

    if (this.#audioEncoder !== null) {
      try {
        this.#audioEncoder.close();
      } catch (e) {}
      this.#audioEncoder = null;
    }

    this.#uniStreamsQueue = [];
    for (const w of this.#uniStreamsListeners) {
      w.reject(new Error("restarting"));
    }
    this.#uniStreamsListeners = [];
  }

  #handleError(err) {
    if (this.#state === "running") {
      this.#state = "restarting";

      this.#cleanup();

      this.#restartTimeout = window.setTimeout(
        () => this.#restart(),
        MediaMTXMoQPublisher.#RETRY_PAUSE,
      );

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

  #start() {
    this.#fetchFingerprint()
      .then(() => this.#connect())
      .then(() => this.#setup())
      .then(() => this.#publishAllTracks())
      .catch((err) => this.#handleError(err.message));
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
      return MediaMTXMoQPublisher.#encodeVarint(0);
    }

    const tokenStruct = MediaMTXMoQPublisher.#concat(
      MediaMTXMoQPublisher.#encodeVarint(MediaMTXMoQPublisher.#USE_VALUE),
      MediaMTXMoQPublisher.#encodeVarint(0), // Token Type (out-of-band)
      tokenValue,
    );

    const param = MediaMTXMoQPublisher.#concat(
      MediaMTXMoQPublisher.#encodeVarint(
        MediaMTXMoQPublisher.#PARAM_AUTH_TOKEN,
      ),
      MediaMTXMoQPublisher.#encodeVarint(tokenStruct.length),
      tokenStruct,
    );
    return MediaMTXMoQPublisher.#concat(
      MediaMTXMoQPublisher.#encodeVarint(1),
      param,
    );
  }

  async #connect() {
    this.#wt = new WebTransport(this.#conf.url, {
      serverCertificateHashes: [
        { algorithm: "sha-256", value: this.#fingerprint.buffer },
      ],
      protocols: [MediaMTXMoQPublisher.#MOQT_VERSION],
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
      MediaMTXMoQPublisher.#encodeVarint(MediaMTXMoQPublisher.#SETUP_TYPE),
    );
    await w.write(new Uint8Array([0x00, 0x00]));
    w.releaseLock();

    const rx = new MediaMTXMoQPublisher.#StreamReader(await this.#nextUni());
    const t = await rx.readVarint();
    if (t !== MediaMTXMoQPublisher.#SETUP_TYPE) {
      throw new Error("unexpected setup type 0x" + t.toString(16));
    }
    await rx.readBytes(await rx.readU16());
    console.log("setup ok");
  }

  #buildCatalog() {
    const tracks = [];
    let trackIdx = 0;

    const videoTracks = this.#conf.stream.getVideoTracks();
    if (videoTracks.length > 0) {
      tracks.push({
        name: String(trackIdx),
        packaging: "loc",
        isLive: true,
        codec: this.#conf.videoCodec,
        clockrate: 90000,
      });
      trackIdx++;
    }

    const audioTracks = this.#conf.stream.getAudioTracks();
    if (audioTracks.length > 0) {
      const audioSettings = audioTracks[0].getSettings();
      tracks.push({
        name: String(trackIdx),
        packaging: "loc",
        isLive: true,
        codec: this.#conf.audioCodec,
        clockrate: audioSettings.sampleRate || 48000,
        samplerate: audioSettings.sampleRate || 48000,
        channels: audioSettings.channelCount || 1,
      });
    }

    return { version: 1, tracks };
  }

  async #publishAllTracks() {
    const catalog = this.#buildCatalog();
    const catalogJSON = JSON.stringify(catalog);
    console.log("catalog:", catalog);

    await Promise.all([
      this.#publish(0n, ".catalog", 0n),
      this.#writeCatalog(catalogJSON, 0n),
    ]);

    await Promise.all(
      catalog.tracks.map((track, idx) =>
        this.#publish(BigInt((idx + 1) * 2), track.name, BigInt(idx + 1)),
      ),
    );

    if (this.#conf.onConnected !== undefined) {
      this.#conf.onConnected();
    }

    const promises = [];
    let alias = 1n;

    const videoTracks = this.#conf.stream.getVideoTracks();
    if (videoTracks.length > 0) {
      promises.push(this.#encodeVideo(videoTracks[0], alias));
      alias++;
    }

    const audioTracks = this.#conf.stream.getAudioTracks();
    if (audioTracks.length > 0) {
      promises.push(this.#encodeAudio(audioTracks[0], alias));
      alias++;
    }

    await Promise.all(promises);
  }

  async #publish(requestId, trackName, trackAlias) {
    const bidi = await this.#wt.createBidirectionalStream();
    const w = bidi.writable.getWriter();
    const r = new MediaMTXMoQPublisher.#StreamReader(bidi.readable);

    await w.write(
      MediaMTXMoQPublisher.#encodeMsg(
        MediaMTXMoQPublisher.#MSG_PUBLISH,
        MediaMTXMoQPublisher.#concat(
          MediaMTXMoQPublisher.#encodeVarint(requestId),
          MediaMTXMoQPublisher.#encodeNamespace(
            MediaMTXMoQPublisher.#NAMESPACE,
          ),
          MediaMTXMoQPublisher.#encodeString(trackName),
          MediaMTXMoQPublisher.#encodeVarint(trackAlias),
          this.#encodeAuthParams(),
        ),
      ),
    );
    w.releaseLock();

    const t = await r.readVarint();
    const payload = await r.readBytes(await r.readU16());
    switch (t) {
      case MediaMTXMoQPublisher.#MSG_REQUEST_ERROR:
        throw new Error(MediaMTXMoQPublisher.#requestErrorReason(payload));
      case MediaMTXMoQPublisher.#MSG_REQUEST_OK:
        break;
      default:
        throw new Error("expected REQUEST_OK (0x7), got 0x" + t.toString(16));
    }
    console.log("REQUEST_OK for track", trackName);
  }

  async #encodeVideo(mediaTrack, trackAlias) {
    const isAvc = this.#conf.videoCodec.startsWith("avc3");
    const isHvc = this.#conf.videoCodec.startsWith("hev1");
    const processor = new MediaStreamTrackProcessor({ track: mediaTrack });
    const frameReader = processor.readable.getReader();

    let groupId = 0n;
    let vps = null;
    let sps = null;
    let pps = null;

    this.#videoEncoder = new VideoEncoder({
      output: (chunk, metadata) => {
        if (isAvc && metadata?.decoderConfig?.description !== undefined) {
          [sps, pps] = MediaMTXMoQPublisher.#parseAvcC(
            metadata.decoderConfig.description,
          );
        } else if (
          isHvc &&
          metadata?.decoderConfig?.description !== undefined
        ) {
          [vps, sps, pps] = MediaMTXMoQPublisher.#parseHvcC(
            metadata.decoderConfig.description,
          );
        }

        let data = new Uint8Array(chunk.byteLength);
        chunk.copyTo(data);

        if (isAvc && chunk.type === "key" && sps !== null && pps !== null) {
          data = MediaMTXMoQPublisher.#concat(
            MediaMTXMoQPublisher.#avccLen(sps.length),
            sps,
            MediaMTXMoQPublisher.#avccLen(pps.length),
            pps,
            data,
          );
        } else if (
          isHvc &&
          chunk.type === "key" &&
          vps !== null &&
          sps !== null &&
          pps !== null
        ) {
          data = MediaMTXMoQPublisher.#concat(
            MediaMTXMoQPublisher.#avccLen(vps.length),
            vps,
            MediaMTXMoQPublisher.#avccLen(sps.length),
            sps,
            MediaMTXMoQPublisher.#avccLen(pps.length),
            pps,
            data,
          );
        }

        this.#writeData(
          trackAlias,
          groupId++,
          chunk.timestamp,
          90000,
          data,
        ).catch((err) => this.#handleError(err.message));
      },
      error: (err) => console.error(err.message),
    });

    const videoConfig = {
      codec: this.#conf.videoCodec,
      bitrate: this.#conf.videoBitrate * 1000,
      framerate: this.#conf.videoFramerate,
      width: this.#conf.videoWidth,
      height: this.#conf.videoHeight,
      latencyMode: "realtime",
    };
    this.#videoEncoder.configure(videoConfig);

    let frameCount = 0n;

    for (;;) {
      const { value: frame, done } = await frameReader.read();
      if (done) break;
      if (this.#state !== "running") {
        frame.close();
        break;
      }
      this.#videoEncoder.encode(frame, {
        keyFrame: frameCount % BigInt(this.#conf.videoKeyframeInterval) === 0n,
      });
      frame.close();
      frameCount++;
    }
  }

  async #encodeAudio(mediaTrack, trackAlias) {
    const processor = new MediaStreamTrackProcessor({ track: mediaTrack });
    const frameReader = processor.readable.getReader();
    const audioSettings = mediaTrack.getSettings();
    const sampleRate = audioSettings.sampleRate || 48000;
    const channelCount = audioSettings.channelCount || 1;

    let groupId = 0n;

    this.#audioEncoder = new AudioEncoder({
      output: (chunk) => {
        const data = new Uint8Array(chunk.byteLength);
        chunk.copyTo(data);
        this.#writeData(
          trackAlias,
          groupId++,
          chunk.timestamp,
          sampleRate,
          data,
        ).catch((err) => this.#handleError(err.message));
      },
      error: (err) => console.error(err.message),
    });

    this.#audioEncoder.configure({
      codec: this.#conf.audioCodec,
      sampleRate: sampleRate,
      numberOfChannels: channelCount,
      bitrate: (this.#conf.audioBitrate || 128) * 1000,
    });

    for (;;) {
      const { value: frame, done } = await frameReader.read();
      if (done) break;
      if (this.#state !== "running" || this.#audioEncoder === null) {
        frame.close();
        break;
      }
      this.#audioEncoder.encode(frame);
      frame.close();
    }
  }

  async #writeCatalog(catalogJSON, trackAlias) {
    const payload = new TextEncoder().encode(catalogJSON);
    const uniStream = await this.#wt.createUnidirectionalStream();
    const w = uniStream.getWriter();
    await w.write(
      MediaMTXMoQPublisher.#concat(
        MediaMTXMoQPublisher.#encodeVarint(MediaMTXMoQPublisher.#SUBGROUP_TYPE),
        MediaMTXMoQPublisher.#encodeVarint(trackAlias),
        MediaMTXMoQPublisher.#encodeVarint(0),
        MediaMTXMoQPublisher.#encodeVarint(0),
        MediaMTXMoQPublisher.#encodeVarint(payload.length),
        payload,
        new Uint8Array([0x00, 0x00, 0x03]),
      ),
    );
    await w.close();
    console.log("catalog sent");
  }

  async #writeData(trackAlias, groupId, timestamp, clockrate, payload) {
    const tsType = MediaMTXMoQPublisher.#encodeVarint(0x06n);
    const tsValue = MediaMTXMoQPublisher.#encodeVarint(
      BigInt(Math.round((timestamp * clockrate) / 1000000)),
    );
    const kv = MediaMTXMoQPublisher.#concat(tsType, tsValue);
    const propsBlock = MediaMTXMoQPublisher.#concat(
      MediaMTXMoQPublisher.#encodeVarint(kv.length),
      kv,
    );

    const uniStream = await this.#wt.createUnidirectionalStream();
    const w = uniStream.getWriter();
    await w.write(
      MediaMTXMoQPublisher.#concat(
        MediaMTXMoQPublisher.#encodeVarint(
          MediaMTXMoQPublisher.#SUBGROUP_TYPE_WITH_PROPS,
        ),
        MediaMTXMoQPublisher.#encodeVarint(trackAlias),
        MediaMTXMoQPublisher.#encodeVarint(groupId),
        MediaMTXMoQPublisher.#encodeVarint(0),
        propsBlock,
        MediaMTXMoQPublisher.#encodeVarint(payload.length),
        payload,
        new Uint8Array([0x00, 0x00, 0x00, 0x03]),
      ),
    );
    await w.close();
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
    throw new Error("varint too long");
  }

  static #requestErrorReason(payload) {
    let off = 0;
    let v = MediaMTXMoQPublisher.#readVarintFromBytes(payload, off);
    off += v.size; // skip error code
    v = MediaMTXMoQPublisher.#readVarintFromBytes(payload, off);
    off += v.size; // skip retry interval
    v = MediaMTXMoQPublisher.#readVarintFromBytes(payload, off);
    off += v.size; // reason length
    return new TextDecoder().decode(payload.slice(off, off + Number(v.value)));
  }

  static #avccLen(l) {
    return new Uint8Array([
      (l >> 24) & 0xff,
      (l >> 16) & 0xff,
      (l >> 8) & 0xff,
      l & 0xff,
    ]);
  }

  static #parseAvcC(description) {
    let sps = null;
    let pps = null;
    const desc = new Uint8Array(description);
    let off = 5;
    const numSPS = desc[off++] & 0x1f;
    for (let i = 0; i < numSPS; i++) {
      const len = (desc[off] << 8) | desc[off + 1];
      off += 2;
      sps = desc.slice(off, off + len);
      off += len;
    }
    const numPPS = desc[off++];
    for (let i = 0; i < numPPS; i++) {
      const len = (desc[off] << 8) | desc[off + 1];
      off += 2;
      pps = desc.slice(off, off + len);
      off += len;
    }
    return [sps, pps];
  }

  static #parseHvcC(description) {
    let vps = null;
    let sps = null;
    let pps = null;
    const desc = new Uint8Array(description);
    let off = 22; // skip 22-byte fixed header
    const numArrays = desc[off++];
    for (let i = 0; i < numArrays; i++) {
      const naluType = desc[off++] & 0x3f;
      const numNalus = (desc[off] << 8) | desc[off + 1];
      off += 2;
      for (let j = 0; j < numNalus; j++) {
        const len = (desc[off] << 8) | desc[off + 1];
        off += 2;
        const nalu = desc.slice(off, off + len);
        off += len;
        if (naluType === 32) vps = nalu;
        else if (naluType === 33) sps = nalu;
        else if (naluType === 34) pps = nalu;
      }
    }
    return [vps, sps, pps];
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
    return MediaMTXMoQPublisher.#concat(
      MediaMTXMoQPublisher.#encodeVarint(b.length),
      b,
    );
  }

  static #encodeNamespace(parts) {
    let b = MediaMTXMoQPublisher.#encodeVarint(parts.length);
    for (const p of parts)
      b = MediaMTXMoQPublisher.#concat(
        b,
        MediaMTXMoQPublisher.#encodeString(p),
      );
    return b;
  }

  static #encodeMsg(type, payload) {
    return MediaMTXMoQPublisher.#concat(
      MediaMTXMoQPublisher.#encodeVarint(type),
      new Uint8Array([(payload.length >> 8) & 0xff, payload.length & 0xff]),
      payload,
    );
  }

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
      } else {
        throw new Error("unsupported varint: 0x" + b.toString(16));
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
      }
    }

    async readU16() {
      const d = await this.readBytes(2);
      return (d[0] << 8) | d[1];
    }
  };
}

window.MediaMTXMoQPublisher = MediaMTXMoQPublisher;
