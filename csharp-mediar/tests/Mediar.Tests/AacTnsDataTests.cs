using System.Collections.Immutable;
using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacTnsDataTests
{
    private static readonly int[] ExpectedCoefs157 = [1, 5, 7];
    private static readonly int[] ExpectedCoefs62 = [6, 2];
    private static readonly int[] ExpectedCoefs158 = [15, 8];
    private static readonly int[] ExpectedSingleZero = [0];
    private static readonly int[] ExpectedCoefs25 = [2, 5];

    [Fact]
    public void TryParse_LongWindow_RejectsEmptyBuffer()
    {
        Assert.False(AacTnsData.TryParse(
            ReadOnlySpan<byte>.Empty,
            AacWindowSequence.OnlyLong,
            out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_EightShort_RejectsEmptyBuffer()
    {
        Assert.False(AacTnsData.TryParse(
            ReadOnlySpan<byte>.Empty,
            AacWindowSequence.EightShort,
            out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_LongWindow_NoFilters_Consumes2Bits()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 2);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        Assert.Equal(AacWindowSequence.OnlyLong, data!.WindowSequence);
        Assert.Single(data.Windows);
        Assert.Empty(data.Windows[0].Filters);
        Assert.Equal(2, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_LongWindow_OneFilter_OrderZero_OmitsDirectionAndCompress()
    {
        var writer = new AacBitWriter();
        writer.Write(1u, 2);
        writer.Write(0u, 1);
        writer.Write(7u, 6);
        writer.Write(0u, 5);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        Assert.Single(data!.Windows);
        Assert.False(data.Windows[0].CoefResHigh);
        Assert.Single(data.Windows[0].Filters);

        var filter = data.Windows[0].Filters[0];
        Assert.Equal(7, filter.Length);
        Assert.Equal(0, filter.Order);
        Assert.False(filter.Direction);
        Assert.False(filter.CoefCompress);
        Assert.Empty(filter.Coefficients);
        Assert.Equal(0, filter.CoefBits);
        Assert.Equal(14, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_LongWindow_OneFilter_Order3_CoefRes0_CoefCompress0()
    {
        var writer = new AacBitWriter();
        writer.Write(1u, 2);
        writer.Write(0u, 1);
        writer.Write(20u, 6);
        writer.Write(3u, 5);
        writer.Write(0u, 1);
        writer.Write(0u, 1);
        writer.Write(1u, 3);
        writer.Write(5u, 3);
        writer.Write(7u, 3);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        var filter = data!.Windows[0].Filters[0];
        Assert.Equal(20, filter.Length);
        Assert.Equal(3, filter.Order);
        Assert.False(filter.Direction);
        Assert.False(filter.CoefCompress);
        Assert.Equal(3, filter.CoefBits);
        Assert.Equal(ExpectedCoefs157, filter.Coefficients);

        Assert.Equal(2 + 1 + 6 + 5 + 1 + 1 + 3 * 3, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_LongWindow_OneFilter_Order2_CoefRes1_CoefCompress1_Yields3BitCoefs()
    {
        var writer = new AacBitWriter();
        writer.Write(1u, 2);
        writer.Write(1u, 1);
        writer.Write(16u, 6);
        writer.Write(2u, 5);
        writer.Write(1u, 1);
        writer.Write(1u, 1);
        writer.Write(6u, 3);
        writer.Write(2u, 3);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        Assert.True(data!.Windows[0].CoefResHigh);
        var filter = data.Windows[0].Filters[0];
        Assert.True(filter.Direction);
        Assert.True(filter.CoefCompress);
        Assert.Equal(3, filter.CoefBits);
        Assert.Equal(ExpectedCoefs62, filter.Coefficients);
    }

    [Fact]
    public void TryParse_LongWindow_OneFilter_Order2_CoefRes1_CoefCompress0_Yields4BitCoefs()
    {
        var writer = new AacBitWriter();
        writer.Write(1u, 2);
        writer.Write(1u, 1);
        writer.Write(10u, 6);
        writer.Write(2u, 5);
        writer.Write(0u, 1);
        writer.Write(0u, 1);
        writer.Write(15u, 4);
        writer.Write(8u, 4);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        var filter = data!.Windows[0].Filters[0];
        Assert.Equal(4, filter.CoefBits);
        Assert.Equal(ExpectedCoefs158, filter.Coefficients);
    }

    [Fact]
    public void TryParse_LongWindow_ThreeFilters_MaximumPerWindow()
    {
        var writer = new AacBitWriter();
        writer.Write(3u, 2);
        writer.Write(0u, 1);

        writer.Write(5u, 6); writer.Write(0u, 5);
        writer.Write(7u, 6); writer.Write(2u, 5);
        writer.Write(0u, 1); writer.Write(0u, 1);
        writer.Write(3u, 3); writer.Write(4u, 3);
        writer.Write(9u, 6); writer.Write(1u, 5);
        writer.Write(1u, 1); writer.Write(1u, 1);
        writer.Write(0u, 2);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.NotNull(data);
        Assert.Equal(3, data!.Windows[0].Filters.Length);
        Assert.Equal(0, data.Windows[0].Filters[0].Order);
        Assert.Equal(2, data.Windows[0].Filters[1].Order);
        Assert.Equal(1, data.Windows[0].Filters[2].Order);
        Assert.Equal(2, data.Windows[0].Filters[2].CoefBits);
        Assert.Equal(ExpectedSingleZero, data.Windows[0].Filters[2].Coefficients);
    }

    [Fact]
    public void TryParse_EightShort_NoFiltersInAnyWindow_Consumes8Bits()
    {
        var writer = new AacBitWriter();
        for (int w = 0; w < 8; w++) writer.Write(0u, 1);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.EightShort, out var data));
        Assert.NotNull(data);
        Assert.Equal(8, data!.Windows.Length);
        foreach (var window in data.Windows) Assert.Empty(window.Filters);
        Assert.Equal(8, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_EightShort_WindowsMixZeroAndOneFilter()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 1);
        writer.Write(1u, 1);
        writer.Write(0u, 1);
        writer.Write(10u, 4);
        writer.Write(2u, 3);
        writer.Write(1u, 1);
        writer.Write(0u, 1);
        writer.Write(2u, 3);
        writer.Write(5u, 3);
        for (int w = 2; w < 8; w++) writer.Write(0u, 1);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.EightShort, out var data));
        Assert.NotNull(data);
        Assert.Empty(data!.Windows[0].Filters);
        Assert.Single(data.Windows[1].Filters);
        var f = data.Windows[1].Filters[0];
        Assert.Equal(10, f.Length);
        Assert.Equal(2, f.Order);
        Assert.True(f.Direction);
        Assert.False(f.CoefCompress);
        Assert.Equal(3, f.CoefBits);
        Assert.Equal(ExpectedCoefs25, f.Coefficients);
        for (int w = 2; w < 8; w++) Assert.Empty(data.Windows[w].Filters);
    }

    [Fact]
    public void TryParse_EightShort_AllWindowsHaveFilters()
    {
        var writer = new AacBitWriter();
        for (int w = 0; w < 8; w++)
        {
            writer.Write(1u, 1);
            writer.Write(0u, 1);
            writer.Write((uint)(w + 1), 4);
            writer.Write(1u, 3);
            writer.Write(0u, 1);
            writer.Write(0u, 1);
            writer.Write((uint)w, 3);
        }

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.EightShort, out var data));
        Assert.NotNull(data);
        Assert.Equal(8, data!.Windows.Length);
        for (int w = 0; w < 8; w++)
        {
            var filter = data.Windows[w].Filters[0];
            Assert.Equal(w + 1, filter.Length);
            Assert.Equal(1, filter.Order);
            Assert.Equal(3, filter.CoefBits);
            Assert.Equal(new[] { w }, filter.Coefficients);
        }
    }

    [Fact]
    public void TryParse_TruncatedHeader_ReturnsFalse()
    {
        Assert.False(AacTnsData.TryParse(
            ReadOnlySpan<byte>.Empty,
            AacWindowSequence.OnlyLong,
            out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_TruncatedMidFilter_ReturnsFalse()
    {
        var writer = new AacBitWriter();
        writer.Write(1u, 2);
        writer.Write(0u, 1);
        writer.Write(3u, 6);
        writer.Write(2u, 5);
        writer.Write(0u, 1);
        writer.Write(0u, 1);

        var bytes = writer.ToArray();
        Assert.False(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_TrailingBitsIgnored_BitsConsumedExact()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 2);

        var bytes = writer.ToArray();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var data));
        Assert.Equal(2, data!.BitsConsumed);
    }

    [Fact]
    public void ToBytes_RoundTrips_LongWindow_ViaTryParse()
    {
        var source = BuildLongWindowFixture();
        var bytes = source.ToBytes();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.OnlyLong, out var parsed));
        Assert.NotNull(parsed);
        AssertTnsEquivalent(source, parsed!);

        var second = parsed! with { };
        var rebytes = second.ToBytes();
        Assert.Equal(bytes, rebytes);
    }

    [Fact]
    public void ToBytes_RoundTrips_EightShort_ViaTryParse()
    {
        var source = BuildEightShortFixture();
        var bytes = source.ToBytes();
        Assert.True(AacTnsData.TryParse(bytes, AacWindowSequence.EightShort, out var parsed));
        Assert.NotNull(parsed);
        AssertTnsEquivalent(source, parsed!);
    }

    [Fact]
    public void ToBytes_TooManyFiltersForLong_Throws()
    {
        var source = BuildLongWindowFixture() with
        {
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters =
                    [
                        new AacTnsFilter { Length = 1, Order = 0 },
                        new AacTnsFilter { Length = 2, Order = 0 },
                        new AacTnsFilter { Length = 3, Order = 0 },
                        new AacTnsFilter { Length = 4, Order = 0 },
                    ],
                },
            ],
        };

        Assert.Throws<InvalidOperationException>(source.ToBytes);
    }

    [Fact]
    public void ToBytes_TooManyFiltersForShort_Throws()
    {
        var windows = ImmutableArray.CreateBuilder<AacTnsWindow>(8);
        for (int w = 0; w < 8; w++)
        {
            windows.Add(new AacTnsWindow
            {
                CoefResHigh = false,
                Filters = w == 0
                    ? [
                        new AacTnsFilter { Length = 1, Order = 0 },
                        new AacTnsFilter { Length = 2, Order = 0 },
                      ]
                    : [],
            });
        }

        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.EightShort,
            Windows = windows.MoveToImmutable(),
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_WrongWindowCountForLong_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow { Filters = [] },
                new AacTnsWindow { Filters = [] },
            ],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_WrongWindowCountForShort_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.EightShort,
            Windows = [new AacTnsWindow { Filters = [] }],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_OverflowingLength_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters = [new AacTnsFilter { Length = 64, Order = 0 }],
                },
            ],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_OverflowingOrder_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters = [new AacTnsFilter { Length = 1, Order = 32 }],
                },
            ],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_OrderMismatchToCoefficients_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters =
                    [
                        new AacTnsFilter
                        {
                            Length = 1,
                            Order = 3,
                            Coefficients = [0, 1],
                            CoefBits = 3,
                        },
                    ],
                },
            ],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_OrderZeroWithCoefficients_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters =
                    [
                        new AacTnsFilter
                        {
                            Length = 1,
                            Order = 0,
                            Coefficients = [0],
                            CoefBits = 0,
                        },
                    ],
                },
            ],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    [Fact]
    public void ToBytes_OverflowingCoefficient_Throws()
    {
        var data = new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters =
                    [
                        new AacTnsFilter
                        {
                            Length = 1,
                            Order = 1,
                            Direction = false,
                            CoefCompress = false,
                            Coefficients = [8],
                            CoefBits = 3,
                        },
                    ],
                },
            ],
            BitsConsumed = 0,
        };

        Assert.Throws<InvalidOperationException>(data.ToBytes);
    }

    private static AacTnsData BuildLongWindowFixture()
    {
        return new AacTnsData
        {
            WindowSequence = AacWindowSequence.OnlyLong,
            Windows =
            [
                new AacTnsWindow
                {
                    CoefResHigh = true,
                    Filters =
                    [
                        new AacTnsFilter
                        {
                            Length = 32,
                            Order = 4,
                            Direction = true,
                            CoefCompress = false,
                            Coefficients = [1, 8, 15, 2],
                            CoefBits = 4,
                        },
                        new AacTnsFilter
                        {
                            Length = 5,
                            Order = 0,
                        },
                    ],
                },
            ],
            BitsConsumed = 0,
        };
    }

    private static AacTnsData BuildEightShortFixture()
    {
        var windows = ImmutableArray.CreateBuilder<AacTnsWindow>(8);
        for (int w = 0; w < 8; w++)
        {
            if (w % 2 == 0)
            {
                windows.Add(new AacTnsWindow
                {
                    CoefResHigh = false,
                    Filters =
                    [
                        new AacTnsFilter
                        {
                            Length = w,
                            Order = 1,
                            Direction = (w & 2) != 0,
                            CoefCompress = false,
                            Coefficients = [w],
                            CoefBits = 3,
                        },
                    ],
                });
            }
            else
            {
                windows.Add(new AacTnsWindow { Filters = [] });
            }
        }

        return new AacTnsData
        {
            WindowSequence = AacWindowSequence.EightShort,
            Windows = windows.MoveToImmutable(),
            BitsConsumed = 0,
        };
    }

    private static void AssertTnsEquivalent(AacTnsData expected, AacTnsData actual)
    {
        Assert.Equal(expected.WindowSequence, actual.WindowSequence);
        Assert.Equal(expected.Windows.Length, actual.Windows.Length);
        for (int w = 0; w < expected.Windows.Length; w++)
        {
            Assert.Equal(expected.Windows[w].Filters.Length, actual.Windows[w].Filters.Length);
            if (expected.Windows[w].Filters.Length > 0)
            {
                Assert.Equal(expected.Windows[w].CoefResHigh, actual.Windows[w].CoefResHigh);
            }
            for (int f = 0; f < expected.Windows[w].Filters.Length; f++)
            {
                var e = expected.Windows[w].Filters[f];
                var a = actual.Windows[w].Filters[f];
                Assert.Equal(e.Length, a.Length);
                Assert.Equal(e.Order, a.Order);
                if (e.Order > 0)
                {
                    Assert.Equal(e.Direction, a.Direction);
                    Assert.Equal(e.CoefCompress, a.CoefCompress);
                    Assert.Equal(e.CoefBits, a.CoefBits);
                    Assert.True(e.Coefficients.SequenceEqual(a.Coefficients));
                }
            }
        }
    }
}
