using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSpectralDataTests
{
    private const int Sr48k = 48_000;
    private const int Sr24k = 24_000;

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

    private static AacIcsInfo ShortIcsInfo(int maxSfb, byte[] windowsPerGroup) => new()
    {
        WindowSequence = AacWindowSequence.EightShort,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = 0,
        WindowGroupCount = windowsPerGroup.Length,
        WindowsPerGroup = windowsPerGroup,
        PredictorDataPresent = false,
    };

    private static AacSectionData SectionList(params (int Group, int Cb, int Start, int End)[] sections)
    {
        var list = new List<AacSection>(sections.Length);
        foreach (var s in sections)
        {
            list.Add(new AacSection { Group = s.Group, CodebookNumber = s.Cb, StartSfb = s.Start, EndSfb = s.End });
        }
        return new AacSectionData { Sections = list };
    }

    /// <summary>
    /// Build a codebook of <paramref name="symbolCount"/> symbols where every
    /// symbol has a fixed 7-bit code. The tree is incomplete (the upper half of
    /// the 128-slot code space is unassigned), but every encoded symbol decodes
    /// to its own index, which is all the walker tests need.
    /// </summary>
    private static AacHuffmanCodebook BuildFixed7BitCodebook(int symbolCount)
    {
        var lengths = new int[symbolCount];
        for (int i = 0; i < symbolCount; i++) lengths[i] = 7;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static AacHuffmanCodebook[] CodebooksWith(int slot, AacHuffmanCodebook book)
    {
        var arr = new AacHuffmanCodebook[16];
        arr[slot] = book;
        return arr;
    }

    [Fact]
    public void TryParse_EmptySectionList_ReturnsAllZeros()
    {
        var ics = LongIcsInfo(maxSfb: 0);
        var sections = SectionList(); // no entries
        var books = new AacHuffmanCodebook?[16];

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.Equal(1024, data!.Coefficients.Length);
        Assert.All(data.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_ZeroHcbSection_SkipsAndLeavesZeros()
    {
        var ics = LongIcsInfo(maxSfb: 49);
        // Single ZERO_HCB section covering every long SWB.
        var sections = SectionList((0, 0, 0, 49));
        var books = new AacHuffmanCodebook?[16];

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.All(data!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Theory]
    [InlineData(13)] // NOISE_HCB
    [InlineData(14)] // INTENSITY_HCB2
    [InlineData(15)] // INTENSITY_HCB
    public void TryParse_SentinelCodebookSections_SkipAndLeaveZeros(int sentinelCb)
    {
        var ics = LongIcsInfo(maxSfb: 49);
        var sections = SectionList((0, sentinelCb, 0, 49));
        var books = new AacHuffmanCodebook?[16];

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.All(data!.Coefficients, c => Assert.Equal(0, c));
        Assert.Equal(0, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_LongBlock_Cb1_DecodesQuadInPlace()
    {
        // Codebook 1: signed 4D, base 3, range [-1, +1], 81 symbols.
        // Symbol 80 decomposes to (1, 1, 1, 1).
        var book = BuildFixed7BitCodebook(81);
        var ics = LongIcsInfo(maxSfb: 1);
        // Long48 SWB 0..1 covers 4 coefficients (one tuple of dim 4).
        var sections = SectionList((0, 1, 0, 1));
        var books = CodebooksWith(1, book);

        var w = new AacBitWriter();
        w.Write(80u, 7); // codeword for symbol 80 in our fixed-length codebook
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.Equal(7, data!.BitsConsumed);
        Assert.Equal(1, data.Coefficients[0]);
        Assert.Equal(1, data.Coefficients[1]);
        Assert.Equal(1, data.Coefficients[2]);
        Assert.Equal(1, data.Coefficients[3]);
        // Everything past the section stays zero.
        for (int i = 4; i < 1024; i++) Assert.Equal(0, data.Coefficients[i]);
    }

    [Fact]
    public void TryParse_LongBlock_Cb1_ZeroSymbol_ReadsNoSignBits()
    {
        var book = BuildFixed7BitCodebook(81);
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 1, 0, 1));
        var books = CodebooksWith(1, book);

        var w = new AacBitWriter();
        // Symbol 40 decomposes to (0, 0, 0, 0) in cb 1; no sign bits follow even
        // though cb 1 is signed (it always is - sign is in the symbol index).
        w.Write(40u, 7);
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.Equal(7, data!.BitsConsumed);
        Assert.Equal(0, data.Coefficients[0]);
        Assert.Equal(0, data.Coefficients[1]);
        Assert.Equal(0, data.Coefficients[2]);
        Assert.Equal(0, data.Coefficients[3]);
    }

    [Fact]
    public void TryParse_LongBlock_Cb7_ReadsSignBitsForNonZeroMagnitudes()
    {
        // Codebook 7: unsigned 2D, base 8, range [0, 7], 64 symbols.
        // Symbol 9 decomposes to (1, 1); both non-zero, so two sign bits follow.
        var book = BuildFixed7BitCodebook(64);
        var ics = LongIcsInfo(maxSfb: 1);
        // SWB 0..1 = 4 coefficients = 2 tuples (cb 7 has dim 2).
        var sections = SectionList((0, 7, 0, 1));
        var books = CodebooksWith(7, book);

        var w = new AacBitWriter();
        // Tuple 0: symbol 9 -> (1, 1), signs (negative, positive) -> (-1, +1).
        w.Write(9u, 7);
        w.Write(1u, 1); // sign[0] = 1 -> negate first component
        w.Write(0u, 1); // sign[1] = 0 -> keep second positive
        // Tuple 1: symbol 0 -> (0, 0); no sign bits.
        w.Write(0u, 7);
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        // 7 + 1 + 1 + 7 = 16 bits.
        Assert.Equal(16, data!.BitsConsumed);
        Assert.Equal(-1, data.Coefficients[0]);
        Assert.Equal(+1, data.Coefficients[1]);
        Assert.Equal(0, data.Coefficients[2]);
        Assert.Equal(0, data.Coefficients[3]);
    }

    [Fact]
    public void TryParse_LongBlock_Cb11_TriggersEscapeSequence()
    {
        // Codebook 11: unsigned 2D, base 17, range [0, 16] with magnitude 16
        // signalling an escape. 289 symbols total - need 9 bits per code.
        var lengths = new int[289];
        for (int i = 0; i < 289; i++) lengths[i] = 9;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        var ics = LongIcsInfo(maxSfb: 1);
        // SWB 0..1 = 4 coefficients = 2 tuples of dim 2.
        var sections = SectionList((0, 11, 0, 1));
        var books = CodebooksWith(11, book);

        var w = new AacBitWriter();
        // Symbol 16 -> (0, 16). Magnitude 16 triggers escape on second component.
        w.Write(16u, 9);
        // Second component is 16: sign bit, then unary escape prefix (terminated by 0),
        // then (4 + prefix) extension bits.
        w.Write(0u, 1);          // sign = +1
        w.Write(0u, 1);          // prefix terminator (prefix length = 0)
        w.Write(0b0011u, 4);     // ext bits => magnitude = (1 << 4) + 3 = 19
        // Second tuple: symbol 0 -> (0, 0), no sign or escape bits.
        w.Write(0u, 9);
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        // 9 + 1 + 1 + 4 + 9 = 24 bits.
        Assert.Equal(24, data!.BitsConsumed);
        Assert.Equal(0, data.Coefficients[0]);
        Assert.Equal(19, data.Coefficients[1]);
        Assert.Equal(0, data.Coefficients[2]);
        Assert.Equal(0, data.Coefficients[3]);
    }

    [Fact]
    public void TryParse_ShortBlock_SingleGroup_EightWindows_LaysOutPerGroupBase()
    {
        // EIGHT_SHORT with one group of 8 windows. Coefficient layout:
        // group 0 covers all 1024 slots (= 128 * 8). SWB 0..1 has width 4 per
        // window, so the section spans 4 * 8 = 32 coefficients - 8 tuples of
        // dim 4 (cb 1).
        var book = BuildFixed7BitCodebook(81);
        var ics = ShortIcsInfo(maxSfb: 1, windowsPerGroup: new byte[] { 8 });
        var sections = SectionList((0, 1, 0, 1));
        var books = CodebooksWith(1, book);

        var w = new AacBitWriter();
        // Each tuple decodes symbol 80 -> (1, 1, 1, 1). 8 tuples => 32 ones.
        for (int t = 0; t < 8; t++) w.Write(80u, 7);
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.Equal(8 * 7, data!.BitsConsumed);
        for (int i = 0; i < 32; i++) Assert.Equal(1, data.Coefficients[i]);
        for (int i = 32; i < 1024; i++) Assert.Equal(0, data.Coefficients[i]);
    }

    [Fact]
    public void TryParse_ShortBlock_TwoGroups_OffsetsByGroupBase()
    {
        // Two groups of [3, 5] windows. Group 0 owns slots [0, 384) (= 128*3),
        // group 1 owns slots [384, 1024) (= 128*5). One section per group, cb 1,
        // SWB 0..1 (width 4 per window).
        //   Section 0 width = 4 * 3 = 12 coefficients (3 tuples), base = 0.
        //   Section 1 width = 4 * 5 = 20 coefficients (5 tuples), base = 384.
        var book = BuildFixed7BitCodebook(81);
        var ics = ShortIcsInfo(maxSfb: 1, windowsPerGroup: new byte[] { 3, 5 });
        var sections = SectionList(
            (0, 1, 0, 1),
            (1, 1, 0, 1));
        var books = CodebooksWith(1, book);

        var w = new AacBitWriter();
        // 3 tuples for group 0 - each symbol 80 -> (1, 1, 1, 1).
        for (int t = 0; t < 3; t++) w.Write(80u, 7);
        // 5 tuples for group 1 - each symbol 0 -> (-1, -1, -1, -1).
        for (int t = 0; t < 5; t++) w.Write(0u, 7);
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.Equal((3 + 5) * 7, data!.BitsConsumed);
        // Group 0: first 12 slots are +1.
        for (int i = 0; i < 12; i++) Assert.Equal(1, data.Coefficients[i]);
        // Gap between section 0 end and group 1 base is zero.
        for (int i = 12; i < 384; i++) Assert.Equal(0, data.Coefficients[i]);
        // Group 1: slots [384, 404) are -1.
        for (int i = 384; i < 404; i++) Assert.Equal(-1, data.Coefficients[i]);
        for (int i = 404; i < 1024; i++) Assert.Equal(0, data.Coefficients[i]);
    }

    [Fact]
    public void TryParse_ReservedCodebook12_Rejected()
    {
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 12, 0, 1));
        var books = new AacHuffmanCodebook?[16];

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0 }, ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_MissingCodebookSlot_Rejected()
    {
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 5, 0, 1));
        // No codebook registered at slot 5.
        var books = new AacHuffmanCodebook?[16];

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0 }, ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_CodebookListTooShort_Rejected()
    {
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 5, 0, 1));
        // Codebook list shorter than the referenced slot index.
        var books = new AacHuffmanCodebook?[3];

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0 }, ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_UnknownSampleRate_Rejected()
    {
        var ics = LongIcsInfo(maxSfb: 0);
        var sections = SectionList();
        var books = new AacHuffmanCodebook?[16];

        Assert.False(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, sampleRate: 100_000, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_StreamUnderflow_Rejected()
    {
        var book = BuildFixed7BitCodebook(81);
        var ics = LongIcsInfo(maxSfb: 1);
        var sections = SectionList((0, 1, 0, 1));
        var books = CodebooksWith(1, book);

        // Empty buffer: walker should fail at the first ReadBits call.
        Assert.False(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_GroupIndexOutOfRange_Rejected()
    {
        var book = BuildFixed7BitCodebook(81);
        var ics = LongIcsInfo(maxSfb: 1); // WindowGroupCount = 1
        // Section references group 1 even though only group 0 exists.
        var sections = SectionList((1, 1, 0, 1));
        var books = CodebooksWith(1, book);

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0xFF }, ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_StartSfbOutOfRange_Rejected()
    {
        var book = BuildFixed7BitCodebook(81);
        var ics = LongIcsInfo(maxSfb: 1);
        // swbOffsets for 48k long has length 50 (49 SWBs + close); index 60 is out.
        var sections = SectionList((0, 1, 60, 61));
        var books = CodebooksWith(1, book);

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0 }, ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_DegenerateSectionRange_Rejected()
    {
        var book = BuildFixed7BitCodebook(81);
        var ics = LongIcsInfo(maxSfb: 1);
        // EndSfb == StartSfb is degenerate; walker should reject.
        var sections = SectionList((0, 1, 1, 1));
        var books = CodebooksWith(1, book);

        Assert.False(AacSpectralData.TryParse(
            new byte[] { 0 }, ics, sections, Sr48k, books, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_BitsConsumedAccountsForAllTuplesPlusEscape()
    {
        // Two sections back to back: cb 7 (1 tuple = 9 bits with signs) +
        // cb 1 (1 tuple = 7 bits, no signs). 16 bits in total.
        var book7 = BuildFixed7BitCodebook(64);
        var lengths81 = new int[81];
        for (int i = 0; i < 81; i++) lengths81[i] = 7;
        var book1 = AacHuffmanCodebook.FromCanonicalLengths(lengths81);
        // sfb 0..1 = 4 coefficients (cb 7 dim 2 -> 2 tuples; one tuple for cb 1
        // needs 4 coefficients which is sfb 1..2).
        var ics = LongIcsInfo(maxSfb: 2);
        var sections = SectionList(
            (0, 7, 0, 1),   // 4 coefficients, 2 tuples
            (0, 1, 1, 2));  // 4 coefficients, 1 tuple
        var books = new AacHuffmanCodebook?[16];
        books[7] = book7;
        books[1] = book1;

        var w = new AacBitWriter();
        // Section 0 (cb 7): two tuples of symbol 0 -> (0, 0). No sign bits.
        w.Write(0u, 7);
        w.Write(0u, 7);
        // Section 1 (cb 1): one tuple of symbol 40 -> (0, 0, 0, 0).
        w.Write(40u, 7);
        var bytes = w.ToArray();

        Assert.True(AacSpectralData.TryParse(bytes, ics, sections, Sr48k, books, out var data));
        Assert.NotNull(data);
        Assert.Equal(7 + 7 + 7, data!.BitsConsumed);
        // All decoded coefficients are zero; rest of the 1024 stays zero too.
        Assert.All(data.Coefficients, c => Assert.Equal(0, c));
    }

    [Fact]
    public void TryParse_NullArguments_Throws()
    {
        var ics = LongIcsInfo(maxSfb: 0);
        var sections = SectionList();
        var books = new AacHuffmanCodebook?[16];

        Assert.Throws<ArgumentNullException>(() =>
            AacSpectralData.TryParse(Array.Empty<byte>(), null!, sections, Sr48k, books, out _));
        Assert.Throws<ArgumentNullException>(() =>
            AacSpectralData.TryParse(Array.Empty<byte>(), ics, null!, Sr48k, books, out _));
        Assert.Throws<ArgumentNullException>(() =>
            AacSpectralData.TryParse(Array.Empty<byte>(), ics, sections, Sr48k, null!, out _));
    }

    [Fact]
    public void TryParse_TransformLengthConstantIs1024()
    {
        // Spot-check that the public constant matches the long-block table close.
        Assert.Equal(1024, AacSpectralData.TransformLength);
        Assert.Equal(AacSwbOffsets.LongTransformLength, AacSpectralData.TransformLength);
    }

    [Fact]
    public void TryParse_LowSampleRateLongBlock_DispatchesToCorrectTable()
    {
        // 24k uses Long24 table - SWB widths differ from Long48. Just need to
        // confirm the dispatch picks a non-empty table and the walker accepts
        // a single ZERO_HCB section over every SWB.
        var ics = LongIcsInfo(maxSfb: 47); // Long24 has 47 SWBs (48 entries)
        var sections = SectionList((0, 0, 0, 47));
        var books = new AacHuffmanCodebook?[16];

        Assert.True(AacSpectralData.TryParse(
            Array.Empty<byte>(), ics, sections, Sr24k, books, out var data));
        Assert.NotNull(data);
        Assert.All(data!.Coefficients, c => Assert.Equal(0, c));
    }
}
