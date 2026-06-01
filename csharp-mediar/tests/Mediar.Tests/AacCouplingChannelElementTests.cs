using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AacCouplingChannelElementTests
{
    // Synthetic 121-symbol scale-factor codebook reused from the SF / ICS tests:
    //   symbol 60 (diff 0) -> "0" (1 bit)
    //   all other symbols  -> 8-bit fixed-length codes starting at 0x80
    private static AacHuffmanCodebook BuildSyntheticSfCodebook()
    {
        var lengths = new int[121];
        for (int i = 0; i < 121; i++) lengths[i] = i == 60 ? 1 : 8;
        return AacHuffmanCodebook.FromCanonicalLengths(lengths);
    }

    private static (uint bits, int length) EncodeSfSymbol(int symbol)
    {
        if (symbol == 60) return (0u, 1);
        int position = symbol < 60 ? symbol : symbol - 1;
        return ((uint)(0x80 + position), 8);
    }

    private static void WriteSfSymbol(AacBitWriter w, int symbol)
    {
        var (bits, len) = EncodeSfSymbol(symbol);
        w.Write(bits, len);
    }

    private static void WriteLongIcsInfo(AacBitWriter w, int maxSfb)
    {
        w.Write(0u, 1);             // ics_reserved_bit
        w.Write(0u, 2);             // window_sequence = ONLY_LONG
        w.Write(0u, 1);             // window_shape = Sine
        w.Write((uint)maxSfb, 6);   // max_sfb
        w.Write(0u, 1);             // predictor_data_present = 0
    }

    private static void WriteZeroSection(AacBitWriter w, int len)
    {
        w.Write(0u, 4);             // sect_cb = 0 (ZERO_HCB)
        w.Write((uint)len, 5);      // sect_len_incr
    }

    private static void WriteSection(AacBitWriter w, int cb, int len)
    {
        w.Write((uint)cb, 4);       // sect_cb
        w.Write((uint)len, 5);      // sect_len_incr
    }

    /// <summary>
    /// Writes an ICS body whose sections are entirely ZERO_HCB - no
    /// scale-factor reads, no spectral coefficients consumed. Useful as
    /// the trailing body of CCE / SCE / CPE element fixtures.
    /// </summary>
    private static void WriteEmptyLongIcsBody(AacBitWriter w, int maxSfb)
    {
        w.Write(0x80u, 8);           // global_gain
        WriteLongIcsInfo(w, maxSfb);
        WriteZeroSection(w, len: maxSfb);
        // scale_factor_data: empty (cb=0)
        w.Write(0u, 1);              // pulse_data_present
        w.Write(0u, 1);              // tns_data_present
        w.Write(0u, 1);              // gain_control_data_present
    }

    [Fact]
    public void TryParse_SingleSceTarget_NoGainLists_Parses()
    {
        // 1 SCE target -> num_gain_element_lists = 1 -> no explicit gain lists.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(4u, 4);              // element_instance_tag = 4
        w.Write(0u, 1);              // ind_sw_cce_flag = 0
        w.Write(0u, 3);              // num_coupled_elements = 0 -> 1 target
        // Target 0: SCE, tag 2
        w.Write(0u, 1);              // cc_target_is_cpe[0] = 0
        w.Write(2u, 4);              // cc_target_tag_select[0] = 2
        // Coupling framing fields
        w.Write(0u, 1);              // cc_domain
        w.Write(0u, 1);              // gain_element_sign
        w.Write(0u, 2);              // gain_element_scale
        // ICS body (all ZERO_HCB)
        WriteEmptyLongIcsBody(w, maxSfb: 10);

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.Equal(4, cce!.ElementInstanceTag);
        Assert.False(cce.IndependentSwitchedCceFlag);
        Assert.Single(cce.Targets);
        Assert.False(cce.Targets[0].IsChannelPairElement);
        Assert.Equal(2, cce.Targets[0].TargetTagSelect);
        Assert.False(cce.Targets[0].CcLeft);
        Assert.False(cce.Targets[0].CcRight);
        Assert.False(cce.Targets[0].ContributesExtraGainList);
        Assert.False(cce.CcDomain);
        Assert.False(cce.GainElementSign);
        Assert.Equal(0, cce.GainElementScale);
        Assert.Empty(cce.GainLists);
        Assert.Equal(1, cce.NumGainElementLists);
        // 4 + 1 + 3 (header) + 5 (target) + 4 (domain/sign/scale) + 31 (ICS) = 48 bits
        Assert.Equal(48, cce.BitsConsumed);
    }

    [Fact]
    public void TryParse_TwoCpeTargets_IndSwSet_ImpliedCommonGains_Parses()
    {
        // num_coupled_elements = 1 -> 2 targets. Both CPE with cc_l = cc_r = 1
        // -> num_gain_element_lists = 2 + 2 (extras) = 4. With ind_sw_cce_flag = 1
        // each of the 3 explicit lists carries an implied common gain element.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(7u, 4);              // element_instance_tag = 7
        w.Write(1u, 1);              // ind_sw_cce_flag = 1
        w.Write(1u, 3);              // num_coupled_elements = 1 -> 2 targets
        // Target 0: CPE, tag 0, ccL = 1, ccR = 1
        w.Write(1u, 1);              // cc_target_is_cpe[0]
        w.Write(0u, 4);              // cc_target_tag_select[0]
        w.Write(1u, 1);              // cc_l[0]
        w.Write(1u, 1);              // cc_r[0]
        // Target 1: CPE, tag 1, ccL = 1, ccR = 1
        w.Write(1u, 1);              // cc_target_is_cpe[1]
        w.Write(1u, 4);              // cc_target_tag_select[1]
        w.Write(1u, 1);              // cc_l[1]
        w.Write(1u, 1);              // cc_r[1]
        // Framing
        w.Write(1u, 1);              // cc_domain
        w.Write(0u, 1);              // gain_element_sign
        w.Write(2u, 2);              // gain_element_scale
        WriteEmptyLongIcsBody(w, maxSfb: 5);
        // 3 implied common gain symbols (lists 1..3), each = symbol 60 (1 bit)
        WriteSfSymbol(w, 60);
        WriteSfSymbol(w, 60);
        WriteSfSymbol(w, 60);

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.True(cce!.IndependentSwitchedCceFlag);
        Assert.Equal(2, cce.Targets.Count);
        Assert.All(cce.Targets, t =>
        {
            Assert.True(t.IsChannelPairElement);
            Assert.True(t.CcLeft);
            Assert.True(t.CcRight);
            Assert.True(t.ContributesExtraGainList);
        });
        Assert.True(cce.CcDomain);
        Assert.False(cce.GainElementSign);
        Assert.Equal(2, cce.GainElementScale);
        Assert.Equal(4, cce.NumGainElementLists);
        Assert.Equal(3, cce.GainLists.Count);
        foreach (var gl in cce.GainLists)
        {
            Assert.True(gl.CommonGainElementPresent);
            Assert.Equal(0, gl.CommonGainDifferential);
            Assert.Empty(gl.DpcmGains);
        }
    }

    [Fact]
    public void TryParse_CpeBothCoupledTarget_TransmittedCgeFlag_PerListCommonGain()
    {
        // 1 CPE target with ccL = ccR = 1 -> 2 gain lists (index 1 explicit).
        // ind_sw_cce_flag = 0 -> cge_present bit is transmitted (we set 1).
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);              // element_instance_tag = 0
        w.Write(0u, 1);              // ind_sw_cce_flag = 0
        w.Write(0u, 3);              // num_coupled_elements = 0 -> 1 target
        // Target 0: CPE, tag 3, ccL = ccR = 1 -> contributes extra gain list
        w.Write(1u, 1);              // cc_target_is_cpe
        w.Write(3u, 4);              // cc_target_tag_select
        w.Write(1u, 1);              // cc_l
        w.Write(1u, 1);              // cc_r
        // Framing
        w.Write(0u, 1);              // cc_domain
        w.Write(1u, 1);              // gain_element_sign
        w.Write(3u, 2);              // gain_element_scale = 3
        WriteEmptyLongIcsBody(w, maxSfb: 5);
        // Gain list 1: cge_present = 1, common gain symbol 75 (diff +15)
        w.Write(1u, 1);              // cge_present
        WriteSfSymbol(w, 75);

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.False(cce!.IndependentSwitchedCceFlag);
        Assert.Single(cce.Targets);
        Assert.True(cce.Targets[0].ContributesExtraGainList);
        Assert.True(cce.GainElementSign);
        Assert.Equal(3, cce.GainElementScale);
        Assert.Equal(2, cce.NumGainElementLists);
        Assert.Single(cce.GainLists);
        Assert.True(cce.GainLists[0].CommonGainElementPresent);
        Assert.Equal(15, cce.GainLists[0].CommonGainDifferential);
        Assert.Empty(cce.GainLists[0].DpcmGains);
    }

    [Fact]
    public void TryParse_DpcmGainList_PerNonZeroSfb()
    {
        // 1 CPE-both target -> 2 lists. ind_sw_cce_flag = 0, cge_present = 0
        // for list 1 -> per-(g, sfb) dpcm gain for every section whose cb != 0.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);              // tag
        w.Write(0u, 1);              // ind_sw_cce_flag
        w.Write(0u, 3);              // num_coupled_elements = 0
        // Target 0: CPE-both
        w.Write(1u, 1); w.Write(3u, 4); w.Write(1u, 1); w.Write(1u, 1);
        // Framing
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        // ICS body with two sections: cb=0 [0,1) + cb=1 [1,3).
        w.Write(0x80u, 8);           // global_gain
        WriteLongIcsInfo(w, maxSfb: 3);
        // section_data: cb=0 sfb 0..1 (skip SF), cb=1 sfb 1..3 (2 SF reads)
        WriteZeroSection(w, len: 1);
        WriteSection(w, cb: 1, len: 2);
        // scale_factor_data: two diff-zero entries for cb=1 sfbs (sfb 1, sfb 2)
        WriteSfSymbol(w, 60);
        WriteSfSymbol(w, 60);
        // Optional flags
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 1);
        // Gain list 1: cge_present = 0, then 2 dpcm symbols (one per cb=1 sfb).
        w.Write(0u, 1);              // cge_present
        WriteSfSymbol(w, 70);        // diff +10 for (g=0, sfb=1)
        WriteSfSymbol(w, 50);        // diff -10 for (g=0, sfb=2)

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.Single(cce!.GainLists);
        var list = cce.GainLists[0];
        Assert.False(list.CommonGainElementPresent);
        Assert.Null(list.CommonGainDifferential);
        Assert.Equal(2, list.DpcmGains.Count);
        Assert.Equal((0, 1, +10), (list.DpcmGains[0].Group, list.DpcmGains[0].Sfb, list.DpcmGains[0].Differential));
        Assert.Equal((0, 2, -10), (list.DpcmGains[1].Group, list.DpcmGains[1].Sfb, list.DpcmGains[1].Differential));
    }

    [Fact]
    public void TryParse_DpcmGainList_AllZeroSections_EmitsNoGains()
    {
        // Per-band dpcm gain list with ICS body that is entirely ZERO_HCB -
        // every sfb is filtered out, so the list ends up with zero entries.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(0u, 1);
        w.Write(0u, 3);
        // CPE-both target
        w.Write(1u, 1); w.Write(0u, 4); w.Write(1u, 1); w.Write(1u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        WriteEmptyLongIcsBody(w, maxSfb: 5);
        // Gain list 1: cge_present = 0; no symbols are read because every
        // section uses ZERO_HCB.
        w.Write(0u, 1);              // cge_present

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.Single(cce!.GainLists);
        var list = cce.GainLists[0];
        Assert.False(list.CommonGainElementPresent);
        Assert.Empty(list.DpcmGains);
    }

    [Fact]
    public void TryParse_NumCoupledElementsMax_AllSceTargets_Parses()
    {
        // num_coupled_elements = 7 -> 8 SCE targets, ind_sw_cce_flag = 0.
        // num_gain_element_lists = 8 (one per SCE, no extras) -> 7 explicit lists.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(0u, 1);
        w.Write(7u, 3);              // num_coupled_elements = 7
        for (int c = 0; c < 8; c++)
        {
            w.Write(0u, 1);          // isCpe = 0
            w.Write((uint)c, 4);     // tag = c
        }
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        WriteEmptyLongIcsBody(w, maxSfb: 5);
        // 7 explicit lists each with cge_present = 1 (transmitted) + symbol 60.
        for (int c = 1; c < 8; c++)
        {
            w.Write(1u, 1);          // cge_present
            WriteSfSymbol(w, 60);
        }

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.Equal(8, cce!.Targets.Count);
        Assert.All(cce.Targets, t => Assert.False(t.IsChannelPairElement));
        Assert.Equal(8, cce.NumGainElementLists);
        Assert.Equal(7, cce.GainLists.Count);
        Assert.All(cce.GainLists, gl =>
        {
            Assert.True(gl.CommonGainElementPresent);
            Assert.Equal(0, gl.CommonGainDifferential);
        });
    }

    [Fact]
    public void TryParse_CpeTarget_OnlyOneCcChannel_NoExtraGainList()
    {
        // CPE target with only one of cc_l / cc_r set -> no extra gain list.
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4);
        w.Write(0u, 1);
        w.Write(0u, 3);              // 1 target
        // CPE target, ccL = 1, ccR = 0 -> ContributesExtraGainList = false
        w.Write(1u, 1); w.Write(0u, 4); w.Write(1u, 1); w.Write(0u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        WriteEmptyLongIcsBody(w, maxSfb: 5);

        Assert.True(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.NotNull(cce);
        Assert.True(cce!.Targets[0].IsChannelPairElement);
        Assert.True(cce.Targets[0].CcLeft);
        Assert.False(cce.Targets[0].CcRight);
        Assert.False(cce.Targets[0].ContributesExtraGainList);
        Assert.Equal(1, cce.NumGainElementLists);
        Assert.Empty(cce.GainLists);
    }

    [Fact]
    public void TryParse_HeaderUnderflow_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        // Less than the 8 header bits (4 + 1 + 3) needed before reading targets.
        Assert.False(AacCouplingChannelElement.TryParse(new byte[] { 0x00 }, book, out var cce));
        Assert.Null(cce);
    }

    [Fact]
    public void TryParse_TargetUnderflow_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        // header says 1 target, but the buffer ends before the target's 5 bits.
        w.Write(0u, 4);
        w.Write(0u, 1);
        w.Write(0u, 3);              // num_coupled_elements = 0 -> 1 target
        // No target bits written.
        var bytes = w.ToArray(); // 1 byte = 8 bits

        Assert.False(AacCouplingChannelElement.TryParse(bytes, book, out var cce));
        Assert.Null(cce);
    }

    [Fact]
    public void TryParse_FramingFieldsUnderflow_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4); w.Write(0u, 1); w.Write(0u, 3); // header (8 bits)
        w.Write(0u, 1); w.Write(0u, 4);                 // SCE target (5 bits) - 13 bits total
        // No cc_domain/sign/scale (4 bits) and no ICS body.
        var bytes = w.ToArray();

        Assert.False(AacCouplingChannelElement.TryParse(bytes, book, out var cce));
        Assert.Null(cce);
    }

    [Fact]
    public void TryParse_IcsBodyUnderflow_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4); w.Write(0u, 1); w.Write(0u, 3); // header
        w.Write(0u, 1); w.Write(0u, 4);                 // SCE target
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2); // framing
        // Truncated ICS body - only the global_gain byte.
        w.Write(0u, 8);

        Assert.False(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.Null(cce);
    }

    [Fact]
    public void TryParse_GainListUnderflow_Rejected()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4); w.Write(0u, 1); w.Write(0u, 3); // header (1 SCE target)
        // CPE-both target so num_gain_element_lists = 2
        w.Write(1u, 1); w.Write(0u, 4); w.Write(1u, 1); w.Write(1u, 1);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        WriteEmptyLongIcsBody(w, maxSfb: 5);
        // ind_sw = 0 so we expect a cge_present bit, but the buffer ends here.
        // (Slice to byte-align so we don't accidentally read trailing zeros.)
        var bytes = w.ToArray();
        // Drop the implicit padding by trimming the buffer to exactly the bits
        // already written - 5 bytes here. The CCE parser will read the
        // gain-list byte boundary and find no bits left.
        Assert.False(AacCouplingChannelElement.TryParse(bytes.AsSpan(0, bytes.Length - 1).ToArray(), book, out var cce));
        Assert.Null(cce);
    }

    [Fact]
    public void TryParse_CodebookCapacityMismatch_Rejected()
    {
        // 100-symbol codebook can't decode the 121-symbol scale-factor space.
        var lengths = new int[100];
        for (int i = 0; i < 100; i++) lengths[i] = 8;
        var book = AacHuffmanCodebook.FromCanonicalLengths(lengths);
        var w = new AacBitWriter();
        w.Write(0u, 4); w.Write(0u, 1); w.Write(0u, 3);
        w.Write(0u, 1); w.Write(0u, 4);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        WriteEmptyLongIcsBody(w, maxSfb: 5);

        Assert.False(AacCouplingChannelElement.TryParse(w.ToArray(), book, out var cce));
        Assert.Null(cce);
    }

    [Fact]
    public void TryParse_NullCodebook_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            AacCouplingChannelElement.TryParse(new byte[] { 0 }, null!, out _));
    }

    [Fact]
    public void TryRead_AfterParse_PositionsReaderPastBody()
    {
        var book = BuildSyntheticSfCodebook();
        var w = new AacBitWriter();
        w.Write(0u, 4); w.Write(0u, 1); w.Write(0u, 3);
        w.Write(0u, 1); w.Write(2u, 4);
        w.Write(0u, 1); w.Write(0u, 1); w.Write(0u, 2);
        WriteEmptyLongIcsBody(w, maxSfb: 10);
        // Trailing payload: 11 bits of spectral_data() the CCE walker must not
        // touch. We append a recognisable byte (0xAB = 1010 1011) and check
        // BitsConsumed sits exactly at the CCE end.
        w.Write(0xABu, 8);
        var bytes = w.ToArray();

        var reader = new BitReader(bytes);
        Assert.True(AacCouplingChannelElement.TryRead(ref reader, book, out var cce));
        Assert.NotNull(cce);
        // Header(8) + target(5) + framing(4) + ICS(31) = 48
        Assert.Equal(48, cce!.BitsConsumed);
        Assert.Equal(48, reader.Position);
        // The 0xAB padding byte is still readable from the reader.
        Assert.Equal(0xABu, reader.ReadBits(8));
    }
}
