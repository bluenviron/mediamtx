using System.Runtime.CompilerServices;

namespace Mediar.IO;

/// <summary>
/// MSB-first bit reader over a <see cref="ReadOnlySpan{Byte}"/>.
/// Used by codec bitstreams (H.264 RBSP, MP3, FLAC frame headers).
/// </summary>
public ref struct BitReader
{
    private readonly ReadOnlySpan<byte> _span;
    private int _bytePos;
    private int _bitPos;

    /// <summary>Create a reader positioned at the start of <paramref name="span"/>.</summary>
    public BitReader(ReadOnlySpan<byte> span)
    {
        _span = span;
        _bytePos = 0;
        _bitPos = 0;
    }

    /// <summary>Total number of bits in the underlying span.</summary>
    public readonly long TotalBits => (long)_span.Length * 8;

    /// <summary>Current bit position (0-based, MSB-first).</summary>
    public readonly long BitPosition => (long)_bytePos * 8 + _bitPos;

    /// <summary>True if at least <paramref name="count"/> bits remain unread.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public readonly bool CanRead(int count) => BitPosition + count <= TotalBits;

    /// <summary>
    /// Read up to 32 bits, MSB first. Returns the value right-aligned.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public uint ReadBits(int count)
    {
        if ((uint)count > 32)
        {
            throw new ArgumentOutOfRangeException(nameof(count), "Max 32 bits per call.");
        }
        if (!CanRead(count)) throw new EndOfStreamException("Bit reader exhausted.");

        uint result = 0;
        while (count > 0)
        {
            int bitsLeftInByte = 8 - _bitPos;
            int take = Math.Min(count, bitsLeftInByte);
            int shift = bitsLeftInByte - take;
            uint chunk = (uint)((_span[_bytePos] >> shift) & ((1 << take) - 1));
            result = (result << take) | chunk;
            _bitPos += take;
            if (_bitPos == 8)
            {
                _bitPos = 0;
                _bytePos++;
            }
            count -= take;
        }
        return result;
    }

    /// <summary>Read up to 64 bits, MSB first. Returns the value right-aligned.</summary>
    public ulong ReadBits64(int count)
    {
        if ((uint)count > 64)
        {
            throw new ArgumentOutOfRangeException(nameof(count));
        }
        if (count <= 32) return ReadBits(count);
        ulong hi = ReadBits(count - 32);
        ulong lo = ReadBits(32);
        return (hi << 32) | lo;
    }

    /// <summary>Read a single bit as a bool.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public bool ReadBit() => ReadBits(1) != 0;

    /// <summary>Skip <paramref name="count"/> bits.</summary>
    public void SkipBits(int count)
    {
        if (!CanRead(count)) throw new EndOfStreamException("Bit reader exhausted.");
        long pos = BitPosition + count;
        _bytePos = (int)(pos / 8);
        _bitPos = (int)(pos % 8);
    }

    /// <summary>
    /// Seek the cursor to an absolute bit position. Position must be in
    /// <c>[0, TotalBits]</c>. Useful for codecs that occasionally rewind a
    /// few bits (e.g. ALAC's truncated-binary suffix puts back the LSB).
    /// </summary>
    public void SeekToBit(long bitPosition)
    {
        if (bitPosition < 0 || bitPosition > TotalBits)
            throw new ArgumentOutOfRangeException(nameof(bitPosition));
        _bytePos = (int)(bitPosition / 8);
        _bitPos = (int)(bitPosition % 8);
    }

    /// <summary>Align the cursor to the next byte boundary.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void AlignToByte()
    {
        if (_bitPos == 0) return;
        _bitPos = 0;
        _bytePos++;
    }

    /// <summary>
    /// Read an unsigned Exp-Golomb code (used by H.264/H.265 syntax elements).
    /// </summary>
    public uint ReadUe()
    {
        int zeros = 0;
        while (CanRead(1) && !ReadBit()) zeros++;
        if (zeros == 0) return 0;
        uint suffix = ReadBits(zeros);
        return (1u << zeros) - 1 + suffix;
    }

    /// <summary>Read a signed Exp-Golomb code.</summary>
    public int ReadSe()
    {
        uint k = ReadUe();
        int magnitude = (int)((k + 1) >> 1);
        return (k & 1) == 1 ? magnitude : -magnitude;
    }
}
