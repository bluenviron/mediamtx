using System.Buffers.Binary;
using Mediar.Containers.Iff8Svx;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class Iff8SvxRoundTripTests
{
    [Fact]
    public async Task Pcm_S8_RoundTrips_With_All_Text_Chunks()
    {
        const int sr = 22050;
        const int frames = 1024;
        byte[] pcm = BuildSignedSawtooth(frames);

        byte[] iff = await MuxAsync(sr, pcm, t: "Sample", a: "Mediar", c: "Amiga voice", cp: "(c) MIT");

        Assert.Equal((byte)'F', iff[0]);
        Assert.Equal((byte)'O', iff[1]);
        Assert.Equal((byte)'R', iff[2]);
        Assert.Equal((byte)'M', iff[3]);
        Assert.Equal((byte)'8', iff[8]);
        Assert.Equal((byte)'S', iff[9]);
        Assert.Equal((byte)'V', iff[10]);
        Assert.Equal((byte)'X', iff[11]);

        using var src = new MemoryRandomAccessSource(iff);
        using var dx = Iff8SvxDemuxer.Open(src);

        Assert.Equal("8svx", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS8, a.Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(1, a.Channels);

        Assert.Equal("Sample", dx.Metadata.Title);
        Assert.Equal("Mediar", dx.Metadata.Artist);
        Assert.Equal("Amiga voice", dx.Metadata.Comment);
        Assert.Equal("(c) MIT", dx.Metadata.Copyright);

        long total = await SumPayloadAsync(dx);
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public void Throws_On_Missing_Marker()
    {
        byte[] junk = new byte[16];
        using var src = new MemoryRandomAccessSource(junk);
        Assert.Throws<InvalidDataException>(() => Iff8SvxDemuxer.Open(src));
    }

    [Fact]
    public void Throws_On_Too_Small_File()
    {
        using var src = new MemoryRandomAccessSource(new byte[4]);
        Assert.Throws<InvalidDataException>(() => Iff8SvxDemuxer.Open(src));
    }

    [Fact]
    public void Throws_On_Missing_8Svx_Marker_After_FORM()
    {
        byte[] bad = new byte[12];
        bad[0] = (byte)'F'; bad[1] = (byte)'O'; bad[2] = (byte)'R'; bad[3] = (byte)'M';
        bad[8] = (byte)'X'; bad[9] = (byte)'X'; bad[10] = (byte)'X'; bad[11] = (byte)'X';
        Assert.Throws<InvalidDataException>(() => Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(bad)));
    }

    [Fact]
    public void Throws_On_Missing_Vhdr()
    {
        // FORM + 8SVX + BODY only.
        byte[] bytes = BuildIff8Svx(includeVhdr: false, includeBody: true, vhdrSampleRate: 8000, body: new byte[8]);
        Assert.Throws<InvalidDataException>(() => Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(bytes)));
    }

    [Fact]
    public void Throws_On_Missing_Body()
    {
        byte[] bytes = BuildIff8Svx(includeVhdr: true, includeBody: false, vhdrSampleRate: 8000, body: Array.Empty<byte>());
        Assert.Throws<InvalidDataException>(() => Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(bytes)));
    }

    [Fact]
    public void Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => Iff8SvxDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public async Task Open_Path_Works()
    {
        byte[] iff = await MuxAsync(8000, new byte[40], null, null, null, null);
        var path = Path.Combine(Path.GetTempPath(), $"mediar-8svx-{Guid.NewGuid():N}.8svx");
        File.WriteAllBytes(path, iff);
        try
        {
            using var dx = Iff8SvxDemuxer.Open(path);
            Assert.Single(dx.Tracks);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-8svx-missing-{Guid.NewGuid():N}.8svx");
        Assert.Throws<FileNotFoundException>(() => Iff8SvxDemuxer.Open(path));
    }

    [Fact]
    public async Task Duration_Reflects_Body_Length()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2]; // 2 seconds
        byte[] iff = await MuxAsync(sr, pcm, null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        Assert.Equal(TimeSpan.FromSeconds(2), dx.Duration);
    }

    [Fact]
    public async Task Track_DurationTicks_Equals_Body_Length()
    {
        byte[] iff = await MuxAsync(8000, new byte[8000], null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        Assert.Equal(8000, dx.Tracks[0].DurationTicks);
    }

    [Fact]
    public async Task Sample_Pts_Increments_By_Packet_Length()
    {
        const int sr = 8000;
        byte[] iff = await MuxAsync(sr, new byte[400], null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        var ptsList = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { ptsList.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        // ~10 ms packet at 8000 Hz = 80 frames; 400/80 = 5 packets
        Assert.Equal(new long[] { 0, 80, 160, 240, 320 }, ptsList);
    }

    [Fact]
    public async Task Seek_Negative_Clamps_To_Zero()
    {
        byte[] iff = await MuxAsync(8000, new byte[160], null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        await dx.SeekAsync(TimeSpan.FromSeconds(-1));
        Assert.Equal(160, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Seek_Past_End_Clamps_To_End()
    {
        byte[] iff = await MuxAsync(8000, new byte[160], null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        await dx.SeekAsync(TimeSpan.FromHours(1));
        Assert.Equal(0, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Seek_Mid_Body_Skips_Earlier_Frames()
    {
        byte[] iff = await MuxAsync(8000, new byte[800], null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        await dx.SeekAsync(TimeSpan.FromMilliseconds(50));
        // 50 ms at 8 kHz = 400 frames; remaining = 400.
        Assert.Equal(400, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Odd_Body_Length_Is_Padded_To_Even()
    {
        byte[] pcm = new byte[5];
        byte[] iff = await MuxAsync(8000, pcm, null, null, null, null);
        // FORM size in file should be even. The padding byte is included in FORM payload.
        Assert.Equal(0, iff.Length & 1);
    }

    [Fact]
    public async Task Empty_Body_Round_Trips()
    {
        byte[] iff = await MuxAsync(8000, Array.Empty<byte>(), null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        Assert.Equal(0, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Read_Without_Text_Chunks_Has_Empty_Metadata()
    {
        byte[] iff = await MuxAsync(8000, new byte[40], null, null, null, null);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        Assert.True(dx.Metadata.IsEmpty);
    }

    [Fact]
    public async Task Muxer_FormatName_Is_8svx()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Equal("8svx", mux.FormatName);
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Null_Stream()
    {
        Assert.Throws<ArgumentNullException>(() => new Iff8SvxMuxer(null!));
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Non_Writable_Stream()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new Iff8SvxMuxer(ms));
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Non_Seekable_Stream()
    {
        using var ns = new NonSeekableStream();
        Assert.Throws<ArgumentException>(() => new Iff8SvxMuxer(ns));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Null()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Video()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        var t = new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new VideoCodecParameters { Codec = CodecId.H264 },
            TimeBase = new Rational(1, 90000),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(t));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Wrong_Codec()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS16Le, SampleRate = 8000, Channels = 1, BitsPerSample = 16 },
            TimeBase = new Rational(1, 8000),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Multi_Channel()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS8, SampleRate = 8000, Channels = 2, BitsPerSample = 8 },
            TimeBase = new Rational(1, 8000),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Wrong_BitsPerSample()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS8, SampleRate = 8000, Channels = 1, BitsPerSample = 16 },
            TimeBase = new Rational(1, 8000),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_SampleRate_Above_UInt16()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS8, SampleRate = 96000, Channels = 1, BitsPerSample = 8 },
            TimeBase = new Rational(1, 96000),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Zero_SampleRate()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS8, SampleRate = 0, Channels = 1, BitsPerSample = 8 },
            TimeBase = new Rational(1, 1),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Second_Track()
    {
        using var ms = new MemoryStream();
        using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildS8Track(8000));
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildS8Track(8000)));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_Double_StartAsync_Is_NoOp()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildS8Track(8000));
        await mux.StartAsync();
        long before = ms.Length;
        await mux.StartAsync();
        Assert.Equal(before, ms.Length);
    }

    [Fact]
    public async Task Muxer_FinishAsync_Is_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildS8Track(8000));
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync();
    }

    [Fact]
    public async Task Muxer_WriteSample_Auto_Starts()
    {
        // WriteSample before Start should StartAsync implicitly.
        var ms = new MemoryStream();
        await using (var mux = new Iff8SvxMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildS8Track(8000));
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 10, IsKeyFrame = true, Data = new byte[10],
            });
        }
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray()));
        Assert.Equal(10, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Keeps_Stream_Open()
    {
        var ms = new MemoryStream();
        await using (var mux = new Iff8SvxMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildS8Track(8000));
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        _ = ms.Length;
        ms.Dispose();
    }

    [Fact]
    public async Task Muxer_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        await using (var mux = new Iff8SvxMuxer(ms, leaveOpen: false))
        {
            mux.AddTrack(BuildS8Track(8000));
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public void Demuxer_Dispose_Is_Idempotent()
    {
        byte[] iff = BuildIff8Svx(includeVhdr: true, includeBody: true, vhdrSampleRate: 8000, body: new byte[8]);
        var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff), ownsSource: true);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] iff = BuildIff8Svx(includeVhdr: true, includeBody: true, vhdrSampleRate: 8000, body: new byte[8]);
        var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff), ownsSource: true);
        await dx.DisposeAsync();
    }

    [Fact]
    public async Task Vhdr_Compression_1_Maps_To_Fibonacci()
    {
        byte[] iff = BuildIff8Svx(includeVhdr: true, includeBody: true, vhdrSampleRate: 8000, body: new byte[8], compression: 1);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        var a = (AudioCodecParameters)dx.Tracks[0].Codec;
        Assert.Equal(CodecId.Fibonacci8Svx, a.Codec);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Vhdr_Compression_Unknown_Maps_To_Unknown_Codec()
    {
        byte[] iff = BuildIff8Svx(includeVhdr: true, includeBody: true, vhdrSampleRate: 8000, body: new byte[8], compression: 99);
        using var dx = Iff8SvxDemuxer.Open(new MemoryRandomAccessSource(iff));
        var a = (AudioCodecParameters)dx.Tracks[0].Codec;
        Assert.Equal(CodecId.Unknown, a.Codec);
        await Task.CompletedTask;
    }

    // -------------------- helpers --------------------

    private static byte[] BuildSignedSawtooth(int frames)
    {
        byte[] pcm = new byte[frames];
        for (int i = 0; i < frames; i++)
        {
            pcm[i] = (byte)(sbyte)(((i * 5) & 0xFF) - 128);
        }
        return pcm;
    }

    private static async Task<byte[]> MuxAsync(int sr, byte[] pcm, string? t, string? a, string? c, string? cp)
    {
        await using var ms = new MemoryStream();
        await using (var mux = new Iff8SvxMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildS8Track(sr));
            if (t is not null) mux.SetTitle(t);
            if (a is not null) mux.SetArtist(a);
            if (c is not null) mux.SetComment(c);
            if (cp is not null) mux.SetCopyright(cp);
            await mux.StartAsync();
            if (pcm.Length > 0)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = 0, Dts = 0, Duration = pcm.Length, IsKeyFrame = true, Data = pcm,
                });
            }
            await mux.FinishAsync();
        }
        return ms.ToArray();
    }

    private static MediaTrack BuildS8Track(int sampleRate)
    {
        return new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS8, SampleRate = sampleRate, Channels = 1, BitsPerSample = 8 },
            TimeBase = new Rational(1, sampleRate),
        };
    }

    private static async Task<long> SumPayloadAsync(Iff8SvxDemuxer dx)
    {
        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        return total;
    }

    private static byte[] BuildIff8Svx(bool includeVhdr, bool includeBody, int vhdrSampleRate, byte[] body, byte compression = 0)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'O', (byte)'R', (byte)'M' });
        long sizePos = ms.Position;
        Span<byte> sizeBuf = stackalloc byte[4];
        ms.Write(sizeBuf); // placeholder
        ms.Write(new byte[] { (byte)'8', (byte)'S', (byte)'V', (byte)'X' });

        if (includeVhdr)
        {
            ms.Write(new byte[] { (byte)'V', (byte)'H', (byte)'D', (byte)'R' });
            WriteBE32(ms, 20);
            byte[] v = new byte[20];
            BinaryPrimitives.WriteUInt32BigEndian(v.AsSpan(0, 4), (uint)body.Length); // OneShot
            BinaryPrimitives.WriteUInt16BigEndian(v.AsSpan(12, 2), (ushort)vhdrSampleRate);
            v[14] = 1;
            v[15] = compression;
            BinaryPrimitives.WriteUInt32BigEndian(v.AsSpan(16, 4), 0x10000);
            ms.Write(v);
        }
        if (includeBody)
        {
            ms.Write(new byte[] { (byte)'B', (byte)'O', (byte)'D', (byte)'Y' });
            WriteBE32(ms, (uint)body.Length);
            ms.Write(body);
            if ((body.Length & 1) != 0) ms.WriteByte(0);
        }

        long end = ms.Position;
        ms.Position = sizePos;
        BinaryPrimitives.WriteUInt32BigEndian(sizeBuf, (uint)(end - sizePos - 4));
        ms.Write(sizeBuf);
        ms.Position = end;
        return ms.ToArray();
    }

    private static void WriteBE32(Stream s, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(b, v);
        s.Write(b);
    }

    private sealed class NonSeekableStream : Stream
    {
        public override bool CanRead => false;
        public override bool CanSeek => false;
        public override bool CanWrite => true;
        public override long Length => throw new NotSupportedException();
        public override long Position { get => throw new NotSupportedException(); set => throw new NotSupportedException(); }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count) => throw new NotSupportedException();
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) { }
    }
}
