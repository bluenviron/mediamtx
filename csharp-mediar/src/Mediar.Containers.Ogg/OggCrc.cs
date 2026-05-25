namespace Mediar.Containers.Ogg;

/// <summary>
/// Ogg's CRC32 variant (polynomial <c>0x04C11DB7</c>, non-reflected, no XOR-out,
/// initial 0). NOTE: this is NOT the more common Ethernet / IEEE CRC32.
/// </summary>
internal static class OggCrc
{
    private static readonly uint[] _table = BuildTable();

    private static uint[] BuildTable()
    {
        var t = new uint[256];
        for (uint i = 0; i < 256; i++)
        {
            uint r = i << 24;
            for (int j = 0; j < 8; j++)
            {
                r = (r & 0x80000000u) != 0 ? (r << 1) ^ 0x04C11DB7u : (r << 1);
            }
            t[i] = r;
        }
        return t;
    }

    /// <summary>Update a running CRC32 with one byte.</summary>
    public static uint Update(uint crc, byte b) => (crc << 8) ^ _table[(crc >> 24) ^ b];

    /// <summary>Compute CRC32 over a span.</summary>
    public static uint Compute(ReadOnlySpan<byte> data)
    {
        uint c = 0;
        for (int i = 0; i < data.Length; i++) c = Update(c, data[i]);
        return c;
    }
}
