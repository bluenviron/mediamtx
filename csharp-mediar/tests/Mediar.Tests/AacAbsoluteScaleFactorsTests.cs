using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacAbsoluteScaleFactorsTests
{
    private static AacScaleFactorEntry MakeEntry(
        AacScaleFactorKind kind,
        int diff,
        int group = 0,
        int sfb = 0)
    {
        return new AacScaleFactorEntry
        {
            Group = group,
            Sfb = sfb,
            Kind = kind,
            Differential = diff,
        };
    }

    private static AacScaleFactorData MakeData(params AacScaleFactorEntry[] entries)
    {
        return new AacScaleFactorData
        {
            Entries = entries,
            BitsConsumed = 0,
        };
    }

    [Fact]
    public void FromDelta_NullDeltas_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacAbsoluteScaleFactors.FromDelta(null!, globalGain: 100));
    }

    [Fact]
    public void FromDelta_NoEntries_ReturnsEmpty()
    {
        var result = AacAbsoluteScaleFactors.FromDelta(MakeData(), globalGain: 100);
        Assert.Empty(result.Entries);
    }

    [Fact]
    public void FromDelta_SingleSpectralBand_AddsToGlobalGain()
    {
        // global_gain=100, diff=+5 → sf = 105
        var deltas = MakeData(MakeEntry(AacScaleFactorKind.SpectralGain, +5));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Single(abs.Entries);
        Assert.Equal(105, abs.Entries[0].Value);
        Assert.Equal(AacScaleFactorKind.SpectralGain, abs.Entries[0].Kind);
    }

    [Fact]
    public void FromDelta_SequentialSpectralBands_AccumulateAgainstGlobalGain()
    {
        // global_gain=100; diffs +5, -3, +7 → 105, 102, 109.
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +5),
            MakeEntry(AacScaleFactorKind.SpectralGain, -3),
            MakeEntry(AacScaleFactorKind.SpectralGain, +7));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Collection(abs.Entries,
            e => Assert.Equal(105, e.Value),
            e => Assert.Equal(102, e.Value),
            e => Assert.Equal(109, e.Value));
    }

    [Fact]
    public void FromDelta_NoneBand_PassedThroughAsZeroValue()
    {
        // ZERO_HCB sections appear as None entries with diff=0; they stay at 0.
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +5),
            MakeEntry(AacScaleFactorKind.None, 0),
            MakeEntry(AacScaleFactorKind.SpectralGain, +2));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(105, abs.Entries[0].Value);
        Assert.Equal(0, abs.Entries[1].Value);
        // Spectral accumulator was unaffected by the None band.
        Assert.Equal(107, abs.Entries[2].Value);
    }

    [Fact]
    public void FromDelta_FirstPnsBand_AddsPcmToGlobalGainMinusNoiseOffset()
    {
        // global_gain=100, NoiseOffset=90; first PNS diff=+10 → 100 - 90 + 10 = 20.
        var deltas = MakeData(MakeEntry(AacScaleFactorKind.NoiseEnergy, +10));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(20, abs.Entries[0].Value);
    }

    [Fact]
    public void FromDelta_FirstPnsBand_NegativePcm_Works()
    {
        // First PNS PCM diff = -100 (raw 156); global_gain=200; result = 200-90-100 = 10.
        var deltas = MakeData(MakeEntry(AacScaleFactorKind.NoiseEnergy, -100));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 200);
        Assert.Equal(10, abs.Entries[0].Value);
    }

    [Fact]
    public void FromDelta_MultiplePnsBands_FirstUsesPcmRestAccumulate()
    {
        // global_gain=100; PNS diffs +10 (PCM init), +3 (huffman), -5 (huffman).
        // First: 100-90+10 = 20. Then 20+3=23. Then 23-5=18.
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +10),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +3),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, -5));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Collection(abs.Entries,
            e => Assert.Equal(20, e.Value),
            e => Assert.Equal(23, e.Value),
            e => Assert.Equal(18, e.Value));
    }

    [Fact]
    public void FromDelta_IntensityBands_AccumulateFromZero()
    {
        // global_gain irrelevant for intensity; diffs +20, -5, +1 → 20, 15, 16.
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.IntensityPosition, +20),
            MakeEntry(AacScaleFactorKind.IntensityPosition, -5),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +1));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Collection(abs.Entries,
            e => Assert.Equal(20, e.Value),
            e => Assert.Equal(15, e.Value),
            e => Assert.Equal(16, e.Value));
    }

    [Fact]
    public void FromDelta_MixedKinds_AccumulatorsAreIndependent()
    {
        // global_gain=100. Sequence:
        //   spectral +5 → spec=105
        //   PNS +10 (first PCM) → noise = 100-90+10 = 20
        //   intensity +7 → is = 7
        //   spectral -3 → spec = 102 (NOT affected by intermediate PNS/IS)
        //   PNS +2 (huffman, not first) → noise = 20+2 = 22
        //   intensity +2 → is = 9
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +5),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +10),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +7),
            MakeEntry(AacScaleFactorKind.SpectralGain, -3),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +2),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +2));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Collection(abs.Entries,
            e => Assert.Equal(105, e.Value),
            e => Assert.Equal(20, e.Value),
            e => Assert.Equal(7, e.Value),
            e => Assert.Equal(102, e.Value),
            e => Assert.Equal(22, e.Value),
            e => Assert.Equal(9, e.Value));
    }

    [Fact]
    public void FromDelta_NoiseOffset_IsNinety()
    {
        Assert.Equal(90, AacAbsoluteScaleFactors.NoiseOffset);
    }

    [Fact]
    public void FromDelta_PreservesGroupAndSfb()
    {
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +1, group: 0, sfb: 3),
            MakeEntry(AacScaleFactorKind.SpectralGain, +1, group: 2, sfb: 7));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(0, abs.Entries[0].Group);
        Assert.Equal(3, abs.Entries[0].Sfb);
        Assert.Equal(2, abs.Entries[1].Group);
        Assert.Equal(7, abs.Entries[1].Sfb);
    }

    [Fact]
    public void FromDelta_GlobalGainZero_SpectralAccumulatorStartsAtZero()
    {
        var deltas = MakeData(MakeEntry(AacScaleFactorKind.SpectralGain, +42));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 0);
        Assert.Equal(42, abs.Entries[0].Value);
    }

    [Fact]
    public void FromDelta_GlobalGainMax_SpectralAccumulatorStartsAtMax()
    {
        var deltas = MakeData(MakeEntry(AacScaleFactorKind.SpectralGain, -5));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 255);
        Assert.Equal(250, abs.Entries[0].Value);
    }

    [Fact]
    public void FromDelta_PnsPcmPending_Resets_Between_Calls()
    {
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +10));
        var a = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        var b = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        // Both calls must apply the PCM-init formula (100 - 90 + 10 = 20).
        Assert.Equal(20, a.Entries[0].Value);
        Assert.Equal(20, b.Entries[0].Value);
    }

    [Fact]
    public void FromDelta_Preserves_Entry_Kind_Across_All_Kinds()
    {
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +1),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +1),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +1),
            MakeEntry(AacScaleFactorKind.None, 0));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(AacScaleFactorKind.SpectralGain, abs.Entries[0].Kind);
        Assert.Equal(AacScaleFactorKind.NoiseEnergy, abs.Entries[1].Kind);
        Assert.Equal(AacScaleFactorKind.IntensityPosition, abs.Entries[2].Kind);
        Assert.Equal(AacScaleFactorKind.None, abs.Entries[3].Kind);
    }

    [Fact]
    public void FromDelta_All_None_Stream_Produces_All_Zero_Values()
    {
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.None, 0, group: 1, sfb: 2),
            MakeEntry(AacScaleFactorKind.None, 0, group: 3, sfb: 4),
            MakeEntry(AacScaleFactorKind.None, 0, group: 5, sfb: 6));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 200);
        foreach (var e in abs.Entries)
        {
            Assert.Equal(0, e.Value);
            Assert.Equal(AacScaleFactorKind.None, e.Kind);
        }
    }

    [Fact]
    public void FromDelta_Preserves_GroupAndSfb_Across_All_Kinds()
    {
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +1, group: 0, sfb: 1),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +1, group: 1, sfb: 2),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +1, group: 2, sfb: 3),
            MakeEntry(AacScaleFactorKind.None, 0, group: 3, sfb: 4));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        for (int i = 0; i < abs.Entries.Count; i++)
        {
            Assert.Equal(i, abs.Entries[i].Group);
            Assert.Equal(i + 1, abs.Entries[i].Sfb);
        }
    }

    [Fact]
    public void FromDelta_None_Between_Pns_Bands_Does_Not_Reset_NoisePcmPending()
    {
        // First entry is None (PCM still pending), then PNS arrives:
        // 100 - 90 + 8 = 18, then 18 + 4 = 22 (PCM only on first PNS).
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.None, 0),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +8),
            MakeEntry(AacScaleFactorKind.NoiseEnergy, +4));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(0, abs.Entries[0].Value);
        Assert.Equal(18, abs.Entries[1].Value);
        Assert.Equal(22, abs.Entries[2].Value);
    }

    [Fact]
    public void FromDelta_Output_Is_Independent_Record()
    {
        var deltas = MakeData(MakeEntry(AacScaleFactorKind.SpectralGain, +1));
        var a = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        var b = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.NotSame(a, b);
        // Same content, but reference inequality (record equality is by entry list ref).
        Assert.Equal(a.Entries.Count, b.Entries.Count);
        Assert.Equal(a.Entries[0].Value, b.Entries[0].Value);
    }

    [Fact]
    public void FromDelta_Entry_With_Expression_Replaces_Value()
    {
        var entry = new AacAbsoluteScaleFactorEntry
        {
            Group = 0, Sfb = 0, Kind = AacScaleFactorKind.SpectralGain, Value = 100,
        };
        var modified = entry with { Value = 42 };
        Assert.Equal(100, entry.Value);
        Assert.Equal(42, modified.Value);
        Assert.NotEqual(entry, modified);
    }

    [Fact]
    public void FromDelta_LargeDifferential_Adds_Without_Throwing()
    {
        // No overflow guard on accumulator; very large diffs add fine.
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +1000),
            MakeEntry(AacScaleFactorKind.SpectralGain, -2000));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(1100, abs.Entries[0].Value);
        Assert.Equal(-900, abs.Entries[1].Value);
    }

    [Fact]
    public void FromDelta_None_Then_Intensity_Starts_Accumulator_From_Zero()
    {
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.None, 0),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +5),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +3));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(0, abs.Entries[0].Value);
        Assert.Equal(5, abs.Entries[1].Value);
        Assert.Equal(8, abs.Entries[2].Value);
    }

    [Fact]
    public void FromDelta_Output_Entries_Count_Matches_Input_Count()
    {
        var entries = new AacScaleFactorEntry[20];
        for (int i = 0; i < entries.Length; i++)
        {
            entries[i] = MakeEntry(AacScaleFactorKind.SpectralGain, +1, group: i % 4, sfb: i);
        }
        var abs = AacAbsoluteScaleFactors.FromDelta(MakeData(entries), globalGain: 100);
        Assert.Equal(20, abs.Entries.Count);
    }

    [Fact]
    public void FromDelta_Spectral_Followed_By_Intensity_Followed_By_Spectral_Holds_Accumulator()
    {
        // global_gain=100. Sequence: spec +5 → 105. is +20 → 20. spec -3 → 102.
        var deltas = MakeData(
            MakeEntry(AacScaleFactorKind.SpectralGain, +5),
            MakeEntry(AacScaleFactorKind.IntensityPosition, +20),
            MakeEntry(AacScaleFactorKind.SpectralGain, -3));
        var abs = AacAbsoluteScaleFactors.FromDelta(deltas, globalGain: 100);
        Assert.Equal(105, abs.Entries[0].Value);
        Assert.Equal(20, abs.Entries[1].Value);
        Assert.Equal(102, abs.Entries[2].Value);
    }
}
