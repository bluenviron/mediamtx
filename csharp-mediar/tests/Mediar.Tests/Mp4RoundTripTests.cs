using Mediar.Containers.IsoBmff;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// End-to-end smoke tests that build a tiny audio MP4 in memory, demux it, mux it back,
/// and verify the round-trip preserves track metadata and sample bytes.
/// </summary>
public sealed class Mp4RoundTripTests
{
    [Fact]
    public async Task SingleAudioTrack_RoundTrips_Through_Mp4Muxer()
    {
        // 1) Mux an in-memory MP4 with one fake "Opus" audio track containing 5 packets
        //    of trivially-recognizable payload.
        byte[] originalBytes;
        await using (var ms = new MemoryStream())
        {
            await using var muxer = new Mp4Muxer(ms, leaveOpen: true);
            var track = new MediaTrack
            {
                Index = 0,
                Id = 1,
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.Opus,
                    SampleRate = 48000,
                    Channels = 2,
                },
                TimeBase = new Rational(1, 48000),
                Language = "und",
            };
            muxer.AddTrack(track);
            await muxer.StartAsync();

            const int framesPerPacket = 960; // 20 ms at 48 kHz
            for (int i = 0; i < 5; i++)
            {
                byte[] payload = new byte[8];
                payload[0] = (byte)i;
                payload[7] = 0xAB;
                await muxer.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * framesPerPacket,
                    Dts = i * framesPerPacket,
                    Duration = framesPerPacket,
                    IsKeyFrame = true,
                    Data = payload,
                });
            }
            await muxer.FinishAsync();
            originalBytes = ms.ToArray();
        }

        Assert.True(originalBytes.Length > 0);

        // 2) Demux it.
        using var source = new MemoryRandomAccessSource(originalBytes);
        using var demuxer = new Mp4Demuxer(source);

        Assert.Equal("mp4", demuxer.FormatName);
        Assert.Single(demuxer.Tracks);
        var demuxedTrack = demuxer.Tracks[0];
        Assert.Equal(StreamKind.Audio, demuxedTrack.Kind);
        Assert.Equal(CodecId.Opus, demuxedTrack.Codec.Codec);
        var audio = Assert.IsType<AudioCodecParameters>(demuxedTrack.Codec);
        Assert.Equal(48000, audio.SampleRate);
        Assert.Equal(2, audio.Channels);

        var samples = new List<(long Pts, byte[] Data)>();
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            try
            {
                samples.Add((s.Pts, s.Data.ToArray()));
            }
            finally
            {
                s.Owner?.Dispose();
            }
        }

        Assert.Equal(5, samples.Count);
        for (int i = 0; i < 5; i++)
        {
            Assert.Equal(i * 960L, samples[i].Pts);
            Assert.Equal(8, samples[i].Data.Length);
            Assert.Equal(i, samples[i].Data[0]);
            Assert.Equal(0xAB, samples[i].Data[7]);
        }
    }
}
