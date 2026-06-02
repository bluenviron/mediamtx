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

    [Fact]
    public void Constants_Have_Spec_Values()
    {
        Assert.Equal(4, AacPulseData.MaxPulses);
        Assert.Equal(63, AacPulseData.MaxStartScaleFactorBand);
        Assert.Equal(31, AacPulseData.MaxPulseOffset);
        Assert.Equal(15, AacPulseData.MaxPulseAmplitude);
    }

    [Fact]
    public void ApplyToQuantised_NullPulses_ExceptionParamName_Is_pulses()
    {
        var ex = Assert.Throws<ArgumentNullException>(() =>
        {
            Span<int> dummy = stackalloc int[32];
            AacPulseApplier.ApplyToQuantised(dummy, null!, new int[] { 0, 4 });
        });
        Assert.Equal("pulses", ex.ParamName);
    }

    [Fact]
    public void ApplyToQuantised_StartSfbOutOfRange_ExceptionParamName_Is_pulses()
    {
        var pulses = BuildPulses(startSfb: 10, (0, 1));
        var ex = Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            Span<int> quant = stackalloc int[32];
            AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 4, 8 });
        });
        Assert.Equal("pulses", ex.ParamName);
    }

    [Fact]
    public void ApplyToQuantised_PositionBeyondBuffer_ExceptionParamName_Is_pulses()
    {
        var pulses = BuildPulses(0, (31, 1));
        var ex = Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            Span<int> quant = stackalloc int[16];
            AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 4 });
        });
        Assert.Equal("pulses", ex.ParamName);
    }

    [Fact]
    public void ApplyToQuantised_EmptyLongSwbOffsets_Throws()
    {
        var pulses = BuildPulses(startSfb: 0, (1, 1));
        Assert.Throws<ArgumentOutOfRangeException>(() =>
        {
            Span<int> quant = stackalloc int[16];
            AacPulseApplier.ApplyToQuantised(quant, pulses, Array.Empty<int>());
        });
    }

    [Theory]
    [InlineData(1, 5)]
    [InlineData(100, 5)]
    [InlineData(int.MaxValue - 100, 5)]
    public void ApplyToQuantised_PositiveValues_Always_Add(int initial, int amplitude)
    {
        var pulses = BuildPulses(startSfb: 0, (2, amplitude));
        Span<int> quant = stackalloc int[16];
        quant[2] = initial;
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });
        Assert.Equal(initial + amplitude, quant[2]);
    }

    [Theory]
    [InlineData(-1, 5)]
    [InlineData(-100, 5)]
    [InlineData(int.MinValue + 100, 5)]
    public void ApplyToQuantised_NegativeValues_Always_Subtract(int initial, int amplitude)
    {
        var pulses = BuildPulses(startSfb: 0, (2, amplitude));
        Span<int> quant = stackalloc int[16];
        quant[2] = initial;
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8 });
        Assert.Equal(initial - amplitude, quant[2]);
    }

    [Fact]
    public void ApplyToQuantised_TwoPulses_OnSameCoefficient_Accumulate()
    {
        // pulses[0]=(1, 3) -> pos 1
        // pulses[1]=(0, 4) -> pos 1+0 = 1 (same position)
        var pulses = BuildPulses(startSfb: 0, (1, 3), (0, 4));
        Span<int> quant = stackalloc int[16];
        quant[1] = 10;
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 16 });
        // Both pulses see positive value, so both add.
        Assert.Equal(10 + 3 + 4, quant[1]);
    }

    [Fact]
    public void ApplyToQuantised_FirstPulse_FlipsSign_SecondSeesNewSign()
    {
        // Initial -2 -> first pulse (offset 0, amp 5) subtracts -> -7.
        // Second pulse (offset 0, amp 3) sees -7 (negative) -> subtracts -> -10.
        var pulses = BuildPulses(startSfb: 0, (0, 5), (0, 3));
        Span<int> quant = stackalloc int[16];
        quant[0] = -2;
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 16 });
        Assert.Equal(-10, quant[0]);
    }

    [Fact]
    public void ApplyToQuantised_LargeStartSfbOffset_LandsAtCorrectIndex()
    {
        // SWB offsets index 4 = 100, first pulse offset 5 -> position 105.
        var pulses = BuildPulses(startSfb: 4, (5, 7));
        Span<int> quant = new int[200];
        quant[105] = 1;
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 20, 40, 70, 100, 150 });
        Assert.Equal(8, quant[105]);
    }

    [Fact]
    public void ApplyToQuantised_BoundarySfb_LastIndex_Succeeds()
    {
        // startSfb == longSwbOffsets.Length - 1 is the last legal index;
        // the parser only rejects >= longSwbOffsets.Length.
        var pulses = BuildPulses(startSfb: 2, (0, 1));
        Span<int> quant = new int[64];
        quant[16] = 5;
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 8, 16 });
        Assert.Equal(6, quant[16]);
    }

    [Fact]
    public void ApplyToQuantised_AllFourPulses_AtMaxAmplitude_Sum()
    {
        // Four pulses each at amplitude 15, hitting four different zero
        // coefficients -> all become -15 (zero -> else branch).
        var pulses = BuildPulses(startSfb: 0,
            (1, 15), (1, 15), (1, 15), (1, 15));
        Span<int> quant = stackalloc int[32];
        AacPulseApplier.ApplyToQuantised(quant, pulses, new int[] { 0, 32 });
        Assert.Equal(-15, quant[1]);
        Assert.Equal(-15, quant[2]);
        Assert.Equal(-15, quant[3]);
        Assert.Equal(-15, quant[4]);
    }
}
