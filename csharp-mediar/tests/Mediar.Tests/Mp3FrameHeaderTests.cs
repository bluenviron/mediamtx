using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp3FrameHeaderTests
{
    [Fact]
    public void Mpeg1Layer3_44100_128kbps_Stereo_Parses()
    {
        // Bytes: 11111111 11111011 10010000 00000000
        // sync = 11 bits, ver = MPEG1, layer = III, no protection, br idx 9 (128 kbps), sr idx 0 (44100),
        // no padding, no priv, stereo channel mode (00).
        byte[] header = { 0xFF, 0xFB, 0x90, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Version);
        Assert.Equal(3, hdr.Layer);
        Assert.Equal(44100, hdr.SampleRate);
        Assert.Equal(128000, hdr.Bitrate);
        Assert.Equal(2, hdr.Channels);
        Assert.Equal(1152, hdr.SamplesPerFrame);
        Assert.Equal(417, hdr.FrameSize);
    }

    [Fact]
    public void Bogus_Sync_Is_Rejected()
    {
        byte[] header = { 0x00, 0x00, 0x00, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }
}
