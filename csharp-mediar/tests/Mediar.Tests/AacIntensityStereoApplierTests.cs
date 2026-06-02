using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacIntensityStereoApplierTests
{
    private const int Sr48k = 48_000;

    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static AacHuffmanCodebook BuildFixed7BitCodebook(int symbolCount)
    {
        var lengths = new int[symbolCount];
        for (int i = 0; i < symbolCount; i++) lengths[i] = 7;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static AacHuffmanCodebook?[] CodebooksWith(int slot, AacHuffmanCodebook book)
    {
        var arr = new AacHuffmanCodebook?[16];
        arr[slot] = book;
        return arr;
    }

    private static void WriteLongIcsInfo(AacBitWriter w, int maxSfb)
    {
        w.Write(0u, 1);
        w.Write((uint)AacWindowSequence.OnlyLong, 2);
        w.Write(0u, 1);
        w.Write((uint)maxSfb, 6);
        w.Write(0u, 1);
    }

    private static (uint code, int len) EncodeSfDiff(int diff)
    {
        int sym = 60 + diff;
        return sym == 60 ? (0u, 1) : ((uint)(0x80 + (sym < 60 ? sym : sym - 1)), 8);
    }

    /// <summary>
    /// Build a 2-SFB long-window frame: SFB 0 = cb 1 (spectral),
    /// SFB 1 = cb <paramref name="isCb"/> (14 or 15, intensity-stereo)
    /// with intensity-position scale factor equal to <paramref name="isPosition"/>.
    /// </summary>
    private static AacChannelFrame BuildFrameWithIntensityStereo(int isCb, int isPosition)
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        const int globalGain = 100;

        var w = new AacBitWriter();
        w.Write((uint)globalGain, 8);
        WriteLongIcsInfo(w, maxSfb: 2);

        // Section 1: cb=1, len=1 (covers SFB 0)
        w.Write(1u, 4);
        w.Write(1u, 5);
        // Section 2: cb=14 or 15 (intensity stereo), len=1 (covers SFB 1)
        w.Write((uint)isCb, 4);
        w.Write(1u, 5);

        // SF stream:
        //   SFB 0: ordinary SF diff = 0.
        var (sfCode, sfLen) = EncodeSfDiff(0);
        w.Write(sfCode, sfLen);
        //   SFB 1: intensity-position differential = isPosition (accumulator starts at 0).
        var (isCode, isLen) = EncodeSfDiff(isPosition);
        w.Write(isCode, isLen);

        // pulse/tns/gain flags
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);

        // spectral_data: only SFB 0 emits (1 quad → symbol 80 → (1,1,1,1)).
        // SFB 1 is intensity stereo, emits no spectral bits.
        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        return frame!;
    }

    private static IReadOnlyList<bool>[] EmptyMsUsed()
        => Array.Empty<IReadOnlyList<bool>>();

    private static IReadOnlyList<bool>[] MsUsedOneGroupOneBand(bool flag)
        => new[] { (IReadOnlyList<bool>)new[] { false, flag } };

    private static float[] LeftSpectrumImpulse(int bandStart, int bandWidth, float value)
    {
        var left = new float[AacDequantizedSpectrum.TransformLength];
        for (int i = 0; i < bandWidth; i++) left[bandStart + i] = value;
        return left;
    }

    [Fact]
    public void ApplyInPlace_NullFrame_Throws()
    {
        var left = new float[AacDequantizedSpectrum.TransformLength];
        var right = new float[AacDequantizedSpectrum.TransformLength];
        Assert.Throws<ArgumentNullException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, null!, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k));
    }

    [Fact]
    public void ApplyInPlace_NullMsUsed_Throws()
    {
        var left = new float[AacDequantizedSpectrum.TransformLength];
        var right = new float[AacDequantizedSpectrum.TransformLength];
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, frame, AacMsMaskPresent.None, null!, Sr48k));
    }

    [Fact]
    public void ApplyInPlace_WrongLeftLength_Throws()
    {
        var left = new float[100];
        var right = new float[AacDequantizedSpectrum.TransformLength];
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k));
        Assert.Equal("left", ex.ParamName);
    }

    [Fact]
    public void ApplyInPlace_WrongRightLength_Throws()
    {
        var left = new float[AacDequantizedSpectrum.TransformLength];
        var right = new float[100];
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k));
        Assert.Equal("right", ex.ParamName);
    }

    [Fact]
    public void ApplyInPlace_ReservedMsMask_Throws()
    {
        var left = new float[AacDequantizedSpectrum.TransformLength];
        var right = new float[AacDequantizedSpectrum.TransformLength];
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, frame, AacMsMaskPresent.Reserved, EmptyMsUsed(), Sr48k));
        Assert.Equal("msMaskPresent", ex.ParamName);
    }

    [Fact]
    public void ApplyInPlace_PositivePolarityIsPosition0_PassesLeftThrough()
    {
        // cb=14, is_position=0 → scale = +1 × 0.5^0 = +1.
        // Expect right band to equal left band exactly.
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 3.5f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(3.5f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_NegativePolarityIsPosition0_NegatesLeft()
    {
        // cb=15, is_position=0 → scale = -1 × 0.5^0 = -1.
        var frame = BuildFrameWithIntensityStereo(isCb: 15, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 2.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(-2.0f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_IsPosition4_HalvesLeft()
    {
        // cb=14, is_position=4 → scale = +1 × 0.5^1 = +0.5.
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 4);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(0.5f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_IsPositionNegative4_DoublesLeft()
    {
        // cb=14, is_position=-4 → scale = +1 × 0.5^-1 = +2.0.
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: -4);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(2.0f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_PositivePolarityMsAllBands_FlipsSign()
    {
        // cb=14 with ms_used=all-bands → sign flip → -1.
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.5f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.AllBands, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(-1.5f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_NegativePolarityMsAllBands_FlipsBackToPositive()
    {
        // cb=15 with ms_used=all-bands → sign double-flip → +1.
        var frame = BuildFrameWithIntensityStereo(isCb: 15, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.5f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.AllBands, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(1.5f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_PerBandMsFlagSetForIsBand_FlipsSign()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame,
            AacMsMaskPresent.PerBand,
            MsUsedOneGroupOneBand(flag: true),
            Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(-1.0f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_PerBandMsFlagClearForIsBand_DoesNotFlipSign()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame,
            AacMsMaskPresent.PerBand,
            MsUsedOneGroupOneBand(flag: false),
            Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(1.0f, right[bandStart + i], 5);
        }
    }

    [Fact]
    public void ApplyInPlace_NonIsBands_LeftUntouched()
    {
        // The SFB 0 band (cb=1, spectral) must not be overwritten.
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 0.5f);
        var right = new float[AacDequantizedSpectrum.TransformLength];
        right[0] = 99.0f;  // SFB 0 region; should remain.

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        Assert.Equal(99.0f, right[0]);
    }

    [Fact]
    public void Apply_ReturnsNewSpectrumWithIsBandsFilled()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var leftArr = LeftSpectrumImpulse(bandStart, bandWidth, 4.0f);
        var rightArr = new float[AacDequantizedSpectrum.TransformLength];
        var leftSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(leftArr),
        };
        var rightSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(rightArr),
        };

        var result = AacIntensityStereoApplier.Apply(
            leftSpec, rightSpec, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(4.0f, result.Coefficients[bandStart + i], 5);
        }

        // Input right is unchanged.
        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(0f, rightSpec.Coefficients[bandStart + i]);
        }
    }

    [Fact]
    public void ApplyInPlace_NoIsSections_LeavesRightUnchanged()
    {
        // Build a frame WITHOUT IS bands (use NoiseHcb=13 instead).
        // The applier should walk the sections, find no cb=14/15,
        // and leave the right channel untouched.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        const int globalGain = 100;
        var w = new AacBitWriter();
        w.Write((uint)globalGain, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4);
        w.Write(1u, 5);

        var (sfCode, sfLen) = EncodeSfDiff(0);
        w.Write(sfCode, sfLen);

        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));

        var left = new float[AacDequantizedSpectrum.TransformLength];
        for (int i = 0; i < left.Length; i++) left[i] = i * 0.01f;
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame!, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < right.Length; i++)
        {
            Assert.Equal(0f, right[i]);
        }
    }

    // ----- EightShort window-sequence intensity-stereo coverage -----

    private static void WriteShortIcsInfo(AacBitWriter w, int maxSfb, byte grouping)
    {
        w.Write(0u, 1);                                 // ics_reserved_bit
        w.Write((uint)AacWindowSequence.EightShort, 2); // window_sequence
        w.Write(0u, 1);                                 // window_shape
        w.Write((uint)maxSfb, 4);                       // max_sfb (4 bits for EightShort)
        w.Write(grouping, 7);                           // scale_factor_grouping
    }

    /// <summary>
    /// Build a 2-SFB EightShort right-channel frame (all 8 windows in one group):
    /// SFB 0 = cb 1 (spectral), SFB 1 = cb <paramref name="isCb"/> (14 or 15, intensity stereo)
    /// with intensity-position accumulator equal to <paramref name="isPosition"/>.
    /// At 48 kHz the IS band spans coefs [32, 64) after grouping (4 × 8 windows).
    /// </summary>
    private static AacChannelFrame BuildShortFrameWithIntensityStereo(int isCb, int isPosition)
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        const int globalGain = 100;

        var w = new AacBitWriter();
        w.Write((uint)globalGain, 8);
        WriteShortIcsInfo(w, maxSfb: 2, grouping: 0x7F);

        // Section data (1 group), 3-bit sect_len_incr for short.
        w.Write(1u, 4); w.Write(1u, 3);
        w.Write((uint)isCb, 4); w.Write(1u, 3);

        // Scale factors (1 group, 2 SFBs):
        //   SFB 0: ordinary SF diff = 0.
        var (sfCode, sfLen) = EncodeSfDiff(0);
        w.Write(sfCode, sfLen);
        //   SFB 1: intensity-position differential (accumulator starts at 0).
        var (isCode, isLen) = EncodeSfDiff(isPosition);
        w.Write(isCode, isLen);

        // pulse/tns/gain flags.
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);

        // Spectral data: SFB 0 covers 4 coefs × 8 windows = 32 bins = 8 quads of sym 80.
        for (int i = 0; i < 8; i++) w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        Assert.Equal(AacWindowSequence.EightShort, frame!.Stream.IcsInfo.WindowSequence);
        return frame;
    }

    [Fact]
    public void ApplyInPlace_ShortWindow_PositivePolarity_ScalesRightAcrossAllEightWindows()
    {
        const int isPosition = 4; // scale = 0.5^(4/4) = 0.5
        var frame = BuildShortFrameWithIntensityStereo(isCb: 14, isPosition);

        // Group-major layout: PNS/IS SFB 1 at coefs [32, 64).
        var left = LeftSpectrumImpulse(bandStart: 32, bandWidth: 32, value: 2.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 32; i < 64; i++)
        {
            Assert.Equal(1.0f, right[i], 6);
        }
        // Outside the IS band, right stays zero.
        for (int i = 0; i < 32; i++) Assert.Equal(0f, right[i]);
        for (int i = 64; i < right.Length; i++) Assert.Equal(0f, right[i]);
    }

    [Fact]
    public void ApplyInPlace_ShortWindow_NegativePolarity_FlipsSign()
    {
        const int isPosition = 0; // scale magnitude = 1.0
        var frame = BuildShortFrameWithIntensityStereo(isCb: 15, isPosition);

        var left = LeftSpectrumImpulse(bandStart: 32, bandWidth: 32, value: 0.5f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 32; i < 64; i++)
        {
            Assert.Equal(-0.5f, right[i], 6);
        }
    }

    [Fact]
    public void ApplyInPlace_ShortWindow_MsAllBands_FlipsSignBackToPositive()
    {
        const int isPosition = 0;
        var frame = BuildShortFrameWithIntensityStereo(isCb: 15, isPosition);

        var left = LeftSpectrumImpulse(bandStart: 32, bandWidth: 32, value: 0.5f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.AllBands, EmptyMsUsed(), Sr48k);

        // cb=15 sign (-1) XOR MS-mask (-1) = +1.
        for (int i = 32; i < 64; i++)
        {
            Assert.Equal(0.5f, right[i], 6);
        }
    }

    [Fact]
    public void ApplyInPlace_UnknownSampleRate_Throws()
    {
        var left = new float[AacDequantizedSpectrum.TransformLength];
        var right = new float[AacDequantizedSpectrum.TransformLength];
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var ex = Assert.Throws<ArgumentException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), sampleRate: 7));
        Assert.Equal("sampleRate", ex.ParamName);
    }

    [Fact]
    public void ApplyInPlace_PerBand_NullGroupFlagsRow_Throws()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var left = new float[AacDequantizedSpectrum.TransformLength];
        var right = new float[AacDequantizedSpectrum.TransformLength];
        // PerBand requires non-null rows; pass a single null row that will
        // be reached when the IS section is processed.
        var msUsed = new IReadOnlyList<bool>?[] { null };
        var ex = Assert.Throws<ArgumentException>(() =>
            AacIntensityStereoApplier.ApplyInPlace(
                left, right, frame, AacMsMaskPresent.PerBand, msUsed!, Sr48k));
        Assert.Equal("msUsed", ex.ParamName);
    }

    [Fact]
    public void ApplyInPlace_LeftSpectrumOutsideIsBand_IsNotMutated()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 2.0f);
        var leftSnapshot = (float[])left.Clone();
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        Assert.Equal(leftSnapshot, left);
    }

    [Fact]
    public void Apply_NullLeftSpectrum_Throws()
    {
        var rightSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(
                new float[AacDequantizedSpectrum.TransformLength]),
        };
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacIntensityStereoApplier.Apply(null!, rightSpec, frame,
                AacMsMaskPresent.None, EmptyMsUsed(), Sr48k));
    }

    [Fact]
    public void Apply_NullRightSpectrum_Throws()
    {
        var leftSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(
                new float[AacDequantizedSpectrum.TransformLength]),
        };
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacIntensityStereoApplier.Apply(leftSpec, null!, frame,
                AacMsMaskPresent.None, EmptyMsUsed(), Sr48k));
    }

    [Fact]
    public void Apply_NullRightFrame_Throws()
    {
        var leftSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(
                new float[AacDequantizedSpectrum.TransformLength]),
        };
        var rightSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(
                new float[AacDequantizedSpectrum.TransformLength]),
        };
        Assert.Throws<ArgumentNullException>(() =>
            AacIntensityStereoApplier.Apply(leftSpec, rightSpec, null!,
                AacMsMaskPresent.None, EmptyMsUsed(), Sr48k));
    }

    [Fact]
    public void Apply_NullMsUsed_Throws()
    {
        var leftSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(
                new float[AacDequantizedSpectrum.TransformLength]),
        };
        var rightSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(
                new float[AacDequantizedSpectrum.TransformLength]),
        };
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        Assert.Throws<ArgumentNullException>(() =>
            AacIntensityStereoApplier.Apply(leftSpec, rightSpec, frame,
                AacMsMaskPresent.None, null!, Sr48k));
    }

    [Fact]
    public void Apply_NegativePolarity_FlipsSign()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 15, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var leftArr = LeftSpectrumImpulse(bandStart, bandWidth, 3.0f);
        var rightArr = new float[AacDequantizedSpectrum.TransformLength];
        var leftSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(leftArr),
        };
        var rightSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(rightArr),
        };

        var result = AacIntensityStereoApplier.Apply(
            leftSpec, rightSpec, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal(-3.0f, result.Coefficients[bandStart + i], 5);
        }
    }

    [Fact]
    public void Apply_LeftSpectrum_IsNotMutated()
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition: 0);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var leftArr = LeftSpectrumImpulse(bandStart, bandWidth, 7.0f);
        var leftSnapshot = (float[])leftArr.Clone();
        var rightArr = new float[AacDequantizedSpectrum.TransformLength];
        var leftSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(leftArr),
        };
        var rightSpec = new AacDequantizedSpectrum
        {
            Coefficients = System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(rightArr),
        };

        _ = AacIntensityStereoApplier.Apply(
            leftSpec, rightSpec, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        Assert.Equal(leftSnapshot, leftArr);
    }

    [Theory]
    [InlineData(-8, 4.0)]
    [InlineData(-4, 2.0)]
    [InlineData(0, 1.0)]
    [InlineData(4, 0.5)]
    [InlineData(8, 0.25)]
    public void ApplyInPlace_PositivePolarity_IsPositionScalesByHalfPowerQuarter(
        int isPosition, double expectedScale)
    {
        var frame = BuildFrameWithIntensityStereo(isCb: 14, isPosition);
        var swbOffsets = AacSwbOffsets.GetLongOffsets(Sr48k);
        int bandStart = swbOffsets[1];
        int bandWidth = swbOffsets[2] - swbOffsets[1];

        var left = LeftSpectrumImpulse(bandStart, bandWidth, 1.0f);
        var right = new float[AacDequantizedSpectrum.TransformLength];

        AacIntensityStereoApplier.ApplyInPlace(
            left, right, frame, AacMsMaskPresent.None, EmptyMsUsed(), Sr48k);

        for (int i = 0; i < bandWidth; i++)
        {
            Assert.Equal((float)expectedScale, right[bandStart + i], 5);
        }
    }
}
