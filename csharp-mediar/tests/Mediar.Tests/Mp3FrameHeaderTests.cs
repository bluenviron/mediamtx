using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

public sealed class Mp3FrameHeaderTests
{
    [Fact]
    public void Mpeg1Layer3_44100_128kbps_Stereo_Parses()
    {
        // Bytes: 11111111 11111011 10010000 00000000
        // sync = 11 bits, ver = MPEG1, layer = III, no protection, br idx 9 (128 kbps), sr idx 0 (44100),
        // no padding, no priv, stereo channel mode (00).
        byte[] header = { 0xFF, 0xFB, 0x90, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Version);
        Assert.Equal(3, hdr.Layer);
        Assert.Equal(44100, hdr.SampleRate);
        Assert.Equal(128000, hdr.Bitrate);
        Assert.Equal(2, hdr.Channels);
        Assert.Equal(1152, hdr.SamplesPerFrame);
        Assert.Equal(417, hdr.FrameSize);
    }

    [Fact]
    public void Bogus_Sync_Is_Rejected()
    {
        byte[] header = { 0x00, 0x00, 0x00, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }

    // ----- Truncated input -----

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    public void TryParse_TruncatedInput_Rejected(int length)
    {
        byte[] full = { 0xFF, 0xFB, 0x90, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(full.AsSpan(0, length), out _));
    }

    // ----- Reserved / invalid bit fields -----

    [Fact]
    public void TryParse_ReservedVersion_Rejected()
    {
        // b[1] = 111_01_01_1 = 0xEB (versionBits=01 → reserved)
        byte[] header = { 0xFF, 0xEB, 0x90, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }

    [Fact]
    public void TryParse_ReservedLayer_Rejected()
    {
        // b[1] = 111_11_00_1 = 0xF9 (layerBits=00 → reserved)
        byte[] header = { 0xFF, 0xF9, 0x90, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }

    [Fact]
    public void TryParse_FreeFormatBitrate_Rejected()
    {
        // b[2] = 0000_0000 → bitrateIx=0 (free-format) — not yet supported.
        byte[] header = { 0xFF, 0xFB, 0x00, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }

    [Fact]
    public void TryParse_BadBitrateIndex_Rejected()
    {
        // b[2] = 1111_0000 → bitrateIx=15 (forbidden).
        byte[] header = { 0xFF, 0xFB, 0xF0, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }

    [Fact]
    public void TryParse_ReservedSampleRateIndex_Rejected()
    {
        // b[2] = 1001_1100 → bitrateIx=9, sampleIx=3 (reserved).
        byte[] header = { 0xFF, 0xFB, 0x9C, 0x00 };
        Assert.False(Mp3FrameHeader.TryParse(header, out _));
    }

    // ----- Layer / version combinations -----

    [Fact]
    public void Mpeg1Layer1_44100_288kbps_Stereo_Parses()
    {
        // b[1] = 111_11_11_1 = 0xFF (M1 L1 prot=1), b[2] = 1001_0000 (br idx 9 = 288 kbps for M1L1, sr=0=44100, pad=0)
        byte[] header = { 0xFF, 0xFF, 0x90, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Version);
        Assert.Equal(1, hdr.Layer);
        Assert.Equal(44100, hdr.SampleRate);
        Assert.Equal(288000, hdr.Bitrate);
        Assert.Equal(2, hdr.Channels);
        Assert.Equal(384, hdr.SamplesPerFrame);
        // Layer-I formula: (12*br/sr + pad)*4 = (12*288000/44100 + 0)*4 = 78*4 = 312.
        Assert.Equal(312, hdr.FrameSize);
    }

    [Fact]
    public void Mpeg1Layer2_44100_192kbps_Stereo_Parses()
    {
        // b[1] = 111_11_10_1 = 0xFD (M1 L2 prot=1), b[2] = 1010_0000 (br=10=192, sr=0=44100, pad=0)
        byte[] header = { 0xFF, 0xFD, 0xA0, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Version);
        Assert.Equal(2, hdr.Layer);
        Assert.Equal(44100, hdr.SampleRate);
        Assert.Equal(192000, hdr.Bitrate);
        Assert.Equal(1152, hdr.SamplesPerFrame);
        // L2/L3 formula with coef=144: 144*192000/44100 + 0 = 626.
        Assert.Equal(626, hdr.FrameSize);
    }

    [Fact]
    public void Mpeg2Layer3_22050_64kbps_Stereo_Parses()
    {
        // b[1] = 111_10_01_1 = 0xF3 (M2 L3 prot=1)
        // M2 L2/L3 row: idx 8 = 64 kbps; sr idx 0 for M2 = 22050; layer 3 + version != 1 → samples=576, coef=72.
        // b[2] = 1000_0000 (br=8, sr=0, pad=0)
        byte[] header = { 0xFF, 0xF3, 0x80, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(2, hdr.Version);
        Assert.Equal(3, hdr.Layer);
        Assert.Equal(22050, hdr.SampleRate);
        Assert.Equal(64000, hdr.Bitrate);
        Assert.Equal(576, hdr.SamplesPerFrame);
        // 72 * 64000 / 22050 + 0 = 4608000 / 22050 = 208 (int trunc).
        Assert.Equal(208, hdr.FrameSize);
    }

    [Fact]
    public void Mpeg25Layer3_11025_64kbps_Mono_Parses()
    {
        // b[1] = 111_00_01_1 = 0xE3 (M2.5 L3 prot=1)
        // M2 L2/L3 row idx 8 = 64; sr idx 0 for M2.5 = 11025; samples=576, coef=72.
        // b[3] = 1100_0000 = 0xC0 (channelMode=3 → mono)
        byte[] header = { 0xFF, 0xE3, 0x80, 0xC0 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(25, hdr.Version);
        Assert.Equal(3, hdr.Layer);
        Assert.Equal(11025, hdr.SampleRate);
        Assert.Equal(64000, hdr.Bitrate);
        Assert.Equal(1, hdr.Channels);
        Assert.Equal(576, hdr.SamplesPerFrame);
        // 72 * 64000 / 11025 + 0 = 4608000 / 11025 = 417.
        Assert.Equal(417, hdr.FrameSize);
    }

    // ----- Padding / channel mode -----

    [Fact]
    public void TryParse_PaddingSet_Layer3_AddsOneByteToFrameSize()
    {
        // Same as the baseline 128 kbps / 44100 / L3, but with padding=1.
        // b[2] = 1001_0010 (br=9, sr=0, pad=1).
        byte[] header = { 0xFF, 0xFB, 0x92, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Padding);
        // 144*128000/44100 + 1 = 417 + 1.
        Assert.Equal(418, hdr.FrameSize);
    }

    [Fact]
    public void TryParse_PaddingSet_Layer1_AddsFourBytesToFrameSize()
    {
        // M1 L1 288kbps/44100 with padding=1.
        byte[] header = { 0xFF, 0xFF, 0x92, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Padding);
        // (12*288000/44100 + 1) * 4 = (78 + 1) * 4 = 316.
        Assert.Equal(316, hdr.FrameSize);
    }

    [Theory]
    [InlineData(0x00, 2)] // stereo
    [InlineData(0x40, 2)] // joint stereo
    [InlineData(0x80, 2)] // dual channel
    [InlineData(0xC0, 1)] // mono
    public void TryParse_ChannelModeMapsToChannelCount(byte byte3, int expectedChannels)
    {
        byte[] header = { 0xFF, 0xFB, 0x90, byte3 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedChannels, hdr.Channels);
    }

    // ----- Sample rate index sweeps -----

    [Theory]
    [InlineData(0x00, 44100)] // M1, sr idx 0
    [InlineData(0x04, 48000)] // M1, sr idx 1
    [InlineData(0x08, 32000)] // M1, sr idx 2
    public void TryParse_Mpeg1SampleRateIndices(byte byte2Low, int expectedSampleRate)
    {
        // b[2] = 1001_<srix-pad> with br=9. byte2Low encodes sr and pad.
        byte b2 = (byte)(0x90 | byte2Low);
        byte[] header = { 0xFF, 0xFB, b2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedSampleRate, hdr.SampleRate);
    }

    [Theory]
    [InlineData(0x00, 22050)] // M2, sr idx 0
    [InlineData(0x04, 24000)] // M2, sr idx 1
    [InlineData(0x08, 16000)] // M2, sr idx 2
    public void TryParse_Mpeg2SampleRateIndices(byte byte2Low, int expectedSampleRate)
    {
        byte b2 = (byte)(0x80 | byte2Low); // br=8=64kbps for M2 L3, padding=0
        byte[] header = { 0xFF, 0xF3, b2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedSampleRate, hdr.SampleRate);
    }

    [Fact]
    public void TryParse_EmptySpan_Returns_False()
    {
        Assert.False(Mp3FrameHeader.TryParse(ReadOnlySpan<byte>.Empty, out _));
    }

    [Fact]
    public void TryParse_ExtraBytes_Ignored()
    {
        // 6 bytes - parser only uses the first 4.
        byte[] header = { 0xFF, 0xFB, 0x90, 0x00, 0xAB, 0xCD };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(128000, hdr.Bitrate);
        Assert.Equal(417, hdr.FrameSize);
    }

    [Fact]
    public void Mpeg2_Layer1_32kbps_24000_Stereo_Parses()
    {
        // M2 L1: b[1] = 111_10_11_1 = 0xF7. b[2] = 0001_0100 (br=1=32 kbps M2L1, sr=1=24000, pad=0).
        byte[] header = { 0xFF, 0xF7, 0x14, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(2, hdr.Version);
        Assert.Equal(1, hdr.Layer);
        Assert.Equal(32000, hdr.Bitrate);
        Assert.Equal(24000, hdr.SampleRate);
        Assert.Equal(384, hdr.SamplesPerFrame);
        Assert.Equal(2, hdr.Channels);
        // (12 * 32000 / 24000 + 0) * 4 = 16 * 4 = 64.
        Assert.Equal(64, hdr.FrameSize);
    }

    [Fact]
    public void ProtectionBit_Cleared_Does_Not_Affect_Parse()
    {
        // Same as baseline (0xFB) but with protection bit cleared → 0xFA.
        byte[] header = { 0xFF, 0xFA, 0x90, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Version);
        Assert.Equal(3, hdr.Layer);
        Assert.Equal(417, hdr.FrameSize);
    }

    [Fact]
    public void PrivateBit_Set_Does_Not_Affect_Parse()
    {
        // Set the private bit (low bit of b[2]): 0x90 | 0x01 = 0x91.
        byte[] header = { 0xFF, 0xFB, 0x91, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(128000, hdr.Bitrate);
        Assert.Equal(417, hdr.FrameSize);
    }

    [Theory]
    [InlineData(0x10, 32000)]
    [InlineData(0x20, 40000)]
    [InlineData(0x30, 48000)]
    [InlineData(0x40, 56000)]
    [InlineData(0x50, 64000)]
    [InlineData(0x60, 80000)]
    [InlineData(0x70, 96000)]
    [InlineData(0x80, 112000)]
    [InlineData(0x90, 128000)]
    [InlineData(0xA0, 160000)]
    [InlineData(0xB0, 192000)]
    [InlineData(0xC0, 224000)]
    [InlineData(0xD0, 256000)]
    [InlineData(0xE0, 320000)]
    public void Mpeg1Layer3_AllValidBitrateIndices_Parse(byte byte2, int expectedBitrate)
    {
        byte[] header = { 0xFF, 0xFB, byte2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedBitrate, hdr.Bitrate);
    }

    [Theory]
    [InlineData(0x10, 8000)]
    [InlineData(0x20, 16000)]
    [InlineData(0x30, 24000)]
    [InlineData(0x40, 32000)]
    [InlineData(0x50, 40000)]
    [InlineData(0x60, 48000)]
    [InlineData(0x70, 56000)]
    [InlineData(0x80, 64000)]
    [InlineData(0x90, 80000)]
    [InlineData(0xA0, 96000)]
    [InlineData(0xB0, 112000)]
    [InlineData(0xC0, 128000)]
    [InlineData(0xD0, 144000)]
    [InlineData(0xE0, 160000)]
    public void Mpeg2Layer3_AllValidBitrateIndices_Parse(byte byte2, int expectedBitrate)
    {
        // M2L3: b[1] = 0xF3.
        byte[] header = { 0xFF, 0xF3, byte2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedBitrate, hdr.Bitrate);
    }

    [Theory]
    [InlineData(0x00, 11025)]
    [InlineData(0x04, 12000)]
    [InlineData(0x08, 8000)]
    public void Mpeg25SampleRateIndices_Parse(byte byte2Low, int expectedSampleRate)
    {
        // M2.5 L3: b[1] = 0xE3, br=8 (64kbps M2L3).
        byte b2 = (byte)(0x80 | byte2Low);
        byte[] header = { 0xFF, 0xE3, b2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedSampleRate, hdr.SampleRate);
    }

    [Theory]
    [InlineData(0xFF, 0x90, 384)]   // M1 L1
    [InlineData(0xFD, 0xA0, 1152)]  // M1 L2
    [InlineData(0xFB, 0x90, 1152)]  // M1 L3
    [InlineData(0xF7, 0x14, 384)]   // M2 L1
    [InlineData(0xF5, 0x80, 1152)]  // M2 L2
    [InlineData(0xF3, 0x80, 576)]   // M2 L3
    [InlineData(0xE3, 0x80, 576)]   // M2.5 L3
    public void SamplesPerFrame_Matches_VersionLayer_Combination(byte byte1, byte byte2, int expectedSamples)
    {
        byte[] header = { 0xFF, byte1, byte2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(expectedSamples, hdr.SamplesPerFrame);
    }

    [Fact]
    public void Layer2_Padding_AddsOneByte()
    {
        // M1L2 192kbps/44100 with padding=1: 144*192000/44100 + 1 = 626 + 1.
        byte[] header = { 0xFF, 0xFD, 0xA2, 0x00 };
        Assert.True(Mp3FrameHeader.TryParse(header, out var hdr));
        Assert.Equal(1, hdr.Padding);
        Assert.Equal(627, hdr.FrameSize);
    }
}
