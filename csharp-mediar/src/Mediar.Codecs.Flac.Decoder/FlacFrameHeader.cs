using Mediar.IO;

namespace Mediar.Codecs.Flac.Decoder;

/// <summary>Channel layout for stereo decorrelation.</summary>
internal enum FlacChannelMode
{
    Independent = 0,
    LeftSide,
    SideRight,
    MidSide,
}

/// <summary>Decoded FLAC frame header (RFC 9639 §10.2).</summary>
internal readonly struct FlacFrameHeader
{
    public int BlockSize { get; init; }
    public int SampleRate { get; init; }
    public int Channels { get; init; }
    public int BitsPerSample { get; init; }
    public FlacChannelMode ChannelMode { get; init; }
    public long FrameOrSampleNumber { get; init; }
    public int HeaderSize { get; init; }
}

internal static class FlacFrameHeaderParser
{
    /// <summary>
    /// Try to parse a FLAC frame header. <paramref name="defaultSampleRate"/>
    /// and <paramref name="defaultBitsPerSample"/> are taken from STREAMINFO
    /// and used when the header carries the "from STREAMINFO" sentinels.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> data,
        int defaultSampleRate,
        int defaultBitsPerSample,
        out FlacFrameHeader header)
    {
        header = default;
        if (data.Length < 5) return false;

        var br = new BitReader(data);
        // 14-bit sync code: 0b11111111111110
        if (!br.CanRead(14)) return false;
        uint sync = br.ReadBits(14);
        if (sync != 0b11111111111110u) return false;
        // 1 bit reserved (must be 0)
        if (br.ReadBit()) return false;
        // 1 bit blocking strategy (we don't need to branch on it for layout)
        _ = br.ReadBit();

        int blockSizeCode = (int)br.ReadBits(4);
        int sampleRateCode = (int)br.ReadBits(4);
        int channelCode = (int)br.ReadBits(4);
        int sampleSizeCode = (int)br.ReadBits(3);
        if (br.ReadBit()) return false;

        // UTF-8-encoded frame or sample number (1..7 bytes)
        long frameNumber = ReadUtf8(ref br, out bool ok);
        if (!ok) return false;

        int blockSize;
        switch (blockSizeCode)
        {
            case 0b0000: return false;
            case 0b0001: blockSize = 192; break;
            case 0b0010: case 0b0011: case 0b0100: case 0b0101:
                blockSize = 576 << (blockSizeCode - 2); break;
            case 0b0110:
                if (!br.CanRead(8)) return false;
                blockSize = (int)br.ReadBits(8) + 1; break;
            case 0b0111:
                if (!br.CanRead(16)) return false;
                blockSize = (int)br.ReadBits(16) + 1; break;
            default: // 1000..1111
                blockSize = 256 << (blockSizeCode - 8); break;
        }

        int sampleRate = sampleRateCode switch
        {
            0b0000 => defaultSampleRate,
            0b0001 => 88200,
            0b0010 => 176400,
            0b0011 => 192000,
            0b0100 => 8000,
            0b0101 => 16000,
            0b0110 => 22050,
            0b0111 => 24000,
            0b1000 => 32000,
            0b1001 => 44100,
            0b1010 => 48000,
            0b1011 => 96000,
            0b1100 => (int)br.ReadBits(8) * 1000,
            0b1101 => (int)br.ReadBits(16),
            0b1110 => (int)br.ReadBits(16) * 10,
            _ => -1,
        };
        if (sampleRate <= 0) return false;

        FlacChannelMode mode;
        int channels;
        switch (channelCode)
        {
            case <= 7: channels = channelCode + 1; mode = FlacChannelMode.Independent; break;
            case 0b1000: channels = 2; mode = FlacChannelMode.LeftSide; break;
            case 0b1001: channels = 2; mode = FlacChannelMode.SideRight; break;
            case 0b1010: channels = 2; mode = FlacChannelMode.MidSide; break;
            default: return false;
        }

        int bps = sampleSizeCode switch
        {
            0b000 => defaultBitsPerSample,
            0b001 => 8,
            0b010 => 12,
            0b100 => 16,
            0b101 => 20,
            0b110 => 24,
            0b111 => 32,
            _ => -1,
        };
        if (bps <= 0) return false;

        // header is byte-aligned at this point
        int headerSize = (int)((br.BitPosition + 7) / 8);
        if (data.Length < headerSize + 1) return false;
        byte storedCrc8 = data[headerSize];
        byte computed = FlacCrc.Crc8(data[..headerSize]);
        if (storedCrc8 != computed) return false;

        header = new FlacFrameHeader
        {
            BlockSize = blockSize,
            SampleRate = sampleRate,
            Channels = channels,
            BitsPerSample = bps,
            ChannelMode = mode,
            FrameOrSampleNumber = frameNumber,
            HeaderSize = headerSize + 1, // include CRC8
        };
        return true;
    }

    private static long ReadUtf8(ref BitReader br, out bool ok)
    {
        ok = false;
        if (!br.CanRead(8)) return 0;
        uint b0 = br.ReadBits(8);
        if ((b0 & 0x80) == 0) { ok = true; return b0; }
        int contBytes;
        long value;
        if ((b0 & 0xE0) == 0xC0) { contBytes = 1; value = b0 & 0x1F; }
        else if ((b0 & 0xF0) == 0xE0) { contBytes = 2; value = b0 & 0x0F; }
        else if ((b0 & 0xF8) == 0xF0) { contBytes = 3; value = b0 & 0x07; }
        else if ((b0 & 0xFC) == 0xF8) { contBytes = 4; value = b0 & 0x03; }
        else if ((b0 & 0xFE) == 0xFC) { contBytes = 5; value = b0 & 0x01; }
        else if (b0 == 0xFE) { contBytes = 6; value = 0; }
        else return 0;

        for (int i = 0; i < contBytes; i++)
        {
            if (!br.CanRead(8)) return 0;
            uint b = br.ReadBits(8);
            if ((b & 0xC0) != 0x80) return 0;
            value = (value << 6) | (b & 0x3F);
        }
        ok = true;
        return value;
    }
}
