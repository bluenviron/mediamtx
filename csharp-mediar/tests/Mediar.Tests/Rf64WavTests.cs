using System.Buffers.Binary;
using Mediar.Containers.Wav;
using Xunit;

namespace Mediar.Tests;

public sealed class Rf64WavTests
{
    // Builds a minimal RF64 stream with ds64 + fmt + data, using 0xFFFFFFFF
    // sentinels so the ds64 64-bit sizes are mandatory.
    private static byte[] BuildRf64(int sampleRate, int channels, int bits, byte[] pcm)
    {
        using var ms = new MemoryStream();
        // RIFF header: "RF64" + 0xFFFFFFFF + "WAVE"
        ms.Write("RF64"u8);
        ms.Write(new byte[] { 0xFF, 0xFF, 0xFF, 0xFF });
        ms.Write("WAVE"u8);

        // ds64 chunk: 28-byte payload.
        ms.Write("ds64"u8);
        Span<byte> dsLen = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(dsLen, 28);
        ms.Write(dsLen);
        // riffSize (i64), dataSize (i64), sampleCount (i64), tableLen (i32)
        Span<byte> i64 = stackalloc byte[8];
        BinaryPrimitives.WriteInt64LittleEndian(i64, 0); ms.Write(i64); // riffSize: ignored
        BinaryPrimitives.WriteInt64LittleEndian(i64, pcm.Length); ms.Write(i64);
        BinaryPrimitives.WriteInt64LittleEndian(i64, pcm.Length / (bits / 8 * channels)); ms.Write(i64);
        Span<byte> tableLen = stackalloc byte[4];
        BinaryPrimitives.WriteInt32LittleEndian(tableLen, 0); ms.Write(tableLen);

        // fmt chunk: 16-byte PCM
        ms.Write("fmt "u8);
        Span<byte> fmtLen = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(fmtLen, 16);
        ms.Write(fmtLen);
        Span<byte> fmt = stackalloc byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(fmt[..2], 1); // PCM
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.Slice(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(fmt.Slice(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(fmt.Slice(8, 4), (uint)(sampleRate * channels * bits / 8));
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.Slice(12, 2), (ushort)(channels * bits / 8));
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.Slice(14, 2), (ushort)bits);
        ms.Write(fmt);

        // data chunk with sentinel 0xFFFFFFFF (real size in ds64).
        ms.Write("data"u8);
        ms.Write(new byte[] { 0xFF, 0xFF, 0xFF, 0xFF });
        ms.Write(pcm);
        if ((pcm.Length & 1) != 0) ms.WriteByte(0);

        return ms.ToArray();
    }

    [Fact]
    public async Task Rf64_With_Ds64_Sentinel_Reads_Correct_DataSize()
    {
        const int sr = 8000;
        const int ch = 1;
        const int bits = 16;
        const int frames = 1024;
        byte[] pcm = new byte[frames * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)((i * 13) & 0x7FFF);
            pcm[i * 2] = (byte)v;
            pcm[i * 2 + 1] = (byte)(v >> 8);
        }
        byte[] file = BuildRf64(sr, ch, bits, pcm);

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        Assert.Equal("wav", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Le, a.Codec);
        Assert.Equal(sr, a.SampleRate);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Bw64_Magic_Also_Accepted()
    {
        const int sr = 16000;
        byte[] pcm = new byte[200];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)i;
        byte[] file = BuildRf64(sr, 1, 8, pcm);
        // Patch magic: RF64 → BW64
        file[0] = (byte)'B'; file[1] = (byte)'W'; file[2] = (byte)'6'; file[3] = (byte)'4';

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Equal("wav", dx.FormatName);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Theory]
    [InlineData(8000, 1, 8)]
    [InlineData(11025, 1, 16)]
    [InlineData(22050, 2, 16)]
    [InlineData(44100, 2, 24)]
    [InlineData(48000, 6, 16)]
    [InlineData(96000, 2, 32)]
    public async Task Rf64_RoundTrips_Various_Formats(int sr, int ch, int bits)
    {
        const int frames = 128;
        byte[] pcm = new byte[frames * ch * bits / 8];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i & 0xFF);
        byte[] file = BuildRf64(sr, ch, bits, pcm);

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(ch, a.Channels);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Rf64_Demuxer_FormatName_Is_Wav()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Equal("wav", dx.FormatName);
    }

    [Fact]
    public async Task Rf64_With_Empty_Data_Returns_No_Samples()
    {
        byte[] file = BuildRf64(8000, 1, 16, Array.Empty<byte>());
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { count++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public async Task Rf64_With_Odd_DataSize_Tolerates_Pad_Byte()
    {
        // 8-bit PCM with odd-length data triggers a pad byte after data.
        byte[] pcm = new byte[151];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)i;
        byte[] file = BuildRf64(8000, 1, 8, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Rf64_Track_Has_Audio_Stream_Kind()
    {
        byte[] file = BuildRf64(48000, 2, 16, new byte[400]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Equal(StreamKind.Audio, dx.Tracks[0].Kind);
    }

    [Fact]
    public async Task Rf64_Duration_Reflects_PCM_Frame_Count()
    {
        // 1000 16-bit mono frames at 8 kHz = 125 ms.
        byte[] pcm = new byte[1000 * 2];
        byte[] file = BuildRf64(8000, 1, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.InRange((dx.Duration - TimeSpan.FromMilliseconds(125)).TotalMilliseconds, -5, 5);
    }

    [Fact]
    public void Rf64_Wrong_Magic_Throws()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8]);
        file[0] = (byte)'X'; file[1] = (byte)'Y'; file[2] = (byte)'Z'; file[3] = (byte)'!';
        using var src = new IO.MemoryRandomAccessSource(file);
        Assert.Throws<InvalidDataException>(() => WavDemuxer.Open(src));
    }

    [Fact]
    public async Task Rf64_Pcm_Bytes_Roundtrip_Identical()
    {
        // Verify the actual PCM bytes (not just total length) survive
        // demuxing intact.
        const int frames = 1000;
        byte[] pcm = new byte[frames * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)((i * 7) & 0x7FFF);
            pcm[i * 2] = (byte)v;
            pcm[i * 2 + 1] = (byte)(v >> 8);
        }
        byte[] file = BuildRf64(44100, 1, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var collected = new MemoryStream();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { collected.Write(s.Data.Span); }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm, collected.ToArray());
    }

    [Fact]
    public void Rf64_Truncated_Header_Throws()
    {
        // RF64 magic without WAVE or ds64 chunk should fail to open.
        byte[] head = new byte[12];
        head[0] = (byte)'R'; head[1] = (byte)'F'; head[2] = (byte)'6'; head[3] = (byte)'4';
        head[4] = 0xFF; head[5] = 0xFF; head[6] = 0xFF; head[7] = 0xFF;
        head[8] = (byte)'W'; head[9] = (byte)'A'; head[10] = (byte)'V'; head[11] = (byte)'E';
        using var src = new IO.MemoryRandomAccessSource(head);
        Assert.ThrowsAny<Exception>(() => WavDemuxer.Open(src));
    }

    [Fact]
    public async Task Rf64_Dispose_Idempotent()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[200]);
        using var src = new IO.MemoryRandomAccessSource(file);
        var dx = WavDemuxer.Open(src);
        dx.Dispose();
        dx.Dispose();
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_Track_Has_Index_0_And_Default_Language()
    {
        byte[] file = BuildRf64(48000, 2, 16, new byte[400]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Single(dx.Tracks);
        Assert.Equal(0, dx.Tracks[0].Index);
        Assert.NotNull(dx.Tracks[0].Language);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_Metadata_Is_Non_Null_When_No_Info_Chunk()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[16]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        // No LIST INFO -> empty metadata, but the property itself must be
        // non-null.
        Assert.NotNull(dx.Metadata);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_ReadSamplesAsync_Honours_Cancellation()
    {
        byte[] pcm = new byte[44100 * 2];
        byte[] file = BuildRf64(44100, 1, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
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
    public async Task Rf64_TwoByteFmt_With_Mismatched_Channels_Detected_Correctly()
    {
        byte[] pcm = new byte[16 * 4]; // 16 frames of 32-bit stereo? 4 bytes/frame
        byte[] file = BuildRf64(44100, 2, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(2, a.Channels);
        await Task.CompletedTask;
    }

    [Theory]
    [InlineData(16, CodecId.PcmS16Le)]
    [InlineData(24, CodecId.PcmS24Le)]
    [InlineData(32, CodecId.PcmS32Le)]
    public async Task Rf64_BitsPerSample_Maps_To_Pcm_Codec(int bits, CodecId expected)
    {
        byte[] pcm = new byte[64 * (bits / 8)];
        byte[] file = BuildRf64(8000, 1, bits, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(expected, a.Codec);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_EightBit_Maps_To_Unknown_Codec()
    {
        // The PCM WAV mapping table only enumerates 16/24/32-bit signed
        // PCM. 8-bit PCM (unsigned per WAV spec) is not in the table and
        // surfaces as Unknown rather than misreporting a signed codec.
        byte[] file = BuildRf64(8000, 1, 8, new byte[100]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.Unknown, a.Codec);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_Samples_All_Have_TrackIndex_Zero_And_Are_KeyFrames()
    {
        byte[] pcm = new byte[8000 * 2]; // 1 second at 8 kHz mono 16-bit
        byte[] file = BuildRf64(8000, 1, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(0, s.TrackIndex);
                Assert.True(s.IsKeyFrame);
                count++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.True(count > 0);
    }

    [Fact]
    public async Task Rf64_Sample_Pts_Monotonically_Increases_By_Duration()
    {
        byte[] pcm = new byte[8000 * 2];
        byte[] file = BuildRf64(8000, 1, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        long? prevPts = null;
        long? prevDuration = null;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                if (prevPts is not null)
                {
                    Assert.Equal(prevPts.Value + prevDuration!.Value, s.Pts);
                }
                else
                {
                    Assert.Equal(0L, s.Pts);
                }
                prevPts = s.Pts;
                prevDuration = s.Duration;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.NotNull(prevPts);
    }

    [Fact]
    public async Task Rf64_Sample_Pts_Equals_Dts()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8000 * 2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { Assert.Equal(s.Pts, s.Dts); }
            finally { s.Owner?.Dispose(); }
        }
    }

    [Fact]
    public async Task Rf64_SeekAsync_Zero_Returns_First_Sample_At_Pts_Zero()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8000 * 2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        await dx.SeekAsync(TimeSpan.FromSeconds(0.5));
        await dx.SeekAsync(TimeSpan.Zero);

        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { Assert.Equal(0L, s.Pts); }
            finally { s.Owner?.Dispose(); }
            break;
        }
    }

    [Fact]
    public async Task Rf64_SeekAsync_Negative_Clamps_To_Zero()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8000 * 2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        await dx.SeekAsync(TimeSpan.FromSeconds(-10));

        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { Assert.Equal(0L, s.Pts); }
            finally { s.Owner?.Dispose(); }
            break;
        }
    }

    [Fact]
    public async Task Rf64_SeekAsync_Past_Duration_Yields_No_Samples()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8000 * 2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        await dx.SeekAsync(TimeSpan.FromSeconds(999));

        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { count++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public async Task Rf64_SeekAsync_Mid_Stream_Pts_Reflects_Position()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8000 * 2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        await dx.SeekAsync(TimeSpan.FromSeconds(0.5));

        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                // 0.5 s * 8000 Hz = 4000 frames
                Assert.Equal(4000L, s.Pts);
            }
            finally { s.Owner?.Dispose(); }
            break;
        }
    }

    [Fact]
    public void Rf64_OwnsSource_True_Disposes_Source_On_Dispose()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[200]);
        var src = new IO.MemoryRandomAccessSource(file);
        var dx = WavDemuxer.Open(src, ownsSource: true);
        dx.Dispose();
        // Subsequent read must throw because the source was disposed.
        Assert.ThrowsAny<Exception>(() => src.ReadAsync(0, new byte[4]).AsTask().GetAwaiter().GetResult());
    }

    [Fact]
    public async Task Rf64_OwnsSource_False_Default_Does_Not_Dispose_Source()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[200]);
        using var src = new IO.MemoryRandomAccessSource(file);
        var dx = WavDemuxer.Open(src);
        dx.Dispose();
        // Source must still be usable; reading the first 4 bytes returns 4.
        var buf = new byte[4];
        int read = await src.ReadAsync(0, buf);
        Assert.Equal(4, read);
    }

    [Fact]
    public async Task Bw64_Magic_Eight_Bit_Pcm_Surfaces_As_Unknown_Codec()
    {
        // Same mapping as RF64: 8-bit PCM is outside the supported
        // signed-PCM table. Codec must be Unknown rather than misreported.
        byte[] pcm = new byte[100];
        byte[] file = BuildRf64(48000, 1, 8, pcm);
        file[0] = (byte)'B'; file[1] = (byte)'W'; file[2] = (byte)'6'; file[3] = (byte)'4';
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(CodecId.Unknown, a.Codec);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_EightChannel_Stereo_71_Reads_All_Channels()
    {
        byte[] pcm = new byte[64 * 8 * 2]; // 64 frames, 8 channels, 16-bit
        byte[] file = BuildRf64(48000, 8, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(8, a.Channels);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Rf64_Multiple_OpenRead_Cycles_Deterministic()
    {
        const int frames = 200;
        byte[] pcm = new byte[frames * 2];
        for (int i = 0; i < frames; i++) pcm[i * 2] = (byte)(i & 0xFF);
        byte[] file = BuildRf64(8000, 1, 16, pcm);

        long firstTotal = 0;
        using (var src = new IO.MemoryRandomAccessSource(file))
        using (var dx = WavDemuxer.Open(src))
        {
            await foreach (var s in dx.ReadSamplesAsync())
            {
                try { firstTotal += s.Data.Length; } finally { s.Owner?.Dispose(); }
            }
        }

        long secondTotal = 0;
        using (var src = new IO.MemoryRandomAccessSource(file))
        using (var dx = WavDemuxer.Open(src))
        {
            await foreach (var s in dx.ReadSamplesAsync())
            {
                try { secondTotal += s.Data.Length; } finally { s.Owner?.Dispose(); }
            }
        }

        Assert.Equal(firstTotal, secondTotal);
        Assert.Equal(pcm.Length, firstTotal);
    }

    [Fact]
    public async Task Rf64_FmtChunk_Preserves_SampleRate_In_AudioCodecParameters()
    {
        foreach (int sr in new[] { 8000, 16000, 22050, 32000, 44100, 48000, 96000 })
        {
            byte[] file = BuildRf64(sr, 1, 16, new byte[200]);
            using var src = new IO.MemoryRandomAccessSource(file);
            using var dx = WavDemuxer.Open(src);
            var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
            Assert.Equal(sr, a.SampleRate);
        }
        await Task.CompletedTask;
    }
}
