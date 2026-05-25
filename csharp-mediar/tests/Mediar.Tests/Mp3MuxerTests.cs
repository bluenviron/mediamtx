using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp3MuxerTests
{
    [Fact]
    public async Task WriteSample_With_Sync_Succeeds()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Mp3, SampleRate = 44100, Channels = 2 },
            TimeBase = new Rational(1, 44100),
        };
        byte[] sample = new byte[417];
        sample[0] = 0xFF; sample[1] = 0xFB; sample[2] = 0x90; sample[3] = 0x00;

        using var ms = new MemoryStream();
        await using (var mux = new Mp3Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 1152,
                IsKeyFrame = true, Data = sample,
            });
            await mux.FinishAsync();
        }

        var bytes = ms.ToArray();
        Assert.Equal(417, bytes.Length);
        Assert.Equal(0xFF, bytes[0]);
        Assert.Equal(0xFB, bytes[1]);
    }

    [Fact]
    public async Task WriteSample_Without_Sync_Throws()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Mp3, SampleRate = 44100, Channels = 2 },
            TimeBase = new Rational(1, 44100),
        };
        using var ms = new MemoryStream();
        await using var mux = new Mp3Muxer(ms, leaveOpen: true);
        mux.AddTrack(track);
        await mux.StartAsync();
        var ex = await Assert.ThrowsAnyAsync<Exception>(async () =>
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 0,
                IsKeyFrame = true, Data = new byte[] { 0x00, 0x00, 0x00, 0x00 },
            }));
        Assert.Contains("sync", ex.Message, StringComparison.OrdinalIgnoreCase);
    }
}
