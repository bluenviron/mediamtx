namespace Mediar.Containers.MpegTs;

/// <summary>
/// Mapping from MPEG-TS <c>stream_type</c> codes (ISO/IEC 13818-1
/// Table 2-34 and registered private types) to Mediar
/// <see cref="CodecId"/> identifiers. Codes that have no Mediar codec
/// representation are mapped to <see cref="CodecId.Unknown"/>.
/// </summary>
internal static class StreamTypes
{
    public static CodecId ToCodecId(byte streamType) => streamType switch
    {
        0x03 => CodecId.Mp3,
        0x04 => CodecId.Mp3,
        0x0F => CodecId.Aac,
        0x11 => CodecId.Aac,
        0x1B => CodecId.H264,
        0x24 => CodecId.H265,
        0x81 => CodecId.Ac3,
        0x87 => CodecId.EAc3,
        _ => CodecId.Unknown,
    };

    public static bool IsVideo(CodecId codec) => codec switch
    {
        CodecId.H264 or CodecId.H265 => true,
        _ => false,
    };
}
