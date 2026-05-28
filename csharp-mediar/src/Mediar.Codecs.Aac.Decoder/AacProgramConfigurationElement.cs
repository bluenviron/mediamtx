using System.Collections.ObjectModel;
using System.Text;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// A channel slot inside an AAC <see cref="AacProgramConfigurationElement"/>.
/// Each front / side / back slot is either a Single Channel Element (SCE,
/// one channel) or a Channel Pair Element (CPE, stereo pair), selected by
/// <see cref="IsCpe"/>. <see cref="TagSelect"/> is the 4-bit
/// element_instance_tag this slot binds to inside the raw_data_block stream.
/// </summary>
public readonly record struct AacPceChannelSlot
{
    /// <summary>True when this slot carries a Channel Pair Element (stereo).</summary>
    public required bool IsCpe { get; init; }

    /// <summary>4-bit element_instance_tag this slot resolves to.</summary>
    public required int TagSelect { get; init; }

    /// <summary>Channels rendered for this slot (1 for SCE, 2 for CPE).</summary>
    public int ChannelCount => IsCpe ? 2 : 1;
}

/// <summary>
/// A coupling-channel slot inside an AAC <see cref="AacProgramConfigurationElement"/>.
/// </summary>
public readonly record struct AacPceCouplingSlot
{
    /// <summary>True when the coupling channel applies before (independent) the TNS / inverse-quantisation stage.</summary>
    public required bool IsIndependentlySwitched { get; init; }

    /// <summary>4-bit element_instance_tag this coupling slot binds to.</summary>
    public required int TagSelect { get; init; }
}

/// <summary>
/// Optional matrix-mixdown descriptor inside an AAC
/// <see cref="AacProgramConfigurationElement"/>. Present only when
/// <c>matrix_mixdown_idx_present</c> is set in the bitstream.
/// </summary>
public readonly record struct AacPceMatrixMixdown
{
    /// <summary>2-bit matrix_mixdown_idx (encoder downmix coefficient set).</summary>
    public required int Index { get; init; }

    /// <summary>True when <c>pseudo_surround_enable</c> is signalled.</summary>
    public required bool PseudoSurroundEnable { get; init; }
}

/// <summary>
/// Parsed MPEG-4 AAC program_config_element (ISO/IEC 14496-3 Table 4.5),
/// the variable-length structure that describes a custom channel layout when
/// <c>channelConfiguration</c> is 0. Carries front / side / back / LFE /
/// associated-data / coupling-channel slots, optional mono / stereo / matrix
/// mixdowns, plus an ISO 8859-1 comment field. Used by the AAC raw_data_block
/// dispatcher (PCE element, id = 5) and emitted by MP4 demuxers that store a
/// PCE blob in the <c>esds</c> DecoderSpecificInfo for non-standard layouts.
/// </summary>
public sealed record AacProgramConfigurationElement
{
    /// <summary>4-bit <c>element_instance_tag</c>.</summary>
    public required int ElementInstanceTag { get; init; }

    /// <summary>2-bit <c>object_type</c> field (AAC profile - 1; AOT = field + 1).</summary>
    public required int ObjectType { get; init; }

    /// <summary>4-bit <c>sampling_frequency_index</c> (no inline-rate escape — PCE is locked to the table).</summary>
    public required int SamplingFrequencyIndex { get; init; }

    /// <summary>Front speaker slots in front-to-back, left-to-right order.</summary>
    public required IReadOnlyList<AacPceChannelSlot> FrontElements { get; init; }

    /// <summary>Side speaker slots.</summary>
    public required IReadOnlyList<AacPceChannelSlot> SideElements { get; init; }

    /// <summary>Back speaker slots.</summary>
    public required IReadOnlyList<AacPceChannelSlot> BackElements { get; init; }

    /// <summary>LFE channel element_instance_tag values (each is a single channel).</summary>
    public required IReadOnlyList<int> LfeElements { get; init; }

    /// <summary>Associated-data element_instance_tag values (auxiliary, not rendered).</summary>
    public required IReadOnlyList<int> AssocDataElements { get; init; }

    /// <summary>Coupling-channel slots.</summary>
    public required IReadOnlyList<AacPceCouplingSlot> CouplingElements { get; init; }

