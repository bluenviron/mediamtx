using System.Buffers.Binary;
using Mediar.Containers.Matroska;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Standalone unit tests for the Matroska lacing size codec plus end-to-end
/// round-trip tests for Xiph / EBML / Fixed laced SimpleBlocks emitted by
/// <see cref="MatroskaMuxer"/> and decoded by <see cref="MatroskaDemuxer"/>.
/// </summary>
public sealed class MatroskaLacingTests
{
    // ---- Standalone codec tests ----

    [Theory]
    [InlineData(2, new int[] { 100, 200 })]
    [InlineData(3, new int[] { 50, 80, 0 })]
    [InlineData(4, new int[] { 255, 510, 100, 1 })]
    [InlineData(4, new int[] { 1000, 0, 1, 0 })]
    public void Xiph_RoundTrips(int frameCount, int[] sizes)
    {
        Assert.Equal(frameCount, sizes.Length);
        byte[] payload = SynthesisePayload(sizes);
        MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Xiph, sizes, out byte[] header);
        byte[] body = Concat(header, payload);
        int payloadOffset = MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Xiph, body, out int[] decoded);
        Assert.Equal(sizes, decoded);
        Assert.Equal(header.Length, payloadOffset);
        // Last frame size is implicit (not in the header) — decoder must recover it from buffer length.
        Assert.Equal(sizes[^1], body.Length - payloadOffset - SumExceptLast(sizes));
    }

    [Theory]
    [InlineData(2, 100)]
    [InlineData(8, 500)]
    [InlineData(1, 1)]
    [InlineData(256, 4)]
    public void Fixed_RoundTrips(int frameCount, int sizePerFrame)
    {
        int[] sizes = Enumerable.Repeat(sizePerFrame, frameCount).ToArray();
        byte[] payload = SynthesisePayload(sizes);
        MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Fixed, sizes, out byte[] header);
        Assert.Single(header); // only the frame-count byte
        byte[] body = Concat(header, payload);
        int payloadOffset = MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Fixed, body, out int[] decoded);
        Assert.Equal(sizes, decoded);
        Assert.Equal(1, payloadOffset);
    }

    [Fact]
    public void Fixed_NonUniform_Sizes_Throws()
    {
        int[] sizes = { 100, 100, 99 };
        Assert.Throws<ArgumentException>(() =>
            MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Fixed, sizes, out _));
    }

    [Fact]
    public void Fixed_Indivisible_Payload_Throws()
    {
        // Manually craft a Fixed-laced body where total payload isn't divisible by frame count.
        byte[] body = new byte[1 + 7];
        body[0] = 3 - 1; // 3 frames; 7 bytes payload ⇒ 7/3 = 2.33 → invalid.
        Assert.Throws<InvalidDataException>(() =>
            MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Fixed, body, out _));
    }

    [Theory]
    // Spec edge cases for the signed VINT bias at L=1: range [-63, +64].
    [InlineData(new int[] { 100, 163, 226 })] // deltas: +63 → encodable in L=1
    [InlineData(new int[] { 100, 164, 100 })] // deltas: +64 → L=1 all-ones (the all-bias-max case)
    [InlineData(new int[] { 100, 165, 100 })] // deltas: +65 → forces L=2
    [InlineData(new int[] { 100, 37, 100 })]  // deltas: -63 → L=1 minimum
    [InlineData(new int[] { 100, 36, 100 })]  // deltas: -64 → forces L=2
    [InlineData(new int[] { 1000, 500, 100 })]
    [InlineData(new int[] { 0, 50000, 100 })]
    public void Ebml_RoundTrips(int[] sizes)
    {
        byte[] payload = SynthesisePayload(sizes);
        MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Ebml, sizes, out byte[] header);
        byte[] body = Concat(header, payload);
        int payloadOffset = MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Ebml, body, out int[] decoded);
        Assert.Equal(sizes, decoded);
        Assert.Equal(header.Length, payloadOffset);
    }

    [Fact]
    public void Ebml_Single_Frame_Header_Is_Just_Count_Byte()
    {
        int[] sizes = { 250 };
        byte[] payload = SynthesisePayload(sizes);
        MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Ebml, sizes, out byte[] header);
        Assert.Single(header);
        Assert.Equal(0, header[0]);
        byte[] body = Concat(header, payload);
        int payloadOffset = MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Ebml, body, out int[] decoded);
        Assert.Equal(sizes, decoded);
        Assert.Equal(1, payloadOffset);
    }

    [Fact]
    public void Xiph_Single_Frame_Header_Is_Just_Count_Byte()
    {
        int[] sizes = { 700 };
        byte[] payload = SynthesisePayload(sizes);
        MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Xiph, sizes, out byte[] header);
        Assert.Single(header);
        byte[] body = Concat(header, payload);
        int payloadOffset = MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Xiph, body, out int[] decoded);
        Assert.Equal(sizes, decoded);
        Assert.Equal(1, payloadOffset);
    }

    [Fact]
    public void Xiph_Endless_FF_Header_Throws()
    {
        // Craft a body whose Xiph size field is just 0xFF bytes forever — i.e. the
        // size byte sequence never closes.
        byte[] body = new byte[10];
        body[0] = 2 - 1; // 2 frames → 1 size to encode
        for (int i = 1; i < body.Length; i++) body[i] = 0xFF;
        Assert.Throws<InvalidDataException>(() =>
            MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Xiph, body, out _));
    }

    [Fact]
    public void Xiph_Sum_Overruns_Payload_Throws()
    {
        // 2 frames, first size = 100, but the body only has 50 trailing bytes.
        byte[] body = new byte[1 + 1 + 50];
        body[0] = 2 - 1;
        body[1] = 100;
        Assert.Throws<InvalidDataException>(() =>
            MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Xiph, body, out _));
    }

    [Fact]
    public void Ebml_Negative_Cumulative_Size_Throws()
    {
        // First size = 5, delta = -10 → second size = -5, must throw.
        var bw = new System.Buffers.ArrayBufferWriter<byte>();
        bw.GetSpan(1)[0] = 3 - 1; bw.Advance(1);
        MatroskaLacingCodec.WriteUnsignedVint(bw, 5);
        MatroskaLacingCodec.WriteSignedVint(bw, -10);
        // Add 100 trailing bytes so the second-frame size error trips before "overrun".
        for (int i = 0; i < 100; i++) { bw.GetSpan(1)[0] = 0; bw.Advance(1); }
        Assert.Throws<InvalidDataException>(() =>
            MatroskaLacingCodec.DecodeSizes(MatroskaLacing.Ebml, bw.WrittenSpan, out _));
    }

    [Fact]
    public void Frame_Count_Exceeds_Max_Throws()
    {
        int[] sizes = new int[257];
        Array.Fill(sizes, 1);
        Assert.Throws<ArgumentException>(() =>
            MatroskaLacingCodec.EncodeSizes(MatroskaLacing.Xiph, sizes, out _));
    }

    // ---- End-to-end mux → demux round-trips ----

    [Theory]
    [InlineData(MatroskaLacing.Xiph)]
    [InlineData(MatroskaLacing.Ebml)]
    [InlineData(MatroskaLacing.Fixed)]
    public async Task Muxer_Lacing_RoundTrips_Through_Demuxer(MatroskaLacing lacing)
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 1000),
        };

        int frameCount = 5;
        byte[][] payloads = new byte[frameCount][];
        for (int i = 0; i < frameCount; i++)
        {
            // For Fixed lacing every payload must be the same size.
            int size = lacing == MatroskaLacing.Fixed ? 120 : 100 + i * 17;
            payloads[i] = new byte[size];
            for (int b = 0; b < size; b++) payloads[i][b] = (byte)((i + 5) * (b + 1));
        }

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            mux.SetLacing(0, lacing, maxFramesPerBlock: frameCount);
            mux.SetDefaultDuration(0, TimeSpan.FromMilliseconds(20));
            await mux.StartAsync();
            for (int i = 0; i < frameCount; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * 20L,
                    Dts = i * 20L,
                    Duration = 20,
                    IsKeyFrame = true,
                    Data = payloads[i],
                });
            }
            await mux.FinishAsync();
        }

        byte[] bytes = ms.ToArray();
        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = MatroskaDemuxer.Open(src);

        int recovered = 0;
        long? lastPts = null;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            Assert.True(recovered < payloads.Length, $"more samples than written ({recovered})");
            Assert.Equal(payloads[recovered], s.Data.ToArray());
            if (lastPts.HasValue) Assert.True(s.Pts > lastPts, "PTS must increase across laced frames.");
            lastPts = s.Pts;
            s.Owner?.Dispose();
            recovered++;
        }
        Assert.Equal(payloads.Length, recovered);
    }

    [Fact]
    public async Task Muxer_Fixed_Lacing_AutoFlushes_On_Size_Mismatch()
    {
        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 1000),
        };

        // 3 same-size frames followed by a 4th different-size frame; the muxer
        // must auto-flush the first three (lace of 3) and then emit the
        // mismatching one on its own (lace of 1 = unlaced block, since lacing
        // requires >= 2 frames to set the lacing bits).
        byte[][] payloads = {
            new byte[100], new byte[100], new byte[100], new byte[173],
        };
        for (int p = 0; p < payloads.Length; p++)
            for (int b = 0; b < payloads[p].Length; b++)
                payloads[p][b] = (byte)((p * 17) ^ b);

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            mux.SetLacing(0, MatroskaLacing.Fixed, maxFramesPerBlock: 16);
            mux.SetDefaultDuration(0, TimeSpan.FromMilliseconds(20));
            await mux.StartAsync();
            for (int i = 0; i < payloads.Length; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = i * 20L, Dts = i * 20L,
                    Duration = 20, IsKeyFrame = true, Data = payloads[i],
                });
            }
            await mux.FinishAsync();
        }

        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = MatroskaDemuxer.Open(src);
        var seen = new List<byte[]>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            seen.Add(s.Data.ToArray());
            s.Owner?.Dispose();
        }
        Assert.Equal(payloads.Length, seen.Count);
        for (int i = 0; i < payloads.Length; i++)
            Assert.Equal(payloads[i], seen[i]);
    }

    [Fact]
    public async Task Muxer_Lacing_With_Multiple_Tracks_Keeps_Buffers_Isolated()
    {
        var audio = new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 1000),
        };
        var video = new MediaTrack
        {
            Index = 1, Id = 2,
            Codec = new VideoCodecParameters { Codec = CodecId.Vp9, Width = 320, Height = 240 },
            TimeBase = new Rational(1, 1000),
        };

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(audio);
            mux.AddTrack(video);
            mux.SetLacing(0, MatroskaLacing.Xiph, maxFramesPerBlock: 4);
            mux.SetDefaultDuration(0, TimeSpan.FromMilliseconds(20));
            await mux.StartAsync();
            // Interleave audio + video. Audio frames lace into a single block;
            // the video frame in the middle must be emitted in isolation.
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 20, IsKeyFrame = true, Data = new byte[64] });
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 20, Dts = 20, Duration = 20, IsKeyFrame = true, Data = new byte[80] });
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 1, Pts = 0, Dts = 0, Duration = 40, IsKeyFrame = true, Data = new byte[256] });
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 40, Dts = 40, Duration = 20, IsKeyFrame = true, Data = new byte[96] });
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 60, Dts = 60, Duration = 20, IsKeyFrame = true, Data = new byte[100] });
            await mux.FinishAsync();
        }

        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = MatroskaDemuxer.Open(src);
        Assert.Equal(2, dx.Tracks.Count);

        int audioCount = 0, videoCount = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            if (s.TrackIndex == 0) audioCount++;
            if (s.TrackIndex == 1) videoCount++;
            s.Owner?.Dispose();
        }
        Assert.Equal(4, audioCount);
        Assert.Equal(1, videoCount);
    }

    [Fact]
    public async Task Muxer_Lacing_Flush_Before_Cluster_Roll()
    {
        // Pending laced audio buffer is non-empty when a > 30 s gap forces a
        // new cluster. The pending lace must be written into the OLD cluster,
        // not silently dropped or written into the new one.
        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 1000),
        };

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            mux.SetLacing(0, MatroskaLacing.Ebml, maxFramesPerBlock: 16);
            mux.SetDefaultDuration(0, TimeSpan.FromMilliseconds(20));
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 20, IsKeyFrame = true, Data = new byte[60] });
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 20, Dts = 20, Duration = 20, IsKeyFrame = true, Data = new byte[60] });
            // Gap > MaxClusterSpanMs forces a new cluster.
            await mux.WriteSampleAsync(new MediaSample
            { TrackIndex = 0, Pts = 60_000, Dts = 60_000, Duration = 20, IsKeyFrame = true, Data = new byte[60] });
            await mux.FinishAsync();
        }
        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = MatroskaDemuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            seen++;
            s.Owner?.Dispose();
        }
        Assert.Equal(3, seen);
    }

    // ---- Demuxer-side hand-crafted blocks ----

    [Fact]
    public async Task Demuxer_Decodes_HandCrafted_Xiph_SimpleBlock()
    {
        byte[] mkv = BuildSingleBlockMkv(MatroskaLacing.Xiph,
            new[] { new byte[] { 1, 2, 3 }, new byte[] { 4, 5, 6, 7 }, new byte[] { 8, 9 } },
            defaultDurationMs: 10);
        using var src = new MemoryRandomAccessSource(mkv);
        using var dx = MatroskaDemuxer.Open(src);
        var seen = new List<MediaSample>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            seen.Add(s);
            s.Owner?.Dispose();
        }
        Assert.Equal(3, seen.Count);
        Assert.Equal(new byte[] { 1, 2, 3 }, seen[0].Data.ToArray());
        Assert.Equal(new byte[] { 4, 5, 6, 7 }, seen[1].Data.ToArray());
        Assert.Equal(new byte[] { 8, 9 }, seen[2].Data.ToArray());
        Assert.Equal(0L, seen[0].Pts);
        Assert.Equal(10L, seen[1].Pts);
        Assert.Equal(20L, seen[2].Pts);
    }

    [Fact]
    public async Task Demuxer_Decodes_HandCrafted_Fixed_SimpleBlock()
    {
        byte[] mkv = BuildSingleBlockMkv(MatroskaLacing.Fixed,
            new[]
            {
                new byte[] { 0xAA, 0xBB, 0xCC, 0xDD },
                new byte[] { 0x11, 0x22, 0x33, 0x44 },
                new byte[] { 0x55, 0x66, 0x77, 0x88 },
                new byte[] { 0x99, 0xA1, 0xB2, 0xC3 },
            },
            defaultDurationMs: 5);
        using var src = new MemoryRandomAccessSource(mkv);
        using var dx = MatroskaDemuxer.Open(src);
        var seen = new List<MediaSample>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            seen.Add(s);
            s.Owner?.Dispose();
        }
        Assert.Equal(4, seen.Count);
        Assert.Equal(0L, seen[0].Pts);
        Assert.Equal(5L, seen[1].Pts);
        Assert.Equal(10L, seen[2].Pts);
        Assert.Equal(15L, seen[3].Pts);
    }

    [Fact]
    public async Task Demuxer_Decodes_HandCrafted_Ebml_SimpleBlock()
    {
        byte[] mkv = BuildSingleBlockMkv(MatroskaLacing.Ebml,
            new[]
            {
                MakeFilledArray(200, 0xAA),
                MakeFilledArray(150, 0xBB), // delta -50
                MakeFilledArray(220, 0xCC), // delta +70
                MakeFilledArray(180, 0xDD), // last frame size implicit
            },
            defaultDurationMs: 20);
        using var src = new MemoryRandomAccessSource(mkv);
        using var dx = MatroskaDemuxer.Open(src);
        var seen = new List<MediaSample>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            seen.Add(s);
            s.Owner?.Dispose();
        }
        Assert.Equal(4, seen.Count);
        Assert.Equal(200, seen[0].Data.Length);
        Assert.Equal(150, seen[1].Data.Length);
        Assert.Equal(220, seen[2].Data.Length);
        Assert.Equal(180, seen[3].Data.Length);
    }

    [Fact]
    public async Task Demuxer_BlockGroup_With_Lacing_Uses_BlockDuration_For_Pts_Spacing()
    {
        // 4 frames, no DefaultDuration on the track, but BlockGroup carries
        // BlockDuration = 80 ms. PTS spacing should be 80/4 = 20 ms each.
        byte[] mkv = BuildBlockGroupMkv(MatroskaLacing.Fixed,
            new[]
            {
                MakeFilledArray(50, 0x11),
                MakeFilledArray(50, 0x22),
                MakeFilledArray(50, 0x33),
                MakeFilledArray(50, 0x44),
            },
            blockDuration: 80,
            defaultDurationMs: null);
        using var src = new MemoryRandomAccessSource(mkv);
        using var dx = MatroskaDemuxer.Open(src);
        var seen = new List<MediaSample>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            seen.Add(s);
            s.Owner?.Dispose();
        }
        Assert.Equal(4, seen.Count);
        Assert.Equal(0L, seen[0].Pts);
        Assert.Equal(20L, seen[1].Pts);
        Assert.Equal(40L, seen[2].Pts);
        Assert.Equal(60L, seen[3].Pts);
    }

    // ---- Helpers ----

    private static byte[] SynthesisePayload(int[] sizes)
    {
        int total = sizes.Sum();
        byte[] data = new byte[total];
        int off = 0;
        byte tag = 1;
        foreach (var s in sizes)
        {
            for (int i = 0; i < s; i++) data[off + i] = (byte)(tag * (i + 1));
            off += s;
            tag++;
        }
        return data;
    }

    private static byte[] MakeFilledArray(int len, byte tag)
    {
        byte[] a = new byte[len];
        for (int i = 0; i < len; i++) a[i] = (byte)((tag * (i + 1)) & 0xFF);
        return a;
    }

    private static byte[] Concat(byte[] a, byte[] b)
    {
        var r = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, r, 0, a.Length);
        Buffer.BlockCopy(b, 0, r, a.Length, b.Length);
        return r;
    }

    private static int SumExceptLast(int[] sizes)
    {
        int total = 0;
        for (int i = 0; i < sizes.Length - 1; i++) total += sizes[i];
        return total;
    }

    /// <summary>
    /// Build a minimal MKV in memory containing exactly one SimpleBlock with
    /// the given lacing and frames. Optionally sets DefaultDuration on the
    /// track to let the demuxer derive per-frame PTS spacing.
    /// </summary>
    private static byte[] BuildSingleBlockMkv(MatroskaLacing lacing, byte[][] frames, int? defaultDurationMs)
        => BuildMkvImpl(lacing, frames, defaultDurationMs, blockDuration: null, useBlockGroup: false);

    private static byte[] BuildBlockGroupMkv(MatroskaLacing lacing, byte[][] frames, long blockDuration, int? defaultDurationMs)
        => BuildMkvImpl(lacing, frames, defaultDurationMs, blockDuration: blockDuration, useBlockGroup: true);

    private static byte[] BuildMkvImpl(MatroskaLacing lacing, byte[][] frames, int? defaultDurationMs, long? blockDuration, bool useBlockGroup)
    {
        // Build payload: track VINT + 16-bit BE relative ts + flags byte +
        // lacing header + concatenated frame payloads.
        int[] sizes = frames.Select(f => f.Length).ToArray();
        MatroskaLacingCodec.EncodeSizes(lacing, sizes, out byte[] lacingHeader);
        int blockBodyLen = 1 /* track VINT */ + 2 + 1 /* ts+flags */ + lacingHeader.Length + sizes.Sum();
        byte[] blockBody = new byte[blockBodyLen];
        int off = 0;
        blockBody[off++] = 0x81; // track number 1, 1-byte VINT
        blockBody[off++] = 0;    // relative ts hi
        blockBody[off++] = 0;    // relative ts lo
        byte flags = 0x80; // keyframe
        flags |= (byte)(((int)lacing & 0x03) << 1);
        blockBody[off++] = flags;
        Buffer.BlockCopy(lacingHeader, 0, blockBody, off, lacingHeader.Length); off += lacingHeader.Length;
        foreach (var f in frames)
        {
            Buffer.BlockCopy(f, 0, blockBody, off, f.Length);
            off += f.Length;
        }

        // Now assemble the surrounding MKV envelope using a real
        // MatroskaMuxer-style sequence so the demuxer accepts it. We bypass
        // the muxer entirely because it would emit the block via its own
        // serialiser.
        var ms = new MemoryStream();
        WriteEbmlHeader(ms);
        // Segment with unknown size.
        WriteId(ms, 0x18538067);
        WriteVintUnknown(ms, 8);

        // Info
        var info = new MemoryStream();
        WriteUInt(info, 0x2AD7B1, 1_000_000); // TimecodeScale = 1 ms
        WriteContainer(ms, 0x1549A966, info);

        // Tracks
        var tracks = new MemoryStream();
        var te = new MemoryStream();
        WriteUInt(te, 0xD7, 1); // TrackNumber
        WriteUInt(te, 0x73C5, 1); // TrackUID
        WriteUInt(te, 0x83, 2); // TrackType = audio
        WriteUInt(te, 0x9C, 1); // FlagLacing
        WriteString(te, 0x86, "A_OPUS");
        if (defaultDurationMs.HasValue)
        {
            WriteUInt(te, 0x23E383, (ulong)defaultDurationMs.Value * 1_000_000UL);
        }
        var audio = new MemoryStream();
        WriteFloat64(audio, 0xB5, 48000);
        WriteUInt(audio, 0x9F, 2);
        WriteContainer(te, 0xE1, audio);
        WriteContainer(tracks, 0xAE, te);
        WriteContainer(ms, 0x1654AE6B, tracks);

        // Cluster
        var cluster = new MemoryStream();
        WriteUInt(cluster, 0xE7, 0); // Timecode
        if (useBlockGroup)
        {
            var bg = new MemoryStream();
            WriteBinary(bg, 0xA1, blockBody);
            WriteUInt(bg, 0x9B, (ulong)blockDuration!.Value);
            WriteContainer(cluster, 0xA0, bg);
        }
        else
        {
            WriteBinary(cluster, 0xA3, blockBody);
        }
        WriteContainer(ms, 0x1F43B675, cluster);

        return ms.ToArray();
    }

    private static void WriteEbmlHeader(Stream s)
    {
        var hdr = new MemoryStream();
        WriteUInt(hdr, 0x4286, 1);
        WriteUInt(hdr, 0x42F7, 1);
        WriteUInt(hdr, 0x42F2, 4);
        WriteUInt(hdr, 0x42F3, 8);
        WriteString(hdr, 0x4282, "matroska");
        WriteUInt(hdr, 0x4287, 4);
        WriteUInt(hdr, 0x4285, 2);
        WriteContainer(s, 0x1A45DFA3, hdr);
    }

    private static void WriteContainer(Stream s, ulong id, MemoryStream contents)
    {
        WriteId(s, id);
        byte[] body = contents.ToArray();
        WriteVintLength(s, body.Length);
        s.Write(body, 0, body.Length);
    }

    private static void WriteId(Stream s, ulong id)
    {
        Span<byte> tmp = stackalloc byte[8];
        int n;
        if (id <= 0xFF) n = 1;
        else if (id <= 0xFFFF) n = 2;
        else if (id <= 0xFFFFFF) n = 3;
        else n = 4;
        for (int i = n - 1; i >= 0; i--) tmp[i] = (byte)(id >> (8 * (n - 1 - i)));
        s.Write(tmp[..n]);
    }

    private static void WriteVintLength(Stream s, long value)
    {
        int width = 0;
        for (int w = 1; w <= 8; w++)
        {
            long max = (1L << (7 * w)) - 2;
            if (value <= max) { width = w; break; }
        }
        Span<byte> tmp = stackalloc byte[width];
        ulong v = (ulong)value | (1UL << (7 * width));
        for (int i = width - 1; i >= 0; i--) { tmp[i] = (byte)v; v >>= 8; }
        s.Write(tmp);
    }

    private static void WriteVintUnknown(Stream s, int width)
    {
        Span<byte> tmp = stackalloc byte[width];
        tmp[0] = width switch { 1 => 0xFF, 2 => 0x7F, 4 => 0x1F, 8 => 0x01, _ => 0x01 };
        for (int i = 1; i < width; i++) tmp[i] = 0xFF;
        s.Write(tmp);
    }

    private static void WriteUInt(Stream s, ulong id, ulong value)
    {
        WriteId(s, id);
        int n;
        if (value == 0) n = 1;
        else if (value <= 0xFF) n = 1;
        else if (value <= 0xFFFF) n = 2;
        else if (value <= 0xFFFFFF) n = 3;
        else if (value <= 0xFFFFFFFF) n = 4;
        else if (value <= 0xFFFFFFFFFFUL) n = 5;
        else if (value <= 0xFFFFFFFFFFFFUL) n = 6;
        else if (value <= 0xFFFFFFFFFFFFFFUL) n = 7;
        else n = 8;
        WriteVintLength(s, n);
        for (int i = n - 1; i >= 0; i--) s.WriteByte((byte)(value >> (8 * i)));
    }

    private static void WriteFloat64(Stream s, ulong id, double value)
    {
        WriteId(s, id);
        WriteVintLength(s, 8);
        Span<byte> tmp = stackalloc byte[8];
        BinaryPrimitives.WriteDoubleBigEndian(tmp, value);
        s.Write(tmp);
    }

    private static void WriteString(Stream s, ulong id, string value)
    {
        WriteId(s, id);
        byte[] b = System.Text.Encoding.UTF8.GetBytes(value);
        WriteVintLength(s, b.Length);
        s.Write(b, 0, b.Length);
    }

    private static void WriteBinary(Stream s, ulong id, byte[] data)
    {
        WriteId(s, id);
        WriteVintLength(s, data.Length);
        s.Write(data, 0, data.Length);
    }
}
