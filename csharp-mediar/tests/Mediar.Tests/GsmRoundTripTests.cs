using Mediar.Containers.Gsm;
using Xunit;

namespace Mediar.Tests;

public sealed class GsmRoundTripTests
{
    [Fact]
    public async Task Gsm_RoundTrips_Frames_Through_Muxer()
    {
        const int frameCount = 50;
        byte[] frames = new byte[frameCount * GsmDemuxer.FrameBytes];
        for (int i = 0; i < frames.Length; i++) frames[i] = (byte)(i ^ 0x5A);

        byte[] file;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new GsmMuxer(ms, leaveOpen: true);
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                TimeBase = new Rational(1, 8000),
                Codec = new AudioCodecParameters { Codec = CodecId.Gsm610, SampleRate = 8000, Channels = 1 },
            });
            await mux.StartAsync();
            for (int i = 0; i < frameCount; i++)
            {
                byte[] one = new byte[GsmDemuxer.FrameBytes];
                Array.Copy(frames, i * GsmDemuxer.FrameBytes, one, 0, GsmDemuxer.FrameBytes);
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = i * GsmDemuxer.FrameSamples,
                    Dts = i * GsmDemuxer.FrameSamples,
                    Duration = GsmDemuxer.FrameSamples, IsKeyFrame = true, Data = one,
                });
            }
            await mux.FinishAsync();
            file = ms.ToArray();
        }
        Assert.Equal(frameCount * GsmDemuxer.FrameBytes, file.Length);

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = GsmDemuxer.Open(src);

        Assert.Equal("gsm", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.Gsm610, a.Codec);
        Assert.Equal(8000, a.SampleRate);

        int seen = 0;
        long totalBytes = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(GsmDemuxer.FrameBytes, s.Data.Length);
                Assert.Equal(GsmDemuxer.FrameSamples, s.Duration);
                totalBytes += s.Data.Length;
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(frameCount, seen);
        Assert.Equal(frames.Length, totalBytes);
    }

    [Fact]
    public async Task Muxer_Rejects_Non_33_Byte_Frames()
    {
        using var ms = new MemoryStream();
        using var mux = new GsmMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            TimeBase = new Rational(1, 8000),
            Codec = new AudioCodecParameters { Codec = CodecId.Gsm610, SampleRate = 8000, Channels = 1 },
        });
        var ex = await Assert.ThrowsAsync<InvalidDataException>(async () =>
        {
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 160, IsKeyFrame = true,
                Data = new byte[20],
            });
        });
        Assert.Contains("33", ex.Message);
    }
}
