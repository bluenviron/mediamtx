using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacCouplingGainExpansionTests
{
    private static AacSectionData SectionDataLong(params (int cb, int startSfb, int endSfb)[] sections)
    {
        var list = new List<AacSection>(sections.Length);
        foreach (var (cb, start, end) in sections)
        {
            list.Add(new AacSection
            {
                Group = 0,
                CodebookNumber = cb,
                StartSfb = start,
                EndSfb = end,
            });
        }
        return new AacSectionData { Sections = list };
    }

    [Fact]
    public void Expand_NullGainList_Throws()
    {
        var sections = SectionDataLong((1, 0, 4));
        Assert.Throws<ArgumentNullException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(null!, sections, 1, 4));
    }

    [Fact]
    public void Expand_NullSectionData_Throws()
    {
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = 0,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        Assert.Throws<ArgumentNullException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, null!, 1, 4));
    }

    [Fact]
    public void Expand_NegativeMaxSfb_Throws()
    {
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = 0,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong();
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, -1));
    }

    [Fact]
    public void Expand_ZeroWindowGroups_Throws()
    {
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = 0,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong();
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 0, 4));
    }

    [Fact]
    public void Expand_CommonGain_AllBandsReceiveSameIndex()
    {
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = 7,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong((1, 0, 4));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 4);

        for (int s = 0; s < 4; s++)
        {
            Assert.Equal(7, indices[0, s]);
        }
    }

    [Fact]
    public void Expand_CommonGain_NullDifferential_Throws()
    {
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = null,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong((1, 0, 4));
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 4));
    }

    [Fact]
    public void Expand_Dpcm_AccumulatesAcrossNonZeroBands()
    {
        // 4 bands, all cb=1: per-band diffs [3, 2, -1, 4]
        // -> cumulative absolute indices [3, 5, 4, 8]
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 3 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 2 },
            new AacCouplingGainEntry { Group = 0, Sfb = 2, Differential = -1 },
            new AacCouplingGainEntry { Group = 0, Sfb = 3, Differential = 4 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 4));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 4);

        Assert.Equal(3, indices[0, 0]);
        Assert.Equal(5, indices[0, 1]);
        Assert.Equal(4, indices[0, 2]);
        Assert.Equal(8, indices[0, 3]);
    }

    [Fact]
    public void Expand_Dpcm_ZeroHcbBand_InheritsRunningValue()
    {
        // Layout: [cb=1, cb=0, cb=1] each spanning 1 SFB. Diffs [5, 7]
        // -> absolute indices for non-zero bands: [5, 12]; ZERO_HCB
        // band at sfb=1 inherits running value (5) from sfb=0.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 5 },
            new AacCouplingGainEntry { Group = 0, Sfb = 2, Differential = 7 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 1), (0, 1, 2), (1, 2, 3));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 3);

        Assert.Equal(5, indices[0, 0]);
        Assert.Equal(5, indices[0, 1]);    // inherited
        Assert.Equal(12, indices[0, 2]);
    }

    [Fact]
    public void Expand_Dpcm_ZeroHcbBeforeAnyNonZero_InheritsZero()
    {
        // Layout: [cb=0, cb=1]. ZERO_HCB at sfb=0 inherits running=0;
        // cb=1 at sfb=1 with diff=3 -> index 3.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 3 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((0, 0, 1), (1, 1, 2));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2);

        Assert.Equal(0, indices[0, 0]);
        Assert.Equal(3, indices[0, 1]);
    }

    [Fact]
    public void Expand_Dpcm_TooFewEntries_Throws()
    {
        // 2 non-ZERO_HCB bands but only 1 DPCM entry.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 1 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 2));
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2));
    }

    [Fact]
    public void Expand_Dpcm_TooManyEntries_Throws()
    {
        // 1 non-ZERO_HCB band but 2 DPCM entries.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 1 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 2 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 1));
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 1));
    }

    [Fact]
    public void Expand_Dpcm_GroupSfbMismatch_Throws()
    {
        // DPCM entry coordinates don't match section scan order.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 5, Differential = 1 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 1));
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 1));
    }

    [Fact]
    public void Expand_MultiGroup_RespectsGroupBoundaries()
    {
        // Two groups, each with 1 cb=1 SFB. Diffs cumulate ACROSS
        // groups (spec scan order is group-major then sfb).
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 4 },
            new AacCouplingGainEntry { Group = 1, Sfb = 0, Differential = 2 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = new AacSectionData
        {
            Sections = new[]
            {
                new AacSection { Group = 0, CodebookNumber = 1, StartSfb = 0, EndSfb = 1 },
                new AacSection { Group = 1, CodebookNumber = 1, StartSfb = 0, EndSfb = 1 },
            },
        };
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 2, 1);

        Assert.Equal(4, indices[0, 0]);
        Assert.Equal(6, indices[1, 0]);
    }

    [Fact]
    public void Expand_CommonGain_MultiGroup_AllBandsSame()
    {
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = -5,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong();   // empty - common-gain doesn't consult sections
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 3, 2);

        for (int g = 0; g < 3; g++)
        {
            for (int s = 0; s < 2; s++)
            {
                Assert.Equal(-5, indices[g, s]);
            }
        }
    }

    [Fact]
    public void Expand_Dpcm_ZeroDifferentials_AllZero()
    {
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 0 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 0 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 2));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2);

        Assert.Equal(0, indices[0, 0]);
        Assert.Equal(0, indices[0, 1]);
    }

    [Fact]
    public void Expand_Dpcm_SectionGroupOverrunsGrid_Throws()
    {
        // Section claims group=5 but windowGroupCount=2 -> overrun.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 5, Sfb = 0, Differential = 1 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = new AacSectionData
        {
            Sections = new[]
            {
                new AacSection { Group = 5, CodebookNumber = 1, StartSfb = 0, EndSfb = 1 },
            },
        };
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 2, 1));
    }
}
