using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacPulseDataTests
{
    [Fact]
    public void TryParse_RejectsEmptyBuffer()
    {
        Assert.False(AacPulseData.TryParse(ReadOnlySpan<byte>.Empty, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_RejectsHeaderOnly_WhenNoPulseBitsFit()
    {
        var bytes = new byte[] { 0xFF };
        Assert.False(AacPulseData.TryParse(bytes, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_OnePulse_MinimalValues()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 2);
        writer.Write(0u, 6);
        writer.Write(0u, 5);
        writer.Write(0u, 4);

        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var data));
        Assert.NotNull(data);
        Assert.Equal(0, data!.StartScaleFactorBand);
        Assert.Single(data.Pulses);
        Assert.Equal(0, data.Pulses[0].Offset);
        Assert.Equal(0, data.Pulses[0].Amplitude);
        Assert.Equal(17, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_FourPulses_MaximumValues()
    {
        var writer = new AacBitWriter();
        writer.Write(3u, 2);
        writer.Write(63u, 6);
        for (int i = 0; i < 4; i++)
        {
            writer.Write(31u, 5);
            writer.Write(15u, 4);
        }

        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var data));
        Assert.NotNull(data);
        Assert.Equal(63, data!.StartScaleFactorBand);
        Assert.Equal(4, data.Pulses.Length);
        foreach (var pulse in data.Pulses)
        {
            Assert.Equal(31, pulse.Offset);
            Assert.Equal(15, pulse.Amplitude);
        }
        Assert.Equal(44, data.BitsConsumed);
    }

    [Theory]
    [InlineData(1, 17)]
    [InlineData(2, 26)]
    [InlineData(3, 35)]
    [InlineData(4, 44)]
    public void TryParse_BitsConsumed_MatchesSpecFormula(int pulses, int expectedBits)
    {
        var writer = new AacBitWriter();
        writer.Write((uint)(pulses - 1), 2);
        writer.Write(7u, 6);
        for (int i = 0; i < pulses; i++)
        {
            writer.Write(3u, 5);
            writer.Write(2u, 4);
        }

        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var data));
        Assert.NotNull(data);
        Assert.Equal(pulses, data!.Pulses.Length);
        Assert.Equal(expectedBits, data.BitsConsumed);
    }

    [Fact]
    public void TryParse_TruncatedPulseList_Rejected()
    {
        var writer = new AacBitWriter();
        writer.Write(3u, 2);
        writer.Write(10u, 6);
        writer.Write(1u, 5);
        writer.Write(1u, 4);
        writer.Write(2u, 5);

        var bytes = writer.ToArray();
        Assert.False(AacPulseData.TryParse(bytes, out var data));
        Assert.Null(data);
    }

    [Fact]
    public void TryParse_ThreePulses_VariedOffsetsAndAmplitudes()
    {
        var writer = new AacBitWriter();
        writer.Write(2u, 2);
        writer.Write(20u, 6);
        writer.Write(5u, 5); writer.Write(7u, 4);
        writer.Write(12u, 5); writer.Write(3u, 4);
        writer.Write(28u, 5); writer.Write(15u, 4);

        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var data));
        Assert.NotNull(data);
        Assert.Equal(20, data!.StartScaleFactorBand);
        Assert.Equal(3, data.Pulses.Length);
        Assert.Equal(new AacPulse(5, 7), data.Pulses[0]);
        Assert.Equal(new AacPulse(12, 3), data.Pulses[1]);
        Assert.Equal(new AacPulse(28, 15), data.Pulses[2]);
    }

    [Fact]
    public void ToBytes_RoundTrips_ViaTryParse()
    {
        var original = new AacPulseData_ForRoundTrip(
            startSfb: 42,
            pulses: new[] { (3, 9), (17, 0), (31, 15) });
        var bytes = original.Build();

        Assert.True(AacPulseData.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);
        Assert.Equal(42, parsed!.StartScaleFactorBand);
        Assert.Equal(3, parsed.Pulses.Length);
        Assert.Equal(new AacPulse(3, 9), parsed.Pulses[0]);
        Assert.Equal(new AacPulse(17, 0), parsed.Pulses[1]);
        Assert.Equal(new AacPulse(31, 15), parsed.Pulses[2]);

        var round = parsed.ToBytes();
        Assert.True(AacPulseData.TryParse(round, out var second));
        Assert.NotNull(second);
        Assert.Equal(parsed.StartScaleFactorBand, second!.StartScaleFactorBand);
        Assert.True(parsed.Pulses.SequenceEqual(second.Pulses));
    }

    [Fact]
    public void ToBytes_OverflowingOffset_Throws()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 2);
        writer.Write(5u, 6);
        writer.Write(31u, 5);
        writer.Write(0u, 4);
        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);

        var bogus = parsed! with { Pulses = [new AacPulse(99, 0)] };
        Assert.Throws<InvalidOperationException>(() => bogus.ToBytes());
    }

    [Fact]
    public void ToBytes_OverflowingAmplitude_Throws()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 2);
        writer.Write(5u, 6);
        writer.Write(0u, 5);
        writer.Write(15u, 4);
        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);

        var bogus = parsed! with { Pulses = [new AacPulse(0, 16)] };
        Assert.Throws<InvalidOperationException>(() => bogus.ToBytes());
    }

    [Fact]
    public void ToBytes_OverflowingPulseCount_Throws()
    {
        var writer = new AacBitWriter();
        writer.Write(0u, 2);
        writer.Write(0u, 6);
        writer.Write(0u, 5);
        writer.Write(0u, 4);
        var bytes = writer.ToArray();
        Assert.True(AacPulseData.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);

        var bogus = parsed! with
        {
            Pulses =
            [
                new AacPulse(0, 0),
                new AacPulse(0, 0),
                new AacPulse(0, 0),
                new AacPulse(0, 0),
                new AacPulse(0, 0),
            ],
        };
        Assert.Throws<InvalidOperationException>(() => bogus.ToBytes());
    }

    [Fact]
    public void TryParse_TrailingBitsIgnored_BitsConsumedExact()
    {
        var writer = new AacBitWriter();
        writer.Write(1u, 2);
        writer.Write(11u, 6);
        writer.Write(4u, 5);
        writer.Write(5u, 4);
        writer.Write(8u, 5);
        writer.Write(2u, 4);
        var bytes = writer.ToArray();

        var padded = new byte[bytes.Length + 3];
        bytes.CopyTo(padded, 0);
        padded[^1] = 0xFF;
        padded[^2] = 0xFF;
        padded[^3] = 0xFF;

        Assert.True(AacPulseData.TryParse(padded, out var data));
        Assert.NotNull(data);
        Assert.Equal(26, data!.BitsConsumed);
        Assert.Equal(11, data.StartScaleFactorBand);
        Assert.Equal(2, data.Pulses.Length);
        Assert.Equal(new AacPulse(4, 5), data.Pulses[0]);
        Assert.Equal(new AacPulse(8, 2), data.Pulses[1]);
    }

    private sealed class AacPulseData_ForRoundTrip
    {
        private readonly int _startSfb;
        private readonly (int Offset, int Amplitude)[] _pulses;

        public AacPulseData_ForRoundTrip(int startSfb, (int Offset, int Amplitude)[] pulses)
        {
            _startSfb = startSfb;
            _pulses = pulses;
        }

        public byte[] Build()
        {
            var writer = new AacBitWriter();
            writer.Write((uint)(_pulses.Length - 1), 2);
            writer.Write((uint)_startSfb, 6);
            foreach (var (offset, amplitude) in _pulses)
            {
                writer.Write((uint)offset, 5);
                writer.Write((uint)amplitude, 4);
            }
            return writer.ToArray();
        }
    }
}
