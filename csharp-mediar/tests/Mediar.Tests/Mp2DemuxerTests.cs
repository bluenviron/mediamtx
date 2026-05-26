using Mediar.Containers.Mp2;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp2DemuxerTests
{
    // Build a synthetic MPEG-1 Layer-II frame at 48 kHz / 128 kbps / mono.
    // Header bytes derived per ISO/IEC 11172-3:
    //   FF FD 84 C0  → MPEG-1, Layer II, 128 kbps, 48 kHz, mono, no padding.
    // Frame size = 144 * bitrate / sampleRate = 144 * 128000 / 48000 = 384 bytes.
    private const byte H0 = 0xFF;
    private const byte H1 = 0xFD;
    private const byte H2 = 0x84;
    private const byte H3 = 0xC0;
    private const int FrameSize = 384;
    private const int SamplesPerFrame = 1152;

    private static byte[] BuildStream(int frameCount)
    {
        byte[] bytes = new byte[FrameSize * frameCount];
        for (int f = 0; f < frameCount; f++)
        {
            int o = f * FrameSize;
            bytes[o + 0] = H0;
            bytes[o + 1] = H1;
            bytes[o + 2] = H2;
            bytes[o + 3] = H3;
            for (int i = 4; i < FrameSize; i++) bytes[o + i] = (byte)(i & 0xFF);
        }
        return bytes;
    }

    [Fact]
    public async Task Reads_Mp2_Frames_And_Reports_Codec()
    {
        byte[] stream = BuildStream(4);
        using var src = new IO.MemoryRandomAccessSource(stream);
        using var dx = Mp2Demuxer.Open(src);

        Assert.Equal("mp2", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.Mp2, a.Codec);
        Assert.Equal(48000, a.SampleRate);
        Assert.Equal(1, a.Channels);

        int seen = 0;
        long lastDuration = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try
            {
                Assert.Equal(FrameSize, s.Data.Length);
                lastDuration = s.Duration;
                seen++;
            }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(4, seen);
        Assert.Equal(SamplesPerFrame, lastDuration);
    }

    [Fact]
    public void Refuses_Layer_III_Stream()
    {
        // Header for MPEG-1 Layer-III @ 128kbps / 44.1kHz / stereo:
        // sync=11 bits, version=11, layer=01 (L3), prot=1 → byte1 = 0xFB
        // bitrate ix for M1L3 / 128k = 9 → 0b1001
        // sample ix 44100 = 0 → 0b00, padding=0, private=0 → byte2 = 0x90
        // channel mode stereo (00) → byte3 = 0x00
        byte[] frame = new byte[417]; // 144 * 128000 / 44100 = 417 (approx)
        frame[0] = 0xFF;
        frame[1] = 0xFB;
        frame[2] = 0x90;
        frame[3] = 0x00;
        using var src = new IO.MemoryRandomAccessSource(frame);
        Assert.Throws<InvalidDataException>(() => Mp2Demuxer.Open(src));
    }
}
