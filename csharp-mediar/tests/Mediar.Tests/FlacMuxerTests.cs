using Mediar.Containers.Flac;
using Xunit;

namespace Mediar.Tests;

public sealed class FlacMuxerTests
{
    [Fact]
    public async Task Writes_FLaC_Marker_And_StreamInfo()
    {
        byte[] streamInfo = new byte[34];
        for (int i = 0; i < 34; i++) streamInfo[i] = (byte)(0x80 + i);

        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Flac,
                SampleRate = 44100,
                Channels = 2,
                BitsPerSample = 16,
                ExtraData = streamInfo,
            },
            TimeBase = new Rational(1, 44100),
        };

        byte[] frame = new byte[256];
        frame[0] = 0xFF; frame[1] = 0xF8;

        using var ms = new MemoryStream();
        await using (var mux = new FlacMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 4096,
                IsKeyFrame = true, Data = frame,
            });
            await mux.FinishAsync();
        }

        var bytes = ms.ToArray();
        Assert.Equal((byte)'f', bytes[0]);
        Assert.Equal((byte)'L', bytes[1]);
        Assert.Equal((byte)'a', bytes[2]);
        Assert.Equal((byte)'C', bytes[3]);
        Assert.Equal(0x80, bytes[4]);
        Assert.Equal(0x00, bytes[5]);
        Assert.Equal(0x00, bytes[6]);
        Assert.Equal(0x22, bytes[7]);
        Assert.Equal(0xFF, bytes[42]);
        Assert.Equal(0xF8, bytes[43]);
    }
}
