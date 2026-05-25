namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis bitstream reader. Vorbis packs bits LSB-first within each byte
/// (i.e. the first bit emitted by the encoder occupies the lowest bit of the
/// first byte). This struct is mutated in place; it does not copy the source
/// buffer.
/// </summary>
internal ref struct VorbisBitReader
{
    private readonly ReadOnlySpan<byte> _buffer;
    private int _bitOffset;

    public VorbisBitReader(ReadOnlySpan<byte> buffer)
    {
        _buffer = buffer;
        _bitOffset = 0;
        EndOfPacket = false;
    }

    /// <summary>Bit offset within the buffer (LSB-first).</summary>
    public int Position => _bitOffset;

    /// <summary>Total number of bits in the buffer.</summary>
    public int Length => _buffer.Length * 8;

    /// <summary>Bits remaining in the buffer.</summary>
    public int Remaining => Length - _bitOffset;

    /// <summary>True if any reads have run past the end of the packet.</summary>
    public bool EndOfPacket { get; private set; }

    /// <summary>
    /// Read <paramref name="bits"/> bits (0–32). Returns the unsigned integer
    /// value packed LSB-first. Reads past end of packet return 0 and set the
    /// <see cref="EndOfPacket"/> flag (spec: "Reading past the end of the
    /// packet returns zeros indefinitely").
    /// </summary>
    public uint ReadBits(int bits)
    {
        if (bits == 0) return 0;
        if ((uint)bits > 32) throw new ArgumentOutOfRangeException(nameof(bits));

        uint value = 0;
        int produced = 0;
        while (produced < bits)
        {
            int byteIndex = _bitOffset >> 3;
            int bitInByte = _bitOffset & 7;
            if (byteIndex >= _buffer.Length)
            {
                EndOfPacket = true;
                return value;
            }
            int available = 8 - bitInByte;
            int wanted = bits - produced;
            int take = wanted < available ? wanted : available;

            uint chunk = (uint)((_buffer[byteIndex] >> bitInByte) & ((1 << take) - 1));
            value |= chunk << produced;
            produced += take;
            _bitOffset += take;
        }
        return value;
    }

    /// <summary>Read a single bit as a bool.</summary>
    public bool ReadBit() => ReadBits(1) != 0;

    /// <summary>Read <paramref name="bits"/> bits and sign-extend the result to <see cref="int"/>.</summary>
    public int ReadBitsSigned(int bits)
    {
        if (bits == 0) return 0;
        uint v = ReadBits(bits);
        if (bits < 32 && (v & (1u << (bits - 1))) != 0)
        {
            v |= ~((1u << bits) - 1);
        }
        return (int)v;
    }

    /// <summary>
    /// Vorbis spec §9.2.1 — <c>ilog(x)</c>: position of the highest set bit in
    /// <paramref name="x"/>, 1-indexed. <c>ilog(0) = 0</c>, <c>ilog(1) = 1</c>,
    /// <c>ilog(2) = 2</c>, <c>ilog(3) = 2</c>, <c>ilog(4) = 3</c>.
    /// </summary>
    public static int Ilog(int x)
    {
        if (x <= 0) return 0;
        int r = 0;
        while (x > 0) { r++; x >>= 1; }
        return r;
    }

    /// <summary>
    /// Vorbis spec §9.2.2 — <c>float32_unpack</c>. Decodes a Vorbis packed
    /// 32-bit float as defined in the codec spec (custom format, not IEEE 754).
    /// </summary>
    public static float Float32Unpack(uint x)
    {
        uint mantissa = x & 0x1FFFFFu;
        uint sign = x & 0x80000000u;
        int exponent = (int)((x & 0x7FE00000u) >> 21);
        double m = mantissa;
        if (sign != 0) m = -m;
        return (float)(m * Math.Pow(2.0, exponent - 788));
    }

    /// <summary>
    /// Vorbis spec §9.2.3 — <c>lookup1_values</c>. The greatest integer
    /// <c>r</c> such that <c>r^dimensions &lt;= entries</c>.
    /// </summary>
    public static int Lookup1Values(int entries, int dimensions)
    {
        if (entries <= 0 || dimensions <= 0) return 0;
        int r = (int)Math.Floor(Math.Pow(entries, 1.0 / dimensions));
        while (Pow(r + 1, dimensions) <= entries) r++;
        while (r > 0 && Pow(r, dimensions) > entries) r--;
        return r;
    }

    private static long Pow(int b, int e)
    {
        long r = 1;
        for (int i = 0; i < e; i++)
        {
            r *= b;
            if (r > int.MaxValue) return long.MaxValue;
        }
        return r;
    }
}
