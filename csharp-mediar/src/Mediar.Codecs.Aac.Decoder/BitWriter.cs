namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Forward-only MSB-first bit writer used to serialise AudioSpecificConfig
/// payloads. The writer grows a backing list on demand and emits a final
/// byte array via <see cref="ToArray"/>; bits beyond the last full byte are
/// flushed left-justified (matching how AAC config blobs are stored).
/// </summary>
internal sealed class BitWriter
{
    private readonly List<byte> _bytes = [];
    private int _bitOffset;

    /// <summary>Append <paramref name="count"/> bits (1..32) MSB-first.</summary>
    public void Write(uint value, int count)
    {
        if (count <= 0 || count > 32) throw new ArgumentOutOfRangeException(nameof(count));

        for (int i = count - 1; i >= 0; i--)
        {
            int bit = (int)((value >> i) & 1);
            int byteIndex = _bitOffset >> 3;
            int bitInByte = _bitOffset & 7;
            if (byteIndex == _bytes.Count) _bytes.Add(0);
            if (bit != 0)
            {
                _bytes[byteIndex] |= (byte)(1 << (7 - bitInByte));
            }
            _bitOffset++;
        }
    }

    /// <summary>Convenience overload for signed values that always fit in <paramref name="count"/> bits.</summary>
    public void Write(int value, int count) => Write((uint)value, count);

    /// <summary>Current bit position from the start of the buffer.</summary>
    public int BitPosition => _bitOffset;

    /// <summary>Pad with zero bits until the cursor lands on the next byte boundary; no-op when already aligned.</summary>
    public void AlignToByte()
    {
        int rem = _bitOffset & 7;
        if (rem != 0) Write(0u, 8 - rem);
    }

    /// <summary>Materialise the buffered bits as a byte array. Trailing bits inside the last byte are zero-padded.</summary>
    public byte[] ToArray() => _bytes.ToArray();
}
