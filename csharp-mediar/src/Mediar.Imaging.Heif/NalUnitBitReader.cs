namespace Mediar.Imaging.Heif;

/// <summary>
/// Shared MSB-first bit reader used by HEVC / AVC / VVC NAL unit
/// RBSP parsers. Supports the Exp-Golomb codes mandated by the
/// ITU-T H.264 / H.265 / H.266 specifications.
/// </summary>
internal ref struct NalUnitBitReader
{
    private readonly ReadOnlySpan<byte> _data;
    private int _bitPos;

    public NalUnitBitReader(ReadOnlySpan<byte> data)
    {
        _data = data;
        _bitPos = 0;
    }

    public int BitPosition => _bitPos;

    public bool ReadBit()
    {
        int bytePos = _bitPos >> 3;
        if (bytePos >= _data.Length) throw new EndOfBitstreamException();
        int bit = (_data[bytePos] >> (7 - (_bitPos & 7))) & 1;
        _bitPos++;
        return bit == 1;
    }

    public uint ReadBits(int count)
    {
        if (count is < 0 or > 32) throw new ArgumentOutOfRangeException(nameof(count));
        uint value = 0;
        for (int i = 0; i < count; i++)
        {
            value = (value << 1) | (ReadBit() ? 1u : 0u);
        }
        return value;
    }

    public ulong ReadBits64(int count)
    {
        if (count is < 0 or > 64) throw new ArgumentOutOfRangeException(nameof(count));
        ulong value = 0;
        for (int i = 0; i < count; i++)
        {
            value = (value << 1) | (ReadBit() ? 1ul : 0ul);
        }
        return value;
    }

    public void SkipBits(int count)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(count);
        int bytePos = (_bitPos + count) >> 3;
        if (bytePos > _data.Length) throw new EndOfBitstreamException();
        _bitPos += count;
    }

    public uint ReadUe()
    {
        int zeros = 0;
        while (!ReadBit())
        {
            zeros++;
            if (zeros > 31) throw new EndOfBitstreamException();
        }
        if (zeros == 0) return 0;
        uint suffix = ReadBits(zeros);
        return (1u << zeros) - 1u + suffix;
    }

    public int ReadSe()
    {
        uint codeNum = ReadUe();
        return (codeNum & 1) == 1 ? (int)((codeNum + 1) >> 1) : -(int)(codeNum >> 1);
    }

    public bool IsByteAligned() => (_bitPos & 7) == 0;

    public void AlignToByte()
    {
        int rem = 8 - (_bitPos & 7);
        if (rem != 8) SkipBits(rem);
    }

    /// <summary>
    /// Returns true iff more RBSP data follows the current bit position
    /// before the trailing <c>rbsp_stop_one_bit</c> + alignment zeros.
    /// Used by H.264 syntax elements gated by <c>more_rbsp_data()</c>
    /// (see ITU-T H.264 7.2).
    /// </summary>
    public bool HasMoreRbspData()
    {
        int lastByteIdx = _data.Length - 1;
        while (lastByteIdx >= 0 && _data[lastByteIdx] == 0) lastByteIdx--;
        if (lastByteIdx < 0) return false;
        int b = _data[lastByteIdx];
        int lowestSetBit = 0;
        while ((b & 1) == 0) { b >>= 1; lowestSetBit++; }
        int stopBitPos = lastByteIdx * 8 + (7 - lowestSetBit);
        return _bitPos < stopBitPos;
    }
}

internal sealed class EndOfBitstreamException : Exception
{
    public EndOfBitstreamException() : base("Bit stream ended unexpectedly.") { }
}
