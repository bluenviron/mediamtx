using Mediar.Containers.Matroska;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class MatroskaMuxerTests
{
    [Fact]
    public async Task RoundTrip_Opus_Audio_Through_Mkv_Demuxer()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 2,
                ExtraData = new byte[19] { 0x4F, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64, 1, 2, 0x90, 0x01, 0x80, 0xBB, 0, 0, 0, 0, 0 },
            },
            TimeBase = new Rational(1, 1000),     // ms timebase
        };

        byte[][] payloads = new byte[5][];
        for (int i = 0; i < payloads.Length; i++)
        {
            payloads[i] = new byte[100 + i * 7];
            for (int b = 0; b < payloads[i].Length; b++) payloads[i][b] = (byte)((i + 13) * (b + 1));
        }

        using var ms = new MemoryStream();
        await using (var mux = new MatroskaMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            for (int i = 0; i < payloads.Length; i++)
            {
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * 20L,
                    Dts = i * 20L,
                    Duration = 20,
                    IsKeyFrame = true,
                    Data = payloads[i],
                });
            }
            await mux.FinishAsync();
        }

        byte[] bytes = ms.ToArray();
        Assert.True(bytes.Length > 100);
        // Should begin with the EBML id 1A45DFA3
        Assert.Equal(0x1A, bytes[0]);
        Assert.Equal(0x45, bytes[1]);
        Assert.Equal(0xDF, bytes[2]);
        Assert.Equal(0xA3, bytes[3]);

        using var src = new MemoryRandomAccessSource(bytes);
        using var demuxer = MatroskaDemuxer.Open(src);
        Assert.Single(demuxer.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(demuxer.Tracks[0].Codec);
        Assert.Equal(CodecId.Opus, audio.Codec);
        Assert.Equal(48000, audio.SampleRate);
        Assert.Equal(2, audio.Channels);

        int recovered = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            Assert.Equal(payloads[recovered], s.Data.ToArray());
            s.Owner?.Dispose();
            recovered++;
        }
        Assert.Equal(payloads.Length, recovered);
    }

    [Fact]
    public async Task Webm_Rejects_NonWebm_Codec()
    {
        using var ms = new MemoryStream();
        await using var mux = new MatroskaMuxer(ms, webm: true, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.Flac, SampleRate = 44100, Channels = 2 },
            TimeBase = new Rational(1, 44100),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }
}
