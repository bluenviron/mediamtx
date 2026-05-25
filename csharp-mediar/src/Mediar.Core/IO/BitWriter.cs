namespace Mediar.IO;

/// <summary>
/// MSB-first bit writer that packs bits into a caller-owned <see cref="Span{Byte}"/>.
/// The destination buffer must be zero-initialised; the writer ORs bits into existing
/// bytes when crossing byte boundaries.
/// </summary>
public ref struct BitWriter
{
    private readonly Span<byte> _span;
    private int _bytePos;
    private int _bitPos;

    /// <summary>Wrap <paramref name="span"/> as a bit-aligned MSB-first writer.</summary>
    public BitWriter(Span<byte> span)
    {
        _span = span;
        _bytePos = 0;
        _bitPos = 0;
    }

    /// <summary>Bits written so far.</summary>
    public readonly long BitPosition => (long)_bytePos * 8 + _bitPos;

    /// <summary>Bytes used so far (rounds up partial byte).</summary>
    public readonly int BytesWritten => _bitPos == 0 ? _bytePos : _bytePos + 1;

    /// <summary>Write <paramref name="count"/> bits of <paramref name="value"/> (MSB-first).</summary>
    public void WriteBits(uint value, int count)
    {
        if ((uint)count > 32)
        {
            throw new ArgumentOutOfRangeException(nameof(count));
        }

        while (count > 0)
        {
            int bitsLeftInByte = 8 - _bitPos;
            int take = Math.Min(count, bitsLeftInByte);
            int shift = bitsLeftInByte - take;
            uint chunk = (value >> (count - take)) & ((1u << take) - 1);
            _span[_bytePos] |= (byte)(chunk << shift);
            _bitPos += take;
            if (_bitPos == 8)
            {
                _bitPos = 0;
                _bytePos++;
            }
            count -= take;
        }
    }

    /// <summary>Write a single bit.</summary>
    public void WriteBit(bool bit) => WriteBits(bit ? 1u : 0u, 1);

    /// <summary>Align to the next byte boundary, filling the remaining bits with 0.</summary>
    public void AlignToByte()
    {
        if (_bitPos == 0) return;
        _bitPos = 0;
        _bytePos++;
    }
}
