namespace Mediar.Codecs.Ccitt;

/// <summary>
/// MSB-first bit writer used by the CCITT encoders. Bits accumulate into
/// the internal buffer; <see cref="ToArray"/> returns the byte-aligned
/// result with any trailing partial byte zero-padded.
/// </summary>
internal sealed class CcittBitWriter
{
    private readonly List<byte> _bytes = new();
    private int _current;
    private int _bitsInCurrent;

    /// <summary>Number of bits emitted so far.</summary>
    public long BitCount => ((long)_bytes.Count * 8) + _bitsInCurrent;

    /// <summary>Append <paramref name="bitCount"/> low-order bits of <paramref name="value"/>, MSB-first.</summary>
    public void Write(uint value, int bitCount)
    {
        for (int i = bitCount - 1; i >= 0; i--)
        {
            int bit = (int)((value >> i) & 1);
            _current = (_current << 1) | bit;
            _bitsInCurrent++;
            if (_bitsInCurrent == 8)
            {
                _bytes.Add((byte)_current);
                _current = 0;
                _bitsInCurrent = 0;
            }
        }
    }

    /// <summary>Pad the current byte with zero bits and emit it.</summary>
    public void FlushByte()
    {
        if (_bitsInCurrent == 0) return;
        _current <<= 8 - _bitsInCurrent;
        _bytes.Add((byte)_current);
        _current = 0;
        _bitsInCurrent = 0;
    }

    /// <summary>Return the encoded bytes (auto-flushes any trailing partial byte).</summary>
    public byte[] ToArray()
    {
        if (_bitsInCurrent != 0)
        {
            int finalByte = _current << (8 - _bitsInCurrent);
            var arr = new byte[_bytes.Count + 1];
            _bytes.CopyTo(arr);
            arr[^1] = (byte)finalByte;
            return arr;
        }
        return _bytes.ToArray();
    }
}
