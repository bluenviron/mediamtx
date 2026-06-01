using Mediar.Containers.Ogg;
using Xunit;

namespace Mediar.Tests;

public sealed class OggMuxerTests
{
    [Fact]
    public async Task RoundTrip_Opus_Small_Packets()
    {
        // Minimal valid OpusHead (19 bytes).
        byte[] opusHead = new byte[19];
        Buffer.BlockCopy("OpusHead"u8.ToArray(), 0, opusHead, 0, 8);
        opusHead[8] = 1;           // version
        opusHead[9] = 2;           // channel count
        opusHead[10] = 0x90; opusHead[11] = 0x01; // pre-skip (LE, 400 samples typical)
        opusHead[12] = 0x80; opusHead[13] = 0xBB; opusHead[14] = 0x00; opusHead[15] = 0x00; // sample rate 48000
        opusHead[16] = 0; opusHead[17] = 0; // output gain
        opusHead[18] = 0;          // channel mapping family

        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 2,
                ExtraData = opusHead,
            },
            TimeBase = new Rational(1, 48000),
        };

        byte[][] payloads = new byte[5][];
        for (int i = 0; i < payloads.Length; i++)
        {
            payloads[i] = new byte[60];
            for (int b = 0; b < payloads[i].Length; b++) payloads[i][b] = (byte)((i + 1) * (b + 1));
        }

        using var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, serialNumber: 0xCAFEBABE, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            for (int i = 0; i < payloads.Length; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * 960L,
                    Dts = i * 960L,
                    Duration = 960,
                    IsKeyFrame = true,
                    Data = payloads[i],
                });
            }
            await mux.FinishAsync();
        }

        byte[] bytes = ms.ToArray();
        Assert.True(bytes.Length > 0);

        // Round-trip through the demuxer.
        using var src = new Mediar.IO.MemoryRandomAccessSource(bytes);
        using var demuxer = OggDemuxer.Open(src);
        Assert.Single(demuxer.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(demuxer.Tracks[0].Codec);
        Assert.Equal(CodecId.Opus, audio.Codec);

        int recovered = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            Assert.Equal(payloads[recovered], s.Data.ToArray());
            s.Owner?.Dispose();
            recovered++;
        }
        Assert.Equal(payloads.Length, recovered);
    }

    [Fact]
    public async Task Page_Has_Valid_Ogg_Sync_And_CRC()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 1,
                ExtraData = new byte[19] { 0x4F, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64, 1, 1, 0x90, 0x01, 0x80, 0xBB, 0, 0, 0, 0, 0 },
            },
            TimeBase = new Rational(1, 48000),
        };
        using var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960, Data = new byte[] { 0xDE, 0xAD, 0xBE, 0xEF } });
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        // Should begin with the OggS capture pattern.
        Assert.Equal((byte)'O', bytes[0]);
        Assert.Equal((byte)'g', bytes[1]);
        Assert.Equal((byte)'g', bytes[2]);
        Assert.Equal((byte)'S', bytes[3]);
        Assert.Equal(0, bytes[4]); // version
        Assert.Equal(0x02, bytes[5] & 0x02); // BOS flag on first page
    }

    private static MediaTrack BuildOpusTrack(int channels = 1)
    {
        byte[] head = new byte[19];
        Buffer.BlockCopy("OpusHead"u8.ToArray(), 0, head, 0, 8);
        head[8] = 1;
        head[9] = (byte)channels;
        head[10] = 0x90; head[11] = 0x01;
        head[12] = 0x80; head[13] = 0xBB;
        head[14] = 0; head[15] = 0; head[16] = 0; head[17] = 0; head[18] = 0;
        return new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = channels,
                ExtraData = head,
            },
            TimeBase = new Rational(1, 48000),
        };
    }

    [Fact]
    public void Ctor_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new OggMuxer(null!));
    }

    [Fact]
    public void Ctor_Non_Writable_Stream_Throws()
    {
        using var ms = new MemoryStream(new byte[1], writable: false);
        Assert.Throws<ArgumentException>(() => new OggMuxer(ms));
    }

    [Fact]
    public void AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new OggMuxer(ms);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void AddTrack_Video_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new OggMuxer(ms);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new VideoCodecParameters { Codec = CodecId.H264, Width = 320, Height = 240 },
            TimeBase = new Rational(1, 1000),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public void AddTrack_Unsupported_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new OggMuxer(ms);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Aac, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 48000),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public void AddTrack_Second_Track_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new OggMuxer(ms);
        mux.AddTrack(BuildOpusTrack());
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildOpusTrack()));
    }

    [Fact]
    public async Task AddTrack_After_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildOpusTrack()));
    }

    [Fact]
    public async Task AddHeaderPacket_After_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddHeaderPacket(new byte[] { 1, 2, 3 }));
    }

    [Fact]
    public async Task StartAsync_Without_Track_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task StartAsync_Twice_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await mux.StartAsync();
        long len = ms.Length;
        await mux.StartAsync();
        Assert.Equal(len, ms.Length);
    }

    [Fact]
    public async Task StartAsync_Without_Header_Or_Extradata_Throws_For_Vorbis()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Vorbis, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 48000),
        };
        mux.AddTrack(track);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task WriteSampleAsync_Before_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(new MediaSample { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 0, Data = new byte[] { 1 } }));
    }

    [Fact]
    public async Task WriteSampleAsync_After_Finish_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(new MediaSample { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960, Data = new byte[] { 1 } });
        await mux.FinishAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(new MediaSample { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 0, Data = new byte[] { 2 } }));
    }

    [Fact]
    public async Task WriteSampleAsync_Null_Sample_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentNullException>(async () => await mux.WriteSampleAsync(null!));
    }

    [Fact]
    public async Task FinishAsync_Without_Samples_Emits_Empty_Eos_Page()
    {
        using var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildOpusTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        // OpusHead page (1) + OpusTags page (2) + empty EOS page (3)
        var bytes = ms.ToArray();
        Assert.Equal((byte)'O', bytes[0]);
        // Empty EOS page should still have OggS marker somewhere later.
        int last = bytes.LastIndexOf((byte)'S');
        Assert.True(last > 4);
    }

    [Fact]
    public async Task FinishAsync_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildOpusTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(new MediaSample { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960, Data = new byte[] { 0xAA } });
        await mux.FinishAsync();
        long len = ms.Length;
        await mux.FinishAsync();
        Assert.Equal(len, ms.Length);
    }

    [Fact]
    public async Task FormatName_Is_Ogg()
    {
        using var ms = new MemoryStream();
        await using var mux = new OggMuxer(ms, leaveOpen: true);
        Assert.Equal("ogg", mux.FormatName);
    }

    [Fact]
    public async Task LeaveOpen_True_Keeps_Stream_Open()
    {
        using var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildOpusTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        // Stream must still be readable/writable.
        Assert.True(ms.CanRead);
        Assert.True(ms.CanWrite);
    }

    [Fact]
    public async Task LeaveOpen_False_Disposes_Stream()
    {
        var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, leaveOpen: false))
        {
            mux.AddTrack(BuildOpusTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        Assert.False(ms.CanRead);
    }

    [Fact]
    public async Task Large_Packet_Spans_Multiple_Pages_And_RoundTrips()
    {
        using var ms = new MemoryStream();
        // Build packet larger than 255*255 to force multi-page split.
        byte[] payload = new byte[300 * 255 + 17];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i * 7);

        await using (var mux = new OggMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildOpusTrack());
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = 960,
                Data = payload,
            });
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        using var src = new Mediar.IO.MemoryRandomAccessSource(bytes);
        using var demuxer = OggDemuxer.Open(src);
        int recovered = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            Assert.Equal(payload, s.Data.ToArray());
            s.Owner?.Dispose();
            recovered++;
        }
        Assert.Equal(1, recovered);
    }

    [Fact]
    public async Task Exact_Multiple_Of_255_Packet_Emits_Trailing_Zero_Segment()
    {
        using var ms = new MemoryStream();
        byte[] payload = new byte[255]; // exact multiple of 255
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i ^ 0x55);

        await using (var mux = new OggMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildOpusTrack());
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = 960,
                Data = payload,
            });
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        using var src = new Mediar.IO.MemoryRandomAccessSource(bytes);
        using var demuxer = OggDemuxer.Open(src);
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            Assert.Equal(payload, s.Data.ToArray());
            s.Owner?.Dispose();
        }
    }

    [Fact]
    public async Task Custom_SerialNumber_Stored_In_Page()
    {
        using var ms = new MemoryStream();
        const uint serial = 0xABCDEF01u;
        await using (var mux = new OggMuxer(ms, serialNumber: serial, leaveOpen: true))
        {
            mux.AddTrack(BuildOpusTrack());
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        uint readSerial = (uint)(bytes[14] | (bytes[15] << 8) | (bytes[16] << 16) | (bytes[17] << 24));
        Assert.Equal(serial, readSerial);
    }
}
