using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacShortWindowDeinterleaverTests
{
    private const int W = AacShortWindowDeinterleaver.WindowLength;
    private const int T = AacShortWindowDeinterleaver.TotalLength;

    private static AacIcsInfo MakeIcsShort(int maxSfb, params byte[] windowsPerGroup)
    {
        return new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.EightShort,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = maxSfb,
            WindowGroupCount = windowsPerGroup.Length,
            WindowsPerGroup = windowsPerGroup.AsMemory(),
        };
    }

    private static AacIcsInfo MakeIcsLong(int maxSfb)
    {
        return new AacIcsInfo
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            WindowShape = AacWindowShape.Sine,
            MaxSfb = maxSfb,
            WindowGroupCount = 1,
            WindowsPerGroup = new byte[] { 1 }.AsMemory(),
        };
    }

    [Fact]
    public void Validate_NullIcs_Throws()
    {
        var grouped = new float[T];
        var windowMajor = new float[T];
        Assert.Throws<ArgumentNullException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, null!, new int[] { 0, 4 }, windowMajor));
    }

    [Fact]
    public void Validate_WrongWindowSequence_Throws()
    {
        var ics = MakeIcsLong(maxSfb: 1);
        var grouped = new float[T];
        var windowMajor = new float[T];
        Assert.Throws<ArgumentException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, new int[] { 0, 4 }, windowMajor));
    }

    [Fact]
    public void Validate_WrongGroupedLength_Throws()
    {
        var ics = MakeIcsShort(maxSfb: 1, 1, 1, 1, 1, 1, 1, 1, 1);
        var grouped = new float[T - 1];
        var windowMajor = new float[T];
        Assert.Throws<ArgumentException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, new int[] { 0, 4 }, windowMajor));
    }

    [Fact]
    public void Validate_WrongWindowMajorLength_Throws()
    {
        var ics = MakeIcsShort(maxSfb: 1, 1, 1, 1, 1, 1, 1, 1, 1);
        var grouped = new float[T];
        var windowMajor = new float[T + 1];
        Assert.Throws<ArgumentException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, new int[] { 0, 4 }, windowMajor));
    }

    [Fact]
    public void Validate_SwbOffsetsTooShort_Throws()
    {
        var ics = MakeIcsShort(maxSfb: 3, 1, 1, 1, 1, 1, 1, 1, 1);
        var grouped = new float[T];
        var windowMajor = new float[T];
        // MaxSfb=3 requires length >= 4.
        Assert.Throws<ArgumentException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, new int[] { 0, 4, 8 }, windowMajor));
    }

    [Fact]
    public void Validate_SwbOffsetExceedsWindowLength_Throws()
    {
        var ics = MakeIcsShort(maxSfb: 1, 1, 1, 1, 1, 1, 1, 1, 1);
        var grouped = new float[T];
        var windowMajor = new float[T];
        Assert.Throws<ArgumentException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, new int[] { 0, W + 1 }, windowMajor));
    }

    [Fact]
    public void Validate_WindowsPerGroupSumWrong_Throws()
    {
        var ics = MakeIcsShort(maxSfb: 1, 1, 1, 1, 1, 1, 1, 1);    // sums to 7, not 8
        var grouped = new float[T];
        var windowMajor = new float[T];
        Assert.Throws<ArgumentException>(() =>
            AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, new int[] { 0, 4 }, windowMajor));
    }

    /// <summary>
    /// Build a simple short-window SWB offset table for a frame
    /// with all bands of width 4 covering all 128 bins (32 bands).
    /// </summary>
    private static int[] BuildUniformOffsets(int bandsCount, int bandWidth)
    {
        var offsets = new int[bandsCount + 1];
        for (int i = 0; i <= bandsCount; i++) offsets[i] = i * bandWidth;
        return offsets;
    }

    [Fact]
    public void Roundtrip_UngroupedFrame_RestoresSourceWithinMaxSfb()
    {
        // 8 groups of 1 window each (no grouping). All-distinct values.
        var ics = MakeIcsShort(maxSfb: 4, 1, 1, 1, 1, 1, 1, 1, 1);
        int[] offsets = BuildUniformOffsets(bandsCount: 4, bandWidth: 4);   // ends at 16

        // Source layout (group-major / SFB-window-interleaved): each
        // group has 1 window so layout is identical to window-major for
        // bins [0..16); we encode bin (g*128 + bin) as a unique value.
        var grouped = new float[T];
        for (int g = 0; g < 8; g++)
        {
            int groupBase = g * W;
            for (int sfb = 0; sfb < 4; sfb++)
            {
                for (int bin = offsets[sfb]; bin < offsets[sfb + 1]; bin++)
                {
                    grouped[groupBase + bin] = g * 1000 + bin;
                }
            }
        }

        var windowMajor = new float[T];
        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        // With no grouping, group-major and window-major coincide for
        // bins inside the band range.
        for (int w = 0; w < 8; w++)
        {
            int windowBase = w * W;
            for (int bin = 0; bin < 16; bin++)
            {
                Assert.Equal(grouped[w * W + bin], windowMajor[windowBase + bin]);
            }
        }

        var roundTrip = new float[T];
        AacShortWindowDeinterleaver.ToGroupMajor(windowMajor, ics, offsets, roundTrip);
        for (int w = 0; w < 8; w++)
        {
            for (int bin = 0; bin < 16; bin++)
            {
                Assert.Equal(grouped[w * W + bin], roundTrip[w * W + bin]);
            }
        }
    }

    [Fact]
    public void ToWindowMajor_GroupedFrame_DeinterleavesPerSfb()
    {
        // One group of 8 windows. Layout: per SFB, 8 windows of bandWidth
        // contiguous values. Use bandWidth=4, 4 bands -> 8*16=128 floats
        // in the first 128 bins of grouped[].
        var ics = MakeIcsShort(maxSfb: 4, 8);
        int[] offsets = BuildUniformOffsets(bandsCount: 4, bandWidth: 4);

        // Encode source so groupedValue(sfb, wInGroup, binWithinBand) =
        //   sfb*1000 + wInGroup*100 + binWithinBand
        var grouped = new float[T];
        for (int sfb = 0; sfb < 4; sfb++)
        {
            int sfbBase = offsets[sfb] * 8;
            for (int wInGroup = 0; wInGroup < 8; wInGroup++)
            {
                int wBase = sfbBase + wInGroup * 4;
                for (int b = 0; b < 4; b++)
                {
                    grouped[wBase + b] = sfb * 1000 + wInGroup * 100 + b;
                }
            }
        }

        var windowMajor = new float[T];
        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        // For window w, sfb s, bin within band b:
        //   windowMajor[w*128 + offsets[s] + b] == sfb*1000 + w*100 + b
        for (int w = 0; w < 8; w++)
        {
            for (int sfb = 0; sfb < 4; sfb++)
            {
                for (int b = 0; b < 4; b++)
                {
                    int idx = w * W + offsets[sfb] + b;
                    Assert.Equal(sfb * 1000 + w * 100 + b, windowMajor[idx]);
                }
            }
        }
    }

    [Fact]
    public void Roundtrip_GroupedFrame_RestoresSource()
    {
        // One group of 8 windows; full short-window roundtrip preserves
        // every group-major value inside the band range.
        var ics = MakeIcsShort(maxSfb: 4, 8);
        int[] offsets = BuildUniformOffsets(bandsCount: 4, bandWidth: 4);

        var grouped = new float[T];
        var rnd = new Random(12345);
        for (int sfb = 0; sfb < 4; sfb++)
        {
            int sfbBase = offsets[sfb] * 8;
            for (int wInGroup = 0; wInGroup < 8; wInGroup++)
            {
                int wBase = sfbBase + wInGroup * 4;
                for (int b = 0; b < 4; b++) grouped[wBase + b] = (float)rnd.NextDouble();
            }
        }

        var windowMajor = new float[T];
        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        var roundTrip = new float[T];
        AacShortWindowDeinterleaver.ToGroupMajor(windowMajor, ics, offsets, roundTrip);

        // Only the in-range bins are guaranteed to roundtrip.
        for (int sfb = 0; sfb < 4; sfb++)
        {
            int sfbBase = offsets[sfb] * 8;
            for (int wInGroup = 0; wInGroup < 8; wInGroup++)
            {
                int wBase = sfbBase + wInGroup * 4;
                for (int b = 0; b < 4; b++)
                {
                    Assert.Equal(grouped[wBase + b], roundTrip[wBase + b]);
                }
            }
        }
    }

    [Fact]
    public void Roundtrip_MixedGroupSizes_RestoresSource()
    {
        // 3 groups of sizes 3, 2, 3 (sums to 8).
        var ics = MakeIcsShort(maxSfb: 3, 3, 2, 3);
        int[] offsets = BuildUniformOffsets(bandsCount: 3, bandWidth: 8);   // ends at 24

        var grouped = new float[T];
        var rnd = new Random(99);

        int groupBase = 0;
        foreach (byte windowsInGroup in new byte[] { 3, 2, 3 })
        {
            for (int sfb = 0; sfb < 3; sfb++)
            {
                int bandStart = offsets[sfb];
                int bandWidth = offsets[sfb + 1] - bandStart;
                for (int wInGroup = 0; wInGroup < windowsInGroup; wInGroup++)
                {
                    int dst = groupBase + bandStart * windowsInGroup + wInGroup * bandWidth;
                    for (int b = 0; b < bandWidth; b++) grouped[dst + b] = (float)rnd.NextDouble();
                }
            }
            groupBase += windowsInGroup * W;
        }

        var windowMajor = new float[T];
        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        var roundTrip = new float[T];
        AacShortWindowDeinterleaver.ToGroupMajor(windowMajor, ics, offsets, roundTrip);

        // Compare in-range bins via the same iteration.
        groupBase = 0;
        foreach (byte windowsInGroup in new byte[] { 3, 2, 3 })
        {
            for (int sfb = 0; sfb < 3; sfb++)
            {
                int bandStart = offsets[sfb];
                int bandWidth = offsets[sfb + 1] - bandStart;
                for (int wInGroup = 0; wInGroup < windowsInGroup; wInGroup++)
                {
                    int dst = groupBase + bandStart * windowsInGroup + wInGroup * bandWidth;
                    for (int b = 0; b < bandWidth; b++)
                    {
                        Assert.Equal(grouped[dst + b], roundTrip[dst + b]);
                    }
                }
            }
            groupBase += windowsInGroup * W;
        }
    }

    [Fact]
    public void ToWindowMajor_AbsoluteWindowOrdering_GroupedConcatenated()
    {
        // 2 groups of 4. Window 0..3 are in group 0; windows 4..7 in
        // group 1. The first 16 bins of windowMajor[0..3] should be
        // sourced from group 0's data and 16 bins of windowMajor[4..7]
        // from group 1's data.
        var ics = MakeIcsShort(maxSfb: 1, 4, 4);
        int[] offsets = new int[] { 0, 16 };

        var grouped = new float[T];
        // Group 0 base = 0. Layout: 1 SFB of width 16, 4 windows.
        for (int wInGroup = 0; wInGroup < 4; wInGroup++)
        {
            for (int b = 0; b < 16; b++)
            {
                grouped[wInGroup * 16 + b] = 100 + wInGroup * 100 + b;
            }
        }
        // Group 1 base = 4*128 = 512.
        for (int wInGroup = 0; wInGroup < 4; wInGroup++)
        {
            for (int b = 0; b < 16; b++)
            {
                grouped[512 + wInGroup * 16 + b] = 1000 + wInGroup * 100 + b;
            }
        }

        var windowMajor = new float[T];
        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        // Windows 0..3 -> group 0; windows 4..7 -> group 1.
        for (int w = 0; w < 4; w++)
        {
            for (int b = 0; b < 16; b++)
            {
                Assert.Equal(100 + w * 100 + b, windowMajor[w * W + b]);
            }
        }
        for (int w = 0; w < 4; w++)
        {
            for (int b = 0; b < 16; b++)
            {
                Assert.Equal(1000 + w * 100 + b, windowMajor[(4 + w) * W + b]);
            }
        }
    }

    [Fact]
    public void ToWindowMajor_DoesNotWriteOutsideMaxSfbRange()
    {
        // MaxSfb=1 covers only bins [0..16); the rest of the
        // destination window must remain untouched.
        var ics = MakeIcsShort(maxSfb: 1, 1, 1, 1, 1, 1, 1, 1, 1);
        int[] offsets = new int[] { 0, 16 };

        var grouped = new float[T];
        // Fill the in-range region only.
        for (int w = 0; w < 8; w++)
        {
            int gbase = w * W;
            for (int b = 0; b < 16; b++) grouped[gbase + b] = w * 100 + b;
        }

        var windowMajor = new float[T];
        for (int i = 0; i < T; i++) windowMajor[i] = -1f;

        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        for (int w = 0; w < 8; w++)
        {
            int wbase = w * W;
            for (int b = 0; b < 16; b++)
            {
                Assert.Equal(w * 100 + b, windowMajor[wbase + b]);
            }
            for (int b = 16; b < W; b++)
            {
                Assert.Equal(-1f, windowMajor[wbase + b]);
            }
        }
    }

    [Fact]
    public void Roundtrip_MaxSfbZero_NoOp()
    {
        // MaxSfb=0: nothing to copy. Destination must remain at initial value.
        var ics = MakeIcsShort(maxSfb: 0, 1, 1, 1, 1, 1, 1, 1, 1);
        int[] offsets = new int[] { 0 };   // single offset (no bands)

        var grouped = new float[T];
        for (int i = 0; i < T; i++) grouped[i] = i;

        var windowMajor = new float[T];
        for (int i = 0; i < T; i++) windowMajor[i] = -1f;

        AacShortWindowDeinterleaver.ToWindowMajor(grouped, ics, offsets, windowMajor);

        for (int i = 0; i < T; i++) Assert.Equal(-1f, windowMajor[i]);
    }
}
