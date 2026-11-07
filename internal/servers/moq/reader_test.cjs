"use strict";

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");

const READER_JS = fs.readFileSync(path.join(__dirname, "reader.js"), "utf8");

const SETUP_TYPE = 0x2f00n;
const MSG_SUBSCRIBE_OK = 0x04n;
const VIDEO_REQUEST_ID = 10;
const AUDIO_REQUEST_ID = 11;

const UA = {
  safariIpad:
    "Mozilla/5.0 (iPad; CPU OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1",
  safariIphone:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Mobile/15E148 Safari/604.1",
  safariDesktop:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.5 Safari/605.1.15",
  chrome:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36",
  edge: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36 Edg/128.0.0.0",
  opera:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36 OPR/114.0.0.0",
  firefox:
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:129.0) Gecko/20100101 Firefox/129.0",
  fxios:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/129.0 Mobile/15E148 Safari/605.1.15",
  android:
    "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Mobile Safari/537.36",
  unknown: "CustomBrowser/1.0",
  empty: "",
};

const SPS_HIGH = Uint8Array.of(0x67, 0x64, 0x00, 0x28, 0xac, 0x2b);
const SPS_BASE = Uint8Array.of(0x67, 0x42, 0xe0, 0x1e, 0xac, 0x2b);
const PPS = Uint8Array.of(0x68, 0xee, 0x3c, 0x80);
const PPS2 = Uint8Array.of(0x68, 0xef, 0x3c, 0x80);
const IDR = Uint8Array.of(0x65, 0x88, 0x84, 0x00);

const HEV1 = "hev1.1.6.L93.B0";
const VPS_HEVC = Uint8Array.of(
  0x40,
  0x01,
  0x0c,
  0x01,
  0xff,
  0xff,
  0x01,
  0x60,
  0x00,
  0x00,
  0x03,
  0x00,
  0x90,
  0x00,
  0x00,
  0x03,
  0x00,
  0x00,
  0x03,
  0x00,
  0x5d,
  0x95,
  0x98,
  0x09,
);
const SPS_HEVC = Uint8Array.of(
  0x42,
  0x01,
  0x01,
  0x01,
  0x60,
  0x00,
  0x00,
  0x03,
  0x00,
  0x90,
  0x00,
  0x00,
  0x03,
  0x00,
  0x00,
  0x03,
  0x00,
  0x5d,
  0xa0,
  0x02,
  0x80,
  0x80,
  0x2d,
  0x16,
  0x36,
  0xb9,
);
const PPS_HEVC = Uint8Array.of(0x44, 0x01, 0xc1, 0x73, 0xd1, 0x89);