    /// <summary>Mono mixdown element_instance_tag, or null when mono_mixdown_present is 0.</summary>
    public int? MonoMixdownElementNumber { get; init; }

    /// <summary>Stereo mixdown element_instance_tag, or null when stereo_mixdown_present is 0.</summary>
    public int? StereoMixdownElementNumber { get; init; }

    /// <summary>Matrix mixdown descriptor, or null when matrix_mixdown_idx_present is 0.</summary>
    public AacPceMatrixMixdown? MatrixMixdown { get; init; }

    /// <summary>Encoder comment string (ISO 8859-1, up to 255 bytes).</summary>
    public required string CommentField { get; init; }

    /// <summary>Strongly-typed AOT view (AOT = <see cref="ObjectType"/> + 1).</summary>
    public AacAudioObjectType ObjectTypeEnum => (AacAudioObjectType)(ObjectType + 1);

    /// <summary>Resolved sample rate in Hz, or 0 when the index is reserved.</summary>
    public int SamplingFrequency => AacSampleRates.FromIndex(SamplingFrequencyIndex);

    /// <summary>Total rendered speaker count (front + side + back CPEs count as 2 each, LFEs as 1 each).</summary>
    public int SpeakerCount =>
        ChannelSum(FrontElements) + ChannelSum(SideElements) + ChannelSum(BackElements) + LfeElements.Count;

    private static int ChannelSum(IReadOnlyList<AacPceChannelSlot> slots)
    {
        int sum = 0;
        for (int i = 0; i < slots.Count; i++) sum += slots[i].ChannelCount;
        return sum;
    }

    /// <summary>
    /// Parse a standalone PCE blob. <paramref name="data"/> must start on a
    /// byte boundary with the 4-bit <c>element_instance_tag</c> (the
    /// raw_data_block dispatcher strips the 3-bit element id before calling
    /// this method). Returns false on truncation or out-of-range comment
    /// length.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AacProgramConfigurationElement? pce)
        => TryParse(data, out pce, out _);

    /// <summary>
    /// Parse a standalone PCE blob and report how many bytes were consumed.
    /// Because PCE ends with <c>byte_alignment()</c> followed by 8-bit-only
    /// fields, the byte count is always exact.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AacProgramConfigurationElement? pce, out int bytesConsumed)
    {
        pce = null;
        bytesConsumed = 0;
        if (data.IsEmpty) return false;

        try
        {
            var reader = new BitReader(data);
            if (!TryRead(ref reader, out pce)) return false;
            bytesConsumed = (reader.Position + 7) >> 3;
            return true;
        }
        catch (EndOfStreamException)
        {
            pce = null;
            bytesConsumed = 0;
            return false;
        }
        catch (ArgumentOutOfRangeException)
        {
            pce = null;
            bytesConsumed = 0;
            return false;
        }
    }

    /// <summary>
    /// Serialise this PCE back to the standalone byte layout produced by
    /// <see cref="TryParse(System.ReadOnlySpan{byte}, out AacProgramConfigurationElement?)"/>.
    /// Trailing bits inside the final byte are zero-padded.
    /// </summary>
    public byte[] ToBytes()
    {
        var writer = new BitWriter();
        WriteTo(writer);
        return writer.ToArray();
    }

