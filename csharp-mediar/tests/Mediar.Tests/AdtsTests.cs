using Mediar.Containers.Adts;
using Xunit;

namespace Mediar.Tests;

public sealed class AdtsTests
{
    [Fact]
    public async Task RoundTrip_SingleFrame()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Aac, SampleRate = 44100, Channels = 2 },
            TimeBase = new Rational(1, 44100),
        };

        byte[] aacPayload = new byte[64];
        for (int i = 0; i < aacPayload.Length; i++) aacPayload[i] = (byte)i;

        using var ms = new MemoryStream();
        await using (var mux = new AdtsMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = 1024,
                IsKeyFrame = true,
                Data = aacPayload,
            });
            await mux.FinishAsync();
        }

        byte[] bytes = ms.ToArray();
        Assert.True(AdtsHeader.TryParse(bytes, out var hdr));
        Assert.Equal(44100, hdr.SampleRate);
        Assert.Equal(2, hdr.ChannelConfig);
        Assert.Equal(7 + 64, hdr.FrameSize);
    }

    [Fact]
    public async Task Demuxer_Reads_Frames()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Aac, SampleRate = 48000, Channels = 2 },
            TimeBase = new Rational(1, 48000),
        };
        byte[] payload = new byte[128];
        for (int i = 0; i < payload.Length; i++) payload[i] = (byte)(i ^ 0xAA);

        using var ms = new MemoryStream();
        await using (var mux = new AdtsMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            for (int i = 0; i < 3; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0, Pts = i * 1024, Dts = i * 1024, Duration = 1024,
                    IsKeyFrame = true, Data = payload,
                });
            }
            await mux.FinishAsync();
        }

        using var src = new Mediar.IO.MemoryRandomAccessSource(ms.ToArray());
        using var demuxer = AdtsDemuxer.Open(src);
        Assert.Single(demuxer.Tracks);
        Assert.Equal(48000, ((AudioCodecParameters)demuxer.Tracks[0].Codec).SampleRate);
        int count = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            Assert.Equal(128, s.Data.Length);
            s.Owner?.Dispose();
            count++;
        }
        Assert.Equal(3, count);
    }
}
