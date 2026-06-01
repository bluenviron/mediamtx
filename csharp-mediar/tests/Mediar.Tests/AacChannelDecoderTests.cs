using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacChannelDecoderTests
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
}
