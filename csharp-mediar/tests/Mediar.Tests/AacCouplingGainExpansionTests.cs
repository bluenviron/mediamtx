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

    [Theory]
    [InlineData(0)]
    [InlineData(-1)]
    [InlineData(-100)]
    public void Expand_NonPositiveWindowGroupCount_Throws(int windowGroupCount)
    {
        var gl = CommonGainList(0);
        var sections = SectionDataLong();
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, windowGroupCount, 4));
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(-5)]
    public void Expand_NegativeMaxSfb_BoundaryValues_Throw(int maxSfb)
    {
        var gl = CommonGainList(0);
        var sections = SectionDataLong();
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, maxSfb));
    }

    [Fact]
    public void Expand_MaxSfb_Zero_ReturnsZeroWidthMatrix()
    {
        // maxSfb=0 is the degenerate "no scale-factor bands" case. The
        // helper must accept it and return a 1x0 matrix without reading
        // any sections or DPCM entries.
        var gl = CommonGainList(99);
        var sections = SectionDataLong();
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 0);

        Assert.Equal(1, indices.GetLength(0));
        Assert.Equal(0, indices.GetLength(1));
    }

    [Fact]
    public void Expand_ReturnsMatrix_With_Expected_Shape()
    {
        var gl = CommonGainList(0);
        var sections = SectionDataLong();
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 4, 7);

        Assert.Equal(4, indices.GetLength(0));
        Assert.Equal(7, indices.GetLength(1));
    }

    [Fact]
    public void Expand_CommonGain_IgnoresNonEmptyDpcmList()
    {
        // CommonGainElementPresent=true short-circuits before DPCM is
        // consulted. Non-empty DpcmGains must not cause a mismatch or
        // affect the output.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 99, Sfb = 99, Differential = -1234 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = 11,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 2));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2);

        Assert.Equal(11, indices[0, 0]);
        Assert.Equal(11, indices[0, 1]);
    }

    [Theory]
    [InlineData(-60)]
    [InlineData(-1)]
    [InlineData(0)]
    [InlineData(60)]
    public void Expand_CommonGain_BoundaryDifferentialValues(int common)
    {
        var gl = CommonGainList(common);
        var sections = SectionDataLong();
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 3);

        for (int s = 0; s < 3; s++)
        {
            Assert.Equal(common, indices[0, s]);
        }
    }

    [Fact]
    public void Expand_Dpcm_AllZeroHcb_NoDpcmEntries_Required()
    {
        // Three bands, all ZERO_HCB -> no DPCM entries are required and
        // every band inherits the initial running value of 0.
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong((0, 0, 3));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 3);

        Assert.Equal(0, indices[0, 0]);
        Assert.Equal(0, indices[0, 1]);
        Assert.Equal(0, indices[0, 2]);
    }

    [Fact]
    public void Expand_Dpcm_NegativeDifferentials_Cumulate()
    {
        // Diffs [-2, -3, -1] -> running [-2, -5, -6]; verify negative
        // index accumulation passes through unchanged.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = -2 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = -3 },
            new AacCouplingGainEntry { Group = 0, Sfb = 2, Differential = -1 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 3));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 3);

        Assert.Equal(-2, indices[0, 0]);
        Assert.Equal(-5, indices[0, 1]);
        Assert.Equal(-6, indices[0, 2]);
    }

    [Fact]
    public void Expand_Dpcm_ZeroHcb_AfterNonZero_KeepsLatestRunning()
    {
        // Layout: [cb=1, cb=1, cb=0, cb=0]. Diffs [3, 4] -> running 3, 7.
        // ZERO_HCB bands at sfb=2, 3 inherit 7 (latest running).
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 3 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 4 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 2), (0, 2, 4));
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 4);

        Assert.Equal(3, indices[0, 0]);
        Assert.Equal(7, indices[0, 1]);
        Assert.Equal(7, indices[0, 2]);
        Assert.Equal(7, indices[0, 3]);
    }

    [Fact]
    public void Expand_Dpcm_GroupMismatch_Throws()
    {
        // DPCM coordinates have right SFB but wrong group.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 1, Sfb = 0, Differential = 1 },
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
            },
        };
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 2, 1));
    }

    [Fact]
    public void Expand_Dpcm_SectionStartSfb_Overruns_MaxSfb_Throws()
    {
        // section.StartSfb < maxSfb, but section.EndSfb is past maxSfb.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 1 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 1 },
            new AacCouplingGainEntry { Group = 0, Sfb = 2, Differential = 1 },
        };
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = dpcm,
        };
        var sections = SectionDataLong((1, 0, 3));
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2));
    }

    [Fact]
    public void Expand_Dpcm_FullGrid_2Groups_4Sfb()
    {
        // 2 groups x 4 cb=1 bands each. Diffs [1,2,3,4,5,6,7,8] -> running
        // is [1,3,6,10,15,21,28,36] in scan order (g-major, then sfb).
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 0, Differential = 1 },
            new AacCouplingGainEntry { Group = 0, Sfb = 1, Differential = 2 },
            new AacCouplingGainEntry { Group = 0, Sfb = 2, Differential = 3 },
            new AacCouplingGainEntry { Group = 0, Sfb = 3, Differential = 4 },
            new AacCouplingGainEntry { Group = 1, Sfb = 0, Differential = 5 },
            new AacCouplingGainEntry { Group = 1, Sfb = 1, Differential = 6 },
            new AacCouplingGainEntry { Group = 1, Sfb = 2, Differential = 7 },
            new AacCouplingGainEntry { Group = 1, Sfb = 3, Differential = 8 },
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
                new AacSection { Group = 0, CodebookNumber = 1, StartSfb = 0, EndSfb = 4 },
                new AacSection { Group = 1, CodebookNumber = 1, StartSfb = 0, EndSfb = 4 },
            },
        };
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 2, 4);

        Assert.Equal(1, indices[0, 0]);
        Assert.Equal(3, indices[0, 1]);
        Assert.Equal(6, indices[0, 2]);
        Assert.Equal(10, indices[0, 3]);
        Assert.Equal(15, indices[1, 0]);
        Assert.Equal(21, indices[1, 1]);
        Assert.Equal(28, indices[1, 2]);
        Assert.Equal(36, indices[1, 3]);
    }

    [Fact]
    public void Expand_Dpcm_Section_With_Range_StartSfb_GreaterThan_MaxSfb_Throws()
    {
        // section starts at sfb=10 which is already beyond maxSfb=5.
        var dpcm = new[]
        {
            new AacCouplingGainEntry { Group = 0, Sfb = 10, Differential = 1 },
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
                new AacSection { Group = 0, CodebookNumber = 1, StartSfb = 10, EndSfb = 11 },
            },
        };
        Assert.Throws<InvalidOperationException>(() =>
            AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 5));
    }

    [Fact]
    public void Expand_NoSections_NoDpcm_NonZeroMaxSfb_LeavesAllZeros()
    {
        // No sections at all means the loop is a no-op and the freshly
        // allocated matrix surfaces as all zeros.
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = false,
            CommonGainDifferential = null,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong();
        int[,] indices = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 2, 3);

        for (int g = 0; g < 2; g++)
        {
            for (int s = 0; s < 3; s++)
            {
                Assert.Equal(0, indices[g, s]);
            }
        }
    }

    [Fact]
    public void Expand_Dpcm_Returns_FreshArray_PerCall()
    {
        // Sanity check: distinct invocations return distinct arrays so
        // callers can safely mutate the result without aliasing.
        var gl = new AacCouplingGainList
        {
            CommonGainElementPresent = true,
            CommonGainDifferential = 0,
            DpcmGains = Array.Empty<AacCouplingGainEntry>(),
        };
        var sections = SectionDataLong();
        int[,] a = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2);
        int[,] b = AacCouplingGainExpansion.ExpandToIndices(gl, sections, 1, 2);

        Assert.NotSame(a, b);
        a[0, 0] = 999;
        Assert.Equal(0, b[0, 0]);
    }

    private static AacCouplingGainList CommonGainList(int differential) => new()
    {
        CommonGainElementPresent = true,
        CommonGainDifferential = differential,
        DpcmGains = Array.Empty<AacCouplingGainEntry>(),
    };
}
