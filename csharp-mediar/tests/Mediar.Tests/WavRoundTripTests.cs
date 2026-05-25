using Mediar.Containers.Wav;
using Xunit;

namespace Mediar.Tests;

public sealed class WavRoundTripTests
{
    [Fact]
    public async Task PcmS16Le_RoundTrips_Through_WavMuxer()
    {
        // Build a 1-second 16-bit stereo 48 kHz PCM sine in memory.
        const int sr = 48000;
        const int ch = 2;
        const int frames = sr; // 1 second
        byte[] pcm = new byte[frames * ch * 2];
        for (int i = 0; i < frames; i++)
        {
            short value = (short)(Math.Sin(2 * Math.PI * 440.0 * i / sr) * 16000);
            int o = i * 4;
            pcm[o + 0] = (byte)value; pcm[o + 1] = (byte)(value >> 8);
            pcm[o + 2] = (byte)value; pcm[o + 3] = (byte)(value >> 8);
        }

        byte[] wavBytes;
        await using (var ms = new MemoryStream())
        {
            await using var muxer = new WavMuxer(ms, leaveOpen: true);
            muxer.AddTrack(new MediaTrack
            {
                Index = 0,
                Id = 1,
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.PcmS16Le,
                    SampleRate = sr,
                    Channels = ch,
                    BitsPerSample = 16,
                },
                TimeBase = new Rational(1, sr),
            });
            await muxer.StartAsync();
            await muxer.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = frames,
                IsKeyFrame = true,
                Data = pcm,
            });
            await muxer.FinishAsync();
            wavBytes = ms.ToArray();
        }

        Assert.True(wavBytes.Length > pcm.Length);

        using var source = new IO.MemoryRandomAccessSource(wavBytes);
        using var demuxer = WavDemuxer.Open(source);

        Assert.Equal("wav", demuxer.FormatName);
        Assert.Single(demuxer.Tracks);
        var t = demuxer.Tracks[0];
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        Assert.Equal(sr, audio.SampleRate);
        Assert.Equal(ch, audio.Channels);

        int totalBytes = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            try { totalBytes += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, totalBytes);
    }
}
