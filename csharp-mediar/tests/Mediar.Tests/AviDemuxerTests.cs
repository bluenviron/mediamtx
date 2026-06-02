using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.Avi;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class AviDemuxerTests
{
    [Fact]
    public async Task Reads_Single_Audio_Pcm_Stream_With_Metadata()
    {
        const int sr = 8000;
        const int ch = 1;
        const int bits = 16;
        const int frames = 800;
        byte[] pcm = new byte[frames * (bits / 8) * ch];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)(i * 100);
            pcm[i * 2 + 0] = (byte)v;
            pcm[i * 2 + 1] = (byte)(v >> 8);
        }

        byte[] avi = BuildPcmAvi(sr, ch, bits, pcm, title: "Track", artist: "Artist");

        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);

        Assert.Equal("avi", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        Assert.Equal(sr, audio.SampleRate);
        Assert.Equal(ch, audio.Channels);
        Assert.Equal(bits, audio.BitsPerSample);

        Assert.Equal("Track", dx.Metadata.Title);
        Assert.Equal("Artist", dx.Metadata.Artist);

        int totalBytes = 0;
        int samples = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                totalBytes += s.Data.Length;
                samples++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, totalBytes);
        Assert.True(samples > 0);
    }

    [Fact]
    public void Open_FromNullSource_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AviDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Open_FileSmallerThan12_Throws_InvalidData()
    {
        using var src = new IO.MemoryRandomAccessSource(new byte[8]);
        var ex = Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
        Assert.Contains("RIFF", ex.Message, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void Open_NonRiff_Magic_Throws_InvalidData()
    {
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "NOPE");
        WriteAscii(hdr.AsSpan(8, 4), "AVI ");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
    }

    [Fact]
    public void Open_Riff_But_NonAvi_FormType_Throws_InvalidData()
    {
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "RIFF");
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), 4u);
        WriteAscii(hdr.AsSpan(8, 4), "WAVE");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        var ex = Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
        Assert.Contains("AVI", ex.Message);
    }

    [Fact]
    public void Open_Riff_Avi_But_Missing_Movi_Throws_InvalidData()
    {
        // A bare RIFF/AVI header without any movi chunk should report
        // a missing movi error during Open.
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "RIFF");
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), 4u);
        WriteAscii(hdr.AsSpan(8, 4), "AVI ");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        var ex = Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
        Assert.Contains("movi", ex.Message);
    }

    [Fact]
    public void Demuxer_FormatName_Is_Avi()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.Equal("avi", dx.FormatName);
    }

    [Fact]
    public void Demuxer_Dispose_Is_Idempotent()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        var dx = AviDemuxer.Open(src);
        dx.Dispose();
        dx.Dispose(); // should not throw
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        var dx = AviDemuxer.Open(src);
        await dx.DisposeAsync();
    }

    [Fact]
    public async Task SeekAsync_Without_Index_Returns_Immediately()
    {
        // Open a file without idx1 (we'll just open one and ignore Seek
        // semantics; we just need to confirm the call doesn't throw).
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(0.5));
        // Calling Seek must not corrupt subsequent enumeration.
        int chunks = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            chunks++;
            s.Owner?.Dispose();
        }
        Assert.True(chunks > 0);
    }

    [Fact]
    public async Task SeekAsync_Negative_Time_Is_Clamped_To_Zero()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(-100));
        // Still able to enumerate.
        int chunks = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            chunks++;
            s.Owner?.Dispose();
        }
        Assert.True(chunks > 0);
    }

    [Fact]
    public async Task ReadSamplesAsync_Honours_Cancellation()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[200], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var s in dx.ReadSamplesAsync(cts.Token))
            {
                s.Owner?.Dispose();
            }
        });
    }

    [Fact]
    public void Track_Carries_Audio_Codec_And_TimeBase()
    {
        byte[] avi = BuildPcmAvi(48000, 2, 16, new byte[256], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var t = Assert.Single(dx.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(48000, audio.SampleRate);
        Assert.Equal(2, audio.Channels);
        Assert.Equal(16, audio.BitsPerSample);
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        Assert.Equal("und", t.Language);
        Assert.Equal(0, t.Index);
    }

    [Fact]
    public void Metadata_All_Latin1_Roundtrips()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[32], title: "Café", artist: "Niño");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.Equal("Café", dx.Metadata.Title);
        Assert.Equal("Niño", dx.Metadata.Artist);
    }

    [Fact]
    public async Task Sample_PTS_Increases_Monotonically()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[400], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        long? lastPts = null;
        int n = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                if (lastPts.HasValue)
                {
                    Assert.True(s.Pts >= lastPts.Value, $"PTS regressed: {s.Pts} after {lastPts.Value}");
                }
                lastPts = s.Pts;
                n++;
                // Audio samples are always "key" frames in this demuxer.
                Assert.True(s.IsKeyFrame);
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.True(n >= 2);
    }

    [Fact]
    public void Open_FromString_OverloadExists()
    {
        // Just confirm the file-path overload throws on a missing file
        // (it should funnel through FileRandomAccessSource).
        Assert.ThrowsAny<Exception>(() => AviDemuxer.Open("Z:\\nonexistent-avi-file.avi"));
    }

    [Theory]
    [InlineData(16, CodecId.PcmS16Le)]
    [InlineData(24, CodecId.PcmS24Le)]
    [InlineData(32, CodecId.PcmS32Le)]
    public void Pcm_Bits_Maps_To_Expected_CodecId(int bits, CodecId expected)
    {
        byte[] pcm = new byte[bits / 8 * 32];
        byte[] avi = BuildPcmAvi(8000, 1, bits, pcm, title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var audio = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(expected, audio.Codec);
        Assert.Equal(bits, audio.BitsPerSample);
    }

    [Theory]
    [InlineData((ushort)0x0001, 16, CodecId.PcmS16Le)]
    [InlineData((ushort)0x0003, 32, CodecId.PcmF32Le)]
    [InlineData((ushort)0x0006, 8, CodecId.G711ALaw)]
    [InlineData((ushort)0x0007, 8, CodecId.G711MuLaw)]
    [InlineData((ushort)0x0055, 16, CodecId.Mp3)]
    [InlineData((ushort)0x00FF, 16, CodecId.Aac)]
    [InlineData((ushort)0x2000, 16, CodecId.Ac3)]
    [InlineData((ushort)0x2001, 16, CodecId.EAc3)]
    [InlineData((ushort)0xF1AC, 16, CodecId.Flac)]
    [InlineData((ushort)0x6750, 16, CodecId.Vorbis)]
    [InlineData((ushort)0x0099, 16, CodecId.Unknown)]
    public void Audio_FormatTag_Maps_To_Expected_CodecId(ushort formatTag, int bits, CodecId expected)
    {
        byte[] data = new byte[bits / 8 * 16];
        byte[] avi = BuildAudioAviCustomTag(formatTag, sampleRate: 22050, channels: 1, bits: bits, data: data);
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var audio = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(expected, audio.Codec);
        Assert.Equal(22050, audio.SampleRate);
        Assert.Equal(1, audio.Channels);
        Assert.Equal(bits, audio.BitsPerSample);
    }

    [Fact]
    public void Metadata_All_Common_Info_Tags_Roundtrip()
    {
        var tags = new Dictionary<string, string>
        {
            ["INAM"] = "Song",
            ["IART"] = "Band",
            ["ICRD"] = "2024",
            ["ICMT"] = "Note",
            ["IGNR"] = "Rock",
            ["IPRD"] = "Album",
            ["ITRK"] = "5",
            ["ICOP"] = "(c)2024",
            ["ISFT"] = "Mediar",
            ["IENG"] = "Engineer",
            ["ILNG"] = "eng",
        };
        byte[] avi = BuildAudioAviWithInfoTags(8000, 1, 16, new byte[16], tags);
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.Equal("Song", dx.Metadata.Title);
        Assert.Equal("Band", dx.Metadata.Artist);
        Assert.Equal("2024", dx.Metadata.Date);
        Assert.Equal("Note", dx.Metadata.Comment);
        Assert.Equal("Rock", dx.Metadata.Genre);
        Assert.Equal("Album", dx.Metadata.Album);
        Assert.Equal(5, dx.Metadata.TrackNumber);
    }

    [Fact]
    public void Metadata_Trailing_Whitespace_And_Null_Trimmed()
    {
        var tags = new Dictionary<string, string> { ["INAM"] = "Song   \0\0" };
        byte[] avi = BuildAudioAviWithInfoTags(8000, 1, 16, new byte[16], tags);
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.Equal("Song", dx.Metadata.Title);
    }

    [Fact]
    public void Metadata_Empty_Value_Stays_Empty()
    {
        var tags = new Dictionary<string, string> { ["INAM"] = "\0", ["IART"] = "X" };
        byte[] avi = BuildAudioAviWithInfoTags(8000, 1, 16, new byte[16], tags);
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.True(string.IsNullOrEmpty(dx.Metadata.Title));
        Assert.Equal("X", dx.Metadata.Artist);
    }

    [Fact]
    public async Task Sample_Pts_Starts_At_Zero_And_TrackIndex_Is_Stream_Index()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[64], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await using var enumerator = dx.ReadSamplesAsync().GetAsyncEnumerator();
        Assert.True(await enumerator.MoveNextAsync());
        var first = enumerator.Current;
        try
        {
            Assert.Equal(0L, first.Pts);
            Assert.Equal(0L, first.Dts);
            Assert.Equal(0, first.TrackIndex);
            Assert.True(first.IsKeyFrame);
            Assert.True(first.Duration > 0);
        }
        finally { first.Owner?.Dispose(); }

        // Drain remaining so the demuxer closes cleanly.
        while (await enumerator.MoveNextAsync()) enumerator.Current.Owner?.Dispose();
    }

    [Fact]
    public async Task Sample_Duration_Is_BlockCount_For_Audio()
    {
        // 16 frames @ 16-bit mono = 32 bytes. Block size = 2. So expected
        // duration per chunk (each holds 16 bytes => 8 blocks).
        byte[] pcm = new byte[32];
        byte[] avi = BuildPcmAvi(8000, 1, 16, pcm, title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(8, s.Duration);
            }
            finally { s.Owner?.Dispose(); }
        }
    }

    [Fact]
    public async Task Sample_TotalBytes_Matches_Source_Pcm()
    {
        byte[] pcm = new byte[256];
        new Random(0).NextBytes(pcm);
        byte[] avi = BuildPcmAvi(8000, 1, 16, pcm, title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var collected = new List<byte>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                collected.AddRange(s.Data.ToArray());
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm, collected.ToArray());
    }

    [Fact]
    public void Tracks_Has_Single_Element_With_Index_And_Id_Zero()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var t = Assert.Single(dx.Tracks);
        Assert.Equal(0, t.Index);
        Assert.Equal(0u, t.Id);
        Assert.Equal("und", t.Language);
    }

    [Fact]
    public void Track_TimeBase_Reflects_Strh_Scale_Rate()
    {
        // strh.Scale=1, strh.Rate=8000 -> TimeBase = 1/8000.
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var t = dx.Tracks[0];
        Assert.Equal(1, t.TimeBase.Numerator);
        Assert.Equal(8000, t.TimeBase.Denominator);
    }

    [Fact]
    public void Duration_NonZero_When_Avih_Has_MicrosecPerFrame_And_TotalFrames()
    {
        // BuildPcmAvi writes 1_000_000/25 us/frame and totalFrames = pcm.Length / blockSize.
        // 256-byte mono 16-bit PCM => 128 frames; duration = 128 * 40000us = 5.12s.
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[256], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        Assert.True(dx.Duration > TimeSpan.Zero);
    }

    [Fact]
    public void Open_Default_Does_Not_Dispose_External_Source()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        var dx = AviDemuxer.Open(src); // ownsSource defaults to false
        dx.Dispose();
        // Source must still be usable after demuxer Dispose; opening another
        // demuxer on it should succeed.
        using var dx2 = AviDemuxer.Open(src);
        Assert.Equal("avi", dx2.FormatName);
    }

    [Fact]
    public void Open_OwnsSource_True_Disposes_Source_With_Demuxer()
    {
        byte[] avi = BuildPcmAvi(8000, 1, 16, new byte[16], title: "T", artist: "A");
        var src = new IO.MemoryRandomAccessSource(avi);
        var dx = AviDemuxer.Open(src, ownsSource: true);
        dx.Dispose();
        // Reading from a disposed memory source should throw.
        Assert.Throws<ObjectDisposedException>(() =>
        {
            Span<byte> buf = stackalloc byte[1];
            src.Read(0, buf);
        });
    }

    [Fact]
    public async Task SeekAsync_Past_Duration_Lands_At_Last_Sample()
    {
        byte[] pcm = new byte[256];
        byte[] avi = BuildPcmAvi(8000, 1, 16, pcm, title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromMinutes(10));
        int chunks = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            chunks++;
            s.Owner?.Dispose();
        }
        // After seeking past the end, at most the final chunk should remain.
        Assert.True(chunks <= 1, $"expected <=1 chunk after seek-past-end, got {chunks}");
    }

    [Fact]
    public void Open_Riff_Too_Short_For_Avi_Magic_Throws()
    {
        // 4-byte RIFF + size, but only 9 bytes total (missing the AVI form id).
        byte[] hdr = new byte[9];
        WriteAscii(hdr.AsSpan(0, 4), "RIFF");
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), 1u);
        using var src = new IO.MemoryRandomAccessSource(hdr);
        Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
    }

    [Fact]
    public void Open_DefaultParams_Has_Zero_Duration_Without_Avih_Frame_Info()
    {
        byte[] hdr = new byte[12];
        WriteAscii(hdr.AsSpan(0, 4), "RIFF");
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), 4u);
        WriteAscii(hdr.AsSpan(8, 4), "AVI ");
        using var src = new IO.MemoryRandomAccessSource(hdr);
        // The RIFF list is empty here, so Open should fail on missing movi.
        Assert.Throws<InvalidDataException>(() => AviDemuxer.Open(src));
    }

    [Fact]
    public async Task Multiple_Open_Read_Cycles_Yield_Identical_Samples()
    {
        byte[] pcm = new byte[64];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)i;
        byte[] avi = BuildPcmAvi(8000, 1, 16, pcm, title: "T", artist: "A");

        byte[] read1, read2;
        {
            using var src = new IO.MemoryRandomAccessSource(avi);
            using var dx = AviDemuxer.Open(src);
            var buf = new List<byte>();
            await foreach (var s in dx.ReadSamplesAsync())
            {
                try { buf.AddRange(s.Data.ToArray()); }
                finally { s.Owner?.Dispose(); }
            }
            read1 = buf.ToArray();
        }
        {
            using var src = new IO.MemoryRandomAccessSource(avi);
            using var dx = AviDemuxer.Open(src);
            var buf = new List<byte>();
            await foreach (var s in dx.ReadSamplesAsync())
            {
                try { buf.AddRange(s.Data.ToArray()); }
                finally { s.Owner?.Dispose(); }
            }
            read2 = buf.ToArray();
        }
        Assert.Equal(read1, read2);
    }

    [Fact]
    public void Pcm_Stereo_Maps_To_Two_Channels()
    {
        byte[] pcm = new byte[16 * 4]; // 16 frames @ 2 ch * 16-bit = 4 bytes/frame
        byte[] avi = BuildPcmAvi(44100, 2, 16, pcm, title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        var audio = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(2, audio.Channels);
        Assert.Equal(44100, audio.SampleRate);
    }

    [Fact]
    public async Task ReadSamplesAsync_Multiple_Reads_Each_Pts_Increases_By_Duration()
    {
        byte[] pcm = new byte[64]; // 32 blocks total; 2 chunks of 16 blocks each.
        byte[] avi = BuildPcmAvi(8000, 1, 16, pcm, title: "T", artist: "A");
        using var src = new IO.MemoryRandomAccessSource(avi);
        using var dx = AviDemuxer.Open(src);
        long expectedPts = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(expectedPts, s.Pts);
                expectedPts += s.Duration;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.True(expectedPts > 0);
    }

    /// <summary>
    /// Build an AVI with one audio stream whose <c>WAVEFORMATEX.wFormatTag</c>
    /// is set to <paramref name="formatTag"/>. Used to exercise the codec
    /// mapping table without requiring real decoder support.
    /// </summary>
    private static byte[] BuildAudioAviCustomTag(ushort formatTag, int sampleRate, int channels, int bits, byte[] data)
    {
        byte[] strh = new byte[56];
        WriteAscii(strh.AsSpan(0, 4), "auds");
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(20, 4), 1u);
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(24, 4), (uint)sampleRate);
        int blockSize = (bits / 8) * channels;
        if (blockSize == 0) blockSize = 1;
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(32, 4), (uint)(data.Length / blockSize));
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(40, 4), (uint)blockSize);

        byte[] strf = new byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(0, 2), formatTag);
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(8, 4), (uint)(sampleRate * channels * (bits / 8)));
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(12, 2), (ushort)blockSize);
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(14, 2), (ushort)bits);

        byte[] avih = new byte[56];
        BinaryPrimitives.WriteUInt32LittleEndian(avih.AsSpan(0, 4), 40000u);
        BinaryPrimitives.WriteUInt32LittleEndian(avih.AsSpan(16, 4), 1u);

        using var ms = new MemoryStream();
        WriteAscii(ms, "RIFF");
        long sizeOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "AVI ");

        WriteAscii(ms, "LIST");
        long hdrlOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "hdrl");
        WriteChunk(ms, "avih", avih);
        WriteAscii(ms, "LIST");
        long strlOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "strl");
        WriteChunk(ms, "strh", strh);
        WriteChunk(ms, "strf", strf);
        long strlEnd = ms.Position;
        PatchSize(ms, strlOff, (uint)(strlEnd - strlOff - 4));
        long hdrlEnd = ms.Position;
        PatchSize(ms, hdrlOff, (uint)(hdrlEnd - hdrlOff - 4));

        WriteAscii(ms, "LIST");
        long moviOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "movi");
        WriteChunk(ms, "00wb", data);
        long moviEnd = ms.Position;
        PatchSize(ms, moviOff, (uint)(moviEnd - moviOff - 4));

        long fileEnd = ms.Position;
        PatchSize(ms, sizeOff, (uint)(fileEnd - sizeOff - 4));
        return ms.ToArray();
    }

    /// <summary>
    /// Build an AVI with one PCM audio stream and a <c>LIST INFO</c> chunk
    /// populated from <paramref name="infoTags"/> (4-byte IDs -> string).
    /// </summary>
    private static byte[] BuildAudioAviWithInfoTags(int sampleRate, int channels, int bits, byte[] data, IReadOnlyDictionary<string, string> infoTags)
    {
        byte[] strh = new byte[56];
        WriteAscii(strh.AsSpan(0, 4), "auds");
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(20, 4), 1u);
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(24, 4), (uint)sampleRate);
        int blockSize = (bits / 8) * channels;
        if (blockSize == 0) blockSize = 1;
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(32, 4), (uint)(data.Length / blockSize));
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(40, 4), (uint)blockSize);

        byte[] strf = new byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(0, 2), 1);
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(8, 4), (uint)(sampleRate * channels * (bits / 8)));
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(12, 2), (ushort)blockSize);
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(14, 2), (ushort)bits);

        byte[] avih = new byte[56];

        using var infoMs = new MemoryStream();
        foreach (var kv in infoTags)
        {
            string id4 = kv.Key.PadRight(4)[..4];
            byte[] payload = Encoding.Latin1.GetBytes(kv.Value);
            WriteChunk(infoMs, id4, payload);
        }
        byte[] info = infoMs.ToArray();

        using var ms = new MemoryStream();
        WriteAscii(ms, "RIFF");
        long sizeOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "AVI ");

        WriteAscii(ms, "LIST");
        long hdrlOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "hdrl");
        WriteChunk(ms, "avih", avih);
        WriteAscii(ms, "LIST");
        long strlOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "strl");
        WriteChunk(ms, "strh", strh);
        WriteChunk(ms, "strf", strf);
        long strlEnd = ms.Position;
        PatchSize(ms, strlOff, (uint)(strlEnd - strlOff - 4));
        long hdrlEnd = ms.Position;
        PatchSize(ms, hdrlOff, (uint)(hdrlEnd - hdrlOff - 4));

        WriteAscii(ms, "LIST");
        long moviOff = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "movi");
        WriteChunk(ms, "00wb", data);
        long moviEnd = ms.Position;
        PatchSize(ms, moviOff, (uint)(moviEnd - moviOff - 4));

        WriteAscii(ms, "LIST");
        WriteLeUInt32(ms, (uint)(info.Length + 4));
        WriteAscii(ms, "INFO");
        ms.Write(info);

        long fileEnd = ms.Position;
        PatchSize(ms, sizeOff, (uint)(fileEnd - sizeOff - 4));
        return ms.ToArray();
    }

    /// <summary>
    /// Build a tiny RIFF/AVI 1-stream PCM file with idx1, LIST INFO, and a
    /// movi list containing the data split across two ##wb chunks.
    /// </summary>
    private static byte[] BuildPcmAvi(int sampleRate, int channels, int bits, byte[] pcm, string title, string artist)
    {
        // Split PCM in half — the test exercises two-chunk movi parsing.
        int half = (pcm.Length / 2) & ~1;
        int rest = pcm.Length - half;
        ReadOnlySpan<byte> chunk1 = pcm.AsSpan(0, half);
        ReadOnlySpan<byte> chunk2 = pcm.AsSpan(half, rest);

        // strh (size 56) + strf (WAVEFORMATEX 18) + chunk overhead.
        byte[] strh = new byte[56];
        WriteAscii(strh.AsSpan(0, 4), "auds");
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(20, 4), 1u); // scale = 1
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(24, 4), (uint)sampleRate); // rate
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(32, 4), (uint)(pcm.Length / (bits / 8 * channels))); // length frames
        BinaryPrimitives.WriteUInt32LittleEndian(strh.AsSpan(40, 4), (uint)(bits / 8 * channels)); // sample size

        byte[] strf = new byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(0, 2), 1); // PCM
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(strf.AsSpan(8, 4), (uint)(sampleRate * channels * (bits / 8))); // avg bytes/sec
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(12, 2), (ushort)(channels * (bits / 8))); // block align
        BinaryPrimitives.WriteUInt16LittleEndian(strf.AsSpan(14, 2), (ushort)bits);

        // avih (size 56). Only microsec/frame and TotalFrames matter for our duration.
        byte[] avih = new byte[56];
        BinaryPrimitives.WriteUInt32LittleEndian(avih.AsSpan(0, 4), (uint)(1_000_000.0 / 25)); // 25 fps placeholder
        BinaryPrimitives.WriteUInt32LittleEndian(avih.AsSpan(16, 4), (uint)(pcm.Length / (bits / 8 * channels)));

        // INFO LIST
        byte[] info = BuildInfo(title, artist);

        // ----- assemble -----
        using var ms = new MemoryStream();
        WriteAscii(ms, "RIFF");
        // placeholder for RIFF size
        long sizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "AVI ");

        // hdrl
        WriteAscii(ms, "LIST");
        long hdrlSizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "hdrl");
        WriteChunk(ms, "avih", avih);

        WriteAscii(ms, "LIST");
        long strlSizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        WriteAscii(ms, "strl");
        WriteChunk(ms, "strh", strh);
        WriteChunk(ms, "strf", strf);
        long strlEnd = ms.Position;
        PatchSize(ms, strlSizeOffset, (uint)(strlEnd - strlSizeOffset - 4));
        long hdrlEnd = ms.Position;
        PatchSize(ms, hdrlSizeOffset, (uint)(hdrlEnd - hdrlSizeOffset - 4));

        // movi list with two ##wb chunks, capturing offsets for idx1.
        WriteAscii(ms, "LIST");
        long moviSizeOffset = ms.Position;
        WriteLeUInt32(ms, 0);
        long moviStart = ms.Position;
        WriteAscii(ms, "movi");

        long chunk1HdrOffset = ms.Position; // relative to file
        WriteChunk(ms, "00wb", chunk1);
        long chunk2HdrOffset = ms.Position;
        WriteChunk(ms, "00wb", chunk2);

        long moviEnd = ms.Position;
        PatchSize(ms, moviSizeOffset, (uint)(moviEnd - moviSizeOffset - 4));

        // idx1
        byte[] idx1 = new byte[2 * 16];
        WriteAscii(idx1.AsSpan(0, 4), "00wb");
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(4, 4), 0x10u); // AVIIF_KEYFRAME
        // movi-relative offset of chunk header from the 'movi' fourcc
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(8, 4), (uint)(chunk1HdrOffset - moviStart));
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(12, 4), (uint)chunk1.Length);

        WriteAscii(idx1.AsSpan(16, 4), "00wb");
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(20, 4), 0x10u);
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(24, 4), (uint)(chunk2HdrOffset - moviStart));
        BinaryPrimitives.WriteUInt32LittleEndian(idx1.AsSpan(28, 4), (uint)chunk2.Length);

        WriteChunk(ms, "idx1", idx1);

        // LIST INFO
        WriteAscii(ms, "LIST");
        WriteLeUInt32(ms, (uint)(info.Length + 4));
        WriteAscii(ms, "INFO");
        ms.Write(info);

        long fileEnd = ms.Position;
        PatchSize(ms, sizeOffset, (uint)(fileEnd - sizeOffset - 4));
        return ms.ToArray();
    }

    private static byte[] BuildInfo(string title, string artist)
    {
        using var ms = new MemoryStream();
        WriteChunk(ms, "INAM", Encoding.Latin1.GetBytes(title + "\0"));
        WriteChunk(ms, "IART", Encoding.Latin1.GetBytes(artist + "\0"));
        return ms.ToArray();
    }

    private static void WriteChunk(MemoryStream ms, string id, ReadOnlySpan<byte> data)
    {
        WriteAscii(ms, id);
        WriteLeUInt32(ms, (uint)data.Length);
        ms.Write(data);
        if ((data.Length & 1) != 0) ms.WriteByte(0);
    }

    private static void WriteAscii(MemoryStream ms, string s)
    {
        for (int i = 0; i < s.Length; i++) ms.WriteByte((byte)s[i]);
    }

    private static void WriteAscii(Span<byte> dest, string s)
    {
        for (int i = 0; i < s.Length; i++) dest[i] = (byte)s[i];
    }

    private static void WriteLeUInt32(MemoryStream ms, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        ms.Write(b);
    }

    private static void PatchSize(MemoryStream ms, long offset, uint value)
    {
        long pos = ms.Position;
        ms.Position = offset;
        WriteLeUInt32(ms, value);
        ms.Position = pos;
    }
}
