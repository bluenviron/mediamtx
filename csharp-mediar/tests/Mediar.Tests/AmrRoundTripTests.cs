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
}
