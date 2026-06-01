using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelDecoderTests
{
    private const int Sr48k = 48_000;
    private const int AacSwbOffsetsLongCount = 49; // 48k long has 49 SWBs (50 offsets)

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

    /// <summary>2-SFB long-window frame: SFB 0 = cb 1 (spectral), SFB 1 = cb 13 (PNS).</summary>
    private static AacChannelFrame BuildFrameWithPns(int globalGain, int spectralSfDiff, int noiseSf)
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write((uint)globalGain, 8);
        WriteLongIcsInfo(w, maxSfb: 2);

        w.Write(1u, 4);
        w.Write(1u, 5);
        w.Write(13u, 4);
        w.Write(1u, 5);

        var (sfCode, sfLen) = EncodeSfDiff(spectralSfDiff);
        w.Write(sfCode, sfLen);

        int diff = noiseSf - (globalGain - AacAbsoluteScaleFactors.NoiseOffset);
        int raw = diff + 256;
        Assert.InRange(raw, 0, 511);
        w.Write((uint)raw, 9);

        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);

        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        return frame!;
    }

    /// <summary>1-SFB long-window frame: SFB 0 = cb 1 (spectral); no PNS bands.</summary>
    private static AacChannelFrame BuildFrameNoPns()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(100u, 8);
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
        return frame!;
    }

    /// <summary>10-SFB long-window frame with order-2 TNS, no PNS, all-1 spectral coefs.</summary>
    private static AacChannelFrame BuildFrameWithTns(int order, int coef)
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        const int sfbs = 10;
        // TNS filter length must cover down to the spectral range or it
        // clamps to a no-op. Use the full long-window SWB count so the
        // filter actually overlaps [0, swb[sfbs]) once mmm clamps top.
        const int tnsLength = AacSwbOffsetsLongCount;
        var w = new AacBitWriter();
        w.Write(100u, 8);
        WriteLongIcsInfo(w, maxSfb: sfbs);

        w.Write(1u, 4);
        w.Write((uint)sfbs, 5);

        for (int i = 0; i < sfbs; i++)
        {
            var (sfCode, sfLen) = EncodeSfDiff(0);
            w.Write(sfCode, sfLen);
        }

        w.Write(0u, 1);
        w.Write(1u, 1);
        w.Write(1u, 2);
        w.Write(0u, 1);
        w.Write((uint)tnsLength, 6);
        w.Write((uint)order, 5);
        if (order > 0)
        {
            w.Write(0u, 1);
            w.Write(0u, 1);
            for (int i = 0; i < order; i++) w.Write((uint)coef, 3);
        }
        w.Write(0u, 1);

        int swb48k0 = AacSwbOffsets.GetLongOffsets(Sr48k)[sfbs];
        int tuples = swb48k0 / 4;
        for (int i = 0; i < tuples; i++) w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        return frame!;
    }

    [Fact]
    public void DecodeMono_NullFrame_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMono(null!, Sr48k, new AacPnsRandom()));
    }

    [Fact]
    public void DecodeMono_NullPrng_Throws()
    {
        var frame = BuildFrameNoPns();
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMono(frame, Sr48k, null!));
    }

    [Fact]
    public void DecodeMono_BadSampleRate_Throws()
    {
        var frame = BuildFrameNoPns();
        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodeMono(frame, 192_000, new AacPnsRandom()));
    }

    [Fact]
    public void DecodeMono_NoPnsBands_ProducesDequantizedSpectrum()
    {
        var frame = BuildFrameNoPns();

        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        var decoded = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 7u));

        Assert.Equal(AacDecodedSpectrum.TransformLength, decoded.Coefficients.Length);
        Assert.Equal(AacWindowSequence.OnlyLong, decoded.WindowSequence);
        Assert.Equal(dq.Coefficients.ToArray(), decoded.Coefficients.ToArray());
    }

    [Fact]
    public void DecodeMono_FillsPnsBandToTargetEnergy()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 40);
        var decoded = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 7u));

        Assert.Equal(AacWindowSequence.OnlyLong, decoded.WindowSequence);
        double energy = 0;
        for (int i = 4; i < 8; i++)
        {
            energy += (double)decoded.Coefficients[i] * decoded.Coefficients[i];
        }
        Assert.Equal(
            AacPnsNoiseGenerator.TargetBandEnergy(40),
            energy,
            AacPnsNoiseGenerator.TargetBandEnergy(40) * 1e-5);
    }

    [Fact]
    public void DecodeMono_LeavesNonPnsBandUnchanged()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 20);

        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        var decoded = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 7u));

        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(dq.Coefficients[i], decoded.Coefficients[i]);
        }
    }

    [Fact]
    public void DecodeMono_SamePrngSeedYieldsIdenticalSpectrum()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30);

        var a = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 42u));
        var b = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 42u));

        Assert.Equal(a.Coefficients.ToArray(), b.Coefficients.ToArray());
    }

    [Fact]
    public void DecodeMono_DifferentPrngSeedsYieldDifferentPnsBand()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30);

        var a = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 1u));
        var b = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 2u));

        bool anyDifferent = false;
        for (int i = 4; i < 8; i++)
        {
            if (a.Coefficients[i] != b.Coefficients[i])
            {
                anyDifferent = true;
                break;
            }
        }
        Assert.True(anyDifferent);
    }

    [Fact]
    public void DecodeMono_DoesNotMutatePreviouslyReturnedSpectrum()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30);

        var first = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 7u));
        var snapshot = first.Coefficients.ToArray();

        _ = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 8u));

        Assert.Equal(snapshot, first.Coefficients.ToArray());
    }

    [Fact]
    public void DecodeMono_OutputIsTheLengthOfTransform()
    {
        var frame = BuildFrameNoPns();
        var decoded = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom());

        Assert.Equal(1024, decoded.Coefficients.Length);
        Assert.Equal(AacSpectralData.TransformLength, decoded.Coefficients.Length);
    }

    [Fact]
    public void DecodeMono_RoundTripsViaApplyMatchesPnsApplierApply()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30);

        var viaComposer = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 5u));

        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        var viaApplier = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom(seed: 5u));

        Assert.Equal(viaApplier.Coefficients.ToArray(), viaComposer.Coefficients.ToArray());
    }

    // --- DecodeMono with AOT (Dequantize + PNS + long-window TNS) ---

    [Fact]
    public void DecodeMono_Aot_NullFrame_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMono(null!, Sr48k, new AacPnsRandom(), AacAudioObjectType.AacLc));
    }

    [Fact]
    public void DecodeMono_Aot_NullPrng_Throws()
    {
        var frame = BuildFrameNoPns();
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMono(frame, Sr48k, null!, AacAudioObjectType.AacLc));
    }

    [Fact]
    public void DecodeMono_Aot_BadSampleRate_Throws()
    {
        var frame = BuildFrameNoPns();
        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodeMono(
                frame, 192_000, new AacPnsRandom(), AacAudioObjectType.AacLc));
    }

    [Fact]
    public void DecodeMono_Aot_NoTnsData_MatchesNonAotOverload()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30);

        var withoutAot = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u));
        var withAot = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u), AacAudioObjectType.AacLc);

        Assert.Equal(withoutAot.Coefficients.ToArray(), withAot.Coefficients.ToArray());
        Assert.Equal(withoutAot.WindowSequence, withAot.WindowSequence);
    }

    [Fact]
    public void DecodeMono_Aot_TnsOrderZero_MatchesNonAotOverload()
    {
        var frame = BuildFrameWithTns(order: 0, coef: 0);

        var withoutAot = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u));
        var withAot = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u), AacAudioObjectType.AacLc);

        Assert.Equal(withoutAot.Coefficients.ToArray(), withAot.Coefficients.ToArray());
    }

    [Fact]
    public void DecodeMono_Aot_TnsAppliedDiffersFromNonAotOverload()
    {
        var frame = BuildFrameWithTns(order: 2, coef: 3);

        var withoutAot = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u));
        var withAot = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u), AacAudioObjectType.AacLc);

        Assert.NotEqual(withoutAot.Coefficients.ToArray(), withAot.Coefficients.ToArray());
        Assert.Equal(AacWindowSequence.OnlyLong, withAot.WindowSequence);
    }

    [Fact]
    public void DecodeMono_Aot_TnsMatchesManualPipeline()
    {
        var frame = BuildFrameWithTns(order: 2, coef: 3);

        var viaComposer = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(seed: 11u), AacAudioObjectType.AacLc);

        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        var buf = dq.Coefficients.ToArray();
        AacPnsApplier.ApplyInPlace(buf, frame, Sr48k, new AacPnsRandom(seed: 11u));

        var ics = frame.Stream.IcsInfo;
        var swb = AacSwbOffsets.GetLongOffsets(Sr48k);
        int sfIdx = AacSampleRates.ToIndex(Sr48k);
        int maxSfb = AacTnsSpecLimits.GetMaxBands(
            AacAudioObjectType.AacLc, sfIdx, ics.WindowSequence);
        int maxOrder = AacTnsSpecLimits.GetMaxOrder(
            AacAudioObjectType.AacLc, ics.WindowSequence);
        if (maxSfb > swb.Length - 1) maxSfb = swb.Length - 1;
        AacTnsSpectrumApplier.Apply(
            frame.Stream.TnsData!, ics, buf, swb, maxSfb, maxOrder);

        Assert.Equal(buf, viaComposer.Coefficients.ToArray());
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacSsr)]
    [InlineData(AacAudioObjectType.Sbr)]
    [InlineData(AacAudioObjectType.AacScalable)]
    public void DecodeMono_Aot_UnsupportedAot_TnsActive_Throws(AacAudioObjectType aot)
    {
        var frame = BuildFrameWithTns(order: 2, coef: 3);
        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(), aot));
    }

    [Fact]
    public void BuildFrameWithTns_SanityCheck_FrameHasTnsData()
    {
        var frame = BuildFrameWithTns(order: 2, coef: 3);
        Assert.True(frame.Stream.TnsDataPresent);
        Assert.NotNull(frame.Stream.TnsData);
        Assert.Single(frame.Stream.TnsData!.Windows);
        Assert.Single(frame.Stream.TnsData.Windows[0].Filters);
        Assert.Equal(2, frame.Stream.TnsData.Windows[0].Filters[0].Order);
        Assert.Equal(3, frame.Stream.TnsData.Windows[0].Filters[0].Coefficients[0]);
        Assert.Equal(3, frame.Stream.TnsData.Windows[0].Filters[0].Coefficients[1]);
    }

    [Fact]
    public void DecodeMono_Aot_UnsupportedAot_NoTnsData_DoesNotThrow()
    {
        var frame = BuildFrameNoPns();
        var decoded = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom(), AacAudioObjectType.AacSsr);
        Assert.Equal(1024, decoded.Coefficients.Length);
    }

    // ---------- DecodePair tests ----------

    private static AacChannelPairElement BuildCpeFromTwoFrames(
        AacChannelFrame leftFrame,
        AacChannelFrame rightFrame,
        bool commonWindow,
        AacMsMaskPresent msMaskPresent,
        IReadOnlyList<IReadOnlyList<bool>>? msUsed = null)
    {
        return new AacChannelPairElement
        {
            ElementInstanceTag = 0,
            CommonWindow = commonWindow,
            SharedIcsInfo = commonWindow ? leftFrame.Stream.IcsInfo : null,
            MsMaskPresent = msMaskPresent,
            MsUsed = msUsed ?? Array.Empty<IReadOnlyList<bool>>(),
            FirstStream = leftFrame.Stream,
            SecondStream = rightFrame.Stream,
            FirstSpectralData = leftFrame.SpectralData,
            SecondSpectralData = rightFrame.SpectralData,
            BitsConsumed = 0,
        };
    }

    [Fact]
    public void DecodePair_NullCpe_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodePair(
                null!, Sr48k, new AacPnsRandom(), new AacPnsRandom()));
    }

    [Fact]
    public void DecodePair_NullLeftPrng_Throws()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None);
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodePair(cpe, Sr48k, null!, new AacPnsRandom()));
    }

    [Fact]
    public void DecodePair_NullRightPrng_Throws()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None);
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodePair(cpe, Sr48k, new AacPnsRandom(), null!));
    }

    [Fact]
    public void DecodePair_MissingFirstSpectralData_Throws()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None) with { FirstSpectralData = null };
        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodePair(cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom()));
    }

    [Fact]
    public void DecodePair_MissingSecondSpectralData_Throws()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None) with { SecondSpectralData = null };
        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodePair(cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom()));
    }

    [Fact]
    public void DecodePair_BadSampleRate_Throws()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None);
        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodePair(cpe, sampleRate: 12345,
                new AacPnsRandom(), new AacPnsRandom()));
    }

    [Fact]
    public void DecodePair_ReturnsTransformLengthCoefficients()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None);
        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());
        Assert.Equal(1024, left.Coefficients.Length);
        Assert.Equal(1024, right.Coefficients.Length);
    }

    [Fact]
    public void DecodePair_NoCommonWindow_DecodesEachChannelIndependently()
    {
        var leftFrame = BuildFrameNoPns();
        var rightFrame = BuildFrameNoPns();
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: false, AacMsMaskPresent.None);

        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());

        var monoLeft = AacChannelDecoder.DecodeMono(leftFrame, Sr48k, new AacPnsRandom());
        var monoRight = AacChannelDecoder.DecodeMono(rightFrame, Sr48k, new AacPnsRandom());
        Assert.Equal(monoLeft.Coefficients.ToArray(), left.Coefficients.ToArray());
        Assert.Equal(monoRight.Coefficients.ToArray(), right.Coefficients.ToArray());
    }

    [Fact]
    public void DecodePair_CommonWindowMsNone_MatchesMonoPipeline()
    {
        var leftFrame = BuildFrameNoPns();
        var rightFrame = BuildFrameNoPns();
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: true, AacMsMaskPresent.None);

        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());

        var monoLeft = AacChannelDecoder.DecodeMono(leftFrame, Sr48k, new AacPnsRandom());
        var monoRight = AacChannelDecoder.DecodeMono(rightFrame, Sr48k, new AacPnsRandom());
        Assert.Equal(monoLeft.Coefficients.ToArray(), left.Coefficients.ToArray());
        Assert.Equal(monoRight.Coefficients.ToArray(), right.Coefficients.ToArray());
    }

    [Fact]
    public void DecodePair_MsAllBands_AppliesSumAndDifference()
    {
        var leftFrame = BuildFrameNoPns();
        var rightFrame = BuildFrameNoPns();
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: true, AacMsMaskPresent.AllBands);

        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());

        // BuildFrameNoPns yields (1, 1, 1, 1, 0, ..., 0) after dequant.
        // MS all-bands on band 0 gives L = 1+1 = 2, R = 1-1 = 0.
        Assert.Equal(2f, left.Coefficients[0]);
        Assert.Equal(2f, left.Coefficients[1]);
        Assert.Equal(2f, left.Coefficients[2]);
        Assert.Equal(2f, left.Coefficients[3]);
        Assert.Equal(0f, right.Coefficients[0]);
        Assert.Equal(0f, right.Coefficients[1]);
        Assert.Equal(0f, right.Coefficients[2]);
        Assert.Equal(0f, right.Coefficients[3]);
    }

    [Fact]
    public void DecodePair_WindowSequenceFromSharedIcs()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None);
        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());
        Assert.Equal(AacWindowSequence.OnlyLong, left.WindowSequence);
        Assert.Equal(AacWindowSequence.OnlyLong, right.WindowSequence);
    }

    [Fact]
    public void DecodePair_DeterministicWithSamePrngSeeds()
    {
        var cpe1 = BuildCpeFromTwoFrames(
            BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30),
            BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30),
            commonWindow: true, AacMsMaskPresent.None);
        var cpe2 = BuildCpeFromTwoFrames(
            BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30),
            BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30),
            commonWindow: true, AacMsMaskPresent.None);

        var (l1, r1) = AacChannelDecoder.DecodePair(
            cpe1, Sr48k, new AacPnsRandom(seed: 123u), new AacPnsRandom(seed: 456u));
        var (l2, r2) = AacChannelDecoder.DecodePair(
            cpe2, Sr48k, new AacPnsRandom(seed: 123u), new AacPnsRandom(seed: 456u));

        Assert.Equal(l1.Coefficients.ToArray(), l2.Coefficients.ToArray());
        Assert.Equal(r1.Coefficients.ToArray(), r2.Coefficients.ToArray());
    }

    [Fact]
    public void DecodePair_LeftAndRightUseIndependentPrngs()
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30),
            BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30),
            commonWindow: true, AacMsMaskPresent.None);

        // Different PRNG seeds for left/right should yield different
        // noise patterns even with identical frame content.
        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(seed: 1u), new AacPnsRandom(seed: 2u));

        Assert.NotEqual(left.Coefficients.ToArray(), right.Coefficients.ToArray());
    }
}
