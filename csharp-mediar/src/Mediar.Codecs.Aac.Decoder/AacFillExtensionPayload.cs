namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Typed view over a FIL element's <c>extension_payload()</c> bytes
/// (ISO/IEC 14496-3 §4.5.2.13, Table 4.51). The FIL dispatcher captures
/// the raw <c>cnt * 8</c> bits of the payload; this record splits off
/// the leading 4-bit <c>extension_type</c> nibble and exposes the
/// remaining body bits as a byte buffer aligned to MSB-first order.
/// </summary>
public sealed record AacFillExtensionPayload
{
    /// <summary>
    /// 4-bit <c>extension_type</c> code. Values matching
    /// <see cref="AacFillExtensionType"/> map to named extensions;
    /// other values are reserved by the spec but still preserved here.
    /// </summary>
    public required byte RawType { get; init; }

    /// <summary>
    /// Named <c>extension_type</c> value. Use <see cref="IsKnownExtensionType"/>
    /// to distinguish a real enum mapping from a reserved cast.
    /// </summary>
    public AacFillExtensionType ExtensionType => (AacFillExtensionType)RawType;

    /// <summary>True when <see cref="RawType"/> matches a value defined in Table 4.51.</summary>
    public bool IsKnownExtensionType => IsKnown(RawType);

    /// <summary>
    /// Body bytes that follow the 4-bit <c>extension_type</c>, shifted to
    /// start MSB-first at bit 0 of <c>Body.Span[0]</c>. The last byte may
    /// have unused low-order padding bits; consult <see cref="BodyBitLength"/>
    /// for the exact count.
    /// </summary>
    public required ReadOnlyMemory<byte> Body { get; init; }

    /// <summary>Number of valid MSB-first bits in <see cref="Body"/>.</summary>
    public required int BodyBitLength { get; init; }

    /// <summary>
    /// Typed view over the body when <see cref="ExtensionType"/> is
    /// <see cref="AacFillExtensionType.DynamicRange"/> (0xB). Populated
    /// automatically by <see cref="TryParse"/>; left <see langword="null"/>
    /// for any other extension type, or when the body is too short or
    /// malformed to parse a <c>dynamic_range_info()</c> structure.
    /// </summary>
    public AacDynamicRangeInfo? DynamicRange { get; init; }

    /// <summary>
    /// Typed framing view of the body when <see cref="ExtensionType"/> is
    /// <see cref="AacFillExtensionType.SbrData"/> (0xD) or
    /// <see cref="AacFillExtensionType.SbrDataCrc"/> (0xE). Populated
    /// automatically by <see cref="TryParse"/> - splits off the 10-bit
    /// <c>sbr_crc_bits</c> field (CRC variant only) and surfaces the
    /// <c>sbr_extension_data()</c> remainder as an opaque bit slice.
    /// </summary>
    public AacSbrExtensionData? Sbr { get; init; }

    /// <summary>
    /// Typed view of the body when <see cref="ExtensionType"/> is
    /// <see cref="AacFillExtensionType.FillData"/> (0x1). Populated by
    /// <see cref="TryParse"/> when the body shape matches Table 4.58
    /// (4-bit <c>fill_nibble</c> followed by whole-byte <c>fill_byte</c>
    /// entries); otherwise null.
    /// </summary>
    public AacFillDataExtension? FillData { get; init; }

    /// <summary>
    /// True when <paramref name="rawType"/> is one of the defined codes in
    /// ISO/IEC 14496-3 Table 4.51.
    /// </summary>
    public static bool IsKnown(byte rawType) => rawType switch
    {
        0x0 or 0x1 or 0x2 or 0xB or 0xC or 0xD or 0xE => true,
        _ => false,
    };

    /// <summary>
    /// Parse the opaque FIL bytes captured by <see cref="AacRawDataBlock"/>
    /// (the dispatcher records <c>cnt * 8</c> bits packed MSB-first into
    /// <c>cnt</c> bytes). Returns false when the buffer is empty - which
    /// corresponds to a FIL element with <c>cnt = 0</c> and therefore no
    /// <c>extension_type</c> field at all.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> fillBytes, out AacFillExtensionPayload? payload)
    {
        payload = null;
        if (fillBytes.IsEmpty) return false;

        byte rawType = (byte)(fillBytes[0] >> 4);
        int totalBits = fillBytes.Length * 8;
        int bodyBits = totalBits - 4;

        byte[] body;
        if (bodyBits == 0)
        {
            body = Array.Empty<byte>();
        }
        else
        {
            int bodyBytes = (bodyBits + 7) >> 3;
            body = new byte[bodyBytes];
            for (int i = 0; i < bodyBytes; i++)
            {
                int hi = (fillBytes[i] & 0x0F) << 4;
                int lo = (i + 1 < fillBytes.Length) ? (fillBytes[i + 1] >> 4) : 0;
                body[i] = (byte)(hi | lo);
            }
        }

        AacDynamicRangeInfo? drc = null;
        AacSbrExtensionData? sbr = null;
        AacFillDataExtension? fillData = null;
        if (rawType == (byte)AacFillExtensionType.DynamicRange)
        {
            _ = AacDynamicRangeInfo.TryParse(body, bodyBits, out drc);
        }
        else if (rawType == (byte)AacFillExtensionType.SbrData
            || rawType == (byte)AacFillExtensionType.SbrDataCrc)
        {
            _ = AacSbrExtensionData.TryParse((AacFillExtensionType)rawType, body, bodyBits, out sbr);
        }
        else if (rawType == (byte)AacFillExtensionType.FillData)
        {
            _ = AacFillDataExtension.TryParse(body, bodyBits, out fillData);
        }

        payload = new AacFillExtensionPayload
        {
            RawType = rawType,
            Body = body,
            BodyBitLength = bodyBits,
            DynamicRange = drc,
            Sbr = sbr,
            FillData = fillData,
        };
        return true;
    }
}
