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
}
