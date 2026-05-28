namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Forward-only MSB-first bit reader over a contiguous byte span. Used by
/// the AAC AudioSpecificConfig and (future) raw_data_block parsers. The
/// reader is a <c>ref struct</c> so it never escapes its caller's frame.
/// </summary>
internal ref struct BitReader
{
    private readonly ReadOnlySpan<byte> _buffer;
    private int _bitOffset;

    public BitReader(ReadOnlySpan<byte> buffer)
    {
        _buffer = buffer;
        _bitOffset = 0;
    }

    /// <summary>Total bit count in the underlying buffer.</summary>
    public int Length => _buffer.Length * 8;

    /// <summary>Current bit position from the start of the buffer.</summary>
    public int Position => _bitOffset;

    /// <summary>Remaining bits available to read.</summary>
    public int Remaining => Length - _bitOffset;

    /// <summary>True when the next bit is on a byte boundary.</summary>
    public bool IsByteAligned => (_bitOffset & 7) == 0;

    /// <summary>
    /// Read <paramref name="count"/> bits (1..32) MSB-first as an unsigned
    /// integer. Throws <see cref="EndOfStreamException"/> when the stream
    /// would underflow.
    /// </summary>
    public uint ReadBits(int count)
    {
        if (count <= 0 || count > 32)
            throw new ArgumentOutOfRangeException(nameof(count), count, "1..32");
        if (count > Remaining)
            throw new EndOfStreamException($"Need {count} bits, only {Remaining} remain.");

        uint result = 0;
        int remaining = count;
        while (remaining > 0)
        {
            int bytePos = _bitOffset >> 3;
            int bitInByte = _bitOffset & 7;
            int take = Math.Min(8 - bitInByte, remaining);
            int shift = 8 - bitInByte - take;
            uint slice = ((uint)_buffer[bytePos] >> shift) & ((1u << take) - 1u);
            result = (result << take) | slice;
            _bitOffset += take;
            remaining -= take;
        }
        return result;
    }

    /// <summary>Read a single bit as a bool.</summary>
    public bool ReadBit() => ReadBits(1) != 0;

    /// <summary>Skip <paramref name="count"/> bits (no-op for zero).</summary>
    public void Skip(int count)
    {
        if (count == 0) return;
        ArgumentOutOfRangeException.ThrowIfNegative(count);
        if (count > Remaining) throw new EndOfStreamException();
        _bitOffset += count;
    }

    /// <summary>Align the cursor to the next byte boundary.</summary>
    public void AlignToByte()
    {
        int rem = _bitOffset & 7;
        if (rem != 0) _bitOffset += 8 - rem;
    }
}
