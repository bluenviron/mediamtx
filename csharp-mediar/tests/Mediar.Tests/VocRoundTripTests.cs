using System.Buffers.Binary;
using Mediar.Containers.Voc;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class VocRoundTripTests
{
    private static MediaTrack BuildTrack(
        CodecId codec = CodecId.PcmU8,
        int sampleRate = 22050,
        int channels = 1,
        int bitsPerSample = 8) => new()
    {
        Index = 0,
        Id = 1,
        TimeBase = new Rational(1, sampleRate),
        Codec = new AudioCodecParameters
        {
            Codec = codec, SampleRate = sampleRate, Channels = channels, BitsPerSample = bitsPerSample,
        },
    };

    private static MediaSample BuildSample(byte[] data, int duration) => new()
    {
        TrackIndex = 0,
        Pts = 0,
        Dts = 0,
        Duration = duration,
        IsKeyFrame = true,
        Data = data,
    };

    [Fact]
    public async Task PcmU8_RoundTrips_Through_VocMuxer_V9()
    {
        const int sr = 22050;
        const int frames = 4096;
        byte[] pcm = new byte[frames];
        for (int i = 0; i < frames; i++) pcm[i] = (byte)(i & 0xFF);

        byte[] voc;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new VocMuxer(ms, leaveOpen: true);
            mux.AddTrack(BuildTrack());
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildSample(pcm, frames));
            await mux.FinishAsync();
            voc = ms.ToArray();
        }

        Assert.Equal((byte)'C', voc[0]);
        Assert.Equal(0x1A, voc[19]);

        using var src = new MemoryRandomAccessSource(voc);
        using var dx = VocDemuxer.Open(src);

        Assert.Equal("voc", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmU8, a.Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(1, a.Channels);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public void Demuxer_Throws_On_Missing_Magic()
    {
        byte[] junk = new byte[64];
        using var src = new MemoryRandomAccessSource(junk);
        Assert.Throws<InvalidDataException>(() => VocDemuxer.Open(src));
    }

    // -------------------- Muxer ctor / lifecycle --------------------

    [Fact]
    public void Muxer_Constructor_Null_Output_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new VocMuxer(null!));
    }

    [Fact]
    public void Muxer_Constructor_NonWritable_Throws()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new VocMuxer(ms));
    }

    [Fact]
    public void Muxer_Constructor_NonSeekable_Throws()
    {
        using var nonSeek = new NonSeekableStream();
        Assert.Throws<ArgumentException>(() => new VocMuxer(nonSeek));
    }

    [Fact]
    public void Muxer_FormatName_Is_Voc()
    {
        using var ms = new MemoryStream();
        using var mux = new VocMuxer(ms, leaveOpen: true);
        Assert.Equal("voc", mux.FormatName);
    }

    [Fact]
    public void Muxer_AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new VocMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void Muxer_AddTrack_Twice_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new VocMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildTrack()));
    }

    [Fact]
    public void Muxer_AddTrack_With_Non_Audio_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new VocMuxer(ms, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0, Id = 1, TimeBase = new Rational(1, 30),
            Codec = new VideoCodecParameters { Codec = CodecId.H264, Width = 320, Height = 240 },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public void Muxer_AddTrack_With_Unsupported_Codec_Throws_NotSupported()
    {
        using var ms = new MemoryStream();
        using var mux = new VocMuxer(ms, leaveOpen: true);
        var track = BuildTrack(codec: CodecId.Aac);
        Assert.Throws<NotSupportedException>(() => mux.AddTrack(track));
    }

    [Theory]
    [InlineData(CodecId.PcmU8, 8)]
    [InlineData(CodecId.PcmS16Le, 16)]
    [InlineData(CodecId.G711ALaw, 8)]
    [InlineData(CodecId.G711MuLaw, 8)]
    public void Muxer_AddTrack_With_Supported_Codecs_Works(CodecId codec, int bits)
    {
        using var ms = new MemoryStream();
        using var mux = new VocMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack(codec, bitsPerSample: bits));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new VocMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_StartAsync_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new VocMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        long firstPosition = ms.Position;
        await mux.StartAsync(); // re-entrant Start is a no-op
        Assert.Equal(firstPosition, ms.Position);
    }

    [Fact]
    public async Task Muxer_WriteSampleAsync_Auto_Starts_When_Not_Started()
    {
        using var ms = new MemoryStream();
        await using (var mux = new VocMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            await mux.WriteSampleAsync(BuildSample(new byte[] { 0x80, 0x81 }, 2));
            await mux.FinishAsync();
        }
        Assert.True(ms.Length > 26 + 16); // header + block-9 header + payload
    }

    [Fact]
    public async Task Muxer_FinishAsync_Without_Start_Is_NoOp()
    {
        using var ms = new MemoryStream();
        await using var mux = new VocMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.FinishAsync();
        Assert.Equal(0, ms.Length);
    }

    [Fact]
    public async Task Muxer_FinishAsync_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new VocMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.StartAsync();
        await mux.WriteSampleAsync(BuildSample(new byte[] { 0x80 }, 1));
        await mux.FinishAsync();
        long lengthAfterFirst = ms.Length;
        await mux.FinishAsync();
        Assert.Equal(lengthAfterFirst, ms.Length);
    }

    [Fact]
    public async Task Muxer_DisposeAsync_With_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new VocMuxer(ms);
        mux.AddTrack(BuildTrack());
        await mux.WriteSampleAsync(BuildSample(new byte[] { 0x80 }, 1));
        await mux.DisposeAsync();
        Assert.Throws<ObjectDisposedException>(() => ms.WriteByte(0));
    }

    [Fact]
    public async Task Muxer_DisposeAsync_With_LeaveOpen_True_Keeps_Stream_Usable()
    {
        var ms = new MemoryStream();
        var mux = new VocMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildTrack());
        await mux.WriteSampleAsync(BuildSample(new byte[] { 0x80 }, 1));
        await mux.DisposeAsync();
        ms.WriteByte(0); // still writable
        ms.Dispose();
    }

    [Fact]
    public void Muxer_Dispose_With_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new VocMuxer(ms);
        mux.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.WriteByte(0));
    }

    [Fact]
    public async Task Header_Contains_Version_1_10_And_Validation_Code()
    {
        using var ms = new MemoryStream();
        await using (var mux = new VocMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            await mux.WriteSampleAsync(BuildSample(new byte[] { 0x80 }, 1));
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        // bytes 22-23: version 0x010A (LE)
        ushort ver = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(22, 2));
        Assert.Equal(0x010A, ver);
        // bytes 24-25: validation code = ~(version + 0x1234) + 0x1234 (mod 0x10000)
        ushort code = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(24, 2));
        ushort expected = unchecked((ushort)(~(0x010A + 0x1234) + 0x1234));
        Assert.Equal(expected, code);
    }

    // -------------------- Codec round-trips --------------------

    [Fact]
    public async Task PcmS16Le_Round_Trip()
    {
        const int sr = 16000;
        const int frames = 1024;
        byte[] pcm = new byte[frames * 2];
        for (int i = 0; i < frames; i++)
        {
            short s = (short)(i * 17);
            BinaryPrimitives.WriteInt16LittleEndian(pcm.AsSpan(i * 2, 2), s);
        }

        byte[] voc;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new VocMuxer(ms, leaveOpen: true);
            mux.AddTrack(BuildTrack(CodecId.PcmS16Le, sampleRate: sr, bitsPerSample: 16));
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildSample(pcm, frames));
            await mux.FinishAsync();
            voc = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(voc);
        using var dx = VocDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.PcmS16Le, a.Codec);
        Assert.Equal(16, a.BitsPerSample);
        Assert.Equal(sr, a.SampleRate);
    }

    [Fact]
    public async Task G711MuLaw_Round_Trip_Detects_Codec()
    {
        const int sr = 8000;
        byte[] payload = new byte[256];
        for (int i = 0; i < 256; i++) payload[i] = (byte)i;

        byte[] voc;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new VocMuxer(ms, leaveOpen: true);
            mux.AddTrack(BuildTrack(CodecId.G711MuLaw, sampleRate: sr, bitsPerSample: 8));
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildSample(payload, 256));
            await mux.FinishAsync();
            voc = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(voc);
        using var dx = VocDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.G711MuLaw, a.Codec);
    }

    [Fact]
    public async Task G711ALaw_Round_Trip_Detects_Codec()
    {
        const int sr = 8000;
        byte[] payload = new byte[64];

        byte[] voc;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new VocMuxer(ms, leaveOpen: true);
            mux.AddTrack(BuildTrack(CodecId.G711ALaw, sampleRate: sr, bitsPerSample: 8));
            await mux.StartAsync();
            await mux.WriteSampleAsync(BuildSample(payload, 64));
            await mux.FinishAsync();
            voc = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(voc);
        using var dx = VocDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.G711ALaw, a.Codec);
    }

    // -------------------- Demuxer surface --------------------

    [Fact]
    public void Demuxer_Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => VocDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Demuxer_Open_Too_Small_Throws()
    {
        using var src = new MemoryRandomAccessSource(new byte[10]);
        Assert.Throws<InvalidDataException>(() => VocDemuxer.Open(src));
    }

    [Fact]
    public void Demuxer_Open_Missing_File_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-voc-missing-{Guid.NewGuid():N}.voc");
        Assert.Throws<FileNotFoundException>(() => VocDemuxer.Open(path));
    }

    [Fact]
    public async Task Demuxer_Open_From_Path_Round_Trips()
    {
        const int frames = 64;
        var path = Path.Combine(Path.GetTempPath(), $"mediar-voc-{Guid.NewGuid():N}.voc");
        try
        {
            await using (var fs = File.Create(path))
            await using (var mux = new VocMuxer(fs, leaveOpen: true))
            {
                mux.AddTrack(BuildTrack());
                await mux.WriteSampleAsync(BuildSample(new byte[frames], frames));
            }
            using var dx = VocDemuxer.Open(path);
            Assert.Equal("voc", dx.FormatName);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public async Task Demuxer_Metadata_Is_Empty()
    {
        using var ms = new MemoryStream();
        await using (var mux = new VocMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            await mux.WriteSampleAsync(BuildSample(new byte[16], 16));
        }
        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = VocDemuxer.Open(src);
        Assert.Same(MediaMetadata.Empty, dx.Metadata);
    }

    [Fact]
    public async Task Demuxer_Duration_Reflects_Sample_Count()
    {
        const int sr = 8000;
        const int frames = 8000; // exactly 1 second
        using var ms = new MemoryStream();
        await using (var mux = new VocMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack(sampleRate: sr));
            await mux.WriteSampleAsync(BuildSample(new byte[frames], frames));
        }
        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = VocDemuxer.Open(src);
        Assert.Equal(TimeSpan.FromSeconds(1), dx.Duration);
    }

    [Fact]
    public async Task Demuxer_SeekAsync_To_End_Skips_All_Blocks()
    {
        const int sr = 8000;
        const int frames = 1000;
        using var ms = new MemoryStream();
        await using (var mux = new VocMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack(sampleRate: sr));
            await mux.WriteSampleAsync(BuildSample(new byte[frames], frames));
        }
        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = VocDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromHours(1));
        await foreach (var s in dx.ReadSamplesAsync())
        {
            s.Owner?.Dispose();
            Assert.Fail("Expected no samples after seek past end.");
        }
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Does_Not_Throw()
    {
        using var ms = new MemoryStream();
        await using (var mux = new VocMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildTrack());
            await mux.WriteSampleAsync(BuildSample(new byte[16], 16));
        }
        var src = new MemoryRandomAccessSource(ms.ToArray());
        var dx = VocDemuxer.Open(src, ownsSource: true);
        await dx.DisposeAsync();
        await dx.DisposeAsync(); // idempotent
    }

    private sealed class NonSeekableStream : Stream
    {
        public override bool CanRead => true;
        public override bool CanSeek => false;
        public override bool CanWrite => true;
        public override long Length => 0;
        public override long Position { get => 0; set { } }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count) => 0;
        public override long Seek(long offset, SeekOrigin origin) => 0;
        public override void SetLength(long value) { }
        public override void Write(byte[] buffer, int offset, int count) { }
    }
}
