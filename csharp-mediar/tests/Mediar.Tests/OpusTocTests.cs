using Mediar.Codecs.Opus.Decoder;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Coverage for the 1-byte Opus TOC header (RFC 6716 §3.1). Every config
/// value (0..31), the stereo bit, and the frame-count code field are
/// exercised; the test matrix mirrors Table 2 in the spec so any drift in
/// the lookup table is caught immediately.
/// </summary>
public sealed class OpusTocTests
{
    [Theory]
    [InlineData(0,  OpusMode.SilkOnly, OpusBandwidth.Narrowband,    10_000)]
    [InlineData(1,  OpusMode.SilkOnly, OpusBandwidth.Narrowband,    20_000)]
    [InlineData(2,  OpusMode.SilkOnly, OpusBandwidth.Narrowband,    40_000)]
    [InlineData(3,  OpusMode.SilkOnly, OpusBandwidth.Narrowband,    60_000)]
    [InlineData(4,  OpusMode.SilkOnly, OpusBandwidth.Mediumband,    10_000)]
    [InlineData(7,  OpusMode.SilkOnly, OpusBandwidth.Mediumband,    60_000)]
    [InlineData(8,  OpusMode.SilkOnly, OpusBandwidth.Wideband,      10_000)]
    [InlineData(11, OpusMode.SilkOnly, OpusBandwidth.Wideband,      60_000)]
    [InlineData(12, OpusMode.Hybrid,   OpusBandwidth.SuperWideband, 10_000)]
    [InlineData(13, OpusMode.Hybrid,   OpusBandwidth.SuperWideband, 20_000)]
    [InlineData(14, OpusMode.Hybrid,   OpusBandwidth.Fullband,      10_000)]
    [InlineData(15, OpusMode.Hybrid,   OpusBandwidth.Fullband,      20_000)]
    [InlineData(16, OpusMode.CeltOnly, OpusBandwidth.Narrowband,     2_500)]
    [InlineData(17, OpusMode.CeltOnly, OpusBandwidth.Narrowband,     5_000)]
    [InlineData(19, OpusMode.CeltOnly, OpusBandwidth.Narrowband,    20_000)]
    [InlineData(20, OpusMode.CeltOnly, OpusBandwidth.Wideband,       2_500)]
    [InlineData(23, OpusMode.CeltOnly, OpusBandwidth.Wideband,      20_000)]
    [InlineData(24, OpusMode.CeltOnly, OpusBandwidth.SuperWideband,  2_500)]
    [InlineData(27, OpusMode.CeltOnly, OpusBandwidth.SuperWideband, 20_000)]
    [InlineData(28, OpusMode.CeltOnly, OpusBandwidth.Fullband,       2_500)]
    [InlineData(31, OpusMode.CeltOnly, OpusBandwidth.Fullband,      20_000)]
    public void ConfigTable_Matches_Rfc6716_Table2(int config, OpusMode mode, OpusBandwidth bw, int frameUs)
    {
        byte toc = (byte)(config << 3); // stereo=0, code=0
        var parsed = OpusToc.Parse(toc);
        Assert.Equal(config, parsed.Config);
        Assert.Equal(mode, parsed.Mode);
        Assert.Equal(bw, parsed.Bandwidth);
        Assert.Equal(frameUs, parsed.FrameSizeMicroseconds);
        Assert.False(parsed.IsStereo);
        Assert.Equal(0, parsed.FrameCountCode);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void Stereo_Bit_Is_Decoded(bool stereo)
    {
        byte toc = (byte)((1 << 3) | (stereo ? 0x4 : 0));
        var parsed = OpusToc.Parse(toc);
        Assert.Equal(stereo, parsed.IsStereo);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    public void FrameCount_Code_Is_Decoded(int code)
    {
        byte toc = (byte)((5 << 3) | (byte)code);
        var parsed = OpusToc.Parse(toc);
        Assert.Equal(code, parsed.FrameCountCode);
    }

    [Fact]
    public void Every_Config_Value_Round_Trips_Through_ToByte()
    {
        for (int config = 0; config <= 31; config++)
        {
            for (int s = 0; s <= 1; s++)
            {
                for (int c = 0; c <= 3; c++)
                {
                    byte expected = (byte)((config << 3) | (s << 2) | c);
                    var parsed = OpusToc.Parse(expected);
                    byte rebuilt = parsed.ToByte();
                    Assert.Equal(expected, rebuilt);
                }
            }
        }
    }

    [Theory]
    [InlineData(2_500,  120)]
    [InlineData(5_000,  240)]
    [InlineData(10_000, 480)]
    [InlineData(20_000, 960)]
    [InlineData(40_000, 1920)]
    [InlineData(60_000, 2880)]
    public void SamplesPerFrameAt48k_Matches_Spec(int frameUs, int expected)
    {
        // Pick the first config with this frame size.
        for (int config = 0; config <= 31; config++)
        {
            var parsed = OpusToc.Parse((byte)(config << 3));
            if (parsed.FrameSizeMicroseconds == frameUs)
            {
                Assert.Equal(expected, parsed.SamplesPerFrameAt48k);
                return;
            }
        }
        Assert.Fail($"No config has frame size {frameUs} us — test setup error.");
    }

    [Fact]
    public void TryParse_Always_Succeeds_For_Any_Byte()
    {
        // RFC 6716 doesn't reserve any of the 32 configs, so every byte is valid.
        for (int b = 0; b <= 255; b++)
        {
            Assert.True(OpusToc.TryParse((byte)b, out var t));
            Assert.Equal(b, t.ToByte());
        }
    }

    [Fact]
    public void ToByte_Rejects_OutOfRange_Config()
    {
        var bad = new OpusToc { Config = 32 };
        Assert.Throws<InvalidOperationException>(() => bad.ToByte());
    }

    [Fact]
    public void ToByte_Rejects_OutOfRange_FrameCountCode()
    {
        var bad = new OpusToc { Config = 0, FrameCountCode = 4 };
        Assert.Throws<InvalidOperationException>(() => bad.ToByte());
    }
}
