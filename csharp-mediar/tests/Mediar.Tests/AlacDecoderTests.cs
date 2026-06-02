using System.Buffers.Binary;
using Mediar.Codecs.Alac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AlacDecoderTests
{
    [Fact]
    public void Decode_UncompressedMono_RoundTrips16BitSamples()
    {
        short[] samples = { 0, 1, -1, 100, -100, 32767, -32768, 1234 };
        byte[] cookie = BuildCookie(frameLength: 64, bitDepth: 16, channels: 1);
        byte[] packet = BuildEscapePacket(samples, samples, frameLength: 64, bitDepth: 16, isStereo: false, partial: true, numSamples: samples.Length);

        using var decoder = new AlacDecoder(MakeParams(cookie, channels: 1));
        var frame = decoder.Decode(packet, pts: 0);

        Assert.Equal(samples.Length, frame.SamplesPerChannel);
        Assert.Equal(1, frame.Channels);

        float scale = 1f / (1 << 15);
        var span = frame.Samples.Span;
        for (int i = 0; i < samples.Length; i++)
        {
            Assert.Equal(samples[i] * scale, span[i], precision: 6);
        }
        frame.Owner?.Dispose();
    }

    [Fact]
    public void Decode_UncompressedStereo_RoundTrips16BitSamples()
    {
        short[] left = { 0, 100, -200, 32767, -32768, 50 };
        short[] right = { 1, -101, 201, -32768, 32767, -50 };
        byte[] cookie = BuildCookie(frameLength: 32, bitDepth: 16, channels: 2);
        byte[] packet = BuildEscapePacket(left, right, frameLength: 32, bitDepth: 16, isStereo: true, partial: true, numSamples: left.Length);

        using var decoder = new AlacDecoder(MakeParams(cookie, channels: 2));
        var frame = decoder.Decode(packet, pts: 0);

        Assert.Equal(left.Length, frame.SamplesPerChannel);
        Assert.Equal(2, frame.Channels);

        float scale = 1f / (1 << 15);
        var span = frame.Samples.Span;
        for (int i = 0; i < left.Length; i++)
        {
            Assert.Equal(left[i] * scale, span[i * 2 + 0], precision: 6);
            Assert.Equal(right[i] * scale, span[i * 2 + 1], precision: 6);
        }
        frame.Owner?.Dispose();
    }

    [Fact]
    public void Decode_UncompressedMono_24Bit()
    {
        int[] samples = { 0, 1, -1, 100, -100, 0x7FFFFF, -0x800000, 0x123456 };
        byte[] cookie = BuildCookie(frameLength: 64, bitDepth: 24, channels: 1);
        byte[] packet = BuildEscapePacket24(samples, samples, frameLength: 64, isStereo: false, partial: true, numSamples: samples.Length);

        using var decoder = new AlacDecoder(MakeParams(cookie, channels: 1));
        var frame = decoder.Decode(packet, pts: 0);

        Assert.Equal(samples.Length, frame.SamplesPerChannel);
        float scale = 1f / (1 << 23);
        var span = frame.Samples.Span;
        for (int i = 0; i < samples.Length; i++)
        {
            Assert.Equal(samples[i] * scale, span[i], precision: 6);
        }
        frame.Owner?.Dispose();
    }

    [Fact]
    public void Decode_PartialFrameOverride_HonoursShortenedCount()
    {
        // Cookie says 4096 samples per frame, but the packet's partial-frame
        // override declares 5. The decoder should produce exactly 5 samples.
        short[] samples = { 11, 22, 33, 44, 55 };
        byte[] cookie = BuildCookie(frameLength: 4096, bitDepth: 16, channels: 1);
        byte[] packet = BuildEscapePacket(samples, samples, frameLength: 4096, bitDepth: 16, isStereo: false, partial: true, numSamples: samples.Length);

        using var decoder = new AlacDecoder(MakeParams(cookie, channels: 1));
        var frame = decoder.Decode(packet, pts: 0);

        Assert.Equal(samples.Length, frame.SamplesPerChannel);
        frame.Owner?.Dispose();
    }

    [Fact]
    public void Decode_EmptyPacket_ReturnsDefault()
    {
        byte[] cookie = BuildCookie(frameLength: 64, bitDepth: 16, channels: 1);
        using var decoder = new AlacDecoder(MakeParams(cookie, channels: 1));
        var frame = decoder.Decode(ReadOnlySpan<byte>.Empty, pts: 0);
        Assert.Equal(0, frame.SamplesPerChannel);
        Assert.Equal(0, frame.Channels);
    }

    [Fact]
    public void Constructor_RejectsMissingCookie()
    {
        var p = new AudioCodecParameters { Codec = CodecId.Alac, SampleRate = 44100, Channels = 1, BitsPerSample = 16 };
        Assert.Throws<ArgumentException>(() => new AlacDecoder(p));
    }

    [Fact]
    public void Constructor_RejectsWrongCodec()
    {
        byte[] cookie = BuildCookie(frameLength: 64, bitDepth: 16, channels: 1);
        var p = new AudioCodecParameters { Codec = CodecId.Mp3, SampleRate = 44100, Channels = 1, BitsPerSample = 16, ExtraData = cookie };
        Assert.Throws<ArgumentException>(() => new AlacDecoder(p));
    }

    private static AudioCodecParameters MakeParams(byte[] cookie, int channels) =>
        new AudioCodecParameters
        {
            Codec = CodecId.Alac,
            SampleRate = 44100,
            Channels = channels,
            BitsPerSample = 16,
            ExtraData = cookie,
        };

    private static byte[] BuildCookie(int frameLength, int bitDepth, int channels)
    {
        var b = new byte[24];
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(0, 4), (uint)frameLength);
        b[4] = 0;
        b[5] = (byte)bitDepth;
        b[6] = (byte)AlacSpecificConfig.DefaultPb;
        b[7] = (byte)AlacSpecificConfig.DefaultMb;
        b[8] = (byte)AlacSpecificConfig.DefaultKb;
        b[9] = (byte)channels;
        BinaryPrimitives.WriteUInt16BigEndian(b.AsSpan(10, 2), 255);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(12, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(16, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(20, 4), 44100);
        return b;
    }

    private static byte[] BuildEscapePacket(
        short[] left, short[] right, int frameLength, int bitDepth,
        bool isStereo, bool partial, int numSamples)
    {
        int bitsPerSample = bitDepth;
        int totalBits = 23 + (partial ? 32 : 0) + (isStereo ? 2 : 1) * numSamples * bitsPerSample + 3 + 7;
        int totalBytes = (totalBits + 7) / 8 + 4;
        var bytes = new byte[totalBytes];
        var bw = new Mediar.IO.BitWriter(bytes);

        // SCE=0 or CPE=1 tag (3 bits)
        bw.WriteBits(isStereo ? 1u : 0u, 3);
        // instance tag (4 bits)
        bw.WriteBits(0, 4);
        // unused (12 bits)
        bw.WriteBits(0, 12);
        // header byte: partialFrame | bytesShifted(00) | escapeFlag(1)
        uint header = (partial ? (1u << 3) : 0u) | 0x1u;
        bw.WriteBits(header, 4);

        if (partial)
        {
            bw.WriteBits((uint)(numSamples >> 16) & 0xFFFFu, 16);
            bw.WriteBits((uint)numSamples & 0xFFFFu, 16);
        }

        for (int i = 0; i < numSamples; i++)
        {
            bw.WriteBits((uint)(ushort)left[i], bitsPerSample);
            if (isStereo)
                bw.WriteBits((uint)(ushort)right[i], bitsPerSample);
        }

        // END tag (3 bits)
        bw.WriteBits(7, 3);
        bw.AlignToByte();

        return bytes.AsSpan(0, bw.BytesWritten).ToArray();
    }

    private static byte[] BuildEscapePacket24(
        int[] left, int[] right, int frameLength,
        bool isStereo, bool partial, int numSamples)
    {
        const int bitsPerSample = 24;
        int totalBits = 23 + (partial ? 32 : 0) + (isStereo ? 2 : 1) * numSamples * bitsPerSample + 3 + 7;
        int totalBytes = (totalBits + 7) / 8 + 4;
        var bytes = new byte[totalBytes];
        var bw = new Mediar.IO.BitWriter(bytes);

        bw.WriteBits(isStereo ? 1u : 0u, 3);
        bw.WriteBits(0, 4);
        bw.WriteBits(0, 12);
        uint header = (partial ? (1u << 3) : 0u) | 0x1u;
        bw.WriteBits(header, 4);

        if (partial)
        {
            bw.WriteBits((uint)(numSamples >> 16) & 0xFFFFu, 16);
            bw.WriteBits((uint)numSamples & 0xFFFFu, 16);
        }

        for (int i = 0; i < numSamples; i++)
        {
            // Write the low 24 bits of the signed sample as MSB-first; sign-
            // extension happens on the decode side.
            bw.WriteBits((uint)(left[i] & 0xFFFFFF), bitsPerSample);
            if (isStereo)
                bw.WriteBits((uint)(right[i] & 0xFFFFFF), bitsPerSample);
        }

        bw.WriteBits(7, 3);
        bw.AlignToByte();

        return bytes.AsSpan(0, bw.BytesWritten).ToArray();
    }
}
