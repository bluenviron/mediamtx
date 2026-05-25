namespace Mediar.Codecs.Flac.Decoder;

/// <summary>
/// FLAC frame integrity hashes:
/// <list type="bullet">
///   <item><description>CRC-8 polynomial <c>x^8 + x^2 + x + 1</c> (0x07) over frame header bytes.</description></item>
///   <item><description>CRC-16-IBM polynomial <c>x^16 + x^15 + x^2 + 1</c> (0x8005) over the full frame minus footer.</description></item>
/// </list>
/// Tables are MSB-first, matching the FLAC bitstream convention.
/// </summary>
internal static class FlacCrc
{
    private static readonly byte[] _crc8Table = BuildCrc8Table();
    private static readonly ushort[] _crc16Table = BuildCrc16Table();

    private static byte[] BuildCrc8Table()
    {
        var t = new byte[256];
        for (int i = 0; i < 256; i++)
        {
            int crc = i;
            for (int j = 0; j < 8; j++)
            {
                crc = ((crc & 0x80) != 0) ? ((crc << 1) ^ 0x07) : (crc << 1);
            }
            t[i] = (byte)(crc & 0xFF);
        }
        return t;
    }

    private static ushort[] BuildCrc16Table()
    {
        var t = new ushort[256];
        for (int i = 0; i < 256; i++)
        {
            int crc = i << 8;
            for (int j = 0; j < 8; j++)
            {
                crc = ((crc & 0x8000) != 0) ? ((crc << 1) ^ 0x8005) : (crc << 1);
            }
            t[i] = (ushort)(crc & 0xFFFF);
        }
        return t;
    }

    /// <summary>Update CRC-8 with one byte.</summary>
    public static byte Crc8(byte crc, byte b) => _crc8Table[crc ^ b];

    /// <summary>Compute CRC-8 over a span.</summary>
    public static byte Crc8(ReadOnlySpan<byte> data)
    {
        byte c = 0;
        for (int i = 0; i < data.Length; i++) c = _crc8Table[c ^ data[i]];
        return c;
    }

    /// <summary>Update CRC-16-IBM with one byte.</summary>
    public static ushort Crc16(ushort crc, byte b) => (ushort)((crc << 8) ^ _crc16Table[(crc >> 8) ^ b]);

    /// <summary>Compute CRC-16-IBM over a span.</summary>
    public static ushort Crc16(ReadOnlySpan<byte> data)
    {
        ushort c = 0;
        for (int i = 0; i < data.Length; i++) c = (ushort)((c << 8) ^ _crc16Table[(c >> 8) ^ data[i]]);
        return c;
    }
}