    internal static bool TryRead(ref BitReader reader, out AacProgramConfigurationElement? pce)
    {
        pce = null;
        if (reader.Remaining < 4 + 2 + 4 + 4 + 4 + 4 + 2 + 3 + 4 + 3) return false;

        int tag = (int)reader.ReadBits(4);
        int objectType = (int)reader.ReadBits(2);
        int sfIndex = (int)reader.ReadBits(4);
        if (sfIndex == AacSampleRates.EscapeIndex) return false; // PCE has no inline-rate escape

        int numFront = (int)reader.ReadBits(4);
        int numSide = (int)reader.ReadBits(4);
        int numBack = (int)reader.ReadBits(4);
        int numLfe = (int)reader.ReadBits(2);
        int numAssoc = (int)reader.ReadBits(3);
        int numCc = (int)reader.ReadBits(4);

        int? monoMixdown = ReadOptionalTag(ref reader);
        int? stereoMixdown = ReadOptionalTag(ref reader);
        AacPceMatrixMixdown? matrixMixdown = null;
        if (reader.ReadBit())
        {
            int idx = (int)reader.ReadBits(2);
            bool pseudo = reader.ReadBit();
            matrixMixdown = new AacPceMatrixMixdown { Index = idx, PseudoSurroundEnable = pseudo };
        }

        var front = ReadChannelSlots(ref reader, numFront);
        var side = ReadChannelSlots(ref reader, numSide);
        var back = ReadChannelSlots(ref reader, numBack);
        var lfe = ReadTagList(ref reader, numLfe);
        var assoc = ReadTagList(ref reader, numAssoc);
        var coupling = ReadCouplingSlots(ref reader, numCc);

        reader.AlignToByte();

        if (reader.Remaining < 8) return false;
        int commentBytes = (int)reader.ReadBits(8);
        if (reader.Remaining < commentBytes * 8) return false;

        Span<byte> commentBuf = commentBytes <= 256 ? stackalloc byte[256] : new byte[commentBytes];
        commentBuf = commentBuf[..commentBytes];
        for (int i = 0; i < commentBytes; i++) commentBuf[i] = (byte)reader.ReadBits(8);
        string comment = Encoding.Latin1.GetString(commentBuf);

        pce = new AacProgramConfigurationElement
        {
            ElementInstanceTag = tag,
            ObjectType = objectType,
            SamplingFrequencyIndex = sfIndex,
            FrontElements = front,
            SideElements = side,
            BackElements = back,
            LfeElements = lfe,
            AssocDataElements = assoc,
            CouplingElements = coupling,
            MonoMixdownElementNumber = monoMixdown,
            StereoMixdownElementNumber = stereoMixdown,
            MatrixMixdown = matrixMixdown,
            CommentField = comment,
        };
        return true;
    }

    internal void WriteTo(BitWriter writer)
    {
        ValidateForSerialisation();

        writer.Write(ElementInstanceTag, 4);
        writer.Write(ObjectType, 2);
        writer.Write(SamplingFrequencyIndex, 4);
        writer.Write(FrontElements.Count, 4);
        writer.Write(SideElements.Count, 4);
        writer.Write(BackElements.Count, 4);
        writer.Write(LfeElements.Count, 2);
        writer.Write(AssocDataElements.Count, 3);
        writer.Write(CouplingElements.Count, 4);

        WriteOptionalTag(writer, MonoMixdownElementNumber);
        WriteOptionalTag(writer, StereoMixdownElementNumber);
        if (MatrixMixdown is { } mm)
        {
            writer.Write(1u, 1);
            writer.Write(mm.Index, 2);
            writer.Write(mm.PseudoSurroundEnable ? 1u : 0u, 1);
        }
        else
        {
            writer.Write(0u, 1);
        }

        WriteChannelSlots(writer, FrontElements);
        WriteChannelSlots(writer, SideElements);
        WriteChannelSlots(writer, BackElements);
        WriteTagList(writer, LfeElements);
        WriteTagList(writer, AssocDataElements);
        WriteCouplingSlots(writer, CouplingElements);

        writer.AlignToByte();

        byte[] commentBytes = Encoding.Latin1.GetBytes(CommentField);
        if (commentBytes.Length > 255)
            throw new InvalidOperationException("CommentField exceeds 255 bytes after Latin-1 encoding.");
        writer.Write((uint)commentBytes.Length, 8);
        for (int i = 0; i < commentBytes.Length; i++) writer.Write(commentBytes[i], 8);
    }

