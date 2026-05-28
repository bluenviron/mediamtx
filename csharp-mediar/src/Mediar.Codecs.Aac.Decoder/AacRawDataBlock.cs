namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Single element surfaced by <see cref="AacRawDataBlock"/>'s walk over an
/// AAC raw_data_block (ISO/IEC 14496-3 Table 4.71). The
/// <see cref="Type"/> selects which of the typed payload fields - if any -
/// is populated; opaque audio elements (SCE/CPE/CCE/LFE) carry no payload
/// because their body parsing is gated on the still-unimplemented
/// spectral / coupling decoder.
/// </summary>
public sealed record AacRawDataBlockEntry
{
    /// <summary>3-bit id_syn_ele identifying the element class.</summary>
    public required AacSyntacticElementType Type { get; init; }

    /// <summary>Bit position of the 3-bit element id within the parsed buffer.</summary>
    public required int BitOffset { get; init; }

    /// <summary>Populated when <see cref="Type"/> is <see cref="AacSyntacticElementType.ProgramConfigElement"/>.</summary>
    public AacProgramConfigurationElement? ProgramConfig { get; init; }

    /// <summary>Populated when <see cref="Type"/> is <see cref="AacSyntacticElementType.DataStreamElement"/>.</summary>
    public AacDataStreamElement? DataStream { get; init; }

    /// <summary>Opaque extension_payload bytes when <see cref="Type"/> is <see cref="AacSyntacticElementType.FillElement"/>; otherwise empty.</summary>
    public ReadOnlyMemory<byte> FillExtensionBytes { get; init; }
}

/// <summary>
/// A walked AAC raw_data_block (ISO/IEC 14496-3 §4.4.2.1, Table 4.71). The
/// raw_data_block is a sequence of syntactic elements terminated by the END
/// marker (id = 7). This phase-1 walker fully parses the codec-state-free
/// elements - PCE (id=5), DSE (id=4), FIL (id=6) - and the END sentinel.
/// Audio elements (SCE / CPE / CCE / LFE) are surfaced as opaque markers
/// and terminate the walk because their bit-length is determined by
/// section / Huffman / spectral state that is not yet implemented.
/// </summary>
public sealed record AacRawDataBlock
{
    /// <summary>Elements surfaced in stream order, terminated either by END or by the first opaque audio element.</summary>
    public required IReadOnlyList<AacRawDataBlockEntry> Entries { get; init; }

    /// <summary>True when the walker reached the END marker (id = 7).</summary>
    public required bool TerminatedByEnd { get; init; }

    /// <summary>Total bits consumed from the input buffer, including the trailing element id.</summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Walk a raw_data_block byte buffer. The walk stops at the END marker
    /// or at the first audio element (SCE/CPE/CCE/LFE) - whichever comes
    /// first - and returns the entries collected up to that point. Returns
    /// false on a malformed PCE/DSE/FIL body (truncation, oversized comment,
    /// invalid sample-frequency-index escape, etc.). An empty buffer also
    /// returns false; a single-byte buffer containing only the END marker
    /// (id = 7) returns true with one terminating End entry. Streams that
    /// exhaust mid-byte still parse successfully as long as no body
    /// underflowed.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AacRawDataBlock? block)
    {
        block = null;
        if (data.IsEmpty) return false;

        try
        {
            var reader = new BitReader(data);
            var entries = new List<AacRawDataBlockEntry>();
            bool terminated = false;

            while (reader.Remaining >= 3)
            {
                int idBitOffset = reader.Position;
                var type = (AacSyntacticElementType)reader.ReadBits(3);

                switch (type)
                {
                    case AacSyntacticElementType.ProgramConfigElement:
                        if (!AacProgramConfigurationElement.TryRead(ref reader, out var pce)) return false;
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            ProgramConfig = pce,
                        });
                        break;

                    case AacSyntacticElementType.DataStreamElement:
                        if (!AacDataStreamElement.TryRead(ref reader, out var dse)) return false;
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            DataStream = dse,
                        });
                        break;

                    case AacSyntacticElementType.FillElement:
                        if (!TryReadFillElement(ref reader, out byte[]? fillBytes)) return false;
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            FillExtensionBytes = fillBytes,
                        });
                        break;

                    case AacSyntacticElementType.End:
                        entries.Add(new AacRawDataBlockEntry { Type = type, BitOffset = idBitOffset });
                        terminated = true;
                        block = new AacRawDataBlock
                        {
                            Entries = entries,
                            TerminatedByEnd = true,
                            BitsConsumed = reader.Position,
                        };
                        return true;

                    case AacSyntacticElementType.SingleChannelElement:
                    case AacSyntacticElementType.ChannelPairElement:
                    case AacSyntacticElementType.CouplingChannelElement:
                    case AacSyntacticElementType.LfeChannelElement:
                        // Body parsing depends on the spectral decoder; surface
                        // the element id and stop. The caller still gets back
                        // any PCE / DSE / FIL entries already collected.
                        entries.Add(new AacRawDataBlockEntry { Type = type, BitOffset = idBitOffset });
                        block = new AacRawDataBlock
                        {
                            Entries = entries,
                            TerminatedByEnd = false,
                            BitsConsumed = reader.Position,
                        };
                        return true;

                    default:
                        return false; // unreachable; 3-bit id is 0..7
                }
            }

            block = new AacRawDataBlock
            {
                Entries = entries,
                TerminatedByEnd = terminated,
                BitsConsumed = reader.Position,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            block = null;
            return false;
        }
        catch (ArgumentOutOfRangeException)
        {
            block = null;
            return false;
        }
    }

    private static bool TryReadFillElement(ref BitReader reader, out byte[]? bytes)
    {
        bytes = null;
        if (reader.Remaining < 4) return false;

        int count = (int)reader.ReadBits(4);
        int cnt = count;
        if (count == 15)
        {
            if (reader.Remaining < 8) return false;
            int esc = (int)reader.ReadBits(8);
            cnt = 14 + esc;
        }

        if (reader.Remaining < cnt * 8) return false;

        bytes = cnt == 0 ? Array.Empty<byte>() : new byte[cnt];
        for (int i = 0; i < cnt; i++) bytes[i] = (byte)reader.ReadBits(8);
        return true;
    }
}
