using Mediar.Containers.Amr;
using Xunit;

namespace Mediar.Tests;

public sealed class AmrRoundTripTests
{
    // Build a frame for AMR-NB mode 0 (4.75 kbps): 12-byte payload, 13 total.
    // Type-octet: bit 7=F(0), bits 6..3=FT(0), bit 2=Q(1), bits 1..0=padding(0) → 0x04
    private static byte[] BuildNbFrame(int seed)
    {
        byte[] frame = new byte[13];
        frame[0] = 0x04;
        for (int i = 1; i < 13; i++) frame[i] = (byte)((seed * 31 + i) & 0xFF);
        return frame;
    }

    // AMR-WB mode 0 (6.6 kbps): 17-byte payload, 18 total.
    private static byte[] BuildWbFrame(int seed)
    {
        byte[] frame = new byte[18];
        frame[0] = 0x04;
        for (int i = 1; i < 18; i++) frame[i] = (byte)((seed * 17 + i) & 0xFF);
        return frame;
    }

    [Fact]
    public async Task AmrNb_RoundTrips_Through_Muxer()
    {
        const int count = 20;
        byte[] amr;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new AmrMuxer(ms, leaveOpen: true);
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                TimeBase = new Rational(1, 8000),
                Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
            });
            await mux.StartAsync();
            for (int i = 0; i < count; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = i * 160, Dts = i * 160,
                    Duration = 160, IsKeyFrame = true, Data = BuildNbFrame(i),
                });
            }
            await mux.FinishAsync();
            amr = ms.ToArray();
        }
        // Magic prefix.
        Assert.Equal((byte)'#', amr[0]);
        Assert.Equal((byte)'!', amr[1]);
        Assert.Equal((byte)'A', amr[2]);
        Assert.Equal((byte)'M', amr[3]);
        Assert.Equal((byte)'R', amr[4]);
        Assert.Equal((byte)'\n', amr[5]);

        using var src = new IO.MemoryRandomAccessSource(amr);
        using var dx = AmrDemuxer.Open(src);
        Assert.Equal("amr-nb", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.AmrNb, a.Codec);
        Assert.Equal(8000, a.SampleRate);

        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(13, s.Data.Length);
                Assert.Equal(160, s.Duration);
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(count, seen);
    }

    [Fact]
    public async Task AmrWb_RoundTrips_Through_Muxer()
    {
        const int count = 10;
        byte[] amr;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new AmrMuxer(ms, leaveOpen: true);
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                TimeBase = new Rational(1, 16000),
                Codec = new AudioCodecParameters { Codec = CodecId.AmrWb, SampleRate = 16000, Channels = 1 },
            });
            await mux.StartAsync();
            for (int i = 0; i < count; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = i * 320, Dts = i * 320,
                    Duration = 320, IsKeyFrame = true, Data = BuildWbFrame(i),
                });
            }
            await mux.FinishAsync();
            amr = ms.ToArray();
        }
        // WB magic = "#!AMR-WB\n" (9 bytes).
        Assert.Equal((byte)'#', amr[0]);
        Assert.Equal((byte)'!', amr[1]);
        Assert.Equal((byte)'A', amr[2]);
        Assert.Equal((byte)'M', amr[3]);
        Assert.Equal((byte)'R', amr[4]);
        Assert.Equal((byte)'-', amr[5]);
        Assert.Equal((byte)'W', amr[6]);
        Assert.Equal((byte)'B', amr[7]);
        Assert.Equal((byte)'\n', amr[8]);

        using var src = new IO.MemoryRandomAccessSource(amr);
        using var dx = AmrDemuxer.Open(src);
        Assert.Equal("amr-wb", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.AmrWb, a.Codec);
        Assert.Equal(16000, a.SampleRate);

        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(18, s.Data.Length);
                Assert.Equal(320, s.Duration);
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(count, seen);
    }

    [Fact]
    public void Throws_On_Missing_Magic()
    {
        byte[] junk = new byte[16];
        using var src = new IO.MemoryRandomAccessSource(junk);
        Assert.Throws<InvalidDataException>(() => AmrDemuxer.Open(src));
    }

    [Fact]
    public void Open_NullSource_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => AmrDemuxer.Open((IO.IRandomAccessSource)null!));
    }

    [Fact]
    public void Open_NullPath_Throws()
    {
        Assert.ThrowsAny<ArgumentException>(() => AmrDemuxer.Open((string)null!));
    }

    [Fact]
    public void Open_MissingFile_Throws()
    {
        Assert.ThrowsAny<IOException>(() => AmrDemuxer.Open(Path.Combine(Path.GetTempPath(), $"missing-{Guid.NewGuid():N}.amr")));
    }

    [Fact]
    public void Demuxer_FormatName_And_Metadata_Defaults()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        data.AddRange(BuildNbFrame(0));
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        Assert.Equal("amr-nb", dx.FormatName);
        Assert.Equal(MediaMetadata.Empty, dx.Metadata);
        Assert.Equal(TimeSpan.Zero, dx.Duration);
    }

    [Fact]
    public void Demuxer_Dispose_Idempotent()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        var dx = AmrDemuxer.Open(src);
        dx.Dispose();
        dx.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Disposes_Without_Throwing()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        var src = new IO.MemoryRandomAccessSource(data.ToArray());
        var dx = AmrDemuxer.Open(src, ownsSource: true);
        await dx.DisposeAsync();
        await dx.DisposeAsync();
    }

    [Fact]
    public async Task Demuxer_Truncated_Frame_Stops_Early()
    {
        // Magic + one full frame + one truncated frame (just the type-octet).
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        data.AddRange(BuildNbFrame(0));
        data.Add(0x04); // start of next frame, but no payload
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(1, seen);
    }

    [Fact]
    public async Task Demuxer_Empty_Stream_After_Magic_Yields_No_Samples()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, seen);
    }

    [Fact]
    public async Task Demuxer_Cancellation_Throws()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        data.AddRange(BuildNbFrame(0));
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
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
    public async Task Demuxer_Pts_Advances_By_160_For_NB()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        data.AddRange(BuildNbFrame(0));
        data.AddRange(BuildNbFrame(1));
        data.AddRange(BuildNbFrame(2));
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        long expected = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(expected, s.Pts);
                Assert.Equal(expected, s.Dts);
                expected += 160;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(480, expected);
    }

    [Fact]
    public async Task Demuxer_Pts_Advances_By_320_For_WB()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR-WB\n"u8.ToArray());
        data.AddRange(BuildWbFrame(0));
        data.AddRange(BuildWbFrame(1));
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        long expected = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(expected, s.Pts);
                expected += 320;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(640, expected);
    }

    [Fact]
    public async Task Demuxer_Seek_To_Beyond_End_Stops_Reading()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        data.AddRange(BuildNbFrame(0));
        data.AddRange(BuildNbFrame(1));
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromSeconds(60));
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, seen);
    }

    [Fact]
    public async Task Demuxer_Seek_To_Mid_Stream_Resumes_After_That_Frame()
    {
        var data = new List<byte>();
        data.AddRange("#!AMR\n"u8.ToArray());
        for (int i = 0; i < 5; i++) data.AddRange(BuildNbFrame(i));
        using var src = new IO.MemoryRandomAccessSource(data.ToArray());
        using var dx = AmrDemuxer.Open(src);
        await dx.SeekAsync(TimeSpan.FromMilliseconds(40)); // 40 ms / 20 ms = 2 frames forward
        var ptsList = new List<long>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { ptsList.Add(s.Pts); } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(3, ptsList.Count);
        Assert.Equal(new long[] { 320, 480, 640 }, ptsList);
    }

    [Fact]
    public void Muxer_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AmrMuxer(null!));
    }

    [Fact]
    public void Muxer_NonWritable_Throws()
    {
        var readonlyMs = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ArgumentException>(() => new AmrMuxer(readonlyMs));
    }

    [Fact]
    public void Muxer_AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AmrMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void Muxer_AddTrack_NonAudio_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AmrMuxer(ms, leaveOpen: true);
        var t = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 90000),
            Codec = new VideoCodecParameters { Codec = CodecId.H264, Width = 16, Height = 16 },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(t));
    }

    [Fact]
    public void Muxer_AddTrack_WrongCodec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AmrMuxer(ms, leaveOpen: true);
        var t = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.Aac, SampleRate = 8000, Channels = 1 },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(t));
    }

    [Fact]
    public void Muxer_AddTrack_Duplicate_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new AmrMuxer(ms, leaveOpen: true);
        var t = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
        };
        mux.AddTrack(t);
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(t));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AmrMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_StartAsync_Is_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AmrMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
        });
        await mux.StartAsync();
        await mux.StartAsync(); // does not write a second magic
        Assert.Equal("#!AMR\n"u8.ToArray().Length, ms.Length);
    }

    [Fact]
    public async Task Muxer_WriteSample_Auto_Starts()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AmrMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
        });
        await mux.WriteSampleAsync(new MediaSample
        {
            TrackIndex = 0, Pts = 0, Dts = 0, Duration = 160,
            IsKeyFrame = true, Data = BuildNbFrame(0),
        });
        // Magic prefix written automatically.
        Assert.Equal((byte)'#', ms.GetBuffer()[0]);
    }

    [Fact]
    public void Muxer_FormatName_Defaults_To_Nb_Without_Track()
    {
        using var ms = new MemoryStream();
        using var mux = new AmrMuxer(ms, leaveOpen: true);
        Assert.Equal("amr-nb", mux.FormatName);
    }

    [Fact]
    public async Task Muxer_FormatName_Reflects_Wb_Codec()
    {
        await using var ms = new MemoryStream();
        await using var mux = new AmrMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 16000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrWb, SampleRate = 16000, Channels = 1 },
        });
        Assert.Equal("amr-wb", mux.FormatName);
    }

    [Fact]
    public async Task Muxer_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new AmrMuxer(ms, leaveOpen: false);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
        });
        await mux.StartAsync();
        await mux.DisposeAsync();
        Assert.False(ms.CanWrite);
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Preserves_Stream()
    {
        var ms = new MemoryStream();
        var mux = new AmrMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
        });
        await mux.StartAsync();
        await mux.DisposeAsync();
        Assert.True(ms.CanWrite);
        ms.Dispose();
    }
}
