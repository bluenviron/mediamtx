using System.Collections.Immutable;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacTnsSpectrumApplierTests
{
    // Synthetic long-window SWB table: 4 SFBs, closing at 1024.
    private static readonly int[] LongSwb = [0, 128, 256, 512, 1024];

    // Synthetic short-window SWB table: 4 SFBs, closing at 128.
    private static readonly int[] ShortSwb = [0, 16, 32, 64, 128];

    private static AacIcsInfo LongIcs(int maxSfb = 4) => new()
    {
        WindowSequence = AacWindowSequence.OnlyLong,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = null,
        WindowGroupCount = 1,
        WindowsPerGroup = new byte[] { 1 },
        PredictorDataPresent = false,
    };

    private static AacIcsInfo ShortIcs(int maxSfb = 4) => new()
    {
        WindowSequence = AacWindowSequence.EightShort,
        WindowShape = AacWindowShape.Sine,
        MaxSfb = maxSfb,
        ScaleFactorGrouping = (byte)0b0111_1111,
        WindowGroupCount = 8,
        WindowsPerGroup = new byte[] { 1, 1, 1, 1, 1, 1, 1, 1 },
        PredictorDataPresent = false,
    };

    /// <summary>
    /// Build a 4-bit-coefficient TNS filter from signed intent values
    /// (-8..7); negative inputs are translated to their two's-complement
    /// 4-bit raw form so they round-trip through
    /// <see cref="AacTnsInverseQuant"/>.
    /// </summary>
    private static AacTnsFilter Filter(int length, int order, bool direction, params int[] signedCoefs)
    {
        var raw = ImmutableArray.CreateBuilder<int>(signedCoefs.Length);
        foreach (var s in signedCoefs)
        {
            if (s < -8 || s > 7)
            {
                throw new ArgumentOutOfRangeException(nameof(signedCoefs),
                    $"{s} is outside the 4-bit signed range [-8, 7].");
            }
            raw.Add(s < 0 ? s + 16 : s);
        }
        return new AacTnsFilter
        {
            Length = length,
            Order = order,
            Direction = direction,
            CoefCompress = false,
            CoefBits = 4,
            Coefficients = raw.MoveToImmutable(),
        };
    }

    private static AacTnsData LongTns(params AacTnsFilter[] filters) => new()
    {
        WindowSequence = AacWindowSequence.OnlyLong,
        BitsConsumed = 0,
        Windows = ImmutableArray.Create(new AacTnsWindow
        {
            CoefResHigh = true,
            Filters = filters.ToImmutableArray(),
        }),
    };

    private static AacTnsData ShortTns(params AacTnsFilter[][] perWindowFilters)
    {
        var windows = ImmutableArray.CreateBuilder<AacTnsWindow>(8);
        for (int w = 0; w < 8; w++)
        {
            var fs = w < perWindowFilters.Length
                ? perWindowFilters[w]
                : Array.Empty<AacTnsFilter>();
            windows.Add(new AacTnsWindow
            {
                CoefResHigh = true,
                Filters = fs.ToImmutableArray(),
            });
        }
        return new AacTnsData
        {
            WindowSequence = AacWindowSequence.EightShort,
            BitsConsumed = 0,
            Windows = windows.MoveToImmutable(),
        };
    }

    private static float[] FillRamp(int length, float start = 0.001f, float step = 0.0005f)
    {
        var data = new float[length];
        for (int i = 0; i < length; i++) data[i] = start + i * step;
        return data;
    }

    /// <summary>
    /// Independent reference: inverse-quant + stepUp + inverse-filter
    /// on a single contiguous slice, mirroring exactly what the
    /// applier should do per filter.
    /// </summary>
    private static void ApplyOneFilterReference(
        Span<float> windowSpectrum,
        AacTnsFilter filter,
        bool coefResHigh,
        int start,
        int end,
        int effectiveOrder)
    {
        if (effectiveOrder == 0 || end <= start) return;
        Span<float> parcorFull = stackalloc float[filter.Order];
        AacTnsInverseQuant.Compute(filter, coefResHigh, parcorFull);
        Span<float> lpc = stackalloc float[effectiveOrder];
        AacTnsLpcStepUp.Compute(parcorFull[..effectiveOrder], lpc);
        AacTnsInverseFilter.Apply(
            windowSpectrum.Slice(start, end - start),
            lpc,
            filter.Direction);
    }

    // ----- argument validation -----

    [Fact]
    public void Apply_NullTnsData_Throws()
    {
        Span<float> spec = stackalloc float[1024];
        Assert.Throws<ArgumentNullException>("tnsData", () =>
        {
            var s = new float[1024];
            AacTnsSpectrumApplier.Apply(null!, LongIcs(), s, LongSwb, 4, 12);
        });
    }

    [Fact]
    public void Apply_NullIcsInfo_Throws()
    {
        Assert.Throws<ArgumentNullException>("icsInfo", () =>
        {
            var s = new float[1024];
            AacTnsSpectrumApplier.Apply(LongTns(), null!, s, LongSwb, 4, 12);
        });
    }

    [Fact]
    public void Apply_WindowSequenceMismatch_Throws()
    {
        var tns = LongTns();
        var ics = ShortIcs();
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(tns, ics, s, ShortSwb, 4, 12));
        Assert.Equal("tnsData", ex.ParamName);
    }

    [Fact]
    public void Apply_WrongWindowCount_Throws()
    {
        // Construct a TNS marked as OnlyLong but carrying 0 windows.
        var bad = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            BitsConsumed = 0,
            Windows = ImmutableArray<AacTnsWindow>.Empty,
        };
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(bad, LongIcs(), s, LongSwb, 4, 12));
        Assert.Equal("tnsData", ex.ParamName);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(512)]
    [InlineData(1023)]
    [InlineData(1025)]
    [InlineData(2048)]
    public void Apply_WrongSpectrumLength_Throws(int len)
    {
        var s = new float[len];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(), LongIcs(), s, LongSwb, 4, 12));
        Assert.Equal("spectrum", ex.ParamName);
    }

    [Fact]
    public void Apply_TooFewSwbOffsets_Throws()
    {
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(), LongIcs(), s, new int[] { 1024 }, 0, 12));
        Assert.Equal("swbOffsets", ex.ParamName);
    }

    [Fact]
    public void Apply_LongSwbClosingMismatch_Throws()
    {
        var s = new float[1024];
        var badSwb = new int[] { 0, 256, 512 }; // closes at 512, not 1024.
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(), LongIcs(maxSfb: 2), s, badSwb, 2, 12));
        Assert.Equal("swbOffsets", ex.ParamName);
    }

    [Fact]
    public void Apply_ShortSwbClosingMismatch_Throws()
    {
        var s = new float[1024];
        var badSwb = new int[] { 0, 32, 64 }; // closes at 64, not 128.
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(ShortTns(), ShortIcs(maxSfb: 2), s, badSwb, 2, 7));
        Assert.Equal("swbOffsets", ex.ParamName);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(5)]
    public void Apply_MaxSfbOutOfRange_Throws(int maxSfb)
    {
        var s = new float[1024];
        // numSwb = 4 for LongSwb; valid maxSfb is [0, 4].
        var ics = new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = maxSfb,
            ScaleFactorGrouping = null,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 },
            PredictorDataPresent = false,
        };
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(), ics, s, LongSwb, 4, 12));
        Assert.Equal("icsInfo", ex.ParamName);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(5)]
    public void Apply_TnsMaxSfbOutOfRange_Throws(int tnsMaxSfb)
    {
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(), LongIcs(), s, LongSwb, tnsMaxSfb, 12));
        Assert.Equal("tnsMaxSfb", ex.ParamName);
    }

    [Theory]
    [InlineData(-1)]
    [InlineData(32)] // AacTnsLpcStepUp.MaxOrder == 31.
    public void Apply_TnsMaxOrderOutOfRange_Throws(int tnsMaxOrder)
    {
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(), LongIcs(), s, LongSwb, 4, tnsMaxOrder));
        Assert.Equal("tnsMaxOrder", ex.ParamName);
    }

    // ----- no-op cases -----

    [Fact]
    public void Apply_NoFilters_Noop()
    {
        var s = FillRamp(1024);
        var copy = s.ToArray();
        AacTnsSpectrumApplier.Apply(LongTns(), LongIcs(), s, LongSwb, 4, 12);
        Assert.Equal(copy, s);
    }

    [Fact]
    public void Apply_ZeroOrderFilter_Noop()
    {
        var s = FillRamp(1024);
        var copy = s.ToArray();
        var f = new AacTnsFilter
        {
            Length = 2,
            Order = 0,
            Direction = false,
            CoefCompress = false,
            CoefBits = 4,
            Coefficients = ImmutableArray<int>.Empty,
        };
        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, 4, 12);
        Assert.Equal(copy, s);
    }

    [Fact]
    public void Apply_TnsMaxOrderZero_FilterClampedAway_Noop()
    {
        var s = FillRamp(1024);
        var copy = s.ToArray();
        var f = Filter(length: 2, order: 4, direction: false, 3, -2, 1, 0);
        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, 4, tnsMaxOrder: 0);
        Assert.Equal(copy, s);
    }

    [Fact]
    public void Apply_EmptyShortWindowFilters_LeavesAllSpectrumUntouched()
    {
        var s = FillRamp(1024);
        var copy = s.ToArray();
        AacTnsSpectrumApplier.Apply(ShortTns(), ShortIcs(), s, ShortSwb, 4, 7);
        Assert.Equal(copy, s);
    }

    // ----- happy-path application -----

    [Fact]
    public void Apply_LongSingleFilter_AffectsExpectedBandOnly()
    {
        // length=2 → bottom=4-2=2, top=4
        // start = swbOffsets[min(2, 4)] = 256
        // end   = swbOffsets[min(4, 4)] = 1024
        // filter range: [256, 1024) of the long window.
        var f = Filter(length: 2, order: 2, direction: false, 3, -2);

        var s = FillRamp(1024);
        var expected = (float[])s.Clone();
        ApplyOneFilterReference(expected.AsSpan(), f, coefResHigh: true,
            start: 256, end: 1024, effectiveOrder: 2);

        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, 4, 12);

        Assert.Equal(expected.Length, s.Length);
        for (int i = 0; i < expected.Length; i++)
        {
            Assert.Equal(expected[i], s[i], precision: 6);
        }
    }

    [Fact]
    public void Apply_LongSingleFilter_BelowBand_Untouched()
    {
        // length=2 → start=256, end=1024. Lines [0..256) must be exactly the input.
        var f = Filter(length: 2, order: 2, direction: false, 3, -2);
        var s = FillRamp(1024);
        var copy = (float[])s.Clone();
        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, 4, 12);
        for (int i = 0; i < 256; i++) Assert.Equal(copy[i], s[i]);
    }

    [Fact]
    public void Apply_LongFilterReverseDirection_MatchesReference()
    {
        var f = Filter(length: 2, order: 2, direction: true, 5, -3);

        var s = FillRamp(1024);
        var expected = (float[])s.Clone();
        ApplyOneFilterReference(expected.AsSpan(), f, coefResHigh: true,
            start: 256, end: 1024, effectiveOrder: 2);

        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, 4, 12);
        for (int i = 0; i < 1024; i++) Assert.Equal(expected[i], s[i], precision: 6);
    }

    [Fact]
    public void Apply_LongTwoFilters_TileBandsCorrectly()
    {
        // numSwb = 4
        // Filter 0: length=1, order=1, coef=2
        //   top=4, bottom=3 → start=swb[3]=512, end=swb[4]=1024
        // Filter 1: length=2, order=1, coef=-1
        //   top=3 (previous bottom), bottom=1 → start=swb[1]=128, end=swb[3]=512
        var f0 = Filter(length: 1, order: 1, direction: false, 2);
        var f1 = Filter(length: 2, order: 1, direction: false, 7);

        var s = FillRamp(1024);
        var expected = (float[])s.Clone();
        // Order matters - applier walks f0 first, then f1. Since the
        // bands are disjoint the order between bands doesn't matter,
        // but inside each band the mutation is local.
        ApplyOneFilterReference(expected.AsSpan(), f0, coefResHigh: true,
            start: 512, end: 1024, effectiveOrder: 1);
        ApplyOneFilterReference(expected.AsSpan(), f1, coefResHigh: true,
            start: 128, end: 512, effectiveOrder: 1);

        AacTnsSpectrumApplier.Apply(LongTns(f0, f1), LongIcs(), s, LongSwb, 4, 12);
        for (int i = 0; i < 1024; i++) Assert.Equal(expected[i], s[i], precision: 6);
        // Lines below SFB 1 must be untouched.
        for (int i = 0; i < 128; i++) Assert.Equal(FillRamp(1024)[i], s[i]);
    }

    [Fact]
    public void Apply_LongFilter_ClampedByTnsMaxSfb()
    {
        // tnsMaxSfb = 3 → end = swb[min(top=4, 3)] = swb[3] = 512.
        // start = swb[min(bottom=2, max_sfb=4)] = swb[2] = 256.
        // Effective range: [256, 512).
        var f = Filter(length: 2, order: 2, direction: false, 4, -1);

        var s = FillRamp(1024);
        var expected = (float[])s.Clone();
        ApplyOneFilterReference(expected.AsSpan(), f, coefResHigh: true,
            start: 256, end: 512, effectiveOrder: 2);

        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, tnsMaxSfb: 3, tnsMaxOrder: 12);
        for (int i = 0; i < 1024; i++) Assert.Equal(expected[i], s[i], precision: 6);
        // Above tnsMaxSfb must be untouched.
        var input = FillRamp(1024);
        for (int i = 512; i < 1024; i++) Assert.Equal(input[i], s[i]);
    }

    [Fact]
    public void Apply_LongFilter_ClampedByMaxSfb()
    {
        // ics max_sfb = 2 → start = swb[min(bottom=2, 2)] = swb[2] = 256.
        // tnsMaxSfb = 4 → end = swb[min(top=4, 4)] = swb[4] = 1024.
        // Range: [256, 1024). Above max_sfb the spectrum *would* be
        // zero in a real decode, but the applier still walks it; the
        // test below just checks the slice matches the reference.
        var f = Filter(length: 2, order: 2, direction: false, 4, -1);

        var s = FillRamp(1024);
        var expected = (float[])s.Clone();
        ApplyOneFilterReference(expected.AsSpan(), f, coefResHigh: true,
            start: 256, end: 1024, effectiveOrder: 2);

        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(maxSfb: 2), s, LongSwb, 4, 12);
        for (int i = 0; i < 1024; i++) Assert.Equal(expected[i], s[i], precision: 6);
    }

    [Fact]
    public void Apply_EmptyRangeFilter_NoMutation()
    {
        // length=4 → bottom=max(4-4,0)=0, top=4.
        // start = swb[min(0, max_sfb=0)] = swb[0] = 0.
        // end   = swb[min(4, tnsMaxSfb=0)] = swb[0] = 0.
        // Range [0,0) → skip.
        var f = Filter(length: 4, order: 2, direction: false, 3, -2);

        var s = FillRamp(1024);
        var copy = (float[])s.Clone();
        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(maxSfb: 0), s, LongSwb, tnsMaxSfb: 0, tnsMaxOrder: 12);
        Assert.Equal(copy, s);
    }

    [Fact]
    public void Apply_OrderExceedsTnsMaxOrder_UsesEffectivePrefix()
    {
        // Filter declares order 4 but tnsMaxOrder caps at 2; the
        // applier should inverse-quant all 4 coefficients but only
        // step up the first 2 into the LPC filter.
        var f = Filter(length: 2, order: 4, direction: false, 5, -3, 2, -1);

        var s = FillRamp(1024);
        var expected = (float[])s.Clone();
        ApplyOneFilterReference(expected.AsSpan(), f, coefResHigh: true,
            start: 256, end: 1024, effectiveOrder: 2);

        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), s, LongSwb, 4, tnsMaxOrder: 2);
        for (int i = 0; i < 1024; i++) Assert.Equal(expected[i], s[i], precision: 6);
    }

    [Fact]
    public void Apply_ShortEightWindows_AppliedIndependentlyPerPhysicalWindow()
    {
        // One TNS filter on each physical window. Filter[w] uses
        // length=1, order=1, direction=false, coef=2 (same shape every
        // window). With ShortSwb (4 SFBs), each filter covers SFB 3
        // (line range [64..128) within its window).
        var filtersPerWindow = new AacTnsFilter[8][];
        for (int w = 0; w < 8; w++)
        {
            filtersPerWindow[w] =
                new[] { Filter(length: 1, order: 1, direction: false, 2) };
        }
        var tns = ShortTns(filtersPerWindow);
        var ics = ShortIcs();

        // Per-window-layout spectrum: 8 contiguous 128-line blocks.
        // Use a different ramp in each window so we can detect cross
        // contamination.
        var s = new float[1024];
        for (int w = 0; w < 8; w++)
        {
            for (int i = 0; i < 128; i++)
            {
                s[w * 128 + i] = (float)(w + 1) * 0.01f + i * 0.001f;
            }
        }
        var expected = (float[])s.Clone();
        for (int w = 0; w < 8; w++)
        {
            ApplyOneFilterReference(
                expected.AsSpan(w * 128, 128),
                filtersPerWindow[w][0],
                coefResHigh: true,
                start: 64,
                end: 128,
                effectiveOrder: 1);
        }

        AacTnsSpectrumApplier.Apply(tns, ics, s, ShortSwb, tnsMaxSfb: 4, tnsMaxOrder: 7);

        for (int i = 0; i < 1024; i++)
        {
            Assert.Equal(expected[i], s[i], precision: 6);
        }
    }

    [Fact]
    public void Apply_ShortOneWindowOnly_OtherWindowsUntouched()
    {
        // Single filter on window 3 only; windows 0,1,2,4..7 must be
        // exactly the input.
        var filtersPerWindow = new AacTnsFilter[8][];
        for (int w = 0; w < 8; w++) filtersPerWindow[w] = Array.Empty<AacTnsFilter>();
        filtersPerWindow[3] = new[] { Filter(length: 2, order: 1, direction: false, 4) };

        var tns = ShortTns(filtersPerWindow);
        var ics = ShortIcs();

        var s = FillRamp(1024);
        var input = (float[])s.Clone();
        AacTnsSpectrumApplier.Apply(tns, ics, s, ShortSwb, 4, 7);

        for (int w = 0; w < 8; w++)
        {
            if (w == 3) continue;
            for (int i = 0; i < 128; i++)
            {
                int idx = w * 128 + i;
                Assert.Equal(input[idx], s[idx]);
            }
        }
    }

    // ----- round-trip stability with strong PARCORs -----

    [Fact]
    public void Apply_RoundTripForwardFirThenInverseIir_RecoversInput()
    {
        // Compose a forward FIR with the spec-paired PLUS convention
        // (y[n] = x[n] + Σ a[k] x[n-k]) and verify the applier's MINUS
        // IIR inverts it back to the original. Use mid-range PARCORs
        // (raw=4 at coefBits=4 → ≈0.74) which exercise the LPC step-up
        // and IIR composition while staying well clear of the
        // numerical edge near |PARCOR|=1.
        var f = Filter(length: 4, order: 2, direction: false, 4, 4);

        // Build the LPC the applier will see.
        Span<float> parcor = stackalloc float[2];
        AacTnsInverseQuant.Compute(f, coefResHigh: true, parcor);
        Span<float> lpc = stackalloc float[2];
        AacTnsLpcStepUp.Compute(parcor, lpc);

        // Pick a pseudo-random input and run forward FIR (encoder).
        var rng = new Random(0xC0FFEE);
        var window = new float[1024];
        for (int i = 0; i < window.Length; i++) window[i] = (float)rng.NextDouble() * 0.1f;
        var input = (float[])window.Clone();

        // Forward FIR (PLUS) on the same band the applier will cover:
        // length=4 → bottom=0, top=4. start=swb[0]=0, end=swb[4]=1024.
        Span<float> past = stackalloc float[2];
        past.Clear();
        for (int n = 0; n < 1024; n++)
        {
            float y = input[n];
            for (int k = 0; k < 2; k++)
            {
                y += lpc[k] * past[k];
            }
            window[n] = y;
            // shift past
            past[1] = past[0];
            past[0] = input[n]; // forward FIR uses INPUTS as state.
        }

        // Now invert via the applier.
        AacTnsSpectrumApplier.Apply(LongTns(f), LongIcs(), window, LongSwb, 4, 12);

        for (int i = 0; i < 1024; i++)
        {
            Assert.Equal(input[i], window[i], precision: 4);
        }
    }

    [Fact]
    public void Apply_NegativeFilterLength_Throws()
    {
        var bad = new AacTnsFilter
        {
            Length = -1,
            Order = 1,
            Direction = false,
            CoefCompress = false,
            CoefBits = 4,
            Coefficients = ImmutableArray.Create(2),
        };
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(bad), LongIcs(), s, LongSwb, 4, 12));
        Assert.Equal("tnsData", ex.ParamName);
    }

    [Fact]
    public void Apply_NegativeFilterOrder_Throws()
    {
        var bad = new AacTnsFilter
        {
            Length = 2,
            Order = -1,
            Direction = false,
            CoefCompress = false,
            CoefBits = 4,
            Coefficients = ImmutableArray<int>.Empty,
        };
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(bad), LongIcs(), s, LongSwb, 4, 12));
        Assert.Equal("tnsData", ex.ParamName);
    }

    [Fact]
    public void Apply_CoefficientsLengthMismatchesOrder_Throws()
    {
        var bad = new AacTnsFilter
        {
            Length = 2,
            Order = 3,
            Direction = false,
            CoefCompress = false,
            CoefBits = 4,
            Coefficients = ImmutableArray.Create(1, 2), // only 2, order says 3.
        };
        var s = new float[1024];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacTnsSpectrumApplier.Apply(LongTns(bad), LongIcs(), s, LongSwb, 4, 12));
        Assert.Equal("tnsData", ex.ParamName);
    }
}
