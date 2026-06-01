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

    // ---------- Pulse-data tests ----------

    /// <summary>
    /// 1-SFB long-window frame: SFB 0 = cb 1 (spectral), with pulse_data
    /// carrying a single pulse at position 0 with the given amplitude.
    /// All four spectral coefs decode to 1 from cb 1 sym 80 before pulse
    /// apply.
    /// </summary>
    private static AacChannelFrame BuildFrameWithPulse(int pulseAmplitude)
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

        // pulse_data_present = 1
        w.Write(1u, 1);
        // pulse_data: numberPulse=0 (1 pulse), startSfb=0, offset=0, amplitude=N
        w.Write(0u, 2);
        w.Write(0u, 6);
        w.Write(0u, 5);
        w.Write((uint)pulseAmplitude, 4);

        // tns_data_present = 0
        w.Write(0u, 1);
        // gain_control_data_present = 0
        w.Write(0u, 1);

        // spectral_data: 1 tuple of cb 1 (4-tuple) = sym 80
        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        return frame!;
    }

    [Fact]
    public void BuildFrameWithPulse_SanityCheck_FrameHasPulseData()
    {
        var frame = BuildFrameWithPulse(pulseAmplitude: 5);
        Assert.True(frame.Stream.PulseDataPresent);
        Assert.NotNull(frame.Stream.PulseData);
        Assert.Equal(0, frame.Stream.PulseData!.StartScaleFactorBand);
        Assert.Single(frame.Stream.PulseData.Pulses);
        Assert.Equal(0, frame.Stream.PulseData.Pulses[0].Offset);
        Assert.Equal(5, frame.Stream.PulseData.Pulses[0].Amplitude);
    }

    [Fact]
    public void DecodeMono_PulseDataPresent_AppliesPulseBeforeDequant()
    {
        var pulseFrame = BuildFrameWithPulse(pulseAmplitude: 5);
        var noPulseFrame = BuildFrameNoPns();

        var withPulse = AacChannelDecoder.DecodeMono(
            pulseFrame, Sr48k, new AacPnsRandom());
        var withoutPulse = AacChannelDecoder.DecodeMono(
            noPulseFrame, Sr48k, new AacPnsRandom());

        // Position 0 was 1 (integer); after pulse it becomes 6.
        // Dequant: 6^(4/3) ≈ 10.903, vs 1^(4/3) = 1. So position 0 should
        // differ significantly between the two outputs.
        Assert.NotEqual(withoutPulse.Coefficients[0], withPulse.Coefficients[0]);
        // Pulse only modifies position 0 here, so positions 1..3 still
        // decode to the same value (the synthetic spectrum is (1,1,1,1)
        // so positions 1..3 remain at 1 in both runs).
        Assert.Equal(withoutPulse.Coefficients[1], withPulse.Coefficients[1]);
        Assert.Equal(withoutPulse.Coefficients[2], withPulse.Coefficients[2]);
        Assert.Equal(withoutPulse.Coefficients[3], withPulse.Coefficients[3]);
    }

    [Fact]
    public void DecodeMono_PulseDataPresent_AmplitudeFiveYieldsKnownValue()
    {
        var frame = BuildFrameWithPulse(pulseAmplitude: 5);
        var decoded = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom());

        // Position 0 was quantised int=1, after pulse +5 = 6; dequant: 6^(4/3).
        float expected = MathF.Pow(6f, 4f / 3f);
        Assert.Equal(expected, decoded.Coefficients[0], precision: 4);
    }

    [Fact]
    public void DecodeMono_PulseDataAbsent_NoModification()
    {
        var frame = BuildFrameNoPns();
        var decoded = AacChannelDecoder.DecodeMono(
            frame, Sr48k, new AacPnsRandom());

        // No pulse, original quantised int=1, dequant: 1^(4/3) = 1.
        Assert.Equal(1f, decoded.Coefficients[0], precision: 4);
    }

    [Fact]
    public void DecodePair_PulseDataOnFirstChannelOnly_AppliesOnFirstOnly()
    {
        var leftFrame = BuildFrameWithPulse(pulseAmplitude: 5);
        var rightFrame = BuildFrameNoPns();
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: false, AacMsMaskPresent.None);

        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());

        float expectedLeftPos0 = MathF.Pow(6f, 4f / 3f);
        Assert.Equal(expectedLeftPos0, left.Coefficients[0], precision: 4);
        Assert.Equal(1f, right.Coefficients[0], precision: 4);
    }

    [Fact]
    public void DecodeMono_PulseDataInputFrameNotMutated()
    {
        var frame = BuildFrameWithPulse(pulseAmplitude: 5);
        int beforeCoef0 = frame.SpectralData.Coefficients[0];

        AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom());

        int afterCoef0 = frame.SpectralData.Coefficients[0];
        // Caller-supplied frame's spectral data must not be mutated by
        // the composer; pulse apply happens on a copy.
        Assert.Equal(beforeCoef0, afterCoef0);
    }

    // ---------- DecodePair AOT overload tests ----------

    [Fact]
    public void DecodePair_Aot_NoTnsData_MatchesNonAotOverload()
    {
        var leftFrame = BuildFrameNoPns();
        var rightFrame = BuildFrameNoPns();
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: true, AacMsMaskPresent.None);

        var (l1, r1) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());
        var (l2, r2) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom(),
            AacAudioObjectType.AacLc);

        Assert.Equal(l1.Coefficients.ToArray(), l2.Coefficients.ToArray());
        Assert.Equal(r1.Coefficients.ToArray(), r2.Coefficients.ToArray());
    }

    [Fact]
    public void DecodePair_Aot_TnsAppliedDiffersFromNonAotOverload()
    {
        var leftFrame = BuildFrameWithTns(order: 2, coef: 3);
        var rightFrame = BuildFrameWithTns(order: 2, coef: 3);
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: false, AacMsMaskPresent.None);

        var (l1, _) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom());
        var (l2, _) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom(),
            AacAudioObjectType.AacLc);

        Assert.NotEqual(l1.Coefficients.ToArray(), l2.Coefficients.ToArray());
    }

    [Fact]
    public void DecodePair_Aot_BothChannelsHaveTns_BothFiltered()
    {
        var leftFrame = BuildFrameWithTns(order: 2, coef: 3);
        var rightFrame = BuildFrameWithTns(order: 2, coef: 3);
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: false, AacMsMaskPresent.None);

        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom(),
            AacAudioObjectType.AacLc);
        var leftOnlyMono = AacChannelDecoder.DecodeMono(
            leftFrame, Sr48k, new AacPnsRandom(), AacAudioObjectType.AacLc);

        // Both channels should match the mono pipeline output since
        // common_window = false skips MS / IS.
        Assert.Equal(leftOnlyMono.Coefficients.ToArray(), left.Coefficients.ToArray());
        Assert.Equal(leftOnlyMono.Coefficients.ToArray(), right.Coefficients.ToArray());
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacSsr)]
    [InlineData(AacAudioObjectType.Sbr)]
    [InlineData(AacAudioObjectType.AacScalable)]
    public void DecodePair_Aot_UnsupportedAot_TnsActive_Throws(AacAudioObjectType aot)
    {
        var leftFrame = BuildFrameWithTns(order: 2, coef: 3);
        var rightFrame = BuildFrameNoPns();
        var cpe = BuildCpeFromTwoFrames(
            leftFrame, rightFrame, commonWindow: false, AacMsMaskPresent.None);

        Assert.Throws<ArgumentOutOfRangeException>(() =>
            AacChannelDecoder.DecodePair(
                cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom(), aot));
    }

    [Theory]
    [InlineData(AacAudioObjectType.AacSsr)]
    [InlineData(AacAudioObjectType.Sbr)]
    [InlineData(AacAudioObjectType.AacScalable)]
    public void DecodePair_Aot_UnsupportedAot_NoTnsData_DoesNotThrow(AacAudioObjectType aot)
    {
        var cpe = BuildCpeFromTwoFrames(
            BuildFrameNoPns(), BuildFrameNoPns(),
            commonWindow: true, AacMsMaskPresent.None);
        var (left, right) = AacChannelDecoder.DecodePair(
            cpe, Sr48k, new AacPnsRandom(), new AacPnsRandom(), aot);
        Assert.Equal(1024, left.Coefficients.Length);
        Assert.Equal(1024, right.Coefficients.Length);
    }

    // ---------- DecodeCce tests ----------

    /// <summary>
    /// 1-target SCE CCE with a 1-SFB cb 1 spectral body (4-tuple = sym 80)
    /// and no PNS / TNS / pulses. Auxiliary coupling channel only.
    /// </summary>
    private static AacCouplingChannelElement BuildCceCb1NoPns()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(4u, 4);              // element_instance_tag
        w.Write(0u, 1);              // ind_sw_cce_flag = 0
        w.Write(0u, 3);              // num_coupled_elements = 0 -> 1 target
        w.Write(0u, 1); w.Write(2u, 4);   // SCE target, tag 2
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);   // cc_domain / sign / scale
        // ICS body: cb=1, 1 SFB
        w.Write(100u, 8);            // global_gain
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);  // section cb=1, len=1
        var (sfCode, sfLen) = EncodeSfDiff(0);
        w.Write(sfCode, sfLen);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1); // pulse/tns/gain flags
        // Spectral: sym 80 -> (1,1,1,1)
        w.Write(80u, 7);

        Assert.True(AacCouplingChannelElement.TryParse(
            w.ToArray(), sfBook, sampleRate: Sr48k, spectralBooks, out var cce));
        return cce!;
    }

    /// <summary>
    /// CCE that carries pulse_data on its auxiliary channel (1-SFB cb 1).
    /// Single pulse at position 0 with amplitude 5.
    /// </summary>
    private static AacCouplingChannelElement BuildCceWithPulse()
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(4u, 4);              // element_instance_tag
        w.Write(0u, 1);              // ind_sw_cce_flag
        w.Write(0u, 3);              // num_coupled_elements = 0
        w.Write(0u, 1); w.Write(2u, 4);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        // ICS body: cb=1, 1 SFB, with pulse data
        w.Write(100u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        var (sfCode, sfLen) = EncodeSfDiff(0);
        w.Write(sfCode, sfLen);
        // pulse_data_present = 1
        w.Write(1u, 1);
        w.Write(0u, 2);              // numberPulse = 0 (1 pulse)
        w.Write(0u, 6);              // pulse_start_sfb = 0
        w.Write(0u, 5);              // pulse_offset = 0
        w.Write(5u, 4);              // pulse_amplitude = 5
        w.Write(0u, 1); w.Write(0u, 1); // tns/gain flags
        // Spectral
        w.Write(80u, 7);

        Assert.True(AacCouplingChannelElement.TryParse(
            w.ToArray(), sfBook, sampleRate: Sr48k, spectralBooks, out var cce));
        return cce!;
    }

    [Fact]
    public void DecodeCce_NullCce_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeCce(null!, Sr48k, new AacPnsRandom()));
    }

    [Fact]
    public void DecodeCce_NullPrng_Throws()
    {
        var cce = BuildCceCb1NoPns();
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeCce(cce, Sr48k, null!));
    }

    [Fact]
    public void DecodeCce_BoundaryParsedCce_MissingSpectralData_Throws()
    {
        // Boundary-stopping overload yields SpectralData == null.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(4u, 4);
        w.Write(0u, 1);
        w.Write(0u, 3);
        w.Write(0u, 1); w.Write(2u, 4);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        w.Write(0x80u, 8);
        WriteLongIcsInfo(w, maxSfb: 10);
        w.Write(0u, 4); w.Write(10u, 5);  // zero section across maxSfb
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.Null(cce!.SpectralData);

        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodeCce(cce, Sr48k, new AacPnsRandom()));
    }

    [Fact]
    public void DecodeCce_NoPnsNoPulse_DequantizesSpectrum()
    {
        var cce = BuildCceCb1NoPns();
        var decoded = AacChannelDecoder.DecodeCce(cce, Sr48k, new AacPnsRandom());

        // 1^(4/3) = 1, position 0..3 from cb 1 sym 80.
        Assert.Equal(1f, decoded.Coefficients[0], precision: 4);
        Assert.Equal(1f, decoded.Coefficients[1], precision: 4);
        Assert.Equal(1f, decoded.Coefficients[2], precision: 4);
        Assert.Equal(1f, decoded.Coefficients[3], precision: 4);
        Assert.Equal(AacWindowSequence.OnlyLong, decoded.WindowSequence);
    }

    [Fact]
    public void DecodeCce_PulseData_AppliedBeforeDequant()
    {
        var cce = BuildCceWithPulse();
        var decoded = AacChannelDecoder.DecodeCce(cce, Sr48k, new AacPnsRandom());

        // Pos 0: quantised 1+5=6, dequant 6^(4/3) ≈ 10.903.
        float expected = MathF.Pow(6f, 4f / 3f);
        Assert.Equal(expected, decoded.Coefficients[0], precision: 4);
        Assert.Equal(1f, decoded.Coefficients[1], precision: 4);
    }

    [Fact]
    public void DecodeCce_ProducesSameResultAsDecodeMono()
    {
        // CCE wrapping the same Stream+SpectralData should decode to
        // exactly the same spectrum as DecodeMono on a synthetic
        // AacChannelFrame holding identical data.
        var cce = BuildCceCb1NoPns();
        var frame = new AacChannelFrame
        {
            Stream = cce.Stream,
            SpectralData = cce.SpectralData!,
            BitsConsumed = 0,
        };

        var fromCce = AacChannelDecoder.DecodeCce(cce, Sr48k, new AacPnsRandom(seed: 42u));
        var fromMono = AacChannelDecoder.DecodeMono(frame, Sr48k, new AacPnsRandom(seed: 42u));

        Assert.Equal(fromMono.Coefficients.ToArray(), fromCce.Coefficients.ToArray());
    }

    [Fact]
    public void DecodeCce_Aot_LcWithoutTns_MatchesNonAotOverload()
    {
        var cce = BuildCceCb1NoPns();
        var withoutAot = AacChannelDecoder.DecodeCce(cce, Sr48k, new AacPnsRandom(seed: 7u));
        var withAot = AacChannelDecoder.DecodeCce(
            cce, Sr48k, new AacPnsRandom(seed: 7u), AacAudioObjectType.AacLc);

        Assert.Equal(withoutAot.Coefficients.ToArray(), withAot.Coefficients.ToArray());
    }

    [Fact]
    public void DecodeCce_Aot_NullCce_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeCce(null!, Sr48k, new AacPnsRandom(), AacAudioObjectType.AacLc));
    }

    [Fact]
    public void DecodeCce_Aot_NullPrng_Throws()
    {
        var cce = BuildCceCb1NoPns();
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeCce(cce, Sr48k, null!, AacAudioObjectType.AacLc));
    }

    // ---------- DecodeMonoToSamples tests ----------

    [Fact]
    public void DecodeMonoToSamples_NullFilterbank_Throws()
    {
        var frame = BuildFrameNoPns();
        var output = new float[AacSynthesisFilterbank.LongFrameLength];
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMonoToSamples(
                frame, Sr48k, new AacPnsRandom(), null!, output));
    }

    [Fact]
    public void DecodeMonoToSamples_NullFrame_Throws()
    {
        var fb = new AacSynthesisFilterbank();
        var output = new float[AacSynthesisFilterbank.LongFrameLength];
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMonoToSamples(
                null!, Sr48k, new AacPnsRandom(), fb, output));
    }

    [Fact]
    public void DecodeMonoToSamples_WrongOutputLength_Throws()
    {
        var frame = BuildFrameNoPns();
        var fb = new AacSynthesisFilterbank();
        var output = new float[512];
        Assert.Throws<ArgumentException>(() =>
            AacChannelDecoder.DecodeMonoToSamples(
                frame, Sr48k, new AacPnsRandom(), fb, output));
    }

    [Fact]
    public void DecodeMonoToSamples_OnlyLongFrame_ProducesExpectedLength()
    {
        var frame = BuildFrameNoPns();
        var fb = new AacSynthesisFilterbank();
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(), fb, output);

        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, output.Length);
        // First frame starts from a zero overlap buffer; output values
        // should be finite even if quiet.
        foreach (var s in output) Assert.False(float.IsNaN(s) || float.IsInfinity(s));
    }

    [Fact]
    public void DecodeMonoToSamples_AdvancesFilterbankOverlap()
    {
        var frame = BuildFrameNoPns();
        var fb = new AacSynthesisFilterbank();
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        // Capture overlap before/after a single frame. With a non-zero
        // input spectrum the overlap buffer must be modified.
        bool overlapWasZero = true;
        foreach (var v in fb.Overlap) if (v != 0f) { overlapWasZero = false; break; }
        Assert.True(overlapWasZero);

        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(), fb, output);

        bool overlapIsZero = true;
        foreach (var v in fb.Overlap) if (v != 0f) { overlapIsZero = false; break; }
        Assert.False(overlapIsZero);
    }

    [Fact]
    public void DecodeMonoToSamples_Aot_NullFilterbank_Throws()
    {
        var frame = BuildFrameNoPns();
        var output = new float[AacSynthesisFilterbank.LongFrameLength];
        Assert.Throws<ArgumentNullException>(() =>
            AacChannelDecoder.DecodeMonoToSamples(
                frame, Sr48k, new AacPnsRandom(), AacAudioObjectType.AacLc, null!, output));
    }

    [Fact]
    public void DecodeMonoToSamples_Aot_LcWithoutTns_MatchesNonAotOverload()
    {
        var frame = BuildFrameNoPns();
        var fb1 = new AacSynthesisFilterbank();
        var fb2 = new AacSynthesisFilterbank();
        var out1 = new float[AacSynthesisFilterbank.LongFrameLength];
        var out2 = new float[AacSynthesisFilterbank.LongFrameLength];

        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(seed: 9u), fb1, out1);
        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(seed: 9u),
            AacAudioObjectType.AacLc, fb2, out2);

        Assert.Equal(out1, out2);
    }

    [Fact]
    public void DecodeMonoToSamples_TwoFrames_OverlapAddCarries()
    {
        var frame = BuildFrameNoPns();
        var fb = new AacSynthesisFilterbank();
        var out1 = new float[AacSynthesisFilterbank.LongFrameLength];
        var out2 = new float[AacSynthesisFilterbank.LongFrameLength];

        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(), fb, out1);
        AacChannelDecoder.DecodeMonoToSamples(
            frame, Sr48k, new AacPnsRandom(), fb, out2);

        // The second frame includes the previous frame's IMDCT tail
        // through the overlap buffer. With identical input spectra,
        // the first half of out2 carries a non-zero tail
        // contribution that out1 did not (out1 was the first frame
        // and the overlap was all-zero).
        bool out1HalfAllZero = true;
        bool out2HalfAllZero = true;
        for (int i = 0; i < 512; i++)
        {
            if (out1[i] != 0f) out1HalfAllZero = false;
            if (out2[i] != 0f) out2HalfAllZero = false;
        }
        // At least one of the outputs must contain non-zero samples
        // (the spectrum decodes to (1,1,1,1) plus zeros).
        Assert.False(out1HalfAllZero && out2HalfAllZero);
    }
}
