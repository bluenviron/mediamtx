using System.Buffers.Binary;
using Mediar.Containers.Caf;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class CafRoundTripTests
{
    [Fact]
    public async Task PcmS16Be_RoundTrips_With_Info_Metadata()
    {
        const int sr = 44100;
        const int ch = 2;
        const int frames = sr / 4;
        byte[] pcm = BuildPcmS16Be(frames, ch);

        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Be, sr, ch, 16), pcm, frames,
            ("title", "Hello CAF"), ("artist", "Mediar"));

        Assert.Equal((byte)'c', caf[0]);
        Assert.Equal((byte)'a', caf[1]);
        Assert.Equal((byte)'f', caf[2]);
        Assert.Equal((byte)'f', caf[3]);

        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        Assert.Equal("caf", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Be, a.Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(ch, a.Channels);
        Assert.Equal(16, a.BitsPerSample);
        Assert.Equal("Hello CAF", dx.Metadata.Title);
        Assert.Equal("Mediar", dx.Metadata.Artist);
        Assert.Equal(pcm.Length, await SumPayloadAsync(dx));
    }

    [Theory]
    [InlineData(CodecId.PcmS8, 8)]
    [InlineData(CodecId.PcmU8, 8)]
    [InlineData(CodecId.PcmS16Le, 16)]
    [InlineData(CodecId.PcmS16Be, 16)]
    [InlineData(CodecId.PcmS24Le, 24)]
    [InlineData(CodecId.PcmS32Le, 32)]
    [InlineData(CodecId.PcmF32Le, 32)]
    public async Task Lpcm_Variants_RoundTrip(CodecId codec, int bps)
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * (bps / 8)];
        byte[] caf = await MuxAsync(BuildAudio(codec, sr, 1, bps), pcm, sr);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        var a = (AudioCodecParameters)dx.Tracks[0].Codec;
        Assert.Equal(codec, a.Codec);
        Assert.Equal(bps, a.BitsPerSample);
        Assert.Equal(pcm.Length, await SumPayloadAsync(dx));
    }

    [Theory]
    [InlineData(CodecId.G711MuLaw)]
    [InlineData(CodecId.G711ALaw)]
    public async Task Companded_Codecs_RoundTrip(CodecId codec)
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr / 2];
        byte[] caf = await MuxAsync(BuildAudio(codec, sr, 1, 8), pcm, sr / 2);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        Assert.Equal(codec, ((AudioCodecParameters)dx.Tracks[0].Codec).Codec);
    }

    [Fact]
    public void Open_Throws_On_Missing_Caff_Marker()
    {
        byte[] junk = new byte[64];
        junk[0] = (byte)'X'; junk[1] = (byte)'X'; junk[2] = (byte)'X'; junk[3] = (byte)'X';
        Assert.Throws<InvalidDataException>(() => CafDemuxer.Open(new MemoryRandomAccessSource(junk)));
    }

    [Fact]
    public void Open_Throws_On_Too_Small_File()
    {
        Assert.Throws<InvalidDataException>(() => CafDemuxer.Open(new MemoryRandomAccessSource(new byte[4])));
    }

    [Fact]
    public void Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => CafDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Open_Throws_When_Desc_Missing()
    {
        // 'caff' header + 'data' chunk with no 'desc'.
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'c', (byte)'a', (byte)'f', (byte)'f', 0, 1, 0, 0 });
        WriteFourCc(ms, "data");
        WriteBE64(ms, 4); // 4-byte edit count only
        WriteBE32(ms, 0);
        Assert.Throws<InvalidDataException>(() => CafDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray())));
    }

    [Fact]
    public void Open_Throws_When_Data_Missing()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'c', (byte)'a', (byte)'f', (byte)'f', 0, 1, 0, 0 });
        WriteFourCc(ms, "desc");
        WriteBE64(ms, 32);
        WriteDescPayload(ms, sampleRate: 8000, formatId: "lpcm", flags: 0x2,
            bytesPerPacket: 2, framesPerPacket: 1, channels: 1, bits: 16);
        Assert.Throws<InvalidDataException>(() => CafDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray())));
    }

    [Fact]
    public async Task Open_Path_Works()
    {
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, 8000, 1, 16), new byte[40], 20);
        var path = Path.Combine(Path.GetTempPath(), $"mediar-caf-{Guid.NewGuid():N}.caf");
        File.WriteAllBytes(path, caf);
        try
        {
            using var dx = CafDemuxer.Open(path);
            Assert.Single(dx.Tracks);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-caf-missing-{Guid.NewGuid():N}.caf");
        Assert.Throws<FileNotFoundException>(() => CafDemuxer.Open(path));
    }

    [Fact]
    public async Task Duration_Reflects_Data_Length()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2 * 2]; // 2s of S16 mono
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, sr, 1, 16), pcm, pcm.Length / 2);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        Assert.Equal(TimeSpan.FromSeconds(2), dx.Duration);
    }

    [Fact]
    public async Task Track_DurationTicks_Equals_Frame_Count()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2];
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, sr, 1, 16), pcm, pcm.Length / 2);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        Assert.Equal(sr, dx.Tracks[0].DurationTicks);
    }

    [Fact]
    public async Task Sample_Pts_Increments_Per_Packet()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr / 2 * 2]; // 0.5s mono S16
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, sr, 1, 16), pcm, pcm.Length / 2);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        long expected = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(expected, s.Pts);
                expected += s.Duration;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length / 2, expected);
    }

    [Fact]
    public async Task Seek_Past_Start_Skips_Frames()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2]; // 1s mono S16
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, sr, 1, 16), pcm, pcm.Length / 2);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        await dx.SeekAsync(TimeSpan.FromMilliseconds(500));
        Assert.Equal(pcm.Length / 2, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Seek_Negative_Clamps_To_Zero()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2];
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, sr, 1, 16), pcm, pcm.Length / 2);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        await dx.SeekAsync(TimeSpan.FromSeconds(-1));
        Assert.Equal(pcm.Length, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Empty_Body_Yields_No_Samples()
    {
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, 8000, 1, 16), Array.Empty<byte>(), 0);
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        Assert.Equal(0, await SumPayloadAsync(dx));
    }

    [Fact]
    public async Task Info_Multiple_Keys_Round_Trip()
    {
        byte[] pcm = new byte[40];
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, 8000, 1, 16), pcm, 20,
            ("title", "T"), ("artist", "A"), ("album", "AL"), ("genre", "G"), ("year", "2024"));
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf));
        Assert.Equal("T", dx.Metadata.Title);
        Assert.Equal("A", dx.Metadata.Artist);
        Assert.Equal("AL", dx.Metadata.Album);
    }

    [Fact]
    public async Task Variable_Packet_Table_Parsed()
    {
        // Build a CAF manually with 'pakt' chunk and 'aac ' codec.
        const int sr = 44100;
        byte[] packet0 = new byte[]  { 0x10, 0x20, 0x30 };
        byte[] packet1 = new byte[5] { 0x40, 0x50, 0x60, 0x70, 0x80 };
        byte[] packet2 = new byte[2] { 0x90, 0xA0 };
        byte[] data = packet0.Concat(packet1).Concat(packet2).ToArray();

        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'c', (byte)'a', (byte)'f', (byte)'f', 0, 1, 0, 0 });

        WriteFourCc(ms, "desc"); WriteBE64(ms, 32);
        WriteDescPayload(ms, sr, "aac ", flags: 0, bytesPerPacket: 0, framesPerPacket: 1024, channels: 2, bits: 0);

        WriteFourCc(ms, "pakt"); WriteBE64(ms, 24 + 3);
        WriteBE64(ms, 3);                    // numberPackets
        WriteBE64(ms, 3072);                 // numberFrames
        WriteBE64(ms, 0);                    // primingFrames + remainder (combined as 8-byte)
        ms.WriteByte(3); ms.WriteByte(5); ms.WriteByte(2); // packet sizes (varint: <128 single byte)

        WriteFourCc(ms, "data");
        WriteBE64(ms, 4 + data.Length);
        WriteBE32(ms, 0);
        ms.Write(data);

        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray()));
        var lens = new List<int>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { lens.Add(s.Data.Length); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Collection(lens,
            x => Assert.Equal(3, x),
            x => Assert.Equal(5, x),
            x => Assert.Equal(2, x));
    }

    [Fact]
    public async Task Variable_Packet_Pts_Uses_FramesPerPacket()
    {
        const int sr = 44100;
        byte[] data = new byte[3 + 5];
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'c', (byte)'a', (byte)'f', (byte)'f', 0, 1, 0, 0 });
        WriteFourCc(ms, "desc"); WriteBE64(ms, 32);
        WriteDescPayload(ms, sr, "aac ", 0, 0, 1024, 2, 0);
        WriteFourCc(ms, "pakt"); WriteBE64(ms, 24 + 2);
        WriteBE64(ms, 2); WriteBE64(ms, 2048); WriteBE64(ms, 0);
        ms.WriteByte(3); ms.WriteByte(5);
        WriteFourCc(ms, "data"); WriteBE64(ms, 4 + data.Length); WriteBE32(ms, 0); ms.Write(data);

        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray()));
        var ptsList = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { ptsList.Add(s.Pts); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(new long[] { 0, 1024 }, ptsList);
    }

    // ---------- Muxer guards ----------

    [Fact]
    public void Muxer_Constructor_Rejects_Null_Stream()
    {
        Assert.Throws<ArgumentNullException>(() => new CafMuxer(null!));
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Non_Writable()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new CafMuxer(ms));
    }

    [Fact]
    public void Muxer_FormatName_Is_Caf()
    {
        using var ms = new MemoryStream();
        using var mux = new CafMuxer(ms, leaveOpen: true);
        Assert.Equal("caf", mux.FormatName);
    }

    [Fact]
    public void Muxer_AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new CafMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void Muxer_AddTrack_Video_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new CafMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new VideoCodecParameters { Codec = CodecId.H264 },
            TimeBase = new Rational(1, 90000),
        }));
    }

    [Fact]
    public void Muxer_AddTrack_Second_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new CafMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16));
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16)));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new CafMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_Double_StartAsync_Is_NoOp()
    {
        await using var ms = new MemoryStream();
        await using var mux = new CafMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16));
        await mux.StartAsync();
        long before = ms.Length;
        await mux.StartAsync();
        Assert.Equal(before, ms.Length);
    }

    [Fact]
    public async Task Muxer_FinishAsync_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new CafMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16));
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync();
    }

    [Fact]
    public async Task Muxer_WriteSample_Auto_Starts()
    {
        var ms = new MemoryStream();
        await using (var mux = new CafMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16));
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 10, IsKeyFrame = true, Data = new byte[10],
            });
        }
        using var dx = CafDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray()));
        Assert.Equal(10, await SumPayloadAsync(dx));
    }

    [Fact]
    public void Muxer_AddInfo_Null_Key_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new CafMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddInfo(null!, "v"));
    }

    [Fact]
    public void Muxer_AddInfo_Empty_Key_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new CafMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentException>(() => mux.AddInfo("", "v"));
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Keeps_Stream()
    {
        var ms = new MemoryStream();
        await using (var mux = new CafMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16));
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
        await using (var mux = new CafMuxer(ms, leaveOpen: false))
        {
            mux.AddTrack(BuildAudioTrack(CodecId.PcmS16Le, 8000, 1, 16));
            await mux.StartAsync();
            await mux.FinishAsync();
        }
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public async Task Muxer_Unsupported_Codec_Throws()
    {
        // Don't use `await using` because failed StartAsync leaves the muxer in a
        // state where DisposeAsync's implicit FinishAsync seeks before stream start.
        await using var ms = new MemoryStream();
        var mux = new CafMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildAudioTrack(CodecId.Mp3, 44100, 2, 16));
        await Assert.ThrowsAsync<NotSupportedException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Demuxer_Dispose_Idempotent()
    {
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, 8000, 1, 16), new byte[16], 8);
        var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf), ownsSource: true);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] caf = await MuxAsync(BuildAudio(CodecId.PcmS16Le, 8000, 1, 16), new byte[16], 8);
        var dx = CafDemuxer.Open(new MemoryRandomAccessSource(caf), ownsSource: true);
        await dx.DisposeAsync();
    }

    // ---------- helpers ----------

    private static byte[] BuildPcmS16Be(int frames, int ch)
    {
        byte[] pcm = new byte[frames * ch * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)((i * 7) & 0x7FFF);
            for (int c = 0; c < ch; c++)
            {
                int o = (i * ch + c) * 2;
                pcm[o] = (byte)(v >> 8); pcm[o + 1] = (byte)v;
            }
        }
        return pcm;
    }

    private static AudioCodecParameters BuildAudio(CodecId codec, int sr, int ch, int bps) => new()
    {
        Codec = codec, SampleRate = sr, Channels = ch, BitsPerSample = bps,
    };

    private static MediaTrack BuildAudioTrack(CodecId codec, int sr, int ch, int bps) => new()
    {
        Index = 0, Id = 1,
        Codec = BuildAudio(codec, sr, ch, bps),
        TimeBase = new Rational(1, sr),
    };

    private static async Task<byte[]> MuxAsync(
        AudioCodecParameters audio, byte[] pcm, int frames, params (string key, string value)[] info)
    {
        await using var ms = new MemoryStream();
        await using (var mux = new CafMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1, TimeBase = new Rational(1, audio.SampleRate), Codec = audio,
            });
            foreach (var (k, v) in info) mux.AddInfo(k, v);
            await mux.StartAsync();
            if (pcm.Length > 0)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = 0, Dts = 0, Duration = frames,
                    IsKeyFrame = true, Data = pcm,
                });
            }
            await mux.FinishAsync();
        }
        return ms.ToArray();
    }

    private static async Task<long> SumPayloadAsync(CafDemuxer dx)
    {
        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        return total;
    }

    private static void WriteFourCc(Stream s, string id)
    {
        for (int i = 0; i < 4; i++) s.WriteByte((byte)id[i]);
    }

    private static void WriteBE32(Stream s, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(b, v);
        s.Write(b);
    }

    private static void WriteBE64(Stream s, long v)
    {
        Span<byte> b = stackalloc byte[8];
        BinaryPrimitives.WriteInt64BigEndian(b, v);
        s.Write(b);
    }

    private static void WriteDescPayload(
        Stream s, int sampleRate, string formatId, uint flags,
        uint bytesPerPacket, uint framesPerPacket, uint channels, uint bits)
    {
        Span<byte> buf = stackalloc byte[32];
        BinaryPrimitives.WriteInt64BigEndian(buf[..8], BitConverter.DoubleToInt64Bits(sampleRate));
        for (int i = 0; i < 4; i++) buf[8 + i] = (byte)formatId[i];
        BinaryPrimitives.WriteUInt32BigEndian(buf[12..16], flags);
        BinaryPrimitives.WriteUInt32BigEndian(buf[16..20], bytesPerPacket);
        BinaryPrimitives.WriteUInt32BigEndian(buf[20..24], framesPerPacket);
        BinaryPrimitives.WriteUInt32BigEndian(buf[24..28], channels);
        BinaryPrimitives.WriteUInt32BigEndian(buf[28..32], bits);
        s.Write(buf);
    }
}
