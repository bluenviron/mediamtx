using Mediar.Containers.Caf;
using Xunit;

namespace Mediar.Tests;

public sealed class CafRoundTripTests
{
    [Fact]
    public async Task PcmS16Be_RoundTrips_With_Info_Metadata()
    {
        // 0.25s @ 44.1 kHz stereo S16BE (CAF defaults integers to BE per spec).
        const int sr = 44100;
        const int ch = 2;
        const int frames = sr / 4;
        byte[] pcm = new byte[frames * ch * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)((i * 7) & 0x7FFF);
            int o = i * 4;
            pcm[o + 0] = (byte)(v >> 8); pcm[o + 1] = (byte)v;
            pcm[o + 2] = (byte)(v >> 8); pcm[o + 3] = (byte)v;
        }

        byte[] caf;
        await using (var ms = new MemoryStream())
        {
            await using var mux = new CafMuxer(ms, leaveOpen: true);
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                TimeBase = new Rational(1, sr),
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.PcmS16Be, SampleRate = sr, Channels = ch, BitsPerSample = 16,
                },
            });
            mux.AddInfo("title", "Hello CAF");
            mux.AddInfo("artist", "Mediar");
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = frames, IsKeyFrame = true, Data = pcm,
            });
            await mux.FinishAsync();
            caf = ms.ToArray();
        }

        Assert.True(caf.Length > pcm.Length);
        // Verify 'caff' marker.
        Assert.Equal((byte)'c', caf[0]);
        Assert.Equal((byte)'a', caf[1]);
        Assert.Equal((byte)'f', caf[2]);
        Assert.Equal((byte)'f', caf[3]);

        using var src = new IO.MemoryRandomAccessSource(caf);
        using var dx = CafDemuxer.Open(src);

        Assert.Equal("caf", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Be, a.Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(ch, a.Channels);
        Assert.Equal(16, a.BitsPerSample);

        Assert.Equal("Hello CAF", dx.Metadata.Title);
        Assert.Equal("Mediar", dx.Metadata.Artist);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Throws_On_Missing_Caff_Marker()
    {
        byte[] junk = new byte[64];
        junk[0] = (byte)'X'; junk[1] = (byte)'X'; junk[2] = (byte)'X'; junk[3] = (byte)'X';
        using var src = new IO.MemoryRandomAccessSource(junk);
        await Task.Yield();
        Assert.Throws<InvalidDataException>(() => CafDemuxer.Open(src));
    }
}
