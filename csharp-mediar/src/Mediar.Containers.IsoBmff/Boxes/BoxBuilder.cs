using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// In-memory builder for ISO BMFF box trees.
/// Supports nesting boxes via <see cref="StartBox(FourCc)"/>/<see cref="EndBox"/>;
/// the size field is patched when the box closes.
/// </summary>
internal sealed class BoxBuilder
{
    // We grow as needed; typical moov sizes are well under 16 MiB even for long files.
    private byte[] _buffer;
    private int _length;
    private readonly Stack<int> _boxStart = new();

    public BoxBuilder(int initialCapacity = 16 * 1024)
    {
        _buffer = new byte[initialCapacity];
    }

    public int Length => _length;
    public ReadOnlySpan<byte> WrittenSpan => _buffer.AsSpan(0, _length);
    public byte[] ToArray() => _buffer.AsSpan(0, _length).ToArray();

    private void EnsureCapacity(int additional)
    {
        int required = _length + additional;
        if (required <= _buffer.Length) return;
        int newCap = Math.Max(_buffer.Length * 2, required);
        var n = new byte[newCap];
        Buffer.BlockCopy(_buffer, 0, n, 0, _length);
        _buffer = n;
    }

    public void StartBox(FourCc type)
    {
        EnsureCapacity(8);
        _boxStart.Push(_length);
        _length += 4; // size placeholder
        var w = new BigEndianSpanWriter(_buffer.AsSpan(_length, 4));
        w.WriteUInt32(type.Value);
        _length += 4;
    }

    /// <summary>Start a "FullBox" (version + 3-byte flags).</summary>
    public void StartFullBox(FourCc type, byte version, uint flags = 0)
    {
        StartBox(type);
        EnsureCapacity(4);
        _buffer[_length++] = version;
        _buffer[_length++] = (byte)(flags >> 16);
        _buffer[_length++] = (byte)(flags >> 8);
        _buffer[_length++] = (byte)flags;
    }

    public void EndBox()
    {
        int start = _boxStart.Pop();
        uint size = (uint)(_length - start);
        BinaryWriteUInt32BE(_buffer.AsSpan(start, 4), size);
    }

    public void WriteUInt8(byte value)
    {
        EnsureCapacity(1);
        _buffer[_length++] = value;
    }

    public void WriteUInt16(ushort value)
    {
        EnsureCapacity(2);
        _buffer[_length++] = (byte)(value >> 8);
        _buffer[_length++] = (byte)value;
    }

    public void WriteUInt24(uint value)
    {
        EnsureCapacity(3);
        _buffer[_length++] = (byte)(value >> 16);
        _buffer[_length++] = (byte)(value >> 8);
        _buffer[_length++] = (byte)value;
    }

    public void WriteUInt32(uint value)
    {
        EnsureCapacity(4);
        BinaryWriteUInt32BE(_buffer.AsSpan(_length, 4), value);
        _length += 4;
    }

    public void WriteInt32(int value) => WriteUInt32((uint)value);

    public void WriteUInt64(ulong value)
    {
        EnsureCapacity(8);
        _buffer[_length++] = (byte)(value >> 56);
        _buffer[_length++] = (byte)(value >> 48);
        _buffer[_length++] = (byte)(value >> 40);
        _buffer[_length++] = (byte)(value >> 32);
        _buffer[_length++] = (byte)(value >> 24);
        _buffer[_length++] = (byte)(value >> 16);
        _buffer[_length++] = (byte)(value >> 8);
        _buffer[_length++] = (byte)value;
    }

    public void WriteBytes(ReadOnlySpan<byte> bytes)
    {
        EnsureCapacity(bytes.Length);
        bytes.CopyTo(_buffer.AsSpan(_length));
        _length += bytes.Length;
    }

    public void WriteAscii(string s)
    {
        EnsureCapacity(s.Length);
        for (int i = 0; i < s.Length; i++) _buffer[_length++] = (byte)s[i];
    }

    public void WriteZeros(int count)
    {
        EnsureCapacity(count);
        _buffer.AsSpan(_length, count).Clear();
        _length += count;
    }

    public void WriteLanguage(string lang)
    {
        if (lang.Length != 3) lang = "und";
        int a = ((lang[0] - 0x60) & 0x1F);
        int b = ((lang[1] - 0x60) & 0x1F);
        int c = ((lang[2] - 0x60) & 0x1F);
        ushort packed = (ushort)((a << 10) | (b << 5) | c);
        WriteUInt16(packed);
    }

    private static void BinaryWriteUInt32BE(Span<byte> dst, uint value)
    {
        dst[0] = (byte)(value >> 24);
        dst[1] = (byte)(value >> 16);
        dst[2] = (byte)(value >> 8);
        dst[3] = (byte)value;
    }
}
