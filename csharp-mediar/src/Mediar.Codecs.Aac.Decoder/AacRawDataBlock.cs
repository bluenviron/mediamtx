namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Caller-supplied context for the "full" <see cref="AacRawDataBlock"/>
/// walker overload. Supplies the sample rate and Huffman codebooks needed
/// to fully consume audio elements (SCE / CPE / CCE / LFE) instead of
/// stopping at them.
/// </summary>
public sealed record AacRawDataBlockContext
{
    /// <summary>Source sample rate (Hz) used to dispatch SWB offset tables.</summary>
    public required int SampleRate { get; init; }

    /// <summary>121-symbol scale-factor Huffman codebook (Annex 4.A.2.1).</summary>
    public required AacHuffmanCodebook ScaleFactorCodebook { get; init; }

    /// <summary>
    /// Spectral Huffman codebook lookup indexed by codebook number; element
    /// <c>i</c> holds the codebook used by <c>sect_cb == i</c>. Slots known
    /// not to be referenced may be <see langword="null"/>.
    /// </summary>
    public required IReadOnlyList<AacHuffmanCodebook?> SpectralCodebooks { get; init; }
}

/// <summary>
/// Single element surfaced by <see cref="AacRawDataBlock"/>'s walk over an
/// AAC raw_data_block (ISO/IEC 14496-3 Table 4.71). The
/// <see cref="Type"/> selects which of the typed payload fields - if any -
/// is populated; audio elements (SCE/CPE/CCE/LFE) carry no payload when the
/// boundary <see cref="AacRawDataBlock.TryParse(ReadOnlySpan{byte}, out AacRawDataBlock)"/>
/// overload is used, and the corresponding typed property when the "full"
/// overload that accepts an <see cref="AacRawDataBlockContext"/> is used.
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

    /// <summary>
    /// Typed view over the FIL <c>extension_payload()</c> bytes. Populated
    /// when <see cref="Type"/> is <see cref="AacSyntacticElementType.FillElement"/>
    /// and the element has at least one payload byte (<c>cnt &gt;= 1</c>);
    /// remains <see langword="null"/> for a zero-byte FIL element because it
    /// carries no <c>extension_type</c> field at all.
    /// </summary>
    public AacFillExtensionPayload? FillExtension { get; init; }

    /// <summary>
    /// Populated when <see cref="Type"/> is
    /// <see cref="AacSyntacticElementType.SingleChannelElement"/> and the
    /// walk was driven through the "full" overload.
    /// </summary>
    public AacSingleChannelElement? SingleChannel { get; init; }

    /// <summary>
    /// Populated when <see cref="Type"/> is
    /// <see cref="AacSyntacticElementType.ChannelPairElement"/> and the
    /// walk was driven through the "full" overload.
    /// </summary>
    public AacChannelPairElement? ChannelPair { get; init; }

    /// <summary>
    /// Populated when <see cref="Type"/> is
    /// <see cref="AacSyntacticElementType.CouplingChannelElement"/> and the
    /// walk was driven through the "full" overload.
    /// </summary>
    public AacCouplingChannelElement? CouplingChannel { get; init; }

    /// <summary>
    /// Populated when <see cref="Type"/> is
    /// <see cref="AacSyntacticElementType.LfeChannelElement"/> and the walk
    /// was driven through the "full" overload.
    /// </summary>
    public AacLowFrequencyElement? LowFrequency { get; init; }
}

