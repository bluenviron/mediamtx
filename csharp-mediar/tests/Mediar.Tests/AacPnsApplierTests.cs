using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacPnsApplierTests
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
    /// Build a 2-SFB long-window frame: SFB 0 = cb 1 (spectral), SFB 1 = cb 13 (PNS).
    /// </summary>
    private static AacChannelFrame BuildFrameWithPns(int globalGain, int spectralSfDiff, int noiseSf)
    {
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write((uint)globalGain, 8);
        WriteLongIcsInfo(w, maxSfb: 2);

        // Section 1: cb=1, len=1 (covers SFB 0)
        w.Write(1u, 4);
        w.Write(1u, 5);
        // Section 2: cb=13 (PNS), len=1 (covers SFB 1)
        w.Write(13u, 4);
        w.Write(1u, 5);

        // SF stream:
        //   SFB 0: ordinary SF diff
        var (sfCode, sfLen) = EncodeSfDiff(spectralSfDiff);
        w.Write(sfCode, sfLen);
        //   SFB 1: PNS first band = 9-bit unsigned PCM. raw - 256 = noiseSf - (globalGain - 90).
        //   Per AacAbsoluteScaleFactors.FromDelta: noise_acc = globalGain - 90 + Differential.
        //   Differential = (raw - 256), so for absolute noiseSf:
        //     raw = noiseSf - (globalGain - 90) + 256
        int diff = noiseSf - (globalGain - AacAbsoluteScaleFactors.NoiseOffset);
        int raw = diff + 256;
        Assert.InRange(raw, 0, 511);
        w.Write((uint)raw, 9);

        // pulse/tns/gain flags
        w.Write(0u, 1);
        w.Write(0u, 1);
        w.Write(0u, 1);

        // spectral_data: only SFB 0 emits (1 quad → symbol 80 → (1,1,1,1)).
        // SFB 1 is cb=13 (PNS), emits no spectral bits.
        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), sharedIcsInfo: null, scaleFlag: false, sfBook,
            Sr48k, spectralBooks, out var frame));
        return frame!;
    }

    private static double EnergyOf(ReadOnlySpan<float> band)
    {
        double e = 0;
        foreach (var v in band)
        {
            e += (double)v * v;
        }
        return e;
    }

    [Fact]
    public void ApplyInPlace_NullFrame_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
        {
            float[] spec = new float[1024];
            AacPnsApplier.ApplyInPlace(spec.AsSpan(), null!, Sr48k, new AacPnsRandom());
        });
    }

    [Fact]
    public void ApplyInPlace_NullPrng_Throws()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 20);
        Assert.Throws<ArgumentNullException>(() =>
        {
            float[] spec = new float[1024];
            AacPnsApplier.ApplyInPlace(spec.AsSpan(), frame, Sr48k, null!);
        });
    }

    [Fact]
    public void ApplyInPlace_WrongLength_Throws()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 20);
        Assert.Throws<ArgumentException>(() =>
        {
            float[] spec = new float[100];
            AacPnsApplier.ApplyInPlace(spec.AsSpan(), frame, Sr48k, new AacPnsRandom());
        });
    }

    [Fact]
    public void ApplyInPlace_BadSampleRate_Throws()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 20);
        Assert.Throws<ArgumentException>(() =>
        {
            float[] spec = new float[1024];
            AacPnsApplier.ApplyInPlace(spec.AsSpan(), frame, 192_000, new AacPnsRandom());
        });
    }

    [Fact]
    public void Apply_FillsPnsBandToTargetEnergy()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 40);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);

        // Before apply: PNS band (SFB 1 -> coefs 4..7 at 48k) is all zero.
        for (int i = 4; i < 8; i++)
        {
            Assert.Equal(0f, dq.Coefficients[i]);
        }

        var result = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom(seed: 7u));

        double energy = 0;
        for (int i = 4; i < 8; i++)
        {
            energy += (double)result.Coefficients[i] * result.Coefficients[i];
        }
        Assert.Equal(AacPnsNoiseGenerator.TargetBandEnergy(40), energy, AacPnsNoiseGenerator.TargetBandEnergy(40) * 1e-5);
    }

    [Fact]
    public void Apply_LeavesNonPnsBandUnchanged()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 20);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);

        // Capture spectral band 0 (coefs 0..3).
        float[] beforeBand0 = new float[4];
        for (int i = 0; i < 4; i++) beforeBand0[i] = dq.Coefficients[i];

        var result = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom());
        for (int i = 0; i < 4; i++)
        {
            Assert.Equal(beforeBand0[i], result.Coefficients[i]);
        }
    }

    [Fact]
    public void Apply_DoesNotMutateInput()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 20);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);

        var snapshot = dq.Coefficients.ToArray();
        _ = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom());

        for (int i = 0; i < snapshot.Length; i++)
        {
            Assert.Equal(snapshot[i], dq.Coefficients[i]);
        }
    }

    [Fact]
    public void Apply_DeterministicForSameSeed()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 30);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);

        var a = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom(seed: 99u));
        var b = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom(seed: 99u));

        for (int i = 0; i < a.Coefficients.Length; i++)
        {
            Assert.Equal(a.Coefficients[i], b.Coefficients[i]);
        }
    }

    [Fact]
    public void ApplyInPlace_AdvancesPrngOncePerCoefficient()
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: 25);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);

        var prng = new AacPnsRandom(seed: 1u);
        uint stateBefore = prng.State;

        float[] copy = dq.Coefficients.ToArray();
        AacPnsApplier.ApplyInPlace(copy.AsSpan(), frame, Sr48k, prng);

        // PNS band has 4 coefs (SWB 1 at 48k).
        var ref2 = new AacPnsRandom(seed: 1u);
        for (int i = 0; i < 4; i++) ref2.NextFloat();
        Assert.Equal(ref2.State, prng.State);
    }

    [Fact]
    public void ApplyInPlace_NoPnsSections_LeavesSpectrumIntact()
    {
        // Frame with one cb=1 SFB only and no PNS.
        var sfBook = BuildSyntheticSfCodebook();
        var spectralBook = BuildFixed7BitCodebook(81);
        var spectralBooks = CodebooksWith(1, spectralBook);

        var w = new AacBitWriter();
        w.Write(100u, 8);
        WriteLongIcsInfo(w, maxSfb: 1);
        w.Write(1u, 4); w.Write(1u, 5);
        var (sfCode, sfLen) = EncodeSfDiff(0);
        w.Write(sfCode, sfLen);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        w.Write(80u, 7);

        Assert.True(AacChannelFrame.TryParse(
            w.ToArray(), null, false, sfBook, Sr48k, spectralBooks, out var frame));

        var dq = AacDequantizedSpectrum.FromFrame(frame!, Sr48k);
        var snapshot = dq.Coefficients.ToArray();

        var prng = new AacPnsRandom(seed: 42u);
        var result = AacPnsApplier.Apply(dq, frame!, Sr48k, prng);

        for (int i = 0; i < snapshot.Length; i++)
        {
            Assert.Equal(snapshot[i], result.Coefficients[i]);
        }
        Assert.Equal(42u, prng.State);
    }

    [Theory]
    [InlineData(10)]
    [InlineData(40)]
    [InlineData(80)]
    [InlineData(120)]
    public void Apply_VariousNoiseSf_HitsExpectedEnergy(int sf)
    {
        var frame = BuildFrameWithPns(globalGain: 100, spectralSfDiff: 0, noiseSf: sf);
        var dq = AacDequantizedSpectrum.FromFrame(frame, Sr48k);
        var result = AacPnsApplier.Apply(dq, frame, Sr48k, new AacPnsRandom(seed: 3u));

        double energy = 0;
        for (int i = 4; i < 8; i++)
        {
            energy += (double)result.Coefficients[i] * result.Coefficients[i];
        }
        double target = AacPnsNoiseGenerator.TargetBandEnergy(sf);
        Assert.InRange(energy, target * (1 - 1e-4), target * (1 + 1e-4));
    }
}
