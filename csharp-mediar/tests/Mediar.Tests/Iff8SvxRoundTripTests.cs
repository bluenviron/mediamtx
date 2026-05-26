using Mediar.Containers.Iff8Svx;
using Xunit;

namespace Mediar.Tests;

public sealed class Iff8SvxRoundTripTests
{
    [Fact]
    public async Task Pcm_S8_RoundTrips_With_All_Text_Chunks()
    {
        const int sr = 22050; // typical Amiga rate, fits ushort
        const int frames = 1024;
        byte[] pcm = new byte[frames];
        for (int i = 0; i < frames; i++)
        {
            pcm[i] = (byte)(sbyte)(((i * 5) & 0xFF) - 128); // signed 8
        }

        byte[] iff;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new Iff8SvxMuxer(ms, leaveOpen: true);
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                TimeBase = new Rational(1, sr),
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.PcmS8, SampleRate = sr, Channels = 1, BitsPerSample = 8,
                },
            });
            mux.SetTitle("Sample");
            mux.SetArtist("Mediar");
            mux.SetComment("Amiga voice");
            mux.SetCopyright("(c) MIT");
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = frames, IsKeyFrame = true, Data = pcm,
            });
            await mux.FinishAsync();
            iff = ms.ToArray();
        }

        Assert.Equal((byte)'F', iff[0]);
        Assert.Equal((byte)'O', iff[1]);
        Assert.Equal((byte)'R', iff[2]);
        Assert.Equal((byte)'M', iff[3]);
        Assert.Equal((byte)'8', iff[8]);
        Assert.Equal((byte)'S', iff[9]);
        Assert.Equal((byte)'V', iff[10]);
        Assert.Equal((byte)'X', iff[11]);

        using var src = new IO.MemoryRandomAccessSource(iff);
        using var dx = Iff8SvxDemuxer.Open(src);

        Assert.Equal("8svx", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS8, a.Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(1, a.Channels);

        Assert.Equal("Sample", dx.Metadata.Title);
        Assert.Equal("Mediar", dx.Metadata.Artist);
        Assert.Equal("Amiga voice", dx.Metadata.Comment);
        Assert.Equal("(c) MIT", dx.Metadata.Copyright);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public void Throws_On_Missing_Marker()
    {
        byte[] junk = new byte[16];
        using var src = new IO.MemoryRandomAccessSource(junk);
        Assert.Throws<InvalidDataException>(() => Iff8SvxDemuxer.Open(src));
    }
}
