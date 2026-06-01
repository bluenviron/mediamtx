using Mediar.Containers.Matroska;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class MatroskaMuxerTests
{
    [Fact]
    public async Task RoundTrip_Opus_Audio_Through_Mkv_Demuxer()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 2,
                ExtraData = new byte[19] { 0x4F, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64, 1, 2, 0x90, 0x01, 0x80, 0xBB, 0, 0, 0, 0, 0 },
            },
            TimeBase = new Rational(1, 1000),     // ms timebase
        };

        byte[][] payloads = new byte[5][];
        for (int i = 0; i < payloads.Length; i++)
        {
            payloads[i] = new byte[100 + i * 7];
            for (int b = 0; b < payloads[i].Length; b++) payloads[i][b] = (byte)((i + 13) * (b + 1));
        }

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            for (int i = 0; i < payloads.Length; i++)
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
        Assert.True(bytes.Length > 100);
        // Should begin with the EBML id 1A45DFA3
        Assert.Equal(0x1A, bytes[0]);
        Assert.Equal(0x45, bytes[1]);
        Assert.Equal(0xDF, bytes[2]);
        Assert.Equal(0xA3, bytes[3]);

        using var src = new MemoryRandomAccessSource(bytes);
        using var demuxer = MatroskaDemuxer.Open(src);
        Assert.Single(demuxer.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(demuxer.Tracks[0].Codec);
        Assert.Equal(CodecId.Opus, audio.Codec);
        Assert.Equal(48000, audio.SampleRate);
        Assert.Equal(2, audio.Channels);

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
    public async Task Webm_Rejects_NonWebm_Codec()
    {
        using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, webm: true, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Flac, SampleRate = 44100, Channels = 2 },
            TimeBase = new Rational(1, 44100),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public async Task RoundTrip_Av2_Passthrough_Through_Mkv()
    {
        // AV2 has no published bitstream spec yet (AOM still finalising as of
        // 2026), but Mediar must already be able to *carry* AV2 samples as
        // opaque payloads through Matroska so that downstream tools can
        // remux them once files start to appear. This test exercises the
        // V_AV2 codec-id round-trip end-to-end.
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new VideoCodecParameters
            {
                Codec = CodecId.Av2,
                Width = 1920,
                Height = 1080,
            },
            TimeBase = new Rational(1, 1000),
        };

        byte[][] payloads = new byte[3][];
        for (int i = 0; i < payloads.Length; i++)
        {
            payloads[i] = new byte[256 + i * 17];
            for (int b = 0; b < payloads[i].Length; b++) payloads[i][b] = (byte)((b + i * 31) & 0xFF);
        }

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            for (int i = 0; i < payloads.Length; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * 40L,
                    Dts = i * 40L,
                    Duration = 40,
                    IsKeyFrame = i == 0,
                    Data = payloads[i],
                });
            }
            await mux.FinishAsync();
        }

        byte[] bytes = ms.ToArray();
        using var src = new MemoryRandomAccessSource(bytes);
        using var demuxer = MatroskaDemuxer.Open(src);
        Assert.Single(demuxer.Tracks);
        var video = Assert.IsType<VideoCodecParameters>(demuxer.Tracks[0].Codec);
        Assert.Equal(CodecId.Av2, video.Codec);
        Assert.Equal(1920, video.Width);

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
    public void Ctor_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new MatroskaMuxer(null!));
    }

    [Fact]
    public void Ctor_NonWritableStream_Throws()
    {
        var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ArgumentException>(() => new MatroskaMuxer(ms));
    }

    [Fact]
    public void FormatName_Matroska_By_Default()
    {
        using var ms = new MemoryStream();
        using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        Assert.Equal("matroska", mux.FormatName);
    }

    [Fact]
    public void FormatName_Webm_When_Flagged()
    {
        using var ms = new MemoryStream();
        using var mux = new MatroskaMuxer(ms, webm: true, leaveOpen: true);
        Assert.Equal("webm", mux.FormatName);
    }

    [Fact]
    public void AddTrack_Null_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        Assert.Throws<ArgumentNullException>(() => mux.AddTrack(null!));
    }

    [Fact]
    public void AddTrack_Unsupported_Codec_Throws()
    {
        using var ms = new MemoryStream();
        using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        var t = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.AmrNb, SampleRate = 8000, Channels = 1 },
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(t));
    }

    [Fact]
    public async Task AddTrack_After_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        var t = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        };
        mux.AddTrack(t);
        await mux.StartAsync();
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(t));
    }

    [Fact]
    public async Task StartAsync_Without_Tracks_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task StartAsync_Is_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.StartAsync();
        long len1 = ms.Length;
        await mux.StartAsync();
        Assert.Equal(len1, ms.Length);
    }

    [Fact]
    public async Task WriteSample_Without_Start_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 20,
                IsKeyFrame = true, Data = new byte[10],
            }));
    }

    [Fact]
    public async Task WriteSample_After_Finish_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.StartAsync();
        await mux.FinishAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 20,
                IsKeyFrame = true, Data = new byte[10],
            }));
    }

    [Fact]
    public async Task WriteSample_Bad_TrackIndex_Throws()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.StartAsync();
        await Assert.ThrowsAsync<ArgumentOutOfRangeException>(async () =>
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 99, Pts = 0, Dts = 0, Duration = 20,
                IsKeyFrame = true, Data = new byte[10],
            }));
    }

    [Fact]
    public async Task FinishAsync_Is_Idempotent()
    {
        await using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync();
    }

    [Fact]
    public async Task Multiple_Tracks_RoundTrip()
    {
        var audio = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        };
        var video = new MediaTrack
        {
            Index = 1, Id = 2,
            TimeBase = new Rational(1, 1000),
            Codec = new VideoCodecParameters { Codec = CodecId.Vp9, Width = 320, Height = 240 },
        };
        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(audio);
            mux.AddTrack(video);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 20, IsKeyFrame = true,
                Data = new byte[64],
            });
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 1, Pts = 0, Dts = 0, Duration = 40, IsKeyFrame = true,
                Data = new byte[128],
            });
            await mux.FinishAsync();
        }
        byte[] bytes = ms.ToArray();
        using var src = new MemoryRandomAccessSource(bytes);
        using var demuxer = MatroskaDemuxer.Open(src);
        Assert.Equal(2, demuxer.Tracks.Count);
    }

    [Fact]
    public async Task LargeTimestamp_Gap_Forces_New_Cluster()
    {
        // > MaxClusterSpanMs (30 s) gap between consecutive samples must
        // create a new cluster so both samples are still recoverable.
        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        };
        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 20, IsKeyFrame = true,
                Data = new byte[32],
            });
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 60_000, Dts = 60_000, Duration = 20, IsKeyFrame = true,
                Data = new byte[32],
            });
            await mux.FinishAsync();
        }
        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dx = MatroskaDemuxer.Open(src);
        int seen = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { seen++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(2, seen);
    }

    [Fact]
    public async Task DisposeAsync_Without_Start_Does_Not_Throw()
    {
        await using var ms = new MemoryStream();
        var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.DisposeAsync();
    }

    [Fact]
    public async Task Dispose_LeaveOpen_True_Preserves_Stream()
    {
        var ms = new MemoryStream();
        var mux = new MatroskaMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.StartAsync();
        await mux.DisposeAsync();
        Assert.True(ms.CanWrite);
        ms.Dispose();
    }

    [Fact]
    public async Task Dispose_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new MatroskaMuxer(ms, leaveOpen: false);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 1000),
            Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
        });
        await mux.StartAsync();
        await mux.DisposeAsync();
        Assert.False(ms.CanWrite);
    }
}
