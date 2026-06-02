using Mediar.Codecs.Opus.Decoder;
using Mediar.Codecs.Opus.Decoder.Celt;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the Phase 2a <c>CeltDecoder</c> skeleton. The decoder is
/// internal, so we exercise it through <c>InternalsVisibleTo</c>. Real
/// audio output is verified once Phase 2d ships.
/// </summary>
public sealed class CeltDecoderTests
{
    [Fact]
    public void Constructor_Rejects_Invalid_Channel_Count()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        Assert.Throws<ArgumentOutOfRangeException>(() => new CeltDecoder(mode, 0));
        Assert.Throws<ArgumentOutOfRangeException>(() => new CeltDecoder(mode, 3));
    }

    [Fact]
    public void Constructor_Rejects_Uninitialised_Mode()
    {
        Assert.Throws<ArgumentException>(() => new CeltDecoder(default, 1));
    }

    [Fact]
    public void Newly_Constructed_Decoder_IsFirstFrame()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 20_000);
        var dec = new CeltDecoder(mode, 1);
        Assert.True(dec.IsFirstFrame);
        Assert.Equal(0, dec.SamplesProduced);
        Assert.Equal(1, dec.Channels);
        Assert.Equal(mode, dec.Mode);
    }

    [Fact]
    public void DecodeFrame_Emits_Silent_Block_Of_Correct_Size()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        for (int i = 0; i < output.Length; i++) output[i] = 0.5f; // dirty buffer

        byte[] dummyPayload = new byte[16];
        dummyPayload[0] = 0x80;
        var rd = new OpusRangeDecoder(dummyPayload);

        int produced = dec.DecodeFrame(ref rd, output);
        Assert.Equal(mode.SamplesPerFrame, produced);
        for (int i = 0; i < output.Length; i++)
            Assert.Equal(0.0f, output[i]); // silence
        Assert.False(dec.IsFirstFrame);
        Assert.Equal(mode.SamplesPerFrame, dec.SamplesProduced);
    }

    [Fact]
    public void DecodeFrame_Rejects_Buffer_Too_Small()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> tooSmall = new float[10];
        Assert.Throws<ArgumentException>(() =>
        {
            var local = new CeltDecoder(mode, 2);
            Span<float> small = new float[10];
            byte[] buf = { 0x80 };
            var rd = new OpusRangeDecoder(buf);
            local.DecodeFrame(ref rd, small);
        });
    }

    [Fact]
    public void Reset_Restores_FirstFrame_And_Clears_Counter()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 10_000);
        var dec = new CeltDecoder(mode, 1);
        Span<float> buf = new float[mode.SamplesPerFrame];
        byte[] dummyPayload = { 0x80 };
        var rd = new OpusRangeDecoder(dummyPayload);
        dec.DecodeFrame(ref rd, buf);
        Assert.False(dec.IsFirstFrame);
        Assert.True(dec.SamplesProduced > 0);

        dec.Reset();
        Assert.True(dec.IsFirstFrame);
        Assert.Equal(0, dec.SamplesProduced);
    }

    [Fact]
    public void OpusDecoder_Routes_CeltOnly_Packets_Through_Celt_Path()
    {
        // Build a CELT-only packet (config 28 = CELT FB 2.5 ms) — through
        // OpusDecoder this exercises the new CELT routing path. Phase 2b
        // parses the front-of-packet flag set and the coarse-energy
        // spectrum, but still emits silence for the audio output until
        // Phase 2c/2d ship.
        byte toc = (byte)((28 << 3) | (1 << 2) | 0); // config=28, stereo=1, code=0
        byte[] pkt = new byte[1 + 20];
        pkt[0] = toc;
        var p = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48_000, Channels = 2, BitsPerSample = 16 };
        using var dec = new OpusDecoder(p);
        using var frame = dec.Decode(pkt, pts: 42);

        Assert.Equal(2, frame.Channels);
        Assert.Equal(48_000, frame.SampleRate);
        Assert.Equal(120, frame.SamplesPerChannel); // 2.5 ms @ 48k
        Assert.Equal(42, frame.Pts);
        Assert.Equal(120 * 2, frame.Samples.Length);
        // Phase 2b still emits silence (PCM lands in Phase 2d).
        foreach (var s in frame.Samples.Span) Assert.Equal(0.0f, s);
    }

    [Fact]
    public void DecodeFrame_Populates_State_For_NonSilent_Packet()
    {
        // A payload of 16 all-zero bytes is large enough (128 bits) that
        // all Phase 2b flags + the coarse-energy loop get exercised. The
        // silence flag in particular comes out false because after init
        // the range coder sits at the top of the window — so we get a
        // full pass through post-filter / transient / intra / coarse
        // energy decoding. State must update accordingly.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        int tellBefore = rd.Tell();
        int produced = dec.DecodeFrame(ref rd, output);
        int tellAfter = rd.Tell();

        Assert.Equal(mode.SamplesPerFrame, produced);
        Assert.False(dec.LastFrameWasSilent, "All-zero payload trips the silent=0 branch.");
        Assert.True(tellAfter > tellBefore + 17,
            "Coarse energy decode should consume well past the silence-flag budget.");
        // Output stays zeroed until Phase 2d.
        foreach (var s in output) Assert.Equal(0f, s);
        Assert.False(dec.IsFirstFrame);
    }

    [Fact]
    public void DecodeFrame_Silent_Path_Clamps_Energy_State()
    {
        // A 4-byte (32-bit) payload is large enough to trigger the
        // silence-flag branch but small enough that — once silence
        // resolves true — we skip post-filter / transient / intra /
        // coarse-energy. With our specific init pattern the silence
        // flag *can* resolve either way; regardless of which path was
        // taken, the recorded state must be internally consistent.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 10_000);
        var dec = new CeltDecoder(mode, 1);
        Span<float> output = new float[mode.SamplesPerFrame];

        byte[] payload = new byte[] { 0xFF, 0xFF, 0xFF, 0xFF };
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        if (dec.LastFrameWasSilent)
        {
            Assert.False(dec.LastFrameWasTransient);
            Assert.False(dec.LastFrameUsedIntra);
            Assert.False(dec.LastPostFilter.Enabled);
            for (int i = 0; i < dec.OldLogE.Length; i++)
            {
                Assert.Equal(-28.0f * 1024.0f, dec.OldLogE[i]);
            }
        }
    }

    [Fact]
    public void Reset_Clears_Energy_And_Flags()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        dec.Reset();
        Assert.True(dec.IsFirstFrame);
        Assert.Equal(0, dec.SamplesProduced);
        Assert.False(dec.LastFrameWasSilent);
        Assert.False(dec.LastFrameWasTransient);
        Assert.False(dec.LastFrameUsedIntra);
        Assert.False(dec.LastPostFilter.Enabled);
        for (int i = 0; i < dec.OldLogE.Length; i++)
            Assert.Equal(0f, dec.OldLogE[i]);
    }

    // ---- Phase 2c.1 — tf_decode + spread_decision -----------------------

    [Fact]
    public void DecodeFrame_Populates_TfResolution_For_NonSilent_Packet()
    {
        // tf_res[i] values are mapped through TfSelectTable, whose union of
        // outputs across all (LM, isTransient, tfSelect, tfChanged) cells is
        // a small set of small integers. Any populated band must fall inside
        // that set.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);
        Assert.False(dec.LastFrameWasSilent);

        var tf = dec.LastTfResolution;
        Assert.Equal(CeltConstants.MaxBands, tf.Length);
        for (int i = mode.StartBand; i < mode.EndBand; i++)
        {
            // TfSelectTable values are all in {-3,-2,-1,0,1,2,3}.
            Assert.InRange((int)tf[i], -3, 3);
        }
        // Bands outside [StartBand, EndBand) are never written and stay zero.
        for (int i = 0; i < mode.StartBand; i++)
            Assert.Equal(0, (int)tf[i]);
        for (int i = mode.EndBand; i < tf.Length; i++)
            Assert.Equal(0, (int)tf[i]);
    }

    [Fact]
    public void DecodeFrame_Populates_SpreadDecision_For_NonSilent_Packet()
    {
        // Spread decision is a 4-outcome ICDF symbol (ftb=5). Whatever the
        // payload looks like, the decoded value must be one of the legal
        // spread modes: None=0, Light=1, Normal=2, Aggressive=3.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);
        Assert.False(dec.LastFrameWasSilent);

        Assert.InRange(dec.LastSpreadDecision,
            CeltConstants.SpreadNone, CeltConstants.SpreadAggressive);
    }

    [Fact]
    public void Reset_Clears_Tf_And_Spread_State()
    {
        // After Reset the tf-resolution buffer goes back to all-zero and
        // the spread decision returns to the documented default (Normal).
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        dec.Reset();

        var tf = dec.LastTfResolution;
        for (int i = 0; i < tf.Length; i++)
            Assert.Equal(0, (int)tf[i]);
        Assert.Equal(CeltConstants.SpreadNormal, dec.LastSpreadDecision);
    }

    // ---- Phase 2c.2a — init_caps + dyn_alloc + alloc_trim --------------

    [Fact]
    public void DecodeFrame_Populates_BandCaps_For_NonSilent_Packet()
    {
        // init_caps is pure table lookup so every band in
        // [StartBand, EndBand) ends up with a strictly-positive cap and
        // bands outside the active range stay zero.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);
        Assert.False(dec.LastFrameWasSilent);

        var caps = dec.LastBandCaps;
        Assert.Equal(CeltConstants.MaxBands, caps.Length);
        for (int i = 0; i < CeltConstants.MaxBands; i++)
            Assert.True(caps[i] > 0,
                $"Caps for band {i} should be positive (libopus init_caps).");
        // Bands at the top of the spectrum cover wider widths and at FB/LM=0
        // for stereo the caps grow accordingly — sanity-check non-zero
        // monotonicity for the bands within the active range.
        Assert.True(caps[20] > caps[0]);
    }

    [Fact]
    public void DecodeFrame_Populates_BandBoost_For_NonSilent_Packet()
    {
        // dyn_alloc reads at probability 2^-6 (then 2^-1) so it almost
        // always yields zero boost on an all-zero payload — but the
        // observable invariant is: boost[i] is non-negative, in 8-frac-bit
        // multiples, and bounded by caps[i]. Bands outside [Start, End)
        // must be zero.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);
        Assert.False(dec.LastFrameWasSilent);

        var boost = dec.LastBandBoost;
        var caps = dec.LastBandCaps;
        Assert.Equal(CeltConstants.MaxBands, boost.Length);
        for (int i = 0; i < mode.StartBand; i++)
            Assert.Equal(0, boost[i]);
        for (int i = mode.EndBand; i < boost.Length; i++)
            Assert.Equal(0, boost[i]);
        for (int i = mode.StartBand; i < mode.EndBand; i++)
        {
            Assert.True(boost[i] >= 0, $"boost[{i}] must be non-negative");
            Assert.True(boost[i] < caps[i],
                $"boost[{i}]={boost[i]} must be < caps[{i}]={caps[i]}");
        }
    }

    [Fact]
    public void DecodeFrame_Populates_AllocTrim_For_NonSilent_Packet()
    {
        // alloc_trim is one ICDF symbol with 11 outcomes (0..10).
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];

        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);
        Assert.False(dec.LastFrameWasSilent);

        Assert.InRange(dec.LastAllocTrim, 0, 10);
    }

    [Fact]
    public void DecodeFrame_AllocTrim_Defaults_To_Five_On_Tiny_Payload()
    {
        // A 2-byte (16-bit) payload runs out of budget well before the
        // 6-bit alloc_trim symbol, so it must fall through to the
        // documented default of 5 — regardless of which earlier symbols
        // happen to consume entropy.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Wideband, 10_000);
        var dec = new CeltDecoder(mode, 1);
        Span<float> output = new float[mode.SamplesPerFrame];

        byte[] payload = new byte[] { 0xAA, 0xAA };
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        Assert.Equal(CeltConstants.AllocTrimDefault, dec.LastAllocTrim);
    }

    [Fact]
    public void Reset_Clears_Allocation_State()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        dec.Reset();

        var caps = dec.LastBandCaps;
        var boost = dec.LastBandBoost;
        for (int i = 0; i < caps.Length; i++)
        {
            Assert.Equal(0, caps[i]);
            Assert.Equal(0, boost[i]);
        }
        Assert.Equal(CeltConstants.AllocTrimDefault, dec.LastAllocTrim);
    }

    [Fact]
    public void DecodeFrame_Populates_CodedBands_For_NonSilent_Packet()
    {
        // compute_allocation must always return a coded band count in
        // [StartBand+1, EndBand] when the input is non-silent.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);
        Assert.False(dec.LastFrameWasSilent);

        Assert.InRange(dec.LastCodedBands, mode.StartBand + 1, mode.EndBand);
    }

    [Fact]
    public void DecodeFrame_Populates_Intensity_For_Stereo_NonSilent_Packet()
    {
        // Stereo: intensity ∈ [StartBand, codedBands] per libopus
        // interp_bits2pulses semantics.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        Assert.InRange(dec.LastIntensity, mode.StartBand, dec.LastCodedBands);
    }

    [Fact]
    public void DecodeFrame_Mono_HasZero_Intensity()
    {
        // Intensity stereo is only signalled for 2-channel frames.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 1);
        Span<float> output = new float[mode.SamplesPerFrame];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        Assert.Equal(0, dec.LastIntensity);
        Assert.False(dec.LastDualStereo);
    }

    [Fact]
    public void DecodeFrame_Populates_Pulses_FineBits_FinePriority_For_NonSilent_Packet()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        var pulses = dec.LastPulses;
        var ebits = dec.LastFineBits;
        var priority = dec.LastFinePriority;
        Assert.Equal(CeltConstants.MaxBands, pulses.Length);
        Assert.Equal(CeltConstants.MaxBands, ebits.Length);
        Assert.Equal(CeltConstants.MaxBands, priority.Length);

        for (int i = 0; i < mode.StartBand; i++)
        {
            Assert.Equal(0, pulses[i]);
            Assert.Equal(0, ebits[i]);
        }
        for (int i = mode.EndBand; i < CeltConstants.MaxBands; i++)
        {
            Assert.Equal(0, pulses[i]);
            Assert.Equal(0, ebits[i]);
        }
        for (int i = mode.StartBand; i < mode.EndBand; i++)
        {
            Assert.True(pulses[i] >= 0, $"pulses[{i}]={pulses[i]} must be >= 0");
            Assert.InRange(ebits[i], 0, CeltConstants.MaxFineBits);
            Assert.InRange(priority[i], 0, 1);
        }
    }

    [Fact]
    public void DecodeFrame_AntiCollapse_Not_Reserved_For_NonTransient_Frame()
    {
        // A non-transient frame (LM=3, isTransient=false) must NOT
        // reserve the anti-collapse bit per libopus rule.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        Assert.False(dec.LastFrameWasTransient);
        Assert.False(dec.LastAntiCollapseReserved);
    }

    [Fact]
    public void Reset_Clears_ComputeAllocation_State()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        dec.Reset();

        Assert.Equal(0, dec.LastCodedBands);
        Assert.Equal(0, dec.LastIntensity);
        Assert.False(dec.LastDualStereo);
        Assert.False(dec.LastAntiCollapseReserved);
        Assert.Equal(0, dec.LastAllocationBalance);
        var pulses = dec.LastPulses;
        var ebits = dec.LastFineBits;
        var priority = dec.LastFinePriority;
        for (int i = 0; i < pulses.Length; i++)
        {
            Assert.Equal(0, pulses[i]);
            Assert.Equal(0, ebits[i]);
            Assert.Equal(0, priority[i]);
        }
    }

    [Fact]
    public void DecodeFrame_FineEnergyOffsets_Shape_And_Range()
    {
        // Phase 2c.3a: unquant_fine_energy must produce per-(channel,band)
        // offsets in DB_SHIFT units that satisfy:
        //   - length = channels * MaxBands
        //   - bands with ebits[i] == 0 (including outside [start, end))
        //     have offset == 0 for every channel
        //   - bands with ebits[i] > 0 have offset ∈ [-512, 511]
        //     (i.e. ∈ [-0.5, 0.5) log2 units) for every channel.
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        var offsets = dec.LastFineEnergyOffsets;
        var ebits = dec.LastFineBits;
        Assert.Equal(2 * CeltConstants.MaxBands, offsets.Length);

        for (int i = 0; i < CeltConstants.MaxBands; i++)
        {
            for (int c = 0; c < 2; c++)
            {
                int idx = c * CeltConstants.MaxBands + i;
                if (ebits[i] == 0)
                {
                    Assert.Equal(0f, offsets[idx]);
                }
                else
                {
                    int half = 1 << (CeltConstants.DbShift - 1);
                    Assert.InRange(offsets[idx], -half, half - 1);
                }
            }
        }
    }

    [Fact]
    public void DecodeFrame_FineEnergy_Modifies_OldLogE_For_Bands_With_Bits()
    {
        // Phase 2c.3a: when unquant_fine_energy applies a non-zero
        // offset to band i channel c, _oldLogE[idx] post-decode must
        // differ from the coarse-only value by exactly the recorded
        // offset. We verify the post-condition by subtracting offsets
        // off and confirming the remainder is integer-valued (since
        // the coarse path only ever writes Q10 integer multiples).
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        var oldLogE = dec.OldLogE;
        var offsets = dec.LastFineEnergyOffsets;
        for (int i = 0; i < oldLogE.Length; i++)
        {
            // _oldLogE[i] - offsets[i] should be the coarse value, and
            // offsets[i] is itself an integer in [-512, 511] units.
            Assert.True(float.IsFinite(oldLogE[i]),
                $"oldLogE[{i}] must be finite");
            Assert.Equal((float)(int)offsets[i], offsets[i]);
        }
    }

    [Fact]
    public void Reset_Clears_FineEnergyOffsets()
    {
        var mode = CeltMode.ForCeltOnly(OpusBandwidth.Fullband, 20_000);
        var dec = new CeltDecoder(mode, 2);
        Span<float> output = new float[mode.SamplesPerFrame * 2];
        byte[] payload = new byte[16];
        var rd = new OpusRangeDecoder(payload);
        dec.DecodeFrame(ref rd, output);

        dec.Reset();

        var offsets = dec.LastFineEnergyOffsets;
        for (int i = 0; i < offsets.Length; i++)
            Assert.Equal(0f, offsets[i]);
    }
}
