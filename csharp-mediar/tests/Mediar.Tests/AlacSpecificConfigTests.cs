using Mediar.Codecs.Alac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AlacSpecificConfigTests
{
    private static byte[] BuildCookie(
        int frameLength = 4096, byte ver = 0, byte bitDepth = 16,
        byte pb = 40, byte mb = 10, byte kb = 14, byte channels = 2,
        ushort maxRun = 255, int maxFrameBytes = 1234, int avgBitRate = 256000,
        int sampleRate = 44100)
    {
        var b = new byte[24];
        b[0] = (byte)(frameLength >> 24);
        b[1] = (byte)(frameLength >> 16);
        b[2] = (byte)(frameLength >> 8);
        b[3] = (byte)frameLength;
        b[4] = ver;
        b[5] = bitDepth;
        b[6] = pb;
        b[7] = mb;
        b[8] = kb;
        b[9] = channels;
        b[10] = (byte)(maxRun >> 8);
        b[11] = (byte)maxRun;
        b[12] = (byte)(maxFrameBytes >> 24);
        b[13] = (byte)(maxFrameBytes >> 16);
        b[14] = (byte)(maxFrameBytes >> 8);
        b[15] = (byte)maxFrameBytes;
        b[16] = (byte)(avgBitRate >> 24);
        b[17] = (byte)(avgBitRate >> 16);
        b[18] = (byte)(avgBitRate >> 8);
        b[19] = (byte)avgBitRate;
        b[20] = (byte)(sampleRate >> 24);
        b[21] = (byte)(sampleRate >> 16);
        b[22] = (byte)(sampleRate >> 8);
        b[23] = (byte)sampleRate;
        return b;
    }

    [Fact]
    public void Parse_ValidCookie_RoundTrips()
    {
        var bytes = BuildCookie();
        var c = AlacSpecificConfig.Parse(bytes);
        Assert.Equal(4096, c.FrameLength);
        Assert.Equal(16, c.BitDepth);
        Assert.Equal(40, c.Pb);
        Assert.Equal(10, c.Mb);
        Assert.Equal(14, c.Kb);
        Assert.Equal(2, c.NumChannels);
        Assert.Equal(255, c.MaxRun);
        Assert.Equal(44100, c.SampleRate);
        Assert.Equal(256000, c.AvgBitRate);
    }

    [Fact]
    public void Parse_TooShort_Throws()
    {
        Assert.Throws<ArgumentException>(() => AlacSpecificConfig.Parse(new byte[10]));
    }

    [Fact]
    public void Parse_InvalidBitDepth_Throws()
    {
        var bytes = BuildCookie(bitDepth: 17);
        Assert.Throws<InvalidDataException>(() => AlacSpecificConfig.Parse(bytes));
    }

    [Fact]
    public void Parse_TooManyChannels_Throws()
    {
        var bytes = BuildCookie(channels: 9);
        Assert.Throws<InvalidDataException>(() => AlacSpecificConfig.Parse(bytes));
    }

    [Fact]
    public void Parse_ZeroFrameLength_Throws()
    {
        var bytes = BuildCookie(frameLength: 0);
        Assert.Throws<InvalidDataException>(() => AlacSpecificConfig.Parse(bytes));
    }

    [Fact]
    public void NormalizeCookie_Raw24_ReturnedAsIs()
    {
        var bytes = BuildCookie();
        var body = AlacExtraData.NormalizeCookie(bytes);
        Assert.Equal(bytes.Length, body.Length);
    }

    [Fact]
    public void NormalizeCookie_FullBoxPrefixed_Strips4Bytes()
    {
        var body = BuildCookie();
        byte[] withPrefix = new byte[4 + body.Length];
        Array.Copy(body, 0, withPrefix, 4, body.Length);
        // Prefix is anything (typically 0x00000000 for version/flags).
        var normalized = AlacExtraData.NormalizeCookie(withPrefix);
        Assert.Equal(body.Length, normalized.Length);
        Assert.True(normalized.SequenceEqual(body));
    }

    [Fact]
    public void NormalizeCookie_BoxedFrmaPlusAlac_LocatesBody()
    {
        var body = BuildCookie();
        // Build a synthetic CAF-style boxed cookie:
        //   ['frma' atom containing 'alac']
        //   ['alac' atom containing 4-byte FullBox + body]
        using var ms = new MemoryStream();
        // frma atom: size(4) + 'frma'(4) + 'alac'(4) = 12 bytes
        Span<byte> frma = stackalloc byte[12];
        frma[0] = 0; frma[1] = 0; frma[2] = 0; frma[3] = 12;
        frma[4] = (byte)'f'; frma[5] = (byte)'r'; frma[6] = (byte)'m'; frma[7] = (byte)'a';
        frma[8] = (byte)'a'; frma[9] = (byte)'l'; frma[10] = (byte)'a'; frma[11] = (byte)'c';
        ms.Write(frma);
        // alac atom: size(4) + 'alac'(4) + version/flags(4) + body(24) = 36 bytes
        int alacSize = 8 + 4 + body.Length;
        Span<byte> hdr = stackalloc byte[12];
        hdr[0] = (byte)(alacSize >> 24);
        hdr[1] = (byte)(alacSize >> 16);
        hdr[2] = (byte)(alacSize >> 8);
        hdr[3] = (byte)alacSize;
        hdr[4] = (byte)'a'; hdr[5] = (byte)'l'; hdr[6] = (byte)'a'; hdr[7] = (byte)'c';
        // version/flags = 0
        ms.Write(hdr);
        ms.Write(body);

        var normalized = AlacExtraData.NormalizeCookie(ms.ToArray());
        Assert.Equal(24, normalized.Length);
        Assert.True(normalized.SequenceEqual(body));
    }

    [Fact]
    public void NormalizeCookie_Unrecognised_ReturnsEmpty()
    {
        var normalized = AlacExtraData.NormalizeCookie(new byte[10]);
        Assert.True(normalized.IsEmpty);
    }
}
