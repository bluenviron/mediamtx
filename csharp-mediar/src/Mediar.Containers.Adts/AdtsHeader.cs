namespace Mediar.Containers.Adts;

/// <summary>
/// Parsed Audio Data Transport Stream (ADTS) header. ISO/IEC 13818-7 §6.2.
/// </summary>
public readonly struct AdtsHeader
{
    /// <summary>Number of bytes in the ADTS header (7 or 9 with CRC).</summary>
    public int HeaderSize { get; init; }

    /// <summary>Total frame length including this header.</summary>
    public int FrameSize { get; init; }

    /// <summary>MPEG-4 audio object type (profile + 1).</summary>
    public int AudioObjectType { get; init; }

    /// <summary>Sampling frequency in Hz.</summary>
    public int SampleRate { get; init; }

    /// <summary>Channel configuration (1..7).</summary>
    public int ChannelConfig { get; init; }

    /// <summary>True for MPEG-4 (ID bit = 0), false for MPEG-2 (ID bit = 1).</summary>
    public bool IsMpeg4 { get; init; }

    /// <summary>True if frame is protected by a CRC (and an extra 2 bytes follow the 7-byte fixed header).</summary>
    public bool HasCrc { get; init; }

    /// <summary>Number of AAC raw_data_blocks in the frame (0 means one).</summary>
    public int NumberOfRawDataBlocks { get; init; }

    private static readonly int[] AdtsSampleRates =
    {
        96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050,
        16000, 12000, 11025, 8000, 7350, 0, 0, 0,
    };

    /// <summary>Attempt to parse an ADTS header from the start of <paramref name="data"/>.</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AdtsHeader header)
    {
        header = default;
        if (data.Length < 7) return false;

        // syncword 12 bits 0xFFF
        if (data[0] != 0xFF || (data[1] & 0xF0) != 0xF0) return false;

        bool isMpeg2 = (data[1] & 0x08) != 0;
        // layer must be 0
        if ((data[1] & 0x06) != 0) return false;
        bool protectionAbsent = (data[1] & 0x01) != 0;

        int profile = (data[2] >> 6) & 0x03;
        int sampleRateIndex = (data[2] >> 2) & 0x0F;
        if (sampleRateIndex >= 13) return false;
        int channelConfig = ((data[2] & 0x01) << 2) | ((data[3] >> 6) & 0x03);
        int frameLength =
            ((data[3] & 0x03) << 11) |
            (data[4] << 3) |
            ((data[5] >> 5) & 0x07);
        if (frameLength < 7) return false;
        int rdb = data[6] & 0x03;

        header = new AdtsHeader
        {
            HeaderSize = protectionAbsent ? 7 : 9,
            FrameSize = frameLength,
            AudioObjectType = profile + 1,
            SampleRate = AdtsSampleRates[sampleRateIndex],
            ChannelConfig = channelConfig,
            IsMpeg4 = !isMpeg2,
            HasCrc = !protectionAbsent,
            NumberOfRawDataBlocks = rdb,
        };
        return true;
    }

    /// <summary>Return the ADTS sampling-frequency-index for the given sample rate, or -1 if unknown.</summary>
    public static int IndexForSampleRate(int sampleRate)
    {
        for (int i = 0; i < AdtsSampleRates.Length; i++)
        {
            if (AdtsSampleRates[i] == sampleRate) return i;
        }
        return -1;
    }
}