function concat(...parts) {
  const total = parts.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

function encodeVarint(value) {
  const v = BigInt(value);
  if (v < 128n) {
    return Uint8Array.of(Number(v));
  }
  if (v < 16384n) {
    return Uint8Array.of(0x80 | Number(v >> 8n), Number(v & 0xffn));
  }
  throw new Error("varint too large for fixture");
}

function encodeU16(n) {
  return Uint8Array.of((n >> 8) & 0xff, n & 0xff);
}

function bytesToReadable(bytes) {
  return new ReadableStream({
    start(controller) {
      controller.enqueue(bytes);
      controller.close();
    },
  });
}

function setupOk() {
  return concat(encodeVarint(SETUP_TYPE), encodeU16(0));
}

function subscribeOk() {
  return concat(encodeVarint(MSG_SUBSCRIBE_OK), encodeU16(0));
}

function makeSubgroup(trackAlias, groupId, data) {
  return concat(
    encodeVarint(0x10),
    encodeVarint(trackAlias),
    encodeVarint(groupId),
    encodeVarint(0),
    encodeVarint(data.length),
    data,
    encodeVarint(0),
    encodeVarint(0),
  );
}

function avcc(...nalus) {
  return concat(
    ...nalus.flatMap((nalu) => {
      const header = new Uint8Array(4);
      header[0] = (nalu.length >> 24) & 0xff;
      header[1] = (nalu.length >> 16) & 0xff;
      header[2] = (nalu.length >> 8) & 0xff;
      header[3] = nalu.length & 0xff;
      return [header, nalu];
    }),
  );
}

function makeAvcC(sps, pps) {
  const avcC = new Uint8Array(11 + sps.length + pps.length);
  let off = 0;
  avcC[off++] = 0x01;
  avcC[off++] = sps[1];
  avcC[off++] = sps[2];
  avcC[off++] = sps[3];
  avcC[off++] = 0xff;
  avcC[off++] = 0xe1;
  avcC[off++] = (sps.length >> 8) & 0xff;
  avcC[off++] = sps.length & 0xff;
  avcC.set(sps, off);
  off += sps.length;
  avcC[off++] = 0x01;
  avcC[off++] = (pps.length >> 8) & 0xff;
  avcC[off++] = pps.length & 0xff;
  avcC.set(pps, off);
  return avcC;
}

function videoCatalog(codec) {
  return { tracks: [{ name: "video", codec }] };
}

function audioCatalog() {
  return {
    tracks: [{ name: "audio", codec: "opus", samplerate: 48000, channels: 2 }],
  };
}

async function waitFor(predicate, label, timeoutMs = 2000) {
  const start = Date.now();
  while (!predicate()) {
    if (Date.now() - start > timeoutMs) {
      throw new Error("timed out waiting for " + label);
    }
    await new Promise((resolve) => setImmediate(resolve));
  }
}

async function settle() {
  for (let i = 0; i < 40; i++) {
    await new Promise((resolve) => setImmediate(resolve));
  }
  await new Promise((resolve) => setTimeout(resolve, 15));
}

function createHarness(opts) {
  const catalogs = opts.catalogs ?? [videoCatalog("avc3.640028")];
  let catalogIndex = 0;
  const sessions = [];
  const timers = new Map();
  let nextTimerId = 0;
  let cleaned = false;
  const supportChecks = [];
  const videoDecoders = [];
  const audioDecoders = [];
  const canvases = [];
  const errors = [];
  let subscribed;
  let subscribeCount = 0;
  let resolveSubscribed;
  const subscribedPromise = new Promise((resolve) => {
    resolveSubscribed = resolve;
  });

  const videoElement = {
    children: [],
    appendChild(el) {
      this.children.push(el);
      return el;
    },
  };

  const document = {
    createElement(name) {
      assert.equal(name, "canvas");
      const canvas = {
        width: 0,
        height: 0,
        removed: false,
        getContext(type) {
          if (type === "webgl2") {
            return null;
          }
          return { drawImage() {} };
        },
        remove() {
          this.removed = true;
        },
      };
      canvases.push(canvas);
      return canvas;
    },
  };

  class EncodedVideoChunk {
    constructor({ type, timestamp, data }) {
      this.type = type;
      this.timestamp = timestamp;
      this.data = data;
    }
  }

  class EncodedAudioChunk {
    constructor({ type, timestamp, data }) {
      this.type = type;
      this.timestamp = timestamp;
      this.data = data;
    }
  }

  class VideoDecoder {
    static isConfigSupported(config) {
      supportChecks.push({
        codec: config.codec,
        optimizeForLatency: config.optimizeForLatency,
        hasDescription: config.description !== undefined,
      });
      const supported = opts.isConfigSupported
        ? opts.isConfigSupported(config)
        : true;
      return Promise.resolve({ supported, config });
    }

    constructor(init) {
      this.init = init;
      this.decodeQueueSize = 0;
      this.configured = [];
      this.decoded = [];
      this.events = [];
      this.closed = false;
      videoDecoders.push(this);
    }

    configure(config) {
      const copy = {
        codec: config.codec,
        optimizeForLatency: config.optimizeForLatency,
      };
      if (config.description !== undefined) {
        copy.description = Uint8Array.from(config.description);
      }
      this.configured.push(copy);
      this.events.push("configure");
      if (opts.configureThrows) {
        throw new Error("configure failed");
      }
    }

    decode(chunk) {
      this.decoded.push({
        type: chunk.type,
        data: Uint8Array.from(chunk.data),
      });
      this.events.push("decode");
    }

    close() {
      this.closed = true;
    }
  }

  class AudioDecoder {
    constructor(init) {
      this.init = init;
      this.decodeQueueSize = 0;
      this.configured = [];
      this.decoded = [];
      this.closed = false;
      audioDecoders.push(this);
    }

    configure(config) {
      this.configured.push({
        codec: config.codec,
        sampleRate: config.sampleRate,
        numberOfChannels: config.numberOfChannels,
      });
    }

    decode(chunk) {
      this.decoded.push({
        type: chunk.type,
        data: Uint8Array.from(chunk.data),
      });
    }

    close() {
      this.closed = true;
    }
  }

  class AudioContext {
    constructor() {
      this.state = "running";
      this.currentTime = 0;
      this.destination = {};
    }

    createBuffer(channels, frames, sampleRate) {
      return {
        duration: frames / sampleRate,
        getChannelData() {
          return new Float32Array(frames);
        },
      };
    }

    createBufferSource() {
      return {
        buffer: null,
        connect() {},
        start() {},
      };
    }

    resume() {
      this.state = "running";
      return Promise.resolve();
    }

    close() {
      this.state = "closed";
      return Promise.resolve();
    }
  }

  class MockWebTransport {
    constructor(url, options) {
      this.url = url;
      this.options = options;
      this._closed = false;
      this._bidiCount = 0;
      this.catalog = catalogs[Math.min(catalogIndex, catalogs.length - 1)];
      catalogIndex++;
      this.ready = Promise.resolve();
      this.closed = new Promise((resolve) => {
        this._resolveClosed = resolve;
      });
      this.incomingUnidirectionalStreams = new ReadableStream({
        start: (controller) => {
          this._incomingController = controller;
        },
      });
      this._incomingController.enqueue(bytesToReadable(setupOk()));
      sessions.push(this);
    }

    async createUnidirectionalStream() {
      return new WritableStream();
    }

    async createBidirectionalStream() {
      this._bidiCount++;
      if (this._bidiCount === 1) {
        const payload = new TextEncoder().encode(JSON.stringify(this.catalog));
        this._incomingController.enqueue(
          bytesToReadable(makeSubgroup(0, 0, payload)),
        );
      }
      return {
        readable: bytesToReadable(subscribeOk()),
        writable: new WritableStream(),
      };
    }

    send(trackAlias, groupId, data) {
      if (this._closed) {
        throw new Error("transport closed");
      }
      this._incomingController.enqueue(
        bytesToReadable(makeSubgroup(trackAlias, groupId, data)),
      );
    }

    close() {
      if (this._closed) {
        return;
      }
      this._closed = true;
      try {
        this._incomingController.close();
      } catch (e) {}
      this._resolveClosed();
    }
  }

  const window = {
    MediaMTXMoQReader: null,
    setTimeout(fn) {
      if (cleaned) {
        return 0;
      }
      const id = ++nextTimerId;
      timers.set(id, fn);
      return id;
    },
    clearTimeout(id) {
      timers.delete(id);
    },
  };

  const sandbox = {
    window,
    document,
    navigator: { userAgent: opts.userAgent ?? "" },
    fetch: async () => ({
      text: async () => "aabbccdd",
    }),
    WebTransport: MockWebTransport,
    VideoDecoder,
    AudioDecoder,
    AudioContext,
    EncodedVideoChunk,
    EncodedAudioChunk,
    performance: { now: () => 1000 },
    console: {
      log() {},
      info() {},
      warn() {},
      error() {},
    },
    Uint8Array,
    Float32Array,
    ArrayBuffer,
    TextEncoder,
    TextDecoder,
    ReadableStream,
    WritableStream,
    Promise,
    Error,
    JSON,
    Map,
    BigInt,
    Number,
    String,
    Object,
    Array,
    Math,
    parseInt,
    atob,
    btoa,
    queueMicrotask,
    setTimeout,
    clearTimeout,
    setImmediate,
    clearImmediate,
  };
  sandbox.globalThis = sandbox;

  vm.createContext(sandbox);
  vm.runInContext(READER_JS, sandbox, { filename: "reader.js" });

  const harness = {
    Reader: window.MediaMTXMoQReader,
    sessions,
    timers,
    supportChecks,
    videoDecoders,
    audioDecoders,
    canvases,
    errors,
    videoElement,
    get subscribed() {
      return subscribed;
    },
    get subscribeCount() {
      return subscribeCount;
    },
    subscribedPromise,
    currentSession() {
      return sessions[sessions.length - 1];
    },
    sendVideo(groupId, data) {
      this.currentSession().send(VIDEO_REQUEST_ID, groupId, data);
    },
    sendAudio(groupId, data) {
      this.currentSession().send(AUDIO_REQUEST_ID, groupId, data);
    },
    fireRetry() {
      const pending = [...timers.values()];
      timers.clear();
      for (const fn of pending) {
        fn();
      }
    },
    cleanup() {
      cleaned = true;
      for (const session of sessions) {
        session.close();
      }
      timers.clear();
    },
  };

  new harness.Reader({
    fingerprintUrl: "https://example.test/fingerprint",
    url: "https://example.test/moq",
    videoElement,
    onError(err) {
      errors.push(err);
    },
    onSubscribed(hasAudio) {
      subscribed = hasAudio;
      subscribeCount++;
      if (resolveSubscribed !== null) {
        resolveSubscribed(hasAudio);
        resolveSubscribed = null;
      }
    },
  });

  return harness;
}

function startHarness(t, opts) {
  const harness = createHarness(opts);
  t.after(() => harness.cleanup());
  return harness;
}

function codecsOf(configs) {
  return configs.map((c) => c.codec);
}

function safariDecoderOpts(extra) {
  return {
    isConfigSupported: (config) => !String(config.codec).startsWith("avc3"),
    ...extra,
  };
}

async function waitDecoded(harness, count, label) {
  await waitFor(
    () =>
      harness.videoDecoders.length > 0 &&
      harness.videoDecoders[harness.videoDecoders.length - 1].decoded.length >=
        count,
    label,
  );
}

async function sendVideoAndWait(harness, groupId, data, decodedCount, label) {
  harness.sendVideo(groupId, data);
  await waitDecoded(harness, decodedCount, label);
}

describe("MediaMTXMoQReader H264 Safari fallback", { concurrency: 1 }, () => {
  for (const [name, ua] of [
    ["iPad Safari", UA.safariIpad],
    ["iPhone Safari", UA.safariIphone],
    ["desktop Safari", UA.safariDesktop],
  ]) {
    it("presents avc3.640028 as avc1 on " + name, async (t) => {
      const sample = avcc(SPS_HIGH, PPS, IDR);
      const harness = startHarness(
        t,
        safariDecoderOpts({
          userAgent: ua,
          catalogs: [videoCatalog("avc3.640028")],
        }),
      );

      await harness.subscribedPromise;
      assert.equal(harness.subscribed, false);
      assert.deepEqual(codecsOf(harness.supportChecks), ["avc1.640028"]);
      assert.equal(harness.videoDecoders.length, 1);
      assert.deepEqual(harness.videoDecoders[0].configured, []);

      await sendVideoAndWait(harness, 1, sample, 1, "first safari decode");

      const dec = harness.videoDecoders[0];
      assert.deepEqual(codecsOf(dec.configured), ["avc1.640028"]);
      assert.equal(dec.events[0], "configure");
      assert.equal(dec.events[1], "decode");
      assert.deepEqual(
        [...dec.configured[0].description],
        [...makeAvcC(SPS_HIGH, PPS)],
      );
      assert.deepEqual([...dec.decoded[0].data], [...sample]);
      assert.equal(
        codecsOf(harness.supportChecks)
          .concat(codecsOf(dec.configured))
          .some((c) => c.startsWith("avc3")),
        false,
      );
    });
  }

  it("rewrites an alternative avc3 profile suffix to avc1", async (t) => {
    const sample = avcc(SPS_BASE, PPS, IDR);
    const harness = startHarness(
      t,
      safariDecoderOpts({
        userAgent: UA.safariIpad,
        catalogs: [videoCatalog("avc3.42E01E")],
      }),
    );

    await harness.subscribedPromise;
    assert.deepEqual(codecsOf(harness.supportChecks), ["avc1.42E01E"]);

    await sendVideoAndWait(harness, 1, sample, 1, "alt profile decode");

    const dec = harness.videoDecoders[0];
    assert.deepEqual(codecsOf(dec.configured), ["avc1.42E01E"]);
    assert.deepEqual(
      [...dec.configured[0].description],
      [...makeAvcC(SPS_BASE, PPS)],
    );
  });

  for (const [name, ua] of [
    ["Chrome", UA.chrome],
    ["Edge", UA.edge],
    ["Opera", UA.opera],
    ["Firefox", UA.firefox],
    ["Firefox iOS", UA.fxios],
    ["Android", UA.android],
    ["unknown", UA.unknown],
    ["empty", UA.empty],
  ]) {
    it("preserves avc3 for " + name, async (t) => {
      const codec = name === "Chrome" ? "avc3.42E01E" : "avc3.640028";
      const sps = name === "Chrome" ? SPS_BASE : SPS_HIGH;
      const sample = avcc(sps, PPS, IDR);
      const harness = startHarness(t, {
        userAgent: ua,
        catalogs: [videoCatalog(codec)],
      });

      await harness.subscribedPromise;
      assert.deepEqual(codecsOf(harness.supportChecks), [codec]);
      assert.equal(harness.videoDecoders[0].configured.length, 1);
      assert.equal(harness.videoDecoders[0].configured[0].codec, codec);
      assert.equal(
        harness.videoDecoders[0].configured[0].description,
        undefined,
      );

      await sendVideoAndWait(harness, 1, sample, 1, name + " decode");

      const dec = harness.videoDecoders[0];
      assert.deepEqual(codecsOf(dec.configured), [codec, codec]);
      assert.deepEqual(
        [...dec.configured[1].description],
        [...makeAvcC(sps, PPS)],
      );
      assert.equal(
        codecsOf(harness.supportChecks)
          .concat(codecsOf(dec.configured))
          .some((c) => c.startsWith("avc1")),
        false,
      );
    });
  }

  it("skips safari samples until a complete SPS/PPS pair arrives", async (t) => {
    const complete = avcc(SPS_HIGH, PPS, IDR);
    const harness = startHarness(
      t,
      safariDecoderOpts({
        userAgent: UA.safariIpad,
        catalogs: [videoCatalog("avc3.640028")],
      }),
    );

    await harness.subscribedPromise;
    harness.sendVideo(1, avcc(IDR));
    await settle();
    harness.sendVideo(2, avcc(SPS_HIGH, IDR));
    await settle();
    harness.sendVideo(3, avcc(PPS, IDR));
    await settle();
    assert.deepEqual(harness.videoDecoders[0].configured, []);
    assert.deepEqual(harness.videoDecoders[0].decoded, []);

    await sendVideoAndWait(harness, 4, complete, 1, "decode after params");

    const dec = harness.videoDecoders[0];
    assert.equal(dec.configured.length, 1);
    assert.equal(dec.decoded.length, 1);
    assert.deepEqual([...dec.decoded[0].data], [...complete]);
    assert.deepEqual(
      [...dec.configured[0].description],
      [...makeAvcC(SPS_HIGH, PPS)],
    );
  });

  it("does not reconfigure safari on identical parameter sets", async (t) => {
    const sample = avcc(SPS_HIGH, PPS, IDR);
    const harness = startHarness(
      t,
      safariDecoderOpts({
        userAgent: UA.safariDesktop,
        catalogs: [videoCatalog("avc3.640028")],
      }),
    );

    await harness.subscribedPromise;
    await sendVideoAndWait(
      harness,
      1,
      sample,
      1,
      "first identical safari sample",
    );
    await sendVideoAndWait(
      harness,
      2,
      sample,
      2,
      "second identical safari sample",
    );

    const dec = harness.videoDecoders[0];
    assert.equal(dec.configured.length, 1);
    assert.equal(dec.decoded.length, 2);
    assert.deepEqual(dec.events, ["configure", "decode", "decode"]);
  });

  it("keeps avc1 across safari parameter changes", async (t) => {
    const first = avcc(SPS_HIGH, PPS, IDR);
    const second = avcc(SPS_HIGH, PPS2, IDR);
    const harness = startHarness(
      t,
      safariDecoderOpts({
        userAgent: UA.safariIpad,
        catalogs: [videoCatalog("avc3.640028")],
      }),
    );

    await harness.subscribedPromise;
    await sendVideoAndWait(harness, 1, first, 1, "safari first params");
    await sendVideoAndWait(harness, 2, second, 2, "safari param change");

    const dec = harness.videoDecoders[0];
    assert.equal(dec.configured.length, 2);
    assert.deepEqual(codecsOf(dec.configured), ["avc1.640028", "avc1.640028"]);
    assert.deepEqual(
      [...dec.configured[0].description],
      [...makeAvcC(SPS_HIGH, PPS)],
    );
    assert.deepEqual(
      [...dec.configured[1].description],
      [...makeAvcC(SPS_HIGH, PPS2)],
    );
    assert.equal(
      codecsOf(dec.configured).some((c) => c.startsWith("avc3")),
      false,
    );
  });

  it("reconfigures chrome with avc3 when parameters change", async (t) => {
    const first = avcc(SPS_HIGH, PPS, IDR);
    const second = avcc(SPS_HIGH, PPS2, IDR);
    const harness = startHarness(t, {
      userAgent: UA.chrome,
      catalogs: [videoCatalog("avc3.640028")],
    });

    await harness.subscribedPromise;
    const dec = harness.videoDecoders[0];
    assert.equal(dec.configured.length, 1);
    assert.equal(dec.configured[0].codec, "avc3.640028");
    assert.equal(dec.configured[0].description, undefined);

    await sendVideoAndWait(
      harness,
      1,
      avcc(IDR),
      1,
      "chrome sample without params",
    );
    assert.equal(dec.configured.length, 1);

    await sendVideoAndWait(harness, 2, first, 2, "chrome first params");
    await sendVideoAndWait(harness, 3, first, 3, "chrome repeated params");
    await sendVideoAndWait(
      harness,
      4,
      second,
      4,
      "chrome param-driven reconfigure",
    );

    assert.deepEqual(codecsOf(dec.configured), [
      "avc3.640028",
      "avc3.640028",
      "avc3.640028",
    ]);
    assert.deepEqual(
      [...dec.configured[1].description],
      [...makeAvcC(SPS_HIGH, PPS)],
    );
    assert.deepEqual(
      [...dec.configured[2].description],
      [...makeAvcC(SPS_HIGH, PPS2)],
    );
  });

  it("clears the safari fallback on reconnect to H265", async (t) => {
    const harness = startHarness(
      t,
      safariDecoderOpts({
        userAgent: UA.safariIpad,
        catalogs: [videoCatalog("avc3.640028"), videoCatalog(HEV1)],
      }),
    );

    await harness.subscribedPromise;
    assert.deepEqual(codecsOf(harness.supportChecks), ["avc1.640028"]);
    await settle();
    harness.currentSession().close();
    await waitFor(() => harness.errors.length > 0, "retry after close");
    assert.match(
      harness.errors[0],
      /connection closed, retrying in some seconds/,
    );
    assert.equal(harness.videoDecoders[0].closed, true);
    assert.equal(harness.canvases[0].removed, true);
    await settle();

    harness.fireRetry();
    await waitFor(() => harness.subscribeCount === 2, "h265 subscribe");
    await waitFor(
      () => harness.supportChecks.length === 2,
      "h265 support probe",
    );

    assert.equal(harness.supportChecks[1].codec, HEV1);
    const hevcDec = harness.videoDecoders[1];
    assert.equal(hevcDec.configured.length, 1);
    assert.equal(hevcDec.configured[0].codec, HEV1);
    assert.equal(hevcDec.configured[0].description, undefined);

    const hevcSample = avcc(VPS_HEVC, SPS_HEVC, PPS_HEVC);
    await sendVideoAndWait(harness, 1, hevcSample, 1, "h265 decode");

    assert.equal(hevcDec.configured.length, 2);
    assert.equal(hevcDec.configured[1].codec, HEV1);
    assert.ok(hevcDec.configured[1].description.length > 0);
    assert.equal(hevcDec.configured[1].description[0], 0x01);
    assert.deepEqual([...hevcDec.decoded[0].data], [...hevcSample]);
    assert.equal(
      codecsOf(hevcDec.configured).some((c) => c.startsWith("avc1")),
      false,
    );
  });

  it("subscribes to audio-only catalogs without touching video codecs", async (t) => {
    const payload = Uint8Array.of(0x01, 0x02, 0x03);
    const harness = startHarness(t, {
      userAgent: UA.safariIpad,
      catalogs: [audioCatalog()],
    });

    await harness.subscribedPromise;
    assert.equal(harness.subscribed, true);
    assert.deepEqual(harness.supportChecks, []);
    assert.equal(harness.videoDecoders.length, 0);
    assert.equal(harness.audioDecoders.length, 1);
    assert.deepEqual(harness.audioDecoders[0].configured, [
      { codec: "opus", sampleRate: 48000, numberOfChannels: 2 },
    ]);

    harness.sendAudio(1, payload);
    await waitFor(
      () => harness.audioDecoders[0].decoded.length === 1,
      "audio decode",
    );
    assert.deepEqual(
      [...harness.audioDecoders[0].decoded[0].data],
      [...payload],
    );
  });

  it("reports unsupported avc1 through the existing retry path", async (t) => {
    const harness = startHarness(t, {
      userAgent: UA.safariIpad,
      catalogs: [videoCatalog("avc3.640028")],
      isConfigSupported: (config) =>
        !String(config.codec).startsWith("avc3") &&
        !String(config.codec).startsWith("avc1"),
    });

    await waitFor(() => harness.errors.length > 0, "unsupported codec error");
    assert.match(
      harness.errors[0],
      /the browser you are using does not support video codec avc1\.640028, retrying in some seconds/,
    );
    assert.deepEqual(codecsOf(harness.supportChecks), ["avc1.640028"]);
    assert.equal(harness.timers.size, 1);
    if (harness.videoDecoders.length > 0) {
      assert.equal(harness.videoDecoders[0].closed, true);
    }
    harness.timers.clear();
    assert.equal(harness.timers.size, 0);
  });

  it("routes configure exceptions through retry and cancels the timer", async (t) => {
    const harness = startHarness(
      t,
      safariDecoderOpts({
        userAgent: UA.safariIpad,
        catalogs: [videoCatalog("avc3.640028")],
        configureThrows: true,
      }),
    );

    await harness.subscribedPromise;
    harness.sendVideo(1, avcc(SPS_HIGH, PPS, IDR));
    await waitFor(() => harness.errors.length > 0, "configure exception");
    assert.match(
      harness.errors[0],
      /configure failed, retrying in some seconds/,
    );
    assert.equal(harness.videoDecoders[0].closed, true);
    assert.equal(harness.canvases[0].removed, true);
    assert.equal(harness.currentSession()._closed, true);
    assert.equal(harness.timers.size, 1);
    harness.timers.clear();
    assert.equal(harness.timers.size, 0);
  });
});
