using System.Buffers.Binary;
using Mediar.Codecs.Alac.Decoder;
using Mediar.Containers.IsoBmff;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// MP4 muxer / demuxer integration for ALAC: round-trips the 24-byte ALAC
/// magic cookie through the `alac` sample entry and verifies the demuxer
/// strips the 4-byte FullBox prefix and presents the raw cookie body via the
/// audio track's ExtraData.
/// </summary>
public sealed class AlacMp4RoundTripTests
{
    [Fact]
    public async Task AlacTrack_RoundTrips_ThroughMp4Muxer()
    {
        byte[] cookie = BuildCookie();
        byte[] mp4 = await MuxAlacAsync(cookie);

        using var src = new MemoryRandomAccessSource(mp4);
        using var dx = new Mp4Demuxer(src);

        var t = Assert.Single(dx.Tracks);
        Assert.Equal(StreamKind.Audio, t.Kind);
        Assert.Equal(CodecId.Alac, t.Codec.Codec);

        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(44100, audio.SampleRate);
        Assert.Equal(2, audio.Channels);

        // The demuxer returns the raw alac child-box payload (4-byte FullBox
        // header + 24-byte cookie body). NormalizeCookie strips the prefix.
        Assert.Equal(28, audio.ExtraData.Length);
        var body = AlacExtraData.NormalizeCookie(audio.ExtraData.Span);
        Assert.Equal(24, body.Length);

        // Constructing a decoder against the demuxer-supplied params must work
        // end-to-end (validates the wiring path Apple → MP4 → demux → decoder).
        var config = AlacSpecificConfig.Parse(body);
        Assert.Equal(4096, config.FrameLength);
        Assert.Equal(16, config.BitDepth);
        Assert.Equal(2, config.NumChannels);
        Assert.Equal(44100, config.SampleRate);
    }

    private static byte[] BuildCookie()
    {
        var b = new byte[24];
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(0, 4), 4096);
        b[4] = 0;
        b[5] = 16;
        b[6] = (byte)AlacSpecificConfig.DefaultPb;
        b[7] = (byte)AlacSpecificConfig.DefaultMb;
        b[8] = (byte)AlacSpecificConfig.DefaultKb;
        b[9] = 2;
        BinaryPrimitives.WriteUInt16BigEndian(b.AsSpan(10, 2), 255);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(12, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(16, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(20, 4), 44100);
        return b;
    }

    private static async Task<byte[]> MuxAlacAsync(byte[] cookie)
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Alac,
                SampleRate = 44100,
                Channels = 2,
                BitsPerSample = 16,
                ExtraData = cookie,
            },
            TimeBase = new Rational(1, 44100),
            Language = "und",
        });
        await mux.StartAsync();
        // One dummy sample is enough — we only care about cookie round-trip.
        await mux.WriteSampleAsync(new MediaSample
        {
            TrackIndex = 0,
            Pts = 0,
            Dts = 0,
            Duration = 4096,
            IsKeyFrame = true,
            Data = new byte[] { 0x07 }, // 3-bit END tag in the high bits, byte-aligned
        });
        await mux.FinishAsync();
        return ms.ToArray();
    }
}
