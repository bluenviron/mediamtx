using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelMappingTests
{
    [Theory]
    [InlineData(-1)]
    [InlineData(8)]
    [InlineData(15)]
    public void Get_OutOfRange_Throws(int config)
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacChannelMapping.GetForConfiguration(config));
    }

    [Fact]
    public void Get_ZeroConfig_ReturnsEmpty()
    {
        var entries = AacChannelMapping.GetForConfiguration(0);
        Assert.Empty(entries);
    }

    [Fact]
    public void Config1_Mono_SceFrontCentre()
    {
        var entries = AacChannelMapping.GetForConfiguration(1);
        Assert.Single(entries);
        Assert.Equal(AacSyntacticElementType.SingleChannelElement, entries[0].Element);
        Assert.Equal(AacSpeaker.FrontCentre, entries[0].FirstSpeaker);
        Assert.Equal(AacSpeaker.None, entries[0].SecondSpeaker);
    }

    [Fact]
    public void Config2_Stereo_CpeLeftRight()
    {
        var entries = AacChannelMapping.GetForConfiguration(2);
        Assert.Single(entries);
        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[0].Element);
        Assert.Equal(AacSpeaker.FrontLeft, entries[0].FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontRight, entries[0].SecondSpeaker);
    }

    [Fact]
    public void Config3_Three_SceCenterThenCpeLR()
    {
        var entries = AacChannelMapping.GetForConfiguration(3);
        Assert.Equal(2, entries.Count);

        Assert.Equal(AacSyntacticElementType.SingleChannelElement, entries[0].Element);
        Assert.Equal(AacSpeaker.FrontCentre, entries[0].FirstSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[1].Element);
        Assert.Equal(AacSpeaker.FrontLeft, entries[1].FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontRight, entries[1].SecondSpeaker);
    }

    [Fact]
    public void Config4_FourPointZero_SceCenterCpeLRSceCs()
    {
        var entries = AacChannelMapping.GetForConfiguration(4);
        Assert.Equal(3, entries.Count);

        Assert.Equal(AacSyntacticElementType.SingleChannelElement, entries[0].Element);
        Assert.Equal(AacSpeaker.FrontCentre, entries[0].FirstSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[1].Element);
        Assert.Equal(AacSpeaker.FrontLeft, entries[1].FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontRight, entries[1].SecondSpeaker);

        Assert.Equal(AacSyntacticElementType.SingleChannelElement, entries[2].Element);
        Assert.Equal(AacSpeaker.BackCentre, entries[2].FirstSpeaker);
    }

    [Fact]
    public void Config5_FivePointZero_SceCenterCpeLRCpeLsRs()
    {
        var entries = AacChannelMapping.GetForConfiguration(5);
        Assert.Equal(3, entries.Count);

        Assert.Equal(AacSyntacticElementType.SingleChannelElement, entries[0].Element);
        Assert.Equal(AacSpeaker.FrontCentre, entries[0].FirstSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[1].Element);
        Assert.Equal(AacSpeaker.FrontLeft, entries[1].FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontRight, entries[1].SecondSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[2].Element);
        Assert.Equal(AacSpeaker.SurroundLeft, entries[2].FirstSpeaker);
        Assert.Equal(AacSpeaker.SurroundRight, entries[2].SecondSpeaker);
    }

    [Fact]
    public void Config6_FivePointOne_AddsLfeTail()
    {
        var entries = AacChannelMapping.GetForConfiguration(6);
        Assert.Equal(4, entries.Count);

        // First 3 entries identical to config 5.
        var five = AacChannelMapping.GetForConfiguration(5);
        for (int i = 0; i < 3; i++) Assert.Equal(five[i], entries[i]);

        Assert.Equal(AacSyntacticElementType.LfeChannelElement, entries[3].Element);
        Assert.Equal(AacSpeaker.Lfe, entries[3].FirstSpeaker);
        Assert.Equal(AacSpeaker.None, entries[3].SecondSpeaker);
    }

    [Fact]
    public void Config7_SevenPointOne_AddsBackPairBeforeLfe()
    {
        var entries = AacChannelMapping.GetForConfiguration(7);
        Assert.Equal(5, entries.Count);

        Assert.Equal(AacSyntacticElementType.SingleChannelElement, entries[0].Element);
        Assert.Equal(AacSpeaker.FrontCentre, entries[0].FirstSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[1].Element);
        Assert.Equal(AacSpeaker.FrontLeft, entries[1].FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontRight, entries[1].SecondSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[2].Element);
        Assert.Equal(AacSpeaker.SurroundLeft, entries[2].FirstSpeaker);
        Assert.Equal(AacSpeaker.SurroundRight, entries[2].SecondSpeaker);

        Assert.Equal(AacSyntacticElementType.ChannelPairElement, entries[3].Element);
        Assert.Equal(AacSpeaker.BackLeft, entries[3].FirstSpeaker);
        Assert.Equal(AacSpeaker.BackRight, entries[3].SecondSpeaker);

        Assert.Equal(AacSyntacticElementType.LfeChannelElement, entries[4].Element);
        Assert.Equal(AacSpeaker.Lfe, entries[4].FirstSpeaker);
    }

    [Theory]
    [InlineData(1, 1)]
    [InlineData(2, 2)]
    [InlineData(3, 3)]
    [InlineData(4, 4)]
    [InlineData(5, 5)]
    [InlineData(6, 6)]
    [InlineData(7, 8)]
    public void SpeakerCount_MatchesElementChannelTotal(int config, int expectedSpeakers)
    {
        int actual = AacChannelMapping.SpeakerCount(config);
        Assert.Equal(expectedSpeakers, actual);

        // The total decoded channels (CPE = 2, others = 1) must match
        // the audible speaker count.
        var entries = AacChannelMapping.GetForConfiguration(config);
        int channels = 0;
        foreach (var entry in entries)
        {
            channels += entry.Element == AacSyntacticElementType.ChannelPairElement ? 2 : 1;
        }
        Assert.Equal(expectedSpeakers, channels);
    }

    [Fact]
    public void AllConfigs_OnlySceCpeLfeUsed()
    {
        // Only those three element kinds appear in the mapping table.
        for (int c = 1; c <= 7; c++)
        {
            foreach (var entry in AacChannelMapping.GetForConfiguration(c))
            {
                Assert.True(
                    entry.Element is AacSyntacticElementType.SingleChannelElement
                                  or AacSyntacticElementType.ChannelPairElement
                                  or AacSyntacticElementType.LfeChannelElement,
                    $"Config {c}: unexpected element {entry.Element}");
            }
        }
    }

    [Fact]
    public void AllConfigs_SceLfeHaveNoSecondSpeaker()
    {
        for (int c = 1; c <= 7; c++)
        {
            foreach (var entry in AacChannelMapping.GetForConfiguration(c))
            {
                if (entry.Element != AacSyntacticElementType.ChannelPairElement)
                {
                    Assert.Equal(AacSpeaker.None, entry.SecondSpeaker);
                }
                else
                {
                    Assert.NotEqual(AacSpeaker.None, entry.SecondSpeaker);
                }
            }
        }
    }

    [Theory]
    [InlineData(6)]
    [InlineData(7)]
    public void LfeElement_Is_Always_Last_When_Present(int config)
    {
        var entries = AacChannelMapping.GetForConfiguration(config);
        Assert.Equal(AacSyntacticElementType.LfeChannelElement, entries[^1].Element);
        for (int i = 0; i < entries.Count - 1; i++)
        {
            Assert.NotEqual(AacSyntacticElementType.LfeChannelElement, entries[i].Element);
        }
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    public void NonLfe_Configs_Have_No_LfeElement(int config)
    {
        var entries = AacChannelMapping.GetForConfiguration(config);
        Assert.DoesNotContain(entries, e => e.Element == AacSyntacticElementType.LfeChannelElement);
    }

    [Fact]
    public void Config4_Has_No_Lfe_Despite_Four_Channels()
    {
        var entries = AacChannelMapping.GetForConfiguration(4);
        Assert.DoesNotContain(entries, e => e.Element == AacSyntacticElementType.LfeChannelElement);
        Assert.DoesNotContain(entries, e => e.FirstSpeaker == AacSpeaker.Lfe);
    }

    [Fact]
    public void Config7_Has_Single_Lfe_Element()
    {
        var entries = AacChannelMapping.GetForConfiguration(7);
        Assert.Single(entries, e => e.Element == AacSyntacticElementType.LfeChannelElement);
        Assert.Single(entries, e => e.FirstSpeaker == AacSpeaker.Lfe);
    }

    [Theory]
    [InlineData(2, AacSpeaker.FrontLeft, AacSpeaker.FrontRight)]
    [InlineData(5, AacSpeaker.SurroundLeft, AacSpeaker.SurroundRight)]
    [InlineData(7, AacSpeaker.BackLeft, AacSpeaker.BackRight)]
    public void Cpe_Entries_Have_Distinct_LeftRight_Speakers(int config, AacSpeaker l, AacSpeaker r)
    {
        var entries = AacChannelMapping.GetForConfiguration(config);
        Assert.Contains(entries, e =>
            e.Element == AacSyntacticElementType.ChannelPairElement
            && e.FirstSpeaker == l && e.SecondSpeaker == r);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    [InlineData(6)]
    [InlineData(7)]
    public void Get_Returns_Equal_Entries_Across_Calls(int config)
    {
        // Record equality means subsequent calls produce structurally
        // equal entries even though instances are allocated fresh.
        var a = AacChannelMapping.GetForConfiguration(config);
        var b = AacChannelMapping.GetForConfiguration(config);
        Assert.Equal(a.Count, b.Count);
        for (int i = 0; i < a.Count; i++)
        {
            Assert.Equal(a[i], b[i]);
        }
    }

    [Fact]
    public void Get_ZeroConfig_Returns_SameSingleton_Empty_Instance()
    {
        // The 0-arm returns Array.Empty<>(), which is the shared singleton.
        var a = AacChannelMapping.GetForConfiguration(0);
        var b = AacChannelMapping.GetForConfiguration(0);
        Assert.Same(a, b);
        Assert.Empty(a);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(7)]
    public void SpeakerCount_Matches_GetForConfiguration_Channel_Total(int config)
    {
        int expected = AacChannelMapping.SpeakerCount(config);
        int channels = 0;
        foreach (var e in AacChannelMapping.GetForConfiguration(config))
        {
            channels += e.Element == AacSyntacticElementType.ChannelPairElement ? 2 : 1;
        }
        Assert.Equal(expected, channels);
    }

    [Fact]
    public void Configs_3_Through_7_Begin_With_FrontCentre_SCE_Then_Front_LR_CPE()
    {
        for (int c = 3; c <= 7; c++)
        {
            var e = AacChannelMapping.GetForConfiguration(c);
            Assert.Equal(AacSyntacticElementType.SingleChannelElement, e[0].Element);
            Assert.Equal(AacSpeaker.FrontCentre, e[0].FirstSpeaker);
            Assert.Equal(AacSyntacticElementType.ChannelPairElement, e[1].Element);
            Assert.Equal(AacSpeaker.FrontLeft, e[1].FirstSpeaker);
            Assert.Equal(AacSpeaker.FrontRight, e[1].SecondSpeaker);
        }
    }

    [Fact]
    public void Each_Speaker_Used_At_Most_Once_Per_Configuration()
    {
        // Within any single config, no real speaker (LFE excluded for
        // CPE second-channel = None) should be driven twice.
        for (int c = 1; c <= 7; c++)
        {
            var seen = new HashSet<AacSpeaker>();
            foreach (var e in AacChannelMapping.GetForConfiguration(c))
            {
                Assert.True(seen.Add(e.FirstSpeaker),
                    $"Config {c}: {e.FirstSpeaker} appears twice");
                if (e.SecondSpeaker != AacSpeaker.None)
                {
                    Assert.True(seen.Add(e.SecondSpeaker),
                        $"Config {c}: {e.SecondSpeaker} appears twice");
                }
            }
        }
    }

    [Fact]
    public void Entry_With_Expression_Preserves_Record_Semantics()
    {
        var original = AacChannelMapping.GetForConfiguration(1)[0];
        var modified = original with { FirstSpeaker = AacSpeaker.BackCentre };
        Assert.Equal(AacSpeaker.BackCentre, modified.FirstSpeaker);
        Assert.Equal(AacSpeaker.FrontCentre, original.FirstSpeaker);
        Assert.NotEqual(original, modified);
    }

    [Fact]
    public void Entry_Default_SecondSpeaker_Is_None()
    {
        var e = new AacChannelMappingEntry
        {
            Element = AacSyntacticElementType.SingleChannelElement,
            FirstSpeaker = AacSpeaker.FrontCentre,
        };
        Assert.Equal(AacSpeaker.None, e.SecondSpeaker);
    }

    [Fact]
    public void Get_OutOfRange_Throws_For_Far_Negative_And_Large_Values()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacChannelMapping.GetForConfiguration(int.MinValue));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacChannelMapping.GetForConfiguration(int.MaxValue));
    }
}
