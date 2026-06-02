using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSpectralDataStandardCodebooksTests
{
    private const int Sr48k = 48_000;

    private static AacIcsInfo LongIcsInfo(int maxSfb) => new()
    {
        WindowSequence = AacWindowSequence.OnlyLong,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = null,
        WindowGroupCount = 1,
        WindowsPerGroup = new byte[] { 1 },
        PredictorDataPresent = false,
    };

    private static AacSectionData SectionList(params (int Group, int Cb, int Start, int End)[] sections)
    {
        var list = new List<AacSection>(sections.Length);
        foreach (var s in sections)
        {
            list.Add(new AacSection
            {
                Group = s.Group,
                CodebookNumber = s.Cb,
                StartSfb = s.Start,
                EndSfb = s.End,
            });
        }
        return new AacSectionData { Sections = list };
    }

    [Fact]
    public void StandardCodebookList_HasExactly16Slots()
    {
        Assert.Equal(16, AacStandardSpectralCodebooks.StandardCodebookList.Count);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(12)]
    [InlineData(13)]
    [InlineData(14)]
    [InlineData(15)]
    public void StandardCodebookList_NullAtSentinelSlots(int slot)
    {
        Assert.Null(AacStandardSpectralCodebooks.StandardCodebookList[slot]);
    }

    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    [InlineData(5)]
    [InlineData(6)]
    [InlineData(7)]
    [InlineData(8)]
    [InlineData(9)]
    [InlineData(10)]
    [InlineData(11)]
    public void StandardCodebookList_HoldsStandardCodebookForSlots1Through11(int slot)
    {
        var book = AacStandardSpectralCodebooks.StandardCodebookList[slot];
        Assert.NotNull(book);
        Assert.Same(AacStandardSpectralCodebooks.GetCodebook(slot), book);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(16)]
    [InlineData(100)]
    public void StandardCodebookList_OutOfRangeIndex_Throws(int slot)
    {
        Assert.Throws<ArgumentOutOfRangeException>(
            () => _ = AacStandardSpectralCodebooks.StandardCodebookList[slot]);
    }

    [Fact]
    public void StandardCodebookList_EnumeratesAll16Slots_WithCorrectNullPattern()
    {
        var list = AacStandardSpectralCodebooks.StandardCodebookList.ToList();
        Assert.Equal(16, list.Count);
        Assert.Null(list[0]);
        for (int i = 1; i <= 11; i++) Assert.NotNull(list[i]);
        Assert.Null(list[12]);
        Assert.Null(list[13]);
        Assert.Null(list[14]);
        Assert.Null(list[15]);
    }

    [Fact]
    public void StandardCodebookList_IsSharedInstance()
    {
        Assert.Same(
            AacStandardSpectralCodebooks.StandardCodebookList,
            AacStandardSpectralCodebooks.StandardCodebookList);
    }

    [Fact]
    public void Overload_EmptySectionList_ReturnsAllZeros()
    {
        var ics = LongIcsInfo(maxSfb: 0);
        var sections = SectionList();

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, out var data));
        Assert.NotNull(data);
        Assert.Equal(1024, data!.Coefficients.Length);
        Assert.All(data.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Overload_ZeroHcbSection_SkipsAndLeavesZeros()
    {
        var ics = LongIcsInfo(maxSfb: 49);
        var sections = SectionList((0, 0, 0, 49));

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, out var data));
        Assert.NotNull(data);
        Assert.All(data!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Overload_Cb1_ZeroSymbol_DecodesWithCanonicalCodebook()
    {
        // codes1[40] = 0x0000, bits1[40] = 1.
        // Single-bit "0" decodes to symbol 40 = (0, 0, 0, 0) in cb 1
        // (4-tuple signed, range [-1, +1]).
        var ics = LongIcsInfo(maxSfb: 1);
        // Long48 SWB 0..1 covers 4 coefficients = exactly one cb-1 tuple.
        var sections = SectionList((0, 1, 0, 1));

        byte[] bytes = { 0b0000_0000 };

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, out var data));
        Assert.NotNull(data);
        Assert.Equal(1, data!.BitsConsumed);
        Assert.Equal(0, data.Coefficients[0]);
        Assert.Equal(0, data.Coefficients[1]);
        Assert.Equal(0, data.Coefficients[2]);
        Assert.Equal(0, data.Coefficients[3]);
    }

    [Fact]
    public void Overload_ProducesIdenticalResult_To_ExplicitOverload()
    {
        // The two convenience entry points (TryParse with vs without the
        // codebook list) must yield byte-identical results when the
        // explicit overload is given the standard codebook list.
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 1, 0, 1));
        byte[] bytes = { 0b0000_0000 };

        Assert.True(AacSpectralData.TryParse(
            bytes, ics, sections, Sr48k, out var convenienceData));
        Assert.True(AacSpectralData.TryParse(
            bytes, ics, sections, Sr48k,
            AacStandardSpectralCodebooks.StandardCodebookList, out var explicitData));

        Assert.NotNull(convenienceData);
        Assert.NotNull(explicitData);
        Assert.Equal(convenienceData!.BitsConsumed, explicitData!.BitsConsumed);
        Assert.Equal(convenienceData.Coefficients.Length, explicitData.Coefficients.Length);
        for (int i = 0; i < convenienceData.Coefficients.Length; i++)
        {
            Assert.Equal(convenienceData.Coefficients[i], explicitData.Coefficients[i]);
        }
    }

    [Fact]
    public void Overload_NoiseHcbSection_SkipsWithoutDereferencingNullSlot()
    {
        // cb 13 (NOISE_HCB) is a sentinel - the walker must skip it
        // without ever indexing slot 13 of the codebook list.
        var ics = LongIcsInfo(maxSfb: 49);
        var sections = SectionList((0, 13, 0, 49));

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, out var data));
        Assert.NotNull(data);
        Assert.All(data!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void Overload_Cb12_RejectedByGeometry_ReturnsFalse()
    {
        // cb 12 is the reserved spectral codebook number. The walker
        // queries AacSpectralCodebookGeometry, which returns null for
        // cb 12, and TryRead must return false without dereferencing
        // the (null) standard codebook slot.
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 12, 0, 1));

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0xFF }, ics, sections, Sr48k, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void StandardCodebookList_Slot1Through11_Are_All_Distinct_Instances()
    {
        // The 11 codebooks must each be a different object - the sentinel
        // slots all collapse to null but slots 1..11 must not alias.
        var seen = new HashSet<object>();
        for (int i = 1; i <= 11; i++)
        {
            var book = AacStandardSpectralCodebooks.StandardCodebookList[i];
            Assert.NotNull(book);
            Assert.True(seen.Add(book!), $"slot {i} aliases another slot");
        }
    }

    [Fact]
    public void StandardCodebookList_Indexer_Returns_Same_Reference_Across_Calls()
    {
        // Repeated indexing returns the same cached codebook instance.
        for (int i = 1; i <= 11; i++)
        {
            var a = AacStandardSpectralCodebooks.StandardCodebookList[i];
            var b = AacStandardSpectralCodebooks.StandardCodebookList[i];
            Assert.Same(a, b);
        }
    }

    [Fact]
    public void Overload_EmptySectionList_HasCoefficientsLength_Equals_1024()
    {
        // Regardless of maxSfb / sample rate, long-window output must be 1024.
        var ics = LongIcsInfo(maxSfb: 0);
        var sections = SectionList();
        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, out var data));
        Assert.Equal(1024, data!.Coefficients.Length);
    }

    [Fact]
    public void Overload_TryParse_Idempotent_For_Same_Input()
    {
        // Two independent TryParse calls on identical bytes must produce
        // coefficient arrays with identical content.
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 1, 0, 1));
        byte[] bytes = { 0b0000_0000 };

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, out var a));
        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, out var b));
        Assert.Equal(a!.BitsConsumed, b!.BitsConsumed);
        Assert.Equal(a.Coefficients.ToArray(), b.Coefficients.ToArray());
    }

    [Theory]
    [InlineData(0)]
    [InlineData(13)]
    [InlineData(14)]
    [InlineData(15)]
    public void Overload_SentinelCb_PerSection_Always_Yields_Zero_Bits(int cb)
    {
        // cb 0 (ZERO_HCB), 13 (NOISE_HCB), 14/15 (INTENSITY_HCB) are all
        // skipped without consuming bits.
        var ics = LongIcsInfo(maxSfb: 49);
        var sections = SectionList((0, cb, 0, 49));

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, out var data));
        Assert.Equal(0, data!.BitsConsumed);
    }
}
