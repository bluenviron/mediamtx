using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacSynthesisFilterbankTests
{
    [Fact]
    public void Constants_HaveExpectedValues()
    {
        Assert.Equal(1024, AacSynthesisFilterbank.LongFrameLength);
    }

    [Fact]
    public void NewInstance_OverlapIsZeroAndPreviousShapeIsSine()
    {
        var fb = new AacSynthesisFilterbank();
        Assert.Equal(AacWindowShape.Sine, fb.PreviousWindowShape);
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, fb.Overlap.Length);
        for (int i = 0; i < fb.Overlap.Length; i++)
        {
            Assert.Equal(0f, fb.Overlap[i]);
        }
    }

    [Fact]
    public void ProcessLongBlock_BadCoefsLength_Throws()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[100];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];
        var ex = Assert.Throws<ArgumentException>(() =>
            fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output));
        Assert.Equal("coefs", ex.ParamName);
    }

    [Fact]
    public void ProcessLongBlock_BadOutputLength_Throws()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[100];
        var ex = Assert.Throws<ArgumentException>(() =>
            fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output));
        Assert.Equal("output", ex.ParamName);
    }

    [Fact]
    public void ProcessLongBlock_EightShortSequence_Throws()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];
        Assert.Throws<ArgumentException>(() =>
            fb.ProcessLongBlock(coefs, AacWindowSequence.EightShort, AacWindowShape.Sine, output));
    }

    [Fact]
    public void ProcessLongBlock_AllZeroCoefs_ProducesZeroOutput()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output);
        for (int i = 0; i < output.Length; i++)
        {
            Assert.Equal(0f, output[i]);
        }

        for (int i = 0; i < AacSynthesisFilterbank.LongFrameLength; i++)
        {
            Assert.Equal(0f, fb.Overlap[i]);
        }
    }

    [Fact]
    public void ProcessLongBlock_UpdatesPreviousWindowShape()
    {
        var fb = new AacSynthesisFilterbank();
        Assert.Equal(AacWindowShape.Sine, fb.PreviousWindowShape);

        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived, output);
        Assert.Equal(AacWindowShape.KaiserBesselDerived, fb.PreviousWindowShape);

        fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output);
        Assert.Equal(AacWindowShape.Sine, fb.PreviousWindowShape);
    }

    [Fact]
    public void Reset_ClearsOverlapAndResetsShape()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        coefs[0] = 100f;
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived, output);

        bool anyNonZero = false;
        for (int i = 0; i < fb.Overlap.Length; i++)
        {
            if (fb.Overlap[i] != 0f) { anyNonZero = true; break; }
        }
        Assert.True(anyNonZero, "Overlap should be non-zero after a non-trivial frame.");

        fb.Reset();
        Assert.Equal(AacWindowShape.Sine, fb.PreviousWindowShape);
        for (int i = 0; i < fb.Overlap.Length; i++)
        {
            Assert.Equal(0f, fb.Overlap[i]);
        }
    }

    [Fact]
    public void ProcessLongBlock_OnlyLong_TdacReconstructsCentralSamplesOfRamp()
    {
        // Build a length-3M ramp signal x[i] = i, split into two
        // overlapping 2M-long frames (frame 0 at 0..2M-1, frame 1
        // at M..3M-1). After OLA, the central M samples of x
        // (positions [M, 2M-1]) should be reconstructed exactly
        // (up to float roundoff) by the second frame's output.
        const int m = AacSynthesisFilterbank.LongFrameLength;
        int n = 2 * m;

        var x = new float[3 * m];
        for (int i = 0; i < x.Length; i++) x[i] = i;

        var sineFull = AacSineWindow.ComputeFull(m);
        var frame0 = new float[n];
        var frame1 = new float[n];
        for (int i = 0; i < n; i++)
        {
            frame0[i] = x[i] * sineFull[i];
            frame1[i] = x[m + i] * sineFull[i];
        }

        var coefs0 = ForwardMdct(frame0, m);
        var coefs1 = ForwardMdct(frame1, m);

        var fb = new AacSynthesisFilterbank();
        var output0 = new float[m];
        var output1 = new float[m];

        // First frame's output covers x[0..M-1] (no OLA partner -
        // the saved overlap state is used in subsequent frames).
        fb.ProcessLongBlock(coefs0, AacWindowSequence.OnlyLong,
            AacWindowShape.Sine, output0);

        // Second frame's output covers x[M..2M-1] after OLA with
        // the saved overlap from frame 0.
        fb.ProcessLongBlock(coefs1, AacWindowSequence.OnlyLong,
            AacWindowShape.Sine, output1);

        for (int i = 0; i < m; i++)
        {
            Assert.Equal(x[m + i], output1[i], 2);
        }
    }

    [Fact]
    public void ProcessLongBlock_OnlyLong_KbdTdacReconstructsCentralSamplesOfRamp()
    {
        const int m = AacSynthesisFilterbank.LongFrameLength;
        int n = 2 * m;

        var x = new float[3 * m];
        for (int i = 0; i < x.Length; i++) x[i] = i;

        var kbdFull = AacKbdWindow.ComputeFull(m, AacKbdWindow.LongAlpha);
        var frame0 = new float[n];
        var frame1 = new float[n];
        for (int i = 0; i < n; i++)
        {
            frame0[i] = x[i] * kbdFull[i];
            frame1[i] = x[m + i] * kbdFull[i];
        }

        var coefs0 = ForwardMdct(frame0, m);
        var coefs1 = ForwardMdct(frame1, m);

        var fb = new AacSynthesisFilterbank();
        var output0 = new float[m];
        var output1 = new float[m];

        fb.ProcessLongBlock(coefs0, AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived, output0);
        fb.ProcessLongBlock(coefs1, AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived, output1);

        for (int i = 0; i < m; i++)
        {
            Assert.Equal(x[m + i], output1[i], 2);
        }
    }

    [Fact]
    public void ProcessLongBlock_FirstFrameOnly_OutputIsWindowedImdctOfThatFrame()
    {
        // With the overlap buffer starting at zero, the first
        // frame's output equals the windowed IMDCT first half.
        const int m = AacSynthesisFilterbank.LongFrameLength;
        var coefs = new float[m];
        coefs[5] = 1.0f;

        var fb = new AacSynthesisFilterbank();
        var output = new float[m];
        fb.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong,
            AacWindowShape.Sine, output);

        // Recompute manually.
        var imdct = AacImdctNaive.Inverse(coefs.AsSpan());
        var window = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.OnlyLong, AacWindowShape.Sine, AacWindowShape.Sine);

        for (int i = 0; i < m; i++)
        {
            Assert.Equal(imdct[i] * window[i], output[i], 6);
        }
        for (int i = 0; i < m; i++)
        {
            Assert.Equal(imdct[m + i] * window[m + i], fb.Overlap[i], 6);
        }
    }

    [Fact]
    public void Overlap_BufferLengthMatchesConstant()
    {
        var fb = new AacSynthesisFilterbank();
        Assert.Equal(AacSynthesisFilterbank.LongFrameLength, fb.Overlap.Length);
    }

    [Fact]
    public void ProcessEightShortBlock_BadCoefsLength_Throws()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[100];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];
        var ex = Assert.Throws<ArgumentException>(() =>
            fb.ProcessEightShortBlock(coefs, AacWindowShape.Sine, output));
        Assert.Equal("coefs", ex.ParamName);
    }

    [Fact]
    public void ProcessEightShortBlock_BadOutputLength_Throws()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[100];
        var ex = Assert.Throws<ArgumentException>(() =>
            fb.ProcessEightShortBlock(coefs, AacWindowShape.Sine, output));
        Assert.Equal("output", ex.ParamName);
    }

    [Fact]
    public void ProcessEightShortBlock_AllZeroCoefs_ProducesZeroOutput()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessEightShortBlock(coefs, AacWindowShape.Sine, output);
        for (int i = 0; i < output.Length; i++)
        {
            Assert.Equal(0f, output[i]);
        }
        for (int i = 0; i < AacSynthesisFilterbank.LongFrameLength; i++)
        {
            Assert.Equal(0f, fb.Overlap[i]);
        }
    }

    [Fact]
    public void ProcessEightShortBlock_UpdatesPreviousWindowShape()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessEightShortBlock(coefs, AacWindowShape.KaiserBesselDerived, output);
        Assert.Equal(AacWindowShape.KaiserBesselDerived, fb.PreviousWindowShape);
    }

    [Fact]
    public void ProcessEightShortBlock_FirstSixteenAndLast384SamplesAreOverlapOnly()
    {
        // The eight short blocks occupy positions [448..1599] within
        // the 2048-sample windowed time buffer. The leading 448
        // samples and the trailing 448 samples are zero before OLA.
        // With a zero overlap buffer (fresh instance), the output
        // [0..447] equals 0, output [448..1023] gets short contributions,
        // and overlap [448..1023-1024] gets short contributions.
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        for (int i = 0; i < coefs.Length; i++) coefs[i] = 0.5f;
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessEightShortBlock(coefs, AacWindowShape.Sine, output);

        for (int i = 0; i < AacBlockWindow.TransitionPlateauLength; i++)
        {
            Assert.Equal(0f, output[i]);
        }
        // Overlap [LongFrame - 448 = 576 .. 1023] should be zero too
        // (corresponds to absolute frame position [1600..2047]).
        for (int i = AacSynthesisFilterbank.LongFrameLength
                     - AacBlockWindow.TransitionPlateauLength;
             i < AacSynthesisFilterbank.LongFrameLength; i++)
        {
            Assert.Equal(0f, fb.Overlap[i]);
        }
    }

    [Fact]
    public void ProcessEightShortBlock_ImpulseInFirstShort_ProducesScaledImdctAtOffset448()
    {
        // Put an impulse only in the first short block (coefs[0]=1).
        // The contribution to the windowed time buffer is at offset 448.
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        coefs[0] = 1.0f;
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessEightShortBlock(coefs, AacWindowShape.Sine, output);

        // Manually compute what the first short contributes.
        var shortCoefs = new float[AacBlockWindow.ShortHalfLength];
        shortCoefs[0] = 1.0f;
        var shortImdct = AacImdctNaive.Inverse(shortCoefs.AsSpan());
        var shortWindow = AacBlockWindow.ComposeShortWindow(
            AacWindowShape.Sine, AacWindowShape.Sine);

        int firstOffset = AacBlockWindow.TransitionPlateauLength; // 448
        for (int i = 0; i < 2 * AacBlockWindow.ShortHalfLength; i++)
        {
            int abs = firstOffset + i;
            float expected = shortImdct[i] * shortWindow[i];
            // Second short adds to abs in [576..831]; first short alone
            // dominates only [448..575]. Check just that exclusive band.
            if (abs < firstOffset + AacBlockWindow.ShortHalfLength)
            {
                // Output covers [0..1023]; overlap covers [1024..2047].
                if (abs < AacSynthesisFilterbank.LongFrameLength)
                {
                    Assert.Equal(expected, output[abs], 6);
                }
            }
        }
    }

    [Fact]
    public void ProcessLongBlock_Reset_ProducesIdenticalOutputAsFreshInstance()
    {
        const int m = AacSynthesisFilterbank.LongFrameLength;
        var coefs = new float[m];
        coefs[10] = 0.5f;
        coefs[100] = -0.25f;

        var fb1 = new AacSynthesisFilterbank();
        var fb2 = new AacSynthesisFilterbank();
        var output1 = new float[m];
        var output2 = new float[m];

        // Run fb1 once on a different input to dirty its state, then reset.
        var dirty = new float[m];
        dirty[3] = 42f;
        fb1.ProcessLongBlock(dirty, AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived, output1);
        fb1.Reset();

        fb1.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output1);
        fb2.ProcessLongBlock(coefs, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output2);

        for (int i = 0; i < m; i++)
        {
            Assert.Equal(output2[i], output1[i], 6);
        }
    }

    private static float[] ForwardMdct(ReadOnlySpan<float> timeDomain, int m)
    {
        // AAC encoder convention with the 2x scale so that the decoder
        // (2/N)x IMDCT plus windowed OLA gives unity reconstruction.
        int n = timeDomain.Length;
        Assert.Equal(2 * m, n);
        double n0 = (m + 1) / 2.0;
        double omega = 2.0 * Math.PI / n;

        var coefs = new float[m];
        for (int k = 0; k < m; k++)
        {
            double sum = 0.0;
            for (int i = 0; i < n; i++)
            {
                sum += timeDomain[i] * Math.Cos(omega * (i + n0) * (k + 0.5));
            }
            coefs[k] = (float)(2.0 * sum);
        }
        return coefs;
    }

    // ----- LongStart / LongStop transition-window coverage -----

    [Fact]
    public void ProcessLongBlock_LongStart_AllZeroCoefs_ProducesZeroOutput()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStart,
            AacWindowShape.Sine, output);

        for (int i = 0; i < output.Length; i++) Assert.Equal(0f, output[i]);
        for (int i = 0; i < fb.Overlap.Length; i++) Assert.Equal(0f, fb.Overlap[i]);
    }

    [Fact]
    public void ProcessLongBlock_LongStop_AllZeroCoefs_ProducesZeroOutput()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStop,
            AacWindowShape.Sine, output);

        for (int i = 0; i < output.Length; i++) Assert.Equal(0f, output[i]);
        for (int i = 0; i < fb.Overlap.Length; i++) Assert.Equal(0f, fb.Overlap[i]);
    }

    [Fact]
    public void ProcessLongBlock_LongStart_UpdatesPreviousWindowShape()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStart,
            AacWindowShape.KaiserBesselDerived, output);
        Assert.Equal(AacWindowShape.KaiserBesselDerived, fb.PreviousWindowShape);

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStart,
            AacWindowShape.Sine, output);
        Assert.Equal(AacWindowShape.Sine, fb.PreviousWindowShape);
    }

    [Fact]
    public void ProcessLongBlock_LongStop_UpdatesPreviousWindowShape()
    {
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStop,
            AacWindowShape.KaiserBesselDerived, output);
        Assert.Equal(AacWindowShape.KaiserBesselDerived, fb.PreviousWindowShape);

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStop,
            AacWindowShape.Sine, output);
        Assert.Equal(AacWindowShape.Sine, fb.PreviousWindowShape);
    }

    [Fact]
    public void ProcessLongBlock_LongStart_OverlapTailIs448Zeros()
    {
        // LongStart window right half is [1×448, w_short_right(0..127), 0×448].
        // After IMDCT × window, the saved overlap (= imdct[M..2M-1] × right-half-window)
        // must have its last 448 samples set to zero by the windowing.
        var fb = new AacSynthesisFilterbank();
        var coefs = new float[AacSynthesisFilterbank.LongFrameLength];
        // Drive non-trivial IMDCT output: place an impulse so the windowed result is rich.
        coefs[10] = 50f;
        coefs[200] = -30f;
        var output = new float[AacSynthesisFilterbank.LongFrameLength];

        fb.ProcessLongBlock(coefs, AacWindowSequence.LongStart,
            AacWindowShape.Sine, output);

        const int zeroTailStart = AacSynthesisFilterbank.LongFrameLength - 448;
        for (int i = zeroTailStart; i < AacSynthesisFilterbank.LongFrameLength; i++)
        {
            Assert.Equal(0f, fb.Overlap[i]);
        }

        // Sanity: the middle of the new overlap (around the short subwindow tail and the
        // 1×448 plateau region) is non-zero — proves the windowing actually ran.
        bool anyNonZero = false;
        for (int i = 0; i < zeroTailStart; i++)
        {
            if (fb.Overlap[i] != 0f) { anyNonZero = true; break; }
        }
        Assert.True(anyNonZero, "Expected LongStart overlap before the 448-zero tail to contain energy.");
    }

    [Fact]
    public void ProcessLongBlock_LongStop_OutputHeadIsPreviousOverlapHead()
    {
        // LongStop window left half is [0×448, w_short_left(0..127), 1×448].
        // The first 448 samples of the windowed current IMDCT are forced to zero, so
        // output[0..447] = imdct[0..447] × 0 + previous_overlap[0..447] = previous_overlap.
        var fb = new AacSynthesisFilterbank();

        // Frame 1: any OnlyLong frame that leaves a non-trivial overlap state.
        var coefs1 = new float[AacSynthesisFilterbank.LongFrameLength];
        coefs1[5] = 25f;
        coefs1[600] = -15f;
        var output1 = new float[AacSynthesisFilterbank.LongFrameLength];
        fb.ProcessLongBlock(coefs1, AacWindowSequence.OnlyLong, AacWindowShape.Sine, output1);

        // Snapshot the overlap after frame 1 — these are the samples LongStop should pass through.
        var savedOverlap = fb.Overlap.ToArray();

        // Frame 2: LongStop with arbitrary coefs; assert head-of-output == head-of-saved-overlap.
        var coefs2 = new float[AacSynthesisFilterbank.LongFrameLength];
        coefs2[3] = 12f;
        coefs2[800] = 7f;
        var output2 = new float[AacSynthesisFilterbank.LongFrameLength];
        fb.ProcessLongBlock(coefs2, AacWindowSequence.LongStop, AacWindowShape.Sine, output2);

        for (int i = 0; i < 448; i++)
        {
            Assert.Equal(savedOverlap[i], output2[i], 5);
        }

        // Sanity: samples after the zero head should NOT be identical to the saved overlap —
        // they pick up the current frame's contribution through the short-left ramp and plateau.
        bool diverges = false;
        for (int i = 448; i < AacSynthesisFilterbank.LongFrameLength; i++)
        {
            if (Math.Abs(output2[i] - savedOverlap[i]) > 1e-4f) { diverges = true; break; }
        }
        Assert.True(diverges, "Expected LongStop output past sample 448 to mix in the current frame.");
    }
}