    private void ValidateForSerialisation()
    {
        if ((uint)ElementInstanceTag > 15) throw new InvalidOperationException("ElementInstanceTag must fit in 4 bits.");
        if ((uint)ObjectType > 3) throw new InvalidOperationException("ObjectType must fit in 2 bits.");
        if ((uint)SamplingFrequencyIndex > 14) throw new InvalidOperationException("SamplingFrequencyIndex must be 0..14 (PCE has no inline-rate escape).");
        if ((uint)FrontElements.Count > 15) throw new InvalidOperationException("FrontElements count must fit in 4 bits.");
        if ((uint)SideElements.Count > 15) throw new InvalidOperationException("SideElements count must fit in 4 bits.");
        if ((uint)BackElements.Count > 15) throw new InvalidOperationException("BackElements count must fit in 4 bits.");
        if ((uint)LfeElements.Count > 3) throw new InvalidOperationException("LfeElements count must fit in 2 bits.");
        if ((uint)AssocDataElements.Count > 7) throw new InvalidOperationException("AssocDataElements count must fit in 3 bits.");
        if ((uint)CouplingElements.Count > 15) throw new InvalidOperationException("CouplingElements count must fit in 4 bits.");
        if (MonoMixdownElementNumber is { } m && (uint)m > 15)
            throw new InvalidOperationException("MonoMixdownElementNumber must fit in 4 bits.");
        if (StereoMixdownElementNumber is { } s && (uint)s > 15)
            throw new InvalidOperationException("StereoMixdownElementNumber must fit in 4 bits.");
        if (MatrixMixdown is { Index: var idx } && (uint)idx > 3)
            throw new InvalidOperationException("MatrixMixdown.Index must fit in 2 bits.");
    }

    private static int? ReadOptionalTag(ref BitReader reader)
    {
        if (!reader.ReadBit()) return null;
        return (int)reader.ReadBits(4);
    }

    private static void WriteOptionalTag(BitWriter writer, int? value)
    {
        if (value is { } v)
        {
            if ((uint)v > 15) throw new InvalidOperationException("PCE tag selector exceeds 4 bits.");
            writer.Write(1u, 1);
            writer.Write(v, 4);
        }
        else
        {
            writer.Write(0u, 1);
        }
    }

    private static IReadOnlyList<AacPceChannelSlot> ReadChannelSlots(ref BitReader reader, int count)
    {
        if (count == 0) return Array.Empty<AacPceChannelSlot>();
        var slots = new AacPceChannelSlot[count];
        for (int i = 0; i < count; i++)
        {
            bool isCpe = reader.ReadBit();
            int tag = (int)reader.ReadBits(4);
            slots[i] = new AacPceChannelSlot { IsCpe = isCpe, TagSelect = tag };
        }
        return new ReadOnlyCollection<AacPceChannelSlot>(slots);
    }

    private static void WriteChannelSlots(BitWriter writer, IReadOnlyList<AacPceChannelSlot> slots)
    {
        for (int i = 0; i < slots.Count; i++)
        {
            var slot = slots[i];
            if ((uint)slot.TagSelect > 15) throw new InvalidOperationException("PCE channel tag exceeds 4 bits.");
            writer.Write(slot.IsCpe ? 1u : 0u, 1);
            writer.Write(slot.TagSelect, 4);
        }
    }

    private static IReadOnlyList<int> ReadTagList(ref BitReader reader, int count)
    {
        if (count == 0) return Array.Empty<int>();
        var tags = new int[count];
        for (int i = 0; i < count; i++) tags[i] = (int)reader.ReadBits(4);
        return new ReadOnlyCollection<int>(tags);
    }

    private static void WriteTagList(BitWriter writer, IReadOnlyList<int> tags)
    {
        for (int i = 0; i < tags.Count; i++)
        {
            if ((uint)tags[i] > 15) throw new InvalidOperationException("PCE tag exceeds 4 bits.");
            writer.Write(tags[i], 4);
        }
    }

    private static IReadOnlyList<AacPceCouplingSlot> ReadCouplingSlots(ref BitReader reader, int count)
    {
        if (count == 0) return Array.Empty<AacPceCouplingSlot>();
        var slots = new AacPceCouplingSlot[count];
        for (int i = 0; i < count; i++)
        {
            bool isInd = reader.ReadBit();
            int tag = (int)reader.ReadBits(4);
            slots[i] = new AacPceCouplingSlot { IsIndependentlySwitched = isInd, TagSelect = tag };
        }
        return new ReadOnlyCollection<AacPceCouplingSlot>(slots);
    }

    private static void WriteCouplingSlots(BitWriter writer, IReadOnlyList<AacPceCouplingSlot> slots)
    {
        for (int i = 0; i < slots.Count; i++)
        {
            var slot = slots[i];
            if ((uint)slot.TagSelect > 15) throw new InvalidOperationException("PCE coupling tag exceeds 4 bits.");
            writer.Write(slot.IsIndependentlySwitched ? 1u : 0u, 1);
            writer.Write(slot.TagSelect, 4);
        }
    }
}
