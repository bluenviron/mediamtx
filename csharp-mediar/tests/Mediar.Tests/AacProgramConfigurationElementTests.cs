using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacProgramConfigurationElementTests
{
    [Fact]
    public void ToBytes_Then_TryParse_MinimalStereoFront_RoundTrips()
    {
        // One SCE (centre-ish) + one CPE (L/R) covers 3 rendered channels, nothing
        // else. No mixdowns, empty comment - the smallest plausible PCE.
        var pce = new AacProgramConfigurationElement
        {
            ElementInstanceTag = 0,
            ObjectType = 1, // AAC-LC (profile = 1 → AOT = 2)
            SamplingFrequencyIndex = 4, // 44100 Hz
            FrontElements =
            [
                new AacPceChannelSlot { IsCpe = false, TagSelect = 0 },
                new AacPceChannelSlot { IsCpe = true, TagSelect = 1 },
            ],
            SideElements = [],
            BackElements = [],
            LfeElements = [],
            AssocDataElements = [],
            CouplingElements = [],
            CommentField = string.Empty,
        };

        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded, out int consumed));
        Assert.NotNull(decoded);
        Assert.Equal(bytes.Length, consumed);

        Assert.Equal(pce.ElementInstanceTag, decoded!.ElementInstanceTag);
        Assert.Equal(pce.ObjectType, decoded.ObjectType);
        Assert.Equal(AacAudioObjectType.AacLc, decoded.ObjectTypeEnum);
        Assert.Equal(pce.SamplingFrequencyIndex, decoded.SamplingFrequencyIndex);
        Assert.Equal(44_100, decoded.SamplingFrequency);
        Assert.Equal(3, decoded.SpeakerCount);
        Assert.Equal(2, decoded.FrontElements.Count);
        Assert.False(decoded.FrontElements[0].IsCpe);
        Assert.True(decoded.FrontElements[1].IsCpe);
        Assert.Equal(1, decoded.FrontElements[1].TagSelect);
        Assert.Empty(decoded.BackElements);
        Assert.Empty(decoded.LfeElements);
        Assert.Empty(decoded.CouplingElements);
        Assert.Null(decoded.MonoMixdownElementNumber);
        Assert.Null(decoded.StereoMixdownElementNumber);
        Assert.Null(decoded.MatrixMixdown);
        Assert.Equal(string.Empty, decoded.CommentField);
    }

    [Fact]
    public void ToBytes_Then_TryParse_5_1_Layout_RoundTrips()
    {
        // 5.1 layout: front centre (SCE) + front L/R (CPE) + back Ls/Rs (CPE) + LFE.
        var pce = new AacProgramConfigurationElement
        {
            ElementInstanceTag = 0,
            ObjectType = 1,
            SamplingFrequencyIndex = 3, // 48 kHz
            FrontElements =
            [
                new AacPceChannelSlot { IsCpe = false, TagSelect = 0 },
                new AacPceChannelSlot { IsCpe = true, TagSelect = 0 },
            ],
            SideElements = [],
            BackElements =
            [
                new AacPceChannelSlot { IsCpe = true, TagSelect = 1 },
            ],
            LfeElements = [0],
            AssocDataElements = [],
            CouplingElements = [],
            CommentField = "5.1",
        };

        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.NotNull(decoded);
        Assert.Equal(6, decoded!.SpeakerCount);
        Assert.Equal(48_000, decoded.SamplingFrequency);
        Assert.Equal(2, decoded.FrontElements.Count);
        Assert.Single(decoded.BackElements);
        Assert.True(decoded.BackElements[0].IsCpe);
        Assert.Single(decoded.LfeElements);
        Assert.Equal(0, decoded.LfeElements[0]);
        Assert.Equal("5.1", decoded.CommentField);
    }

    [Fact]
    public void ToBytes_Then_TryParse_7_1_Layout_RoundTrips()
    {
        // 7.1: centre + front L/R CPE + side Ls/Rs CPE + back Lb/Rb CPE + LFE = 8 ch.
        var pce = new AacProgramConfigurationElement
        {
            ElementInstanceTag = 1,
            ObjectType = 1,
            SamplingFrequencyIndex = 3,
            FrontElements =
            [
                new AacPceChannelSlot { IsCpe = false, TagSelect = 0 },
                new AacPceChannelSlot { IsCpe = true, TagSelect = 0 },
            ],
            SideElements =
            [
                new AacPceChannelSlot { IsCpe = true, TagSelect = 1 },
            ],
            BackElements =
            [
                new AacPceChannelSlot { IsCpe = true, TagSelect = 2 },
            ],
            LfeElements = [0],
            AssocDataElements = [],
            CouplingElements = [],
            CommentField = "Mediar 7.1",
        };

        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.NotNull(decoded);
        Assert.Equal(8, decoded!.SpeakerCount);
        Assert.Equal(1, decoded.ElementInstanceTag);
        Assert.Equal("Mediar 7.1", decoded.CommentField);
    }

    [Fact]
    public void ToBytes_Then_TryParse_MonoMixdown_RoundTrips()
    {
        var pce = MinimalStereo() with { MonoMixdownElementNumber = 7 };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Equal(7, decoded!.MonoMixdownElementNumber);
        Assert.Null(decoded.StereoMixdownElementNumber);
        Assert.Null(decoded.MatrixMixdown);
    }

    [Fact]
    public void ToBytes_Then_TryParse_StereoMixdown_RoundTrips()
    {
        var pce = MinimalStereo() with { StereoMixdownElementNumber = 12 };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Null(decoded!.MonoMixdownElementNumber);
        Assert.Equal(12, decoded.StereoMixdownElementNumber);
    }

    [Fact]
    public void ToBytes_Then_TryParse_MatrixMixdown_RoundTrips()
    {
        var pce = MinimalStereo() with
        {
            MatrixMixdown = new AacPceMatrixMixdown { Index = 2, PseudoSurroundEnable = true },
        };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.NotNull(decoded!.MatrixMixdown);
        Assert.Equal(2, decoded.MatrixMixdown!.Value.Index);
        Assert.True(decoded.MatrixMixdown.Value.PseudoSurroundEnable);
    }

    [Fact]
    public void ToBytes_Then_TryParse_CouplingChannels_RoundTrip()
    {
        var pce = MinimalStereo() with
        {
            CouplingElements =
            [
                new AacPceCouplingSlot { IsIndependentlySwitched = true, TagSelect = 3 },
                new AacPceCouplingSlot { IsIndependentlySwitched = false, TagSelect = 9 },
            ],
            AssocDataElements = [4],
        };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Equal(2, decoded!.CouplingElements.Count);
        Assert.True(decoded.CouplingElements[0].IsIndependentlySwitched);
        Assert.Equal(3, decoded.CouplingElements[0].TagSelect);
        Assert.False(decoded.CouplingElements[1].IsIndependentlySwitched);
        Assert.Equal(9, decoded.CouplingElements[1].TagSelect);
        Assert.Single(decoded.AssocDataElements);
        Assert.Equal(4, decoded.AssocDataElements[0]);
    }

    [Fact]
    public void ToBytes_Then_TryParse_LongComment_RoundTrips()
    {
        // 200-byte comment hits the byte-aligned comment path without saturating the 255 cap.
        string comment = new string('A', 200);
        var pce = MinimalStereo() with { CommentField = comment };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Equal(comment, decoded!.CommentField);
    }

    [Fact]
    public void ToBytes_Then_TryParse_MaxCommentLength_RoundTrips()
    {
        string comment = new string('Z', 255);
        var pce = MinimalStereo() with { CommentField = comment };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Equal(255, decoded!.CommentField.Length);
    }

    [Fact]
    public void ToBytes_Latin1_NonAscii_Comment_RoundTrips()
    {
        // ISO 8859-1 supports e.g. é (0xE9). Round-trip should preserve bytes.
        string comment = "café";
        var pce = MinimalStereo() with { CommentField = comment };
        byte[] bytes = pce.ToBytes();
        Assert.True(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Equal(comment, decoded!.CommentField);
    }

    [Fact]
    public void ToBytes_CommentField_Over_255_Bytes_Throws()
    {
        var pce = MinimalStereo() with { CommentField = new string('!', 256) };
        Assert.Throws<InvalidOperationException>(() => pce.ToBytes());
    }

    [Fact]
    public void TryParse_Truncated_Returns_False()
    {
        var pce = MinimalStereo();
        byte[] bytes = pce.ToBytes();
        // Lop off the comment payload (and beyond).
        Assert.False(AacProgramConfigurationElement.TryParse(bytes.AsSpan(0, bytes.Length / 2), out var decoded));
        Assert.Null(decoded);
    }

    [Fact]
    public void TryParse_Empty_Returns_False()
    {
        Assert.False(AacProgramConfigurationElement.TryParse(ReadOnlySpan<byte>.Empty, out var decoded));
        Assert.Null(decoded);
    }

    [Fact]
    public void TryParse_EscapeSampleFrequencyIndex_Is_Rejected()
    {
        // Build a synthetic PCE blob with sfIndex = 15 (escape) which is invalid per spec.
        var writer = new AacBitWriter();
        writer.Write(0u, 4);  // tag
        writer.Write(1u, 2);  // objectType = LC
        writer.Write(15u, 4); // sfIndex = escape (REJECT)
        writer.Write(0u, 4);  // num_front
        writer.Write(0u, 4);  // num_side
        writer.Write(0u, 4);  // num_back
        writer.Write(0u, 2);  // num_lfe
        writer.Write(0u, 3);  // num_assoc
        writer.Write(0u, 4);  // num_cc
        writer.Write(0u, 3);  // no mixdowns
        writer.AlignToByte();
        writer.Write(0u, 8);  // comment_field_bytes = 0
        byte[] bytes = writer.ToArray();

        Assert.False(AacProgramConfigurationElement.TryParse(bytes, out var decoded));
        Assert.Null(decoded);
    }

    [Fact]
    public void TryParse_Reports_BytesConsumed_Past_Trailing_Padding()
    {
        var pce = MinimalStereo();
        byte[] inner = pce.ToBytes();

        // Append 3 bytes of trailing junk and confirm the parser stops at the comment.
        byte[] padded = new byte[inner.Length + 3];
        inner.CopyTo(padded, 0);
        padded[inner.Length] = 0xDE;
        padded[inner.Length + 1] = 0xAD;
        padded[inner.Length + 2] = 0xBE;

        Assert.True(AacProgramConfigurationElement.TryParse(padded, out var decoded, out int consumed));
        Assert.NotNull(decoded);
        Assert.Equal(inner.Length, consumed);
        Assert.Equal(pce.CommentField, decoded!.CommentField);
    }

    [Fact]
    public void SpeakerCount_Tracks_Channel_Element_Mix()
    {
        var pce = new AacProgramConfigurationElement
        {
            ElementInstanceTag = 0,
            ObjectType = 1,
            SamplingFrequencyIndex = 3,
            FrontElements =
            [
                new AacPceChannelSlot { IsCpe = false, TagSelect = 0 }, // 1
                new AacPceChannelSlot { IsCpe = true, TagSelect = 0 },  // 2
            ],
            SideElements =
            [
                new AacPceChannelSlot { IsCpe = false, TagSelect = 0 }, // 1
            ],
            BackElements =
            [
                new AacPceChannelSlot { IsCpe = true, TagSelect = 1 },  // 2
            ],
            LfeElements = [0, 1], // 2 LFEs
            AssocDataElements = [],
            CouplingElements = [],
            CommentField = string.Empty,
        };
        Assert.Equal(8, pce.SpeakerCount);
    }

    private static AacProgramConfigurationElement MinimalStereo() => new()
    {
        ElementInstanceTag = 0,
        ObjectType = 1,
        SamplingFrequencyIndex = 4,
        FrontElements =
        [
            new AacPceChannelSlot { IsCpe = true, TagSelect = 0 },
        ],
        SideElements = [],
        BackElements = [],
        LfeElements = [],
        AssocDataElements = [],
        CouplingElements = [],
        CommentField = string.Empty,
    };
}
