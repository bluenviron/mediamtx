using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacMsStereoDecoderTests
{
    private const int Sr48k = 48_000;
    private const int TransformLength = AacSpectralData.TransformLength;

    private static AacIcsInfo LongIcs(int maxSfb) => new()
    {
        WindowSequence = AacWindowSequence.OnlyLong,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = null,
        PredictorDataPresent = false,
        WindowGroupCount = 1,
        WindowsPerGroup = new byte[] { 1 },
    };

    private static AacIcsInfo ShortIcs(int maxSfb, byte[] windowsPerGroup, byte grouping)
        => new()
        {
            WindowSequence = AacWindowSequence.EightShort,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = maxSfb,
            ScaleFactorGrouping = grouping,
            PredictorDataPresent = false,
            WindowGroupCount = windowsPerGroup.Length,
            WindowsPerGroup = windowsPerGroup,
        };

    private static AacSectionData OneSection(int group, int startSfb, int endSfb, int cb)
        => new()
        {
            Sections = new[]
            {
                new AacSection
                {
                    Group = group,
                    CodebookNumber = cb,
                    StartSfb = startSfb,
                    EndSfb = endSfb,
                },
            },
        };

    private static AacSectionData EmptySections() => new()
    {
        Sections = Array.Empty<AacSection>(),
    };

    private static (float[] Left, float[] Right) MakeBuffers(int filledBands, int bandSize, float lFill, float rFill)
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < filledBands * bandSize; i++)
        {
            left[i] = lFill;
            right[i] = rFill;
        }
        return (left, right);
    }

    [Fact]
    public void Decode_NullIcsInfo_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentNullException>(() =>
            DecodeWith(left, right, null!, AacMsMaskPresent.AllBands, Array.Empty<IReadOnlyList<bool>>(), EmptySections(), Sr48k));
    }

    [Fact]
    public void Decode_NullMsUsed_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentNullException>(() =>
            DecodeWith(left, right, LongIcs(1), AacMsMaskPresent.PerBand, null!, EmptySections(), Sr48k));
    }

    [Fact]
    public void Decode_NullRightSections_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentNullException>(() =>
            DecodeWith(left, right, LongIcs(1), AacMsMaskPresent.AllBands, Array.Empty<IReadOnlyList<bool>>(), null!, Sr48k));
    }

    [Fact]
    public void Decode_ReservedMask_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentException>(() =>
            DecodeWith(left, right, LongIcs(1), AacMsMaskPresent.Reserved, Array.Empty<IReadOnlyList<bool>>(), EmptySections(), Sr48k));
    }

    [Fact]
    public void Decode_BadSampleRate_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentException>(() =>
            DecodeWith(left, right, LongIcs(1), AacMsMaskPresent.AllBands, Array.Empty<IReadOnlyList<bool>>(), EmptySections(), sampleRate: 192_000));
    }

    [Fact]
    public void Decode_WrongLeftLength_Throws()
    {
        var left = new float[100];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentException>(() =>
            DecodeWith(left, right, LongIcs(1), AacMsMaskPresent.AllBands, Array.Empty<IReadOnlyList<bool>>(), EmptySections(), Sr48k));
    }

    [Fact]
    public void Decode_WrongRightLength_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[100];
        Assert.Throws<ArgumentException>(() =>
            DecodeWith(left, right, LongIcs(1), AacMsMaskPresent.AllBands, Array.Empty<IReadOnlyList<bool>>(), EmptySections(), Sr48k));
    }

    [Fact]
    public void Decode_None_LeavesBuffersUntouched()
    {
        var (left, right) = MakeBuffers(filledBands: 1, bandSize: 4, lFill: 3f, rFill: 1f);
        var lcopy = (float[])left.Clone();
        var rcopy = (float[])right.Clone();
        AacMsStereoDecoder.Decode(
            left, right, LongIcs(1), AacMsMaskPresent.None,
            Array.Empty<IReadOnlyList<bool>>(), EmptySections(), Sr48k);
        Assert.Equal(lcopy, left);
        Assert.Equal(rcopy, right);
    }

    [Fact]
    public void Decode_AllBands_AppliesSumAndDifference_OnSingleLongBand()
    {
        // SWB 0 on 48 kHz long covers samples [0, 4): width 4.
        // Left = 3, Right = 1 → New left = 4, New right = 2.
        var (left, right) = MakeBuffers(filledBands: 1, bandSize: 4, lFill: 3f, rFill: 1f);
        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(1),
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            OneSection(0, 0, 1, cb: 1),
            Sr48k);

        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(4f, left[i]);
            Assert.Equal(2f, right[i]);
        }
        for (int i = 4; i < TransformLength; i++)
        {
            Assert.Equal(0f, left[i]);
            Assert.Equal(0f, right[i]);
        }
    }

    [Fact]
    public void Decode_PerBand_OnlyMarkedBandsAreTransformed()
    {
        // 2 SFBs. SWB 0 = [0,4), SWB 1 = [4,8). Mark only band 1.
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 4; i++) { left[i] = 5f; right[i] = 2f; }
        for (int i = 4; i < 8; i++) { left[i] = 7f; right[i] = 3f; }

        var msUsed = new bool[][] { new[] { false, true } };
        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(2),
            AacMsMaskPresent.PerBand,
            msUsed,
            new AacSectionData
            {
                Sections = new[]
                {
                    new AacSection { Group = 0, CodebookNumber = 1, StartSfb = 0, EndSfb = 2 },
                },
            },
            Sr48k);

        // Band 0 untouched.
        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(5f, left[i]);
            Assert.Equal(2f, right[i]);
        }
        // Band 1 transformed: L=10, R=4.
        for (int i = 4; i < 8; i++)
        {
            Assert.Equal(10f, left[i]);
            Assert.Equal(4f, right[i]);
        }
    }

    [Fact]
    public void Decode_AllBands_SkipsIntensityStereoBand_Cb15()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 4; i++) { left[i] = 9f; right[i] = 3f; }   // band 0 (IS in right ch)
        for (int i = 4; i < 8; i++) { left[i] = 9f; right[i] = 3f; }   // band 1 (normal)

        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(2),
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            new AacSectionData
            {
                Sections = new[]
                {
                    new AacSection { Group = 0, CodebookNumber = 15, StartSfb = 0, EndSfb = 1 }, // IS
                    new AacSection { Group = 0, CodebookNumber = 1, StartSfb = 1, EndSfb = 2 },  // normal
                },
            },
            Sr48k);

        // Band 0 skipped (IS).
        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(9f, left[i]);
            Assert.Equal(3f, right[i]);
        }
        // Band 1 transformed: L=12, R=6.
        for (int i = 4; i < 8; i++)
        {
            Assert.Equal(12f, left[i]);
            Assert.Equal(6f, right[i]);
        }
    }

    [Fact]
    public void Decode_AllBands_SkipsIntensityStereoBand_Cb14()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 4; i++) { left[i] = 9f; right[i] = 3f; }

        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(1),
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            OneSection(0, 0, 1, cb: 14),
            Sr48k);

        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(9f, left[i]);
            Assert.Equal(3f, right[i]);
        }
    }

    [Fact]
    public void Decode_AllBands_SkipsPnsBand_Cb13()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 4; i++) { left[i] = 9f; right[i] = 3f; }

        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(1),
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            OneSection(0, 0, 1, cb: 13),
            Sr48k);

        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(9f, left[i]);
            Assert.Equal(3f, right[i]);
        }
    }

    [Fact]
    public void Decode_EmptyMaxSfb_NoOp()
    {
        var (left, right) = MakeBuffers(filledBands: 1, bandSize: 4, lFill: 3f, rFill: 1f);
        var lcopy = (float[])left.Clone();
        var rcopy = (float[])right.Clone();
        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(0),
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            EmptySections(),
            Sr48k);
        Assert.Equal(lcopy, left);
        Assert.Equal(rcopy, right);
    }

    [Fact]
    public void Decode_PerBand_RowShortCount_ThrowsArgumentException()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        Assert.Throws<ArgumentException>(() =>
            AacMsStereoDecoder.Decode(
                left, right,
                LongIcs(1),
                AacMsMaskPresent.PerBand,
                Array.Empty<IReadOnlyList<bool>>(),
                EmptySections(),
                Sr48k));
    }

    [Fact]
    public void Decode_PerBand_NullRowEntry_Throws()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        var msUsed = new IReadOnlyList<bool>[] { null! };
        Assert.Throws<ArgumentException>(() =>
            AacMsStereoDecoder.Decode(
                left, right,
                LongIcs(1),
                AacMsMaskPresent.PerBand,
                msUsed,
                EmptySections(),
                Sr48k));
    }

    [Fact]
    public void Decode_NegativeValues_SumAndDifferencePreserveSigns()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 4; i++) { left[i] = -2f; right[i] = 3f; }
        // M+S = 1, M-S = -5.
        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(1),
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            OneSection(0, 0, 1, cb: 1),
            Sr48k);
        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(1f, left[i]);
            Assert.Equal(-5f, right[i]);
        }
    }

    [Fact]
    public void Decode_ShortWindow_WalksGroupSizes()
    {
        // EightShort with grouping such that one group of 8 windows. SWB 0
        // at 48 kHz short covers samples [0, 4) in each window. With
        // wig=8 the band spans 32 contiguous samples in the group's flat
        // layout (SFB-major).
        var ics = ShortIcs(maxSfb: 1, windowsPerGroup: new byte[] { 8 }, grouping: 0);
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 32; i++) { left[i] = 7f; right[i] = 5f; }

        AacMsStereoDecoder.Decode(
            left, right,
            ics,
            AacMsMaskPresent.AllBands,
            Array.Empty<IReadOnlyList<bool>>(),
            OneSection(0, 0, 1, cb: 1),
            Sr48k);

        for (int i = 0; i < 32; i++)
        {
            Assert.Equal(12f, left[i]);
            Assert.Equal(2f, right[i]);
        }
        for (int i = 32; i < TransformLength; i++)
        {
            Assert.Equal(0f, left[i]);
            Assert.Equal(0f, right[i]);
        }
    }

    [Fact]
    public void Decode_PerBand_RowLongerThanMaxSfb_IsToleratedAndExtraBandsAreIgnored()
    {
        var left = new float[TransformLength];
        var right = new float[TransformLength];
        for (int i = 0; i < 4; i++) { left[i] = 6f; right[i] = 2f; }

        var msUsed = new bool[][] { new[] { true, true, true } }; // 3 entries, max_sfb=1
        AacMsStereoDecoder.Decode(
            left, right,
            LongIcs(1),
            AacMsMaskPresent.PerBand,
            msUsed,
            OneSection(0, 0, 1, cb: 1),
            Sr48k);

        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(8f, left[i]);
            Assert.Equal(4f, right[i]);
        }
    }

    [Fact]
    public void DecodeFromCpe_NullCpe_Throws()
    {
        var spec = new AacDequantizedSpectrum { Coefficients = System.Collections.Immutable.ImmutableArray.Create(new float[TransformLength]) };
        Assert.Throws<ArgumentNullException>(() =>
            AacMsStereoDecoder.DecodeFromCpe(null!, spec, spec, Sr48k));
    }

    [Fact]
    public void DecodeFromCpe_MsNone_ReturnsSameSpectraReference()
    {
        var cpe = BuildSimpleCpe(AacMsMaskPresent.None);
        var left = MakeSpectrum(filledBand: 4, lFill: 7f);
        var right = MakeSpectrum(filledBand: 4, lFill: 3f);

        var (newLeft, newRight) = AacMsStereoDecoder.DecodeFromCpe(cpe, left, right, Sr48k);
        Assert.Same(left, newLeft);
        Assert.Same(right, newRight);
    }

    [Fact]
    public void DecodeFromCpe_NoCommonWindow_Throws()
    {
        var cpe = BuildSimpleCpe(AacMsMaskPresent.AllBands, commonWindow: false);
        var left = MakeSpectrum(filledBand: 4, lFill: 7f);
        var right = MakeSpectrum(filledBand: 4, lFill: 3f);

        Assert.Throws<ArgumentException>(() =>
            AacMsStereoDecoder.DecodeFromCpe(cpe, left, right, Sr48k));
    }

    [Fact]
    public void DecodeFromCpe_AllBands_AppliesSumAndDifference()
    {
        var cpe = BuildSimpleCpe(AacMsMaskPresent.AllBands);
        var left = MakeSpectrum(filledBand: 4, lFill: 7f);
        var right = MakeSpectrum(filledBand: 4, lFill: 3f);

        var (newLeft, newRight) = AacMsStereoDecoder.DecodeFromCpe(cpe, left, right, Sr48k);

        Assert.Equal(10f, newLeft.Coefficients[0]);
        Assert.Equal(10f, newLeft.Coefficients[3]);
        Assert.Equal(4f, newRight.Coefficients[0]);
        Assert.Equal(4f, newRight.Coefficients[3]);
        // Originals untouched.
        Assert.Equal(7f, left.Coefficients[0]);
        Assert.Equal(3f, right.Coefficients[0]);
    }

    private static AacDequantizedSpectrum MakeSpectrum(int filledBand, float lFill)
    {
        var buf = new float[TransformLength];
        for (int i = 0; i < filledBand; i++) buf[i] = lFill;
        return new AacDequantizedSpectrum
        {
            Coefficients = System.Collections.Immutable.ImmutableArray.Create(buf),
        };
    }

    private static AacChannelPairElement BuildSimpleCpe(AacMsMaskPresent mask, bool commonWindow = true)
    {
        var ics = LongIcs(1);
        var stream = new AacIndividualChannelStream
        {
            GlobalGain = 100,
            OwnIcsInfo = null,
            IcsInfo = ics,
            SectionData = OneSection(0, 0, 1, cb: 1),
            ScaleFactorData = new AacScaleFactorData
            {
                Entries = Array.Empty<AacScaleFactorEntry>(),
                BitsConsumed = 0,
            },
            PulseDataPresent = false,
            PulseData = null,
            TnsDataPresent = false,
            TnsData = null,
            GainControlDataPresent = false,
            GainControlData = null,
            BitsConsumed = 0,
        };
        return new AacChannelPairElement
        {
            ElementInstanceTag = 0,
            CommonWindow = commonWindow,
            SharedIcsInfo = commonWindow ? ics : null,
            MsMaskPresent = mask,
            MsUsed = Array.Empty<IReadOnlyList<bool>>(),
            FirstStream = stream,
            SecondStream = stream,
            FirstSpectralData = null,
            SecondSpectralData = null,
            BitsConsumed = 0,
        };
    }

    // Wrapper that adapts the ref-parameter-free public surface to a delegate the Assert.Throws overload can call.
    private static void DecodeWith(
        float[] left,
        float[] right,
        AacIcsInfo ics,
        AacMsMaskPresent mask,
        IReadOnlyList<IReadOnlyList<bool>> msUsed,
        AacSectionData rightSections,
        int sampleRate)
    {
        AacMsStereoDecoder.Decode(left, right, ics, mask, msUsed, rightSections, sampleRate);
    }
}
