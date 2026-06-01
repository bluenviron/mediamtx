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
}