/// <summary>
/// A walked AAC raw_data_block (ISO/IEC 14496-3 §4.4.2.1, Table 4.71). The
/// raw_data_block is a sequence of syntactic elements terminated by the END
/// marker (id = 7). The boundary
/// <see cref="TryParse(ReadOnlySpan{byte}, out AacRawDataBlock)"/> overload
/// fully parses the codec-state-free elements - PCE (id=5), DSE (id=4),
/// FIL (id=6) - and the END sentinel; audio elements (SCE / CPE / CCE / LFE)
/// are surfaced as opaque markers and terminate the walk. The "full"
/// <see cref="TryParse(ReadOnlySpan{byte}, AacRawDataBlockContext, out AacRawDataBlock)"/>
/// overload accepts a sample rate + codebook context and additionally
/// consumes the audio elements via their "full" overloads, populating the
/// typed payload properties on each <see cref="AacRawDataBlockEntry"/>.
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
        return TryParseCore(data, context: null, out block);
    }

    /// <summary>
    /// Walk a raw_data_block byte buffer and additionally consume any audio
    /// elements (SCE / CPE / CCE / LFE) via their "full" overloads using the
    /// supplied <paramref name="context"/>. The walk stops only at the END
    /// marker; malformed audio elements cause the walk to fail rather than
    /// terminate gracefully.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> data,
        AacRawDataBlockContext context,
        out AacRawDataBlock? block)
    {
        ArgumentNullException.ThrowIfNull(context);
        return TryParseCore(data, context, out block);
    }

    private static bool TryParseCore(
        ReadOnlySpan<byte> data,
        AacRawDataBlockContext? context,
        out AacRawDataBlock? block)
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
                        AacFillExtensionPayload? fillExt = null;
                        if (fillBytes!.Length > 0)
                        {
                            _ = AacFillExtensionPayload.TryParse(fillBytes, out fillExt);
                        }
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            FillExtensionBytes = fillBytes,
                            FillExtension = fillExt,
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
                        if (context is null)
                        {
                            entries.Add(new AacRawDataBlockEntry { Type = type, BitOffset = idBitOffset });
                            block = new AacRawDataBlock
                            {
                                Entries = entries,
                                TerminatedByEnd = false,
                                BitsConsumed = reader.Position,
                            };
                            return true;
                        }
                        if (!AacSingleChannelElement.TryRead(
                                ref reader,
                                context.ScaleFactorCodebook,
                                context.SampleRate,
                                context.SpectralCodebooks,
                                out var sce)
                            || sce is null)
                        {
                            return false;
                        }
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            SingleChannel = sce,
                        });
                        break;

                    case AacSyntacticElementType.ChannelPairElement:
                        if (context is null)
                        {
                            entries.Add(new AacRawDataBlockEntry { Type = type, BitOffset = idBitOffset });
                            block = new AacRawDataBlock
                            {
                                Entries = entries,
                                TerminatedByEnd = false,
                                BitsConsumed = reader.Position,
                            };
                            return true;
                        }
                        if (!AacChannelPairElement.TryRead(
                                ref reader,
                                context.ScaleFactorCodebook,
                                context.SampleRate,
                                context.SpectralCodebooks,
                                out var cpe)
                            || cpe is null)
                        {
                            return false;
                        }
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            ChannelPair = cpe,
                        });
                        break;

                    case AacSyntacticElementType.CouplingChannelElement:
                        if (context is null)
                        {
                            entries.Add(new AacRawDataBlockEntry { Type = type, BitOffset = idBitOffset });
                            block = new AacRawDataBlock
                            {
                                Entries = entries,
                                TerminatedByEnd = false,
                                BitsConsumed = reader.Position,
                            };
                            return true;
                        }
                        if (!AacCouplingChannelElement.TryRead(
                                ref reader,
                                context.ScaleFactorCodebook,
                                context.SampleRate,
                                context.SpectralCodebooks,
                                out var cce)
                            || cce is null)
                        {
                            return false;
                        }
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            CouplingChannel = cce,
                        });
                        break;

                    case AacSyntacticElementType.LfeChannelElement:
                        if (context is null)
                        {
                            entries.Add(new AacRawDataBlockEntry { Type = type, BitOffset = idBitOffset });
                            block = new AacRawDataBlock
                            {
                                Entries = entries,
                                TerminatedByEnd = false,
                                BitsConsumed = reader.Position,
                            };
                            return true;
                        }
                        if (!AacLowFrequencyElement.TryRead(
                                ref reader,
                                context.ScaleFactorCodebook,
                                context.SampleRate,
                                context.SpectralCodebooks,
                                out var lfe)
                            || lfe is null)
                        {
                            return false;
                        }
                        entries.Add(new AacRawDataBlockEntry
                        {
                            Type = type,
                            BitOffset = idBitOffset,
                            LowFrequency = lfe,
                        });
                        break;

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
