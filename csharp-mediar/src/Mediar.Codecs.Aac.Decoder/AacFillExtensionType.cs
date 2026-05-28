namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Identifier for the 4-bit <c>extension_type</c> field that introduces every
/// non-empty FIL element's <c>extension_payload()</c> (ISO/IEC 14496-3
/// Table 4.51). Values not enumerated here (0x3..0xA, 0xF) are reserved by
/// the spec but still surfaced via <see cref="AacFillExtensionPayload.RawType"/>.
/// </summary>
public enum AacFillExtensionType : byte
{
    /// <summary>EXT_FILL - pure fill bits (4-bit unused flag + arbitrary bits).</summary>
    Fill = 0x0,

    /// <summary>EXT_FILL_DATA - fill bits that may be discarded by the decoder.</summary>
    FillData = 0x1,

    /// <summary>EXT_DATA_ELEMENT - generic ancillary data carrier (data_element_version + payload).</summary>
    DataElement = 0x2,

    /// <summary>EXT_DYNAMIC_RANGE - DRC info (dynamic_range_info(), Table 4.52).</summary>
    DynamicRange = 0xB,

    /// <summary>EXT_SAC_DATA - MPEG Surround spatial-audio coding payload.</summary>
    SacData = 0xC,

    /// <summary>EXT_SBR_DATA - SBR / HE-AAC v1 raw extension payload.</summary>
    SbrData = 0xD,

    /// <summary>EXT_SBR_DATA_CRC - SBR / HE-AAC v1 with CRC.</summary>
    SbrDataCrc = 0xE,
}
