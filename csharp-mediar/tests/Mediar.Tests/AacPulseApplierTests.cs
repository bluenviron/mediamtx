using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacPulseApplierTests
{
    private static AacPulseData BuildPulses(int startSfb, params (int Offset, int Amplitude)[] pulses)
    {
        Assert.InRange(pulses.Length, 1, AacPulseData.MaxPulses);

        var w = new AacBitWriter();
        w.Write((uint)(pulses.Length - 1), 2);
        w.Write((uint)startSfb, 6);
        foreach (var p in pulses)
        {
            w.Write((uint)p.Offset, 5);
            w.Write((uint)p.Amplitude, 4);
        }

        Assert.True(AacPulseData.TryParse(w.ToArray(), out var data));
        return data!;
    }

    [Fact]
    public void ApplyToQuantised_NullPulses_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
        {
            Span<int> dummy = stackalloc int[32];
            AacPulseApplier.ApplyToQuantised(dummy, null!, new int[] { 0, 4 });
        });
    }

    [Fact]
    public void ApplyToQuantised_StartSfbOutOfRange_Throws()
    {
        var pulses = BuildPulses(5, (0, 1));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            Span<int> quant = stackalloc int[32];
            AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 4, 8 });
        });
    }

    [Fact]
    public void ApplyToQuantised_PositionBeyondBuffer_Throws()
    {
        var pulses = BuildPulses(0, (31, 1));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            Span<int> quant = stackalloc int[16];
            AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 4 });
        });
    }

    [Fact]
    public void ApplyToQuantised_PositivePulse_AddsAmplitude()
    {
        var pulses = BuildPulses(startSfb: 0, (3, 5));
        Span<int> quant = stackalloc int[16];
        quant[3] = 7;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });

        Assert.Equal(12, quant[3]); // 7 + 5
    }

    [Fact]
    public void ApplyToQuantised_NegativePulse_SubtractsAmplitude()
    {
        var pulses = BuildPulses(startSfb: 0, (3, 5));
        Span<int> quant = stackalloc int[16];
        quant[3] = -7;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });

        Assert.Equal(-12, quant[3]); // -7 - 5
    }

    [Fact]
    public void ApplyToQuantised_ZeroBucket_GoesNegative()
    {
        var pulses = BuildPulses(startSfb: 0, (3, 5));
        Span<int> quant = stackalloc int[16];
        // quant[3] = 0 (default)

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });

        // Per spec: if (q > 0) +=, else -=. Zero falls to else.
        Assert.Equal(-5, quant[3]);
    }

    [Fact]
    public void ApplyToQuantised_PositionIsStartOffsetPlusPulseOffset()
    {
        // SWB offset[2] = 16, pulse_offset = 7 -> position = 23.
        var pulses = BuildPulses(startSfb: 2, (7, 3));
        Span<int> quant = stackalloc int[32];
        quant[23] = 1;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8, 16, 24 });

        Assert.Equal(4, quant[23]);
        Assert.Equal(0, quant[16]);
        Assert.Equal(0, quant[22]);
    }

    [Fact]
    public void ApplyToQuantised_FourPulses_AccumulatePositions()
    {
        // Positions: 0+1=1, +2=3, +3=6, +4=10
        var pulses = BuildPulses(startSfb: 0,
            (1, 2),
            (2, 3),
            (3, 4),
            (4, 5));
        Span<int> quant = stackalloc int[32];
        quant[1] = 10;
        quant[3] = -10;
        // quant[6] = 0
        quant[10] = 10;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 16 });

        Assert.Equal(12, quant[1]);   // 10 + 2
        Assert.Equal(-13, quant[3]);  // -10 - 3
        Assert.Equal(-4, quant[6]);   // 0 - 4 (zero falls to else)
        Assert.Equal(15, quant[10]);  // 10 + 5
    }

    [Fact]
    public void ApplyToQuantised_LeavesUnrelatedCoefficientsUntouched()
    {
        var pulses = BuildPulses(startSfb: 0, (3, 5));
        Span<int> quant = stackalloc int[16];
        for (int i = 0; i < quant.Length; i++) quant[i] = 100;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });

        Assert.Equal(105, quant[3]);
        for (int i = 0; i < quant.Length; i++)
        {
            if (i == 3) continue;
            Assert.Equal(100, quant[i]);
        }
    }

    [Fact]
    public void ApplyToQuantised_ZeroAmplitudePulse_LeavesValueUnchanged()
    {
        var pulses = BuildPulses(startSfb: 0, (3, 0));
        Span<int> quant = stackalloc int[16];
        quant[3] = 42;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });

        Assert.Equal(42, quant[3]);
    }

    [Fact]
    public void ApplyToQuantised_MaxAmplitude_DoesNotOverflowInt()
    {
        var pulses = BuildPulses(startSfb: 0, (3, AacPulseData.MaxPulseAmplitude));
        Span<int> quant = stackalloc int[16];
        quant[3] = 100;

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });

        Assert.Equal(100 + AacPulseData.MaxPulseAmplitude, quant[3]);
    }

    [Fact]
    public void ApplyToQuantised_PositionExactlyAtBufferEnd_Throws()
    {
        // SWB[1] = 16, pulse_offset = 0 -> position = 16, buffer length 16 → invalid.
        var pulses = BuildPulses(startSfb: 1, (0, 1));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            Span<int> quant = stackalloc int[16];
            AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 16, 32 });
        });
    }

    [Fact]
    public void ApplyToQuantised_SignFlipsCorrectlyAcrossPulses()
    {
        var pulses = BuildPulses(startSfb: 0, (1, 3), (2, 4));
        Span<int> quant = stackalloc int[16];
        quant[1] = 1;   // positive -> add
        quant[3] = -1;  // negative -> subtract

        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 16 });

        Assert.Equal(4, quant[1]);
        Assert.Equal(-5, quant[3]);
    }
}
