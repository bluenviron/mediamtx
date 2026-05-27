using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Ccitt;

/// <summary>
/// MSB-first bit reader over a byte buffer, tuned for CCITT decoding.
/// Provides <see cref="Peek(int)"/> for fast lookup-table lookups and
/// <see cref="Skip(int)"/> to commit bits after a successful decode.
/// </summary>
/// <remarks>
/// Reading past the end of the buffer returns zero bits so that decoders
/// can detect run-out via the same "invalid code" path they use for
/// corrupt input.
/// </remarks>
internal struct CcittBitReader
{
    private readonly ReadOnlyMemory<byte> _buffer;
    private long _bitPos;

    public CcittBitReader(ReadOnlyMemory<byte> buffer)
    {
        _buffer = buffer;
        _bitPos = 0;
    }

    /// <summary>Total bits in the input buffer.</summary>
    public readonly long TotalBits => (long)_buffer.Length * 8;

    /// <summary>Current bit position (0-based, MSB-first).</summary>
    public readonly long BitPosition => _bitPos;

    /// <summary>True once the reader has consumed every bit in the buffer.</summary>
    public readonly bool IsAtEnd => _bitPos >= TotalBits;

    /// <summary>
    /// Returns the next <paramref name="bitCount"/> bits (1..24) as an
    /// integer, MSB-first. If fewer bits are available, the missing
    /// low-order bits are treated as zero.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public readonly uint Peek(int bitCount)
    {
        var span = _buffer.Span;
        long bp = _bitPos;
        uint result = 0;
        for (int i = 0; i < bitCount; i++)
        {
            long byteIdx = (bp + i) >> 3;
            if (byteIdx >= span.Length)
            {
                result <<= 1;
                continue;
            }
            int bitIdx = 7 - (int)((bp + i) & 7);
            uint bit = (uint)((span[(int)byteIdx] >> bitIdx) & 1);
            result = (result << 1) | bit;
        }
        return result;
    }

    /// <summary>Advance the cursor by <paramref name="bitCount"/> bits.</summary>
    public void Skip(int bitCount) => _bitPos += bitCount;

    /// <summary>Read a single bit and advance.</summary>
    public uint ReadBit()
    {
        uint b = Peek(1);
        _bitPos++;
        return b;
    }

    /// <summary>
    /// Align the cursor to the next byte boundary. Used by T.4 with the
    /// EOL byte-align option, and to skip fill bits after EOL markers.
    /// </summary>
    public void AlignToByte()
    {
        long mod = _bitPos & 7;
        if (mod != 0) _bitPos += 8 - mod;
    }
}
