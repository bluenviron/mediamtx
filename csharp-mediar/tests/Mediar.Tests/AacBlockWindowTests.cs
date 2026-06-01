using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacBlockWindowTests
{
    [Fact]
    public void Constants_HaveExpectedValues()
    {
        Assert.Equal(2048, AacBlockWindow.LongBlockLength);
        Assert.Equal(1024, AacBlockWindow.LongHalfLength);
        Assert.Equal(128, AacBlockWindow.ShortHalfLength);
        Assert.Equal(448, AacBlockWindow.TransitionPlateauLength);
        Assert.Equal(2048, 2 * AacBlockWindow.LongHalfLength);
        Assert.Equal(
            AacBlockWindow.LongHalfLength,
            2 * AacBlockWindow.TransitionPlateauLength + AacBlockWindow.ShortHalfLength);
    }

    [Fact]
    public void ComposeLongBlock_OnlyLongSineSine_MatchesSineWindow()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.OnlyLong, AacWindowShape.Sine, AacWindowShape.Sine);
        var sine = AacSineWindow.ComputeFull(AacBlockWindow.LongHalfLength);
        Assert.Equal(sine.Length, win.Length);
        for (int i = 0; i < win.Length; i++)
        {
            Assert.Equal(sine[i], win[i], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_OnlyLongKbdKbd_MatchesKbdWindow()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived,
            AacWindowShape.KaiserBesselDerived);
        var kbd = AacKbdWindow.ComputeFull(AacBlockWindow.LongHalfLength, AacKbdWindow.LongAlpha);
        Assert.Equal(kbd.Length, win.Length);
        for (int i = 0; i < win.Length; i++)
        {
            Assert.Equal(kbd[i], win[i], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_OnlyLongSineKbd_LeftIsSineRightIsKbd()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.OnlyLong,
            AacWindowShape.Sine,
            AacWindowShape.KaiserBesselDerived);

        var sineRising = AacSineWindow.ComputeRisingHalf(AacBlockWindow.LongHalfLength);
        var kbdRising = AacKbdWindow.ComputeRisingHalf(
            AacBlockWindow.LongHalfLength, AacKbdWindow.LongAlpha);

        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            Assert.Equal(sineRising[n], win[n], 6);
        }
        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            // Right half: falling KBD = reverse(rising).
            float expected = kbdRising[AacBlockWindow.LongHalfLength - 1 - n];
            Assert.Equal(expected, win[AacBlockWindow.LongHalfLength + n], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_OnlyLongKbdSine_LeftIsKbdRightIsSine()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.OnlyLong,
            AacWindowShape.KaiserBesselDerived,
            AacWindowShape.Sine);

        var kbdRising = AacKbdWindow.ComputeRisingHalf(
            AacBlockWindow.LongHalfLength, AacKbdWindow.LongAlpha);
        var sineRising = AacSineWindow.ComputeRisingHalf(AacBlockWindow.LongHalfLength);

        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            Assert.Equal(kbdRising[n], win[n], 6);
        }
        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            float expected = sineRising[AacBlockWindow.LongHalfLength - 1 - n];
            Assert.Equal(expected, win[AacBlockWindow.LongHalfLength + n], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_LongStart_HasFlatPlateauAndZeroTail()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.LongStart, AacWindowShape.Sine, AacWindowShape.Sine);

        var sineRisingLong = AacSineWindow.ComputeRisingHalf(AacBlockWindow.LongHalfLength);
        var sineRisingShort = AacSineWindow.ComputeRisingHalf(AacBlockWindow.ShortHalfLength);

        // [0..1023] = long left half rising.
        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            Assert.Equal(sineRisingLong[n], win[n], 6);
        }

        // [1024..1471] = flat 1.
        for (int n = 0; n < AacBlockWindow.TransitionPlateauLength; n++)
        {
            Assert.Equal(1.0f, win[AacBlockWindow.LongHalfLength + n]);
        }

        // [1472..1599] = short falling = reverse(rising).
        int shortStart = AacBlockWindow.LongHalfLength + AacBlockWindow.TransitionPlateauLength;
        for (int n = 0; n < AacBlockWindow.ShortHalfLength; n++)
        {
            float expected = sineRisingShort[AacBlockWindow.ShortHalfLength - 1 - n];
            Assert.Equal(expected, win[shortStart + n], 6);
        }

        // [1600..2047] = zero tail.
        int zeroStart = shortStart + AacBlockWindow.ShortHalfLength;
        for (int n = zeroStart; n < AacBlockWindow.LongBlockLength; n++)
        {
            Assert.Equal(0.0f, win[n]);
        }
    }

    [Fact]
    public void ComposeLongBlock_LongStop_HasZeroHeadAndFlatPlateau()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.LongStop, AacWindowShape.Sine, AacWindowShape.Sine);

        var sineRisingLong = AacSineWindow.ComputeRisingHalf(AacBlockWindow.LongHalfLength);
        var sineRisingShort = AacSineWindow.ComputeRisingHalf(AacBlockWindow.ShortHalfLength);

        // [0..447] = zero head.
        for (int n = 0; n < AacBlockWindow.TransitionPlateauLength; n++)
        {
            Assert.Equal(0.0f, win[n]);
        }

        // [448..575] = short rising left half.
        int shortStart = AacBlockWindow.TransitionPlateauLength;
        for (int n = 0; n < AacBlockWindow.ShortHalfLength; n++)
        {
            Assert.Equal(sineRisingShort[n], win[shortStart + n], 6);
        }

        // [576..1023] = flat 1.
        int flatStart = shortStart + AacBlockWindow.ShortHalfLength;
        for (int n = 0; n < AacBlockWindow.TransitionPlateauLength; n++)
        {
            Assert.Equal(1.0f, win[flatStart + n]);
        }

        // [1024..2047] = long falling right half.
        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            float expected = sineRisingLong[AacBlockWindow.LongHalfLength - 1 - n];
            Assert.Equal(expected, win[AacBlockWindow.LongHalfLength + n], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_LongStart_KbdLeftSineRight_UsesPerHalfShape()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.LongStart,
            AacWindowShape.KaiserBesselDerived,
            AacWindowShape.Sine);

        var kbdRising = AacKbdWindow.ComputeRisingHalf(
            AacBlockWindow.LongHalfLength, AacKbdWindow.LongAlpha);
        var sineRisingShort = AacSineWindow.ComputeRisingHalf(AacBlockWindow.ShortHalfLength);

        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            Assert.Equal(kbdRising[n], win[n], 6);
        }
        int shortStart = AacBlockWindow.LongHalfLength + AacBlockWindow.TransitionPlateauLength;
        for (int n = 0; n < AacBlockWindow.ShortHalfLength; n++)
        {
            float expected = sineRisingShort[AacBlockWindow.ShortHalfLength - 1 - n];
            Assert.Equal(expected, win[shortStart + n], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_LongStop_SineLeftKbdRight_UsesPerHalfShape()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.LongStop,
            AacWindowShape.Sine,
            AacWindowShape.KaiserBesselDerived);

        var sineRisingShort = AacSineWindow.ComputeRisingHalf(AacBlockWindow.ShortHalfLength);
        var kbdRisingLong = AacKbdWindow.ComputeRisingHalf(
            AacBlockWindow.LongHalfLength, AacKbdWindow.LongAlpha);

        int shortStart = AacBlockWindow.TransitionPlateauLength;
        for (int n = 0; n < AacBlockWindow.ShortHalfLength; n++)
        {
            Assert.Equal(sineRisingShort[n], win[shortStart + n], 6);
        }
        for (int n = 0; n < AacBlockWindow.LongHalfLength; n++)
        {
            float expected = kbdRisingLong[AacBlockWindow.LongHalfLength - 1 - n];
            Assert.Equal(expected, win[AacBlockWindow.LongHalfLength + n], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_EightShort_Throws()
    {
        var ex = Assert.Throws<ArgumentException>(() =>
            AacBlockWindow.ComposeLongBlock(
                AacWindowSequence.EightShort, AacWindowShape.Sine, AacWindowShape.Sine));
        Assert.Equal("sequence", ex.ParamName);
    }

    [Fact]
    public void ComposeLongBlock_UnknownSequence_Throws()
    {
        var ex = Assert.Throws<ArgumentException>(() =>
            AacBlockWindow.ComposeLongBlock(
                (AacWindowSequence)99, AacWindowShape.Sine, AacWindowShape.Sine));
        Assert.Equal("sequence", ex.ParamName);
    }

    [Fact]
    public void WriteLongBlock_WrongDestinationLength_Throws()
    {
        var buf = new float[100];
        var ex = Assert.Throws<ArgumentException>(() =>
            AacBlockWindow.WriteLongBlock(
                buf.AsSpan(), AacWindowSequence.OnlyLong,
                AacWindowShape.Sine, AacWindowShape.Sine));
        Assert.Equal("destination", ex.ParamName);
    }

    [Fact]
    public void WriteLongBlock_OnlyLong_MatchesAllocatingOverload()
    {
        var stack = new float[AacBlockWindow.LongBlockLength];
        AacBlockWindow.WriteLongBlock(
            stack.AsSpan(), AacWindowSequence.OnlyLong,
            AacWindowShape.Sine, AacWindowShape.Sine);
        var heap = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.OnlyLong, AacWindowShape.Sine, AacWindowShape.Sine);
        for (int i = 0; i < stack.Length; i++)
        {
            Assert.Equal(heap[i], stack[i], 6);
        }
    }

    [Fact]
    public void ComposeShortWindow_SineSine_IsFullSineWindow()
    {
        var win = AacBlockWindow.ComposeShortWindow(AacWindowShape.Sine, AacWindowShape.Sine);
        var full = AacSineWindow.ComputeFull(AacBlockWindow.ShortHalfLength);
        Assert.Equal(full.Length, win.Length);
        for (int i = 0; i < win.Length; i++)
        {
            Assert.Equal(full[i], win[i], 6);
        }
    }

    [Fact]
    public void ComposeShortWindow_KbdKbd_IsFullKbdWindow()
    {
        var win = AacBlockWindow.ComposeShortWindow(
            AacWindowShape.KaiserBesselDerived, AacWindowShape.KaiserBesselDerived);
        var full = AacKbdWindow.ComputeFull(AacBlockWindow.ShortHalfLength, AacKbdWindow.ShortAlpha);
        Assert.Equal(full.Length, win.Length);
        for (int i = 0; i < win.Length; i++)
        {
            Assert.Equal(full[i], win[i], 6);
        }
    }

    [Fact]
    public void ComposeShortWindow_MixedShapes_LeftAndRightDifferent()
    {
        var win = AacBlockWindow.ComposeShortWindow(
            AacWindowShape.Sine, AacWindowShape.KaiserBesselDerived);

        var sineRising = AacSineWindow.ComputeRisingHalf(AacBlockWindow.ShortHalfLength);
        var kbdRising = AacKbdWindow.ComputeRisingHalf(
            AacBlockWindow.ShortHalfLength, AacKbdWindow.ShortAlpha);

        for (int n = 0; n < AacBlockWindow.ShortHalfLength; n++)
        {
            Assert.Equal(sineRising[n], win[n], 6);
        }
        for (int n = 0; n < AacBlockWindow.ShortHalfLength; n++)
        {
            float expected = kbdRising[AacBlockWindow.ShortHalfLength - 1 - n];
            Assert.Equal(expected, win[AacBlockWindow.ShortHalfLength + n], 6);
        }
    }

    [Fact]
    public void ComposeLongBlock_LongStartZeroTail_Has448Zeros()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.LongStart,
            AacWindowShape.KaiserBesselDerived,
            AacWindowShape.KaiserBesselDerived);

        int zeroStart = AacBlockWindow.LongHalfLength
            + AacBlockWindow.TransitionPlateauLength
            + AacBlockWindow.ShortHalfLength;
        Assert.Equal(1600, zeroStart);

        int zeroCount = 0;
        for (int n = zeroStart; n < AacBlockWindow.LongBlockLength; n++)
        {
            if (win[n] == 0f) zeroCount++;
        }
        Assert.Equal(448, zeroCount);
    }

    [Fact]
    public void ComposeLongBlock_LongStopZeroHead_Has448Zeros()
    {
        var win = AacBlockWindow.ComposeLongBlock(
            AacWindowSequence.LongStop,
            AacWindowShape.KaiserBesselDerived,
            AacWindowShape.KaiserBesselDerived);

        int zeroCount = 0;
        for (int n = 0; n < AacBlockWindow.TransitionPlateauLength; n++)
        {
            if (win[n] == 0f) zeroCount++;
        }
        Assert.Equal(448, zeroCount);
    }
}
