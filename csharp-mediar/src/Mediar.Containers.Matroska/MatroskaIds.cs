namespace Mediar.Containers.Matroska;

/// <summary>Matroska EBML element identifiers used by the demuxer.</summary>
internal static class MatroskaIds
{
    public const ulong Ebml = 0x1A45DFA3;
    public const ulong Segment = 0x18538067;
    public const ulong SeekHead = 0x114D9B74;
    public const ulong Info = 0x1549A966;
    public const ulong TimecodeScale = 0x2AD7B1;
    public const ulong Duration = 0x4489;
    public const ulong Tracks = 0x1654AE6B;
    public const ulong TrackEntry = 0xAE;
    public const ulong TrackNumber = 0xD7;
    public const ulong TrackType = 0x83;
    public const ulong CodecId = 0x86;
    public const ulong CodecPrivate = 0x63A2;
    public const ulong Audio = 0xE1;
    public const ulong SamplingFrequency = 0xB5;
    public const ulong Channels = 0x9F;
    public const ulong BitDepth = 0x6264;
    public const ulong Video = 0xE0;
    public const ulong PixelWidth = 0xB0;
    public const ulong PixelHeight = 0xBA;
    public const ulong Cluster = 0x1F43B675;
    public const ulong Timecode = 0xE7;
    public const ulong SimpleBlock = 0xA3;
    public const ulong BlockGroup = 0xA0;
    public const ulong Block = 0xA1;
    public const ulong BlockDuration = 0x9B;
    public const ulong Void = 0xEC;
    public const ulong Crc32 = 0xBF;

    // Tags
    public const ulong Tags = 0x1254C367;
    public const ulong Tag = 0x7373;
    public const ulong Targets = 0x63C0;
    public const ulong TargetTypeValue = 0x68CA;
    public const ulong TargetType = 0x63CA;
    public const ulong SimpleTag = 0x67C8;
    public const ulong TagName = 0x45A3;
    public const ulong TagLanguage = 0x447A;
    public const ulong TagString = 0x4487;
    public const ulong TagBinary = 0x4485;
}
