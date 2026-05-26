using Mediar.Containers.Voc;
using Xunit;

namespace Mediar.Tests;

public sealed class VocRoundTripTests
{
    [Fact]
    public async Task PcmU8_RoundTrips_Through_VocMuxer_V9()
    {
        const int sr = 22050;
        const int frames = 4096;
        byte[] pcm = new byte[frames];
        for (int i = 0; i < frames; i++) pcm[i] = (byte)(i & 0xFF);

        byte[] voc;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new VocMuxer(ms, leaveOpen: true);
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                TimeBase = new Rational(1, sr),
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.PcmU8, SampleRate = sr, Channels = 1, BitsPerSample = 8,
                },
            });
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = frames, IsKeyFrame = true, Data = pcm,
            });
            await mux.FinishAsync();
            voc = ms.ToArray();
        }

        // "Creative Voice File\x1A" magic at offset 0.
        Assert.Equal((byte)'C', voc[0]);
        Assert.Equal(0x1A, voc[19]);

        using var src = new IO.MemoryRandomAccessSource(voc);
        using var dx = VocDemuxer.Open(src);

        Assert.Equal("voc", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmU8, a.Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(1, a.Channels);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public void Throws_On_Missing_Magic()
    {
        byte[] junk = new byte[64];
        using var src = new IO.MemoryRandomAccessSource(junk);
        Assert.Throws<InvalidDataException>(() => VocDemuxer.Open(src));
    }
}
