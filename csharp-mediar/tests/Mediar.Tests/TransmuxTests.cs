using Mediar.Containers.Ogg;
using Xunit;

namespace Mediar.Tests;

public sealed class TransmuxTests
{
    [Fact]
    public async Task Transmux_Ogg_To_Ogg_Roundtrips_Samples()
    {
        // Build a tiny Opus-in-Ogg source file on disk.
        byte[] opusHead = new byte[19];
        Buffer.BlockCopy("OpusHead"u8.ToArray(), 0, opusHead, 0, 8);
        opusHead[8] = 1;            // version
        opusHead[9] = 2;            // channels
        opusHead[10] = 0x90; opusHead[11] = 0x01;
        opusHead[12] = 0x80; opusHead[13] = 0xBB; opusHead[14] = 0x00; opusHead[15] = 0x00;
        opusHead[16] = 0; opusHead[17] = 0; opusHead[18] = 0;

        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 2,
                ExtraData = opusHead,
            },
            TimeBase = new Rational(1, 48000),
        };

        byte[][] payloads = new byte[4][];
        for (int i = 0; i < payloads.Length; i++)
        {
            payloads[i] = new byte[40];
            for (int b = 0; b < payloads[i].Length; b++) payloads[i][b] = (byte)((i * 13 + b) & 0xFF);
        }

        var src = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName() + ".ogg");
        var dst = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName() + ".ogg");
        try
        {
            await using (var fs = File.Create(src))
            await using (var mux = new OggMuxer(fs))
            {
                mux.AddTrack(track);
                await mux.StartAsync();
                for (int i = 0; i < payloads.Length; i++)
                {
                    await mux.WriteSampleAsync(new MediaSample
                    {
                        TrackIndex = 0,
                        Pts = i * 960L,
                        Dts = i * 960L,
                        Duration = 960,
                        IsKeyFrame = true,
                        Data = payloads[i],
                    });
                }
                await mux.FinishAsync();
            }

            await MediarOperations.TransmuxAsync(src, dst);

            await using var dem = OggDemuxer.Open(dst);
            Assert.Single(dem.Tracks);
            var audio = Assert.IsType<AudioCodecParameters>(dem.Tracks[0].Codec);
            Assert.Equal(CodecId.Opus, audio.Codec);

            int recovered = 0;
            await foreach (var s in dem.ReadSamplesAsync())
            {
                Assert.Equal(payloads[recovered], s.Data.ToArray());
                s.Owner?.Dispose();
                recovered++;
            }
            Assert.Equal(payloads.Length, recovered);
        }
        finally
        {
            try { File.Delete(src); } catch { /* best effort */ }
            try { File.Delete(dst); } catch { /* best effort */ }
        }
    }
}
