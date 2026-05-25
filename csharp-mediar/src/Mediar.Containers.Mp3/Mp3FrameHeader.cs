namespace Mediar.Containers.Mp3;

/// <summary>
/// Decoded view of a 4-byte MPEG-1/2/2.5 Audio Layer I/II/III frame header.
/// Only the metadata required to walk to the next frame and to seed
/// <see cref="MediaTrack"/> parameters is decoded.
/// </summary>
public readonly struct Mp3FrameHeader
{
    public int Version { get; private init; }       // 1, 2, or 25 (for 2.5)
    public int Layer { get; private init; }         // 1, 2, or 3
    public int Bitrate { get; private init; }       // bits/s
    public int SampleRate { get; private init; }    // Hz
    public int Padding { get; private init; }
    public int Channels { get; private init; }
    public int FrameSize { get; private init; }     // bytes (incl. header)
    public int SamplesPerFrame { get; private init; }

    private static readonly int[,] BitrateTable =
    {
        // Mpeg1 L1, Mpeg1 L2, Mpeg1 L3, Mpeg2 L1, Mpeg2 L2/L3
        // index 0 = free, index 15 = bad
        {0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, -1}, // M1L1
        {0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, -1},   // M1L2
        {0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, -1},    // M1L3
        {0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, -1},   // M2L1
        {0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, -1},        // M2L2/L3
    };

    private static readonly int[,] SampleRateTable =
    {
        {44100, 48000, 32000, 0}, // MPEG1
        {22050, 24000, 16000, 0}, // MPEG2
        {11025, 12000,  8000, 0}, // MPEG2.5
    };

    public static bool TryParse(ReadOnlySpan<byte> b, out Mp3FrameHeader header)
    {
        header = default;
        if (b.Length < 4) return false;

        // Sync = 11 bits all 1.
        if (b[0] != 0xFF || (b[1] & 0xE0) != 0xE0) return false;

        int versionBits = (b[1] >> 3) & 0x03;
        int layerBits   = (b[1] >> 1) & 0x03;
        int bitrateIx   = (b[2] >> 4) & 0x0F;
        int sampleIx    = (b[2] >> 2) & 0x03;
        int padding     = (b[2] >> 1) & 0x01;
        int channelMode = (b[3] >> 6) & 0x03;

        if (versionBits == 1) return false; // reserved
        if (layerBits == 0) return false;
        if (bitrateIx == 0 || bitrateIx == 15) return false;
        if (sampleIx == 3) return false;

        int version = versionBits switch { 3 => 1, 2 => 2, 0 => 25, _ => 0 };
        int layer = layerBits switch { 3 => 1, 2 => 2, 1 => 3, _ => 0 };

        int bitrateRow = version == 1
            ? layer - 1                  // 0..2
            : (layer == 1 ? 3 : 4);      // M2L1=3, M2L2/L3=4
        int bitrateKbps = BitrateTable[bitrateRow, bitrateIx];
        if (bitrateKbps < 0) return false;

        int sampleRow = version switch { 1 => 0, 2 => 1, 25 => 2, _ => 0 };
        int sampleRate = SampleRateTable[sampleRow, sampleIx];
        if (sampleRate == 0) return false;

        int samples = layer == 1 ? 384 : (layer == 3 && version != 1 ? 576 : 1152);

        int bitrateBps = bitrateKbps * 1000;
        int frameSize;
        if (layer == 1)
        {
            frameSize = (12 * bitrateBps / sampleRate + padding) * 4;
        }
        else
        {
            int coef = (layer == 3 && version != 1) ? 72 : 144;
            frameSize = coef * bitrateBps / sampleRate + padding;
        }
        if (frameSize <= 4) return false;

        header = new Mp3FrameHeader
        {
            Version = version,
            Layer = layer,
            Bitrate = bitrateBps,
            SampleRate = sampleRate,
            Padding = padding,
            Channels = channelMode == 3 ? 1 : 2,
            FrameSize = frameSize,
            SamplesPerFrame = samples,
        };
        return true;
    }
}
