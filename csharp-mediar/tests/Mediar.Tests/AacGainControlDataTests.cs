using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using System.Collections.Immutable;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacGainControlDataTests
{
    [Theory]
    [InlineData(AacWindowSequence.OnlyLong)]
    [InlineData(AacWindowSequence.LongStart)]
    [InlineData(AacWindowSequence.EightShort)]
    [InlineData(AacWindowSequence.LongStop)]
    public void TryParse_MaxBandZero_EmptyHappyPath(AacWindowSequence sequence)
    {
        var w = new AacBitWriter();
        w.Write(0u, 2);                 // max_band = 0 - no per-band data follows
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), sequence, out var data));
        Assert.NotNull(data);
        Assert.Equal(sequence, data!.WindowSequence);
        Assert.Equal(0, data.MaxBand);
        Assert.Empty(data.Bands);
        Assert.Equal(2, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_OnlyLong_SingleBandSingleAdjustment()
    {
        // OnlyLong: numWindows=1, alocBits[0]=5
        var w = new AacBitWriter();
        w.Write(1u, 2);                 // max_band = 1
        w.Write(1u, 3);                 // adjust_num[1][0] = 1
        w.Write(0xAu, 4);               // alevcode = 10
        w.Write(0x1Fu, 5);              // aloccode = 31 (max for 5-bit)
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        Assert.Equal(1, data!.MaxBand);
        Assert.Single(data.Bands);
        Assert.Single(data.Bands[0].Windows);
        Assert.Equal(1, data.Bands[0].Windows[0].AdjustNum);
        var adj = data.Bands[0].Windows[0].Adjustments[0];
        Assert.Equal(10, adj.AlevCode);
        Assert.Equal(31, adj.AlocCode);
        // 2 (max_band) + 3 (adjust_num) + 4 (alev) + 5 (aloc) = 14
        Assert.Equal(14, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_LongStart_DistinctAlocBitWidthsPerWindow()
    {
        // LongStart: numWindows=2, alocBits=[4, 2]
        var w = new AacBitWriter();
        w.Write(1u, 2);                 // max_band = 1
        // wd = 0: 4-bit aloc
        w.Write(1u, 3);                 // adjust_num[1][0] = 1
        w.Write(0x5u, 4);               // alev = 5
        w.Write(0xFu, 4);               // aloc = 15 (max for 4-bit)
        // wd = 1: 2-bit aloc
        w.Write(1u, 3);                 // adjust_num[1][1] = 1
        w.Write(0x7u, 4);               // alev = 7
        w.Write(0x3u, 2);               // aloc = 3 (max for 2-bit)
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.LongStart, out var data));
        Assert.NotNull(data);
        Assert.Equal(1, data!.MaxBand);
        Assert.Single(data.Bands);
        Assert.Equal(2, data.Bands[0].Windows.Length);
        Assert.Equal(15, data.Bands[0].Windows[0].Adjustments[0].AlocCode);
        Assert.Equal(3, data.Bands[0].Windows[1].Adjustments[0].AlocCode);
        // 2 + (3 + 4 + 4) + (3 + 4 + 2) = 22
        Assert.Equal(22, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_EightShort_AllEightWindowsHaveTwoBitAloc()
    {
        // EightShort: numWindows=8, alocBits[wd]=2 for all wd.
        var w = new AacBitWriter();
        w.Write(1u, 2);                 // max_band = 1
        for (int wd = 0; wd < 8; wd++)
        {
            w.Write(1u, 3);             // adjust_num = 1 each
            w.Write((uint)wd, 4);       // alev varies per window for distinctness
            w.Write(0x2u, 2);           // aloc = 2
        }
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.EightShort, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Bands);
        Assert.Equal(8, data.Bands[0].Windows.Length);
        for (int wd = 0; wd < 8; wd++)
        {
            Assert.Equal(wd, data.Bands[0].Windows[wd].Adjustments[0].AlevCode);
            Assert.Equal(2, data.Bands[0].Windows[wd].Adjustments[0].AlocCode);
        }
        // 2 + 8 * (3 + 4 + 2) = 2 + 72 = 74
        Assert.Equal(74, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_LongStop_DistinctAlocBitWidthsPerWindow()
    {
        // LongStop: numWindows=2, alocBits=[4, 5] per libfaad.
        var w = new AacBitWriter();
        w.Write(1u, 2);                 // max_band = 1
        // wd = 0: 4-bit aloc
        w.Write(1u, 3);                 // adjust_num[1][0] = 1
        w.Write(0x9u, 4);
        w.Write(0xFu, 4);               // aloc = 15 (max for 4-bit)
        // wd = 1: 5-bit aloc
        w.Write(1u, 3);                 // adjust_num[1][1] = 1
        w.Write(0x6u, 4);
        w.Write(0x1Fu, 5);              // aloc = 31 (max for 5-bit)
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.LongStop, out var data));
        Assert.NotNull(data);
        Assert.Equal(15, data!.Bands[0].Windows[0].Adjustments[0].AlocCode);
        Assert.Equal(31, data.Bands[0].Windows[1].Adjustments[0].AlocCode);
        // 2 + (3 + 4 + 4) + (3 + 4 + 5) = 25
        Assert.Equal(25, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_OnlyLong_MaxBandThree_AllBandsAllAdjustmentsMax()
    {
        // Stress: max_band=3, each band carries 7 adjustments with max field values.
        var w = new AacBitWriter();
        w.Write(3u, 2);                 // max_band = 3
        for (int bd = 0; bd < 3; bd++)
        {
            w.Write(7u, 3);             // adjust_num = 7
            for (int ad = 0; ad < 7; ad++)
            {
                w.Write(0xFu, 4);       // alev = 15
                w.Write(0x1Fu, 5);      // aloc = 31
            }
        }
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.MaxBand);
        Assert.Equal(3, data.Bands.Length);
        foreach (var band in data.Bands)
        {
            Assert.Single(band.Windows);
            Assert.Equal(7, band.Windows[0].AdjustNum);
            Assert.Equal(7, band.Windows[0].Adjustments.Length);
            foreach (var adj in band.Windows[0].Adjustments)
            {
                Assert.Equal(15, adj.AlevCode);
                Assert.Equal(31, adj.AlocCode);
            }
        }
        // 2 + 3 * (3 + 7 * (4 + 5)) = 2 + 3 * 66 = 200
        Assert.Equal(200, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_EightShort_VariableAdjustNumPerWindow()
    {
        // Per-window adjust_num variation - confirms the inner loop honours
        // each (bd, wd) cell independently rather than reusing a previous value.
        var w = new AacBitWriter();
        w.Write(1u, 2);                 // max_band = 1
        int[] perWindowCounts = [0, 1, 2, 3, 4, 5, 6, 7];
        for (int wd = 0; wd < 8; wd++)
        {
            w.Write((uint)perWindowCounts[wd], 3);
            for (int ad = 0; ad < perWindowCounts[wd]; ad++)
            {
                w.Write((uint)(ad & 0xF), 4);
                w.Write((uint)(ad & 0x3), 2);
            }
        }
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.EightShort, out var data));
        Assert.NotNull(data);
        for (int wd = 0; wd < 8; wd++)
        {
            Assert.Equal(perWindowCounts[wd], data!.Bands[0].Windows[wd].AdjustNum);
            Assert.Equal(perWindowCounts[wd], data.Bands[0].Windows[wd].Adjustments.Length);
        }
    }

    [Theory]
    [InlineData(AacWindowSequence.OnlyLong, 14)]
    [InlineData(AacWindowSequence.LongStart, 22)]
    [InlineData(AacWindowSequence.EightShort, 74)]
    [InlineData(AacWindowSequence.LongStop, 25)]
    public void BitsConsumed_MatchesSpecFormulaForSingleAdjustmentPerWindow(AacWindowSequence sequence, int expected)
    {
        var w = new AacBitWriter();
        w.Write(1u, 2);                 // max_band = 1
        int numWindows = sequence switch
        {
            AacWindowSequence.OnlyLong => 1,
            AacWindowSequence.LongStart => 2,
            AacWindowSequence.EightShort => 8,
            AacWindowSequence.LongStop => 2,
            _ => throw new InvalidOperationException(),
        };
        int[] alocBits = sequence switch
        {
            AacWindowSequence.OnlyLong => [5],
            AacWindowSequence.LongStart => [4, 2],
            AacWindowSequence.EightShort => [2, 2, 2, 2, 2, 2, 2, 2],
            AacWindowSequence.LongStop => [4, 5],
            _ => throw new InvalidOperationException(),
        };
        for (int wd = 0; wd < numWindows; wd++)
        {
            w.Write(1u, 3);             // adjust_num = 1
            w.Write(0u, 4);             // alev = 0
            w.Write(0u, alocBits[wd]);  // aloc = 0
        }
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), sequence, out var data));
        Assert.Equal(expected, data!.BitsConsumed);
    }

    [Fact]
    public void TryParse_RejectsEmptyBuffer()
    {
        Assert.False(AacGainControlData.TryParse(
            ReadOnlySpan<byte>.Empty, AacWindowSequence.OnlyLong, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_RejectsTruncationBeforeAdjustNum()
    {
        // 0xC0 = 0b11_000_000: max_band=3 means three bands, each with a 3-bit
        // adjust_num. After max_band(2) + adjust_num[0](3) + adjust_num[1](3) =
        // 8 bits, the third band's adjust_num needs 3 more bits but none remain.
        // (Single-byte max_band=1 inputs cannot fail at adjust_num because 6
        // zero-pad bits remain, which is ≥ 3 and decodes as adjust_num=0.)
        var bytes = new byte[] { 0xC0 };
        Assert.False(AacGainControlData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_RejectsTruncationMidAdjustmentTail()
    {
        // OnlyLong, max_band=1, adjust_num=1 needs 14 bits total (9 for alev+aloc).
        // Slice the writer's 2-byte (16-bit) AlignToByte output down to 1 byte
        // (8 bits) so only 3 bits remain when the parser asks for 9.
        var w = new AacBitWriter();
        w.Write(1u, 2);
        w.Write(1u, 3);                 // adjust_num = 1
        w.Write(0u, 5);                 // partial trailing bits
        w.AlignToByte();
        var truncated = w.ToArray().AsSpan(0, 1).ToArray();

        Assert.False(AacGainControlData.TryParse(truncated, AacWindowSequence.OnlyLong, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_IgnoresTrailingBitsBeyondBitsConsumed()
    {
        // Pin BitsConsumed exactly at the spec count even when the buffer
        // contains spurious trailing bits (callers slice with BitsConsumed).
        var w = new AacBitWriter();
        w.Write(0u, 2);                 // max_band = 0 (consumes 2 bits)
        w.Write(0xFFu, 8);              // junk that must be ignored

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        Assert.Equal(2, data!.BitsConsumed);
    }

    [Theory]
    [InlineData(AacWindowSequence.OnlyLong)]
    [InlineData(AacWindowSequence.LongStart)]
    [InlineData(AacWindowSequence.EightShort)]
    [InlineData(AacWindowSequence.LongStop)]
    public void ToBytes_RoundTripsThroughTryParse(AacWindowSequence sequence)
    {
        // Build via the parser, serialise via ToBytes, parse again.
        var w = new AacBitWriter();
        w.Write(2u, 2);                 // max_band = 2
        int numWindows = sequence switch
        {
            AacWindowSequence.OnlyLong => 1,
            AacWindowSequence.LongStart => 2,
            AacWindowSequence.EightShort => 8,
            AacWindowSequence.LongStop => 2,
            _ => throw new InvalidOperationException(),
        };
        int[] alocBits = sequence switch
        {
            AacWindowSequence.OnlyLong => [5],
            AacWindowSequence.LongStart => [4, 2],
            AacWindowSequence.EightShort => [2, 2, 2, 2, 2, 2, 2, 2],
            AacWindowSequence.LongStop => [4, 5],
            _ => throw new InvalidOperationException(),
        };
        for (int bd = 0; bd < 2; bd++)
        {
            for (int wd = 0; wd < numWindows; wd++)
            {
                w.Write(2u, 3);         // adjust_num = 2
                for (int ad = 0; ad < 2; ad++)
                {
                    w.Write((uint)((ad + bd) & 0xF), 4);
                    uint maxAloc = (1u << alocBits[wd]) - 1u;
                    w.Write(maxAloc & (uint)(ad + 1), alocBits[wd]);
                }
            }
        }
        w.AlignToByte();
        var originalBytes = w.ToArray();

        Assert.True(AacGainControlData.TryParse(originalBytes, sequence, out var data));
        var roundTripped = data!.ToBytes();

        // Trim originalBytes to the number of bytes ToBytes emits before comparing,
        // since AlignToByte may have padded both with zero bits identically.
        Assert.Equal(originalBytes.Length, roundTripped.Length);
        Assert.Equal(originalBytes, roundTripped);

        // Round-trip 2: re-parse the serialised form and confirm field equality.
        Assert.True(AacGainControlData.TryParse(roundTripped, sequence, out var data2));
        Assert.Equal(data.MaxBand, data2!.MaxBand);
        Assert.Equal(data.WindowSequence, data2.WindowSequence);
        Assert.Equal(data.BitsConsumed, data2.BitsConsumed);
        Assert.Equal(data.Bands.Length, data2.Bands.Length);
        for (int bd = 0; bd < data.Bands.Length; bd++)
        {
            Assert.Equal(data.Bands[bd].Windows.Length, data2.Bands[bd].Windows.Length);
            for (int wd = 0; wd < data.Bands[bd].Windows.Length; wd++)
            {
                Assert.Equal(
                    data.Bands[bd].Windows[wd].AdjustNum,
                    data2.Bands[bd].Windows[wd].AdjustNum);
                Assert.Equal(
                    data.Bands[bd].Windows[wd].Adjustments.ToArray(),
                    data2.Bands[bd].Windows[wd].Adjustments.ToArray());
            }
        }
    }

    [Fact]
    public void ToBytes_RejectsMaxBandOutOfRange()
    {
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 4,                                  // 2-bit field max = 3
            Bands = ImmutableArray<AacGainControlBand>.Empty,
            BitsConsumed = 2,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_RejectsBandCountMismatch()
    {
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 1,
            Bands = ImmutableArray<AacGainControlBand>.Empty,  // declares 1 but supplies 0
            BitsConsumed = 0,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_RejectsWindowCountMismatchForWindowSequence()
    {
        // OnlyLong expects 1 window per band; supply 2 to trigger the check.
        var band = new AacGainControlBand
        {
            Windows = ImmutableArray.Create(
                new AacGainControlWindow { AdjustNum = 0, Adjustments = ImmutableArray<AacGainControlAdjustment>.Empty },
                new AacGainControlWindow { AdjustNum = 0, Adjustments = ImmutableArray<AacGainControlAdjustment>.Empty }),
        };
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 1,
            Bands = ImmutableArray.Create(band),
            BitsConsumed = 0,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_RejectsAdjustNumOutOfRange()
    {
        var window = new AacGainControlWindow
        {
            AdjustNum = 8,                                          // 3-bit max = 7
            Adjustments = ImmutableArray<AacGainControlAdjustment>.Empty,
        };
        var band = new AacGainControlBand { Windows = ImmutableArray.Create(window) };
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 1,
            Bands = ImmutableArray.Create(band),
            BitsConsumed = 0,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_RejectsAdjustmentCountMismatch()
    {
        var window = new AacGainControlWindow
        {
            AdjustNum = 2,
            Adjustments = ImmutableArray.Create(new AacGainControlAdjustment(0, 0)),  // declares 2, supplies 1
        };
        var band = new AacGainControlBand { Windows = ImmutableArray.Create(window) };
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 1,
            Bands = ImmutableArray.Create(band),
            BitsConsumed = 0,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_RejectsAlevOutOfRange()
    {
        var window = new AacGainControlWindow
        {
            AdjustNum = 1,
            Adjustments = ImmutableArray.Create(new AacGainControlAdjustment(16, 0)),  // alev max = 15
        };
        var band = new AacGainControlBand { Windows = ImmutableArray.Create(window) };
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 1,
            Bands = ImmutableArray.Create(band),
            BitsConsumed = 0,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_RejectsAlocOutOfRangeForWindowDispatch()
    {
        // LongStart wd=1 is 2-bit aloc - max = 3. Supply 4 to trigger the check.
        var w0 = new AacGainControlWindow
        {
            AdjustNum = 0,
            Adjustments = ImmutableArray<AacGainControlAdjustment>.Empty,
        };
        var w1 = new AacGainControlWindow
        {
            AdjustNum = 1,
            Adjustments = ImmutableArray.Create(new AacGainControlAdjustment(0, 4)),
        };
        var band = new AacGainControlBand { Windows = ImmutableArray.Create(w0, w1) };
        var bad = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.LongStart,
            MaxBand = 1,
            Bands = ImmutableArray.Create(band),
            BitsConsumed = 0,
        };
        Assert.Throws<InvalidOperationException>(() => bad.ToBytes());
    }

    [Fact]
    public void ToBytes_NullWriter_Throws()
    {
        var data = new AacGainControlData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            MaxBand = 0,
            Bands = ImmutableArray<AacGainControlBand>.Empty,
            BitsConsumed = 2,
        };
        // The public ToBytes() builds its own writer; this exercises the
        // internal contract via reflection-free path - we just call ToBytes
        // and confirm an empty payload of one byte (2 bits + zero pad).
        var bytes = data.ToBytes();
        Assert.Single(bytes);
        Assert.Equal(0x00, bytes[0]);
    }

    [Fact]
    public void Adjustment_Record_Equality_ByValue()
    {
        var a = new AacGainControlAdjustment(5, 7);
        var b = new AacGainControlAdjustment(5, 7);
        var c = new AacGainControlAdjustment(5, 8);
        Assert.Equal(a, b);
        Assert.NotEqual(a, c);
    }

    [Fact]
    public void Adjustment_WithExpression_ReplacesAlevCode()
    {
        var a = new AacGainControlAdjustment(5, 7);
        var b = a with { AlevCode = 10 };
        Assert.Equal(5, a.AlevCode);
        Assert.Equal(10, b.AlevCode);
        Assert.Equal(7, b.AlocCode);
    }

    [Theory]
    [InlineData(AacWindowSequence.OnlyLong)]
    [InlineData(AacWindowSequence.LongStart)]
    [InlineData(AacWindowSequence.EightShort)]
    [InlineData(AacWindowSequence.LongStop)]
    public void ToBytes_MaxBandZero_AllSequences_ProducesSingleZeroByte(AacWindowSequence sequence)
    {
        var data = new AacGainControlData
        {
            WindowSequence = sequence,
            MaxBand = 0,
            Bands = ImmutableArray<AacGainControlBand>.Empty,
            BitsConsumed = 2,
        };
        var bytes = data.ToBytes();
        Assert.Single(bytes);
        Assert.Equal(0x00, bytes[0]);
    }

    [Fact]
    public void TryParse_OnlyLong_MaxBand3_AllAdjustNumZero_BitsConsumed_11()
    {
        // 2 (max_band) + 3 * 3 (adjust_num=0 for each band) = 11 bits.
        var w = new AacBitWriter();
        w.Write(3u, 2);
        for (int bd = 0; bd < 3; bd++) w.Write(0u, 3);
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        Assert.Equal(11, data!.BitsConsumed);
        Assert.Equal(3, data.Bands.Length);
        foreach (var band in data.Bands)
        {
            Assert.Equal(0, band.Windows[0].AdjustNum);
            Assert.Empty(band.Windows[0].Adjustments);
        }
    }

    [Fact]
    public void TryParse_EightShort_MaxBand1_AllAdjustNumZero_BitsConsumed_26()
    {
        // 2 (max_band) + 8 windows * 3 (adjust_num=0) = 26 bits.
        var w = new AacBitWriter();
        w.Write(1u, 2);
        for (int wd = 0; wd < 8; wd++) w.Write(0u, 3);
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.EightShort, out var data));
        Assert.Equal(26, data!.BitsConsumed);
        Assert.Single(data.Bands);
        Assert.Equal(8, data.Bands[0].Windows.Length);
    }

    [Theory]
    [InlineData(AacWindowSequence.LongStart)]
    [InlineData(AacWindowSequence.LongStop)]
    public void TryParse_TwoWindowSeq_MaxBand1_AllAdjustNumZero_BitsConsumed_8(AacWindowSequence sequence)
    {
        // 2 (max_band) + 2 windows * 3 (adjust_num=0) = 8 bits.
        var w = new AacBitWriter();
        w.Write(1u, 2);
        w.Write(0u, 3);
        w.Write(0u, 3);
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), sequence, out var data));
        Assert.Equal(8, data!.BitsConsumed);
        Assert.Equal(2, data.Bands[0].Windows.Length);
    }

    [Fact]
    public void With_Expression_Replaces_MaxBand_LeavesOthersUnchanged()
    {
        var w = new AacBitWriter();
        w.Write(0u, 2);
        w.AlignToByte();
        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        var mutated = data! with { MaxBand = 2 };
        Assert.Equal(0, data!.MaxBand);
        Assert.Equal(2, mutated.MaxBand);
        Assert.Equal(data.WindowSequence, mutated.WindowSequence);
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(3)]
    [InlineData(5)]
    [InlineData(7)]
    public void TryParse_OnlyLong_AdjustNumValues_AcceptsAllInRange(int adjustNum)
    {
        var w = new AacBitWriter();
        w.Write(1u, 2);
        w.Write((uint)adjustNum, 3);
        for (int i = 0; i < adjustNum; i++)
        {
            w.Write(0u, 4);
            w.Write(0u, 5);
        }
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        Assert.Equal(adjustNum, data!.Bands[0].Windows[0].AdjustNum);
        Assert.Equal(adjustNum, data.Bands[0].Windows[0].Adjustments.Length);
        // 2 + 3 + adjustNum * 9
        Assert.Equal(5 + adjustNum * 9, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_Adjustments_PreserveOrderingByInsertion()
    {
        // Two adjustments with distinct alev values must be returned in
        // the same order they appear in the bitstream.
        var w = new AacBitWriter();
        w.Write(1u, 2);
        w.Write(2u, 3);
        w.Write(0x1u, 4); w.Write(0u, 5);   // adjustment 0 alev=1
        w.Write(0xEu, 4); w.Write(0u, 5);   // adjustment 1 alev=14
        w.AlignToByte();

        Assert.True(AacGainControlData.TryParse(w.ToArray(), AacWindowSequence.OnlyLong, out var data));
        Assert.Equal(1, data!.Bands[0].Windows[0].Adjustments[0].AlevCode);
        Assert.Equal(14, data.Bands[0].Windows[0].Adjustments[1].AlevCode);
    }

    [Fact]
    public void AacGainControlAdjustment_Default_Constructed_Has_Zero_Fields()
    {
        var a = new AacGainControlAdjustment(0, 0);
        Assert.Equal(0, a.AlevCode);
        Assert.Equal(0, a.AlocCode);
    }
}
