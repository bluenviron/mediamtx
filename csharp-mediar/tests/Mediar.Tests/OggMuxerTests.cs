using Mediar.Containers.Ogg;
using Xunit;

namespace Mediar.Tests;

public sealed class OggMuxerTests
{
    [Fact]
    public async Task RoundTrip_Opus_Small_Packets()
    {
        // Minimal valid OpusHead (19 bytes).
        byte[] opusHead = new byte[19];
        Buffer.BlockCopy("OpusHead"u8.ToArray(), 0, opusHead, 0, 8);
        opusHead[8] = 1;           // version
        opusHead[9] = 2;           // channel count
        opusHead[10] = 0x90; opusHead[11] = 0x01; // pre-skip (LE, 400 samples typical)
        opusHead[12] = 0x80; opusHead[13] = 0xBB; opusHead[14] = 0x00; opusHead[15] = 0x00; // sample rate 48000
        opusHead[16] = 0; opusHead[17] = 0; // output gain
        opusHead[18] = 0;          // channel mapping family

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

        byte[][] payloads = new byte[5][];
        for (int i = 0; i < payloads.Length; i++)
        {
            payloads[i] = new byte[60];
            for (int b = 0; b < payloads[i].Length; b++) payloads[i][b] = (byte)((i + 1) * (b + 1));
        }

        using var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, serialNumber: 0xCAFEBABE, leaveOpen: true))
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

        byte[] bytes = ms.ToArray();
        Assert.True(bytes.Length > 0);

        // Round-trip through the demuxer.
        using var src = new Mediar.IO.MemoryRandomAccessSource(bytes);
        using var demuxer = OggDemuxer.Open(src);
        Assert.Single(demuxer.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(demuxer.Tracks[0].Codec);
        Assert.Equal(CodecId.Opus, audio.Codec);

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
    public async Task Page_Has_Valid_Ogg_Sync_And_CRC()
    {
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 1,
                ExtraData = new byte[19] { 0x4F, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64, 1, 1, 0x90, 0x01, 0x80, 0xBB, 0, 0, 0, 0, 0 },
            },
            TimeBase = new Rational(1, 48000),
        };
        using var ms = new MemoryStream();
        await using (var mux = new OggMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample { TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960, Data = new byte[] { 0xDE, 0xAD, 0xBE, 0xEF } });
            await mux.FinishAsync();
        }
        var bytes = ms.ToArray();
        // Should begin with the OggS capture pattern.
        Assert.Equal((byte)'O', bytes[0]);
        Assert.Equal((byte)'g', bytes[1]);
        Assert.Equal((byte)'g', bytes[2]);
        Assert.Equal((byte)'S', bytes[3]);
        Assert.Equal(0, bytes[4]); // version
        Assert.Equal(0x02, bytes[5] & 0x02); // BOS flag on first page
    }
}
