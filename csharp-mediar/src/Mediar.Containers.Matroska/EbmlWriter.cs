using System.Buffers.Binary;

namespace Mediar.Containers.Matroska;

/// <summary>
/// EBML write helpers used by <see cref="MatroskaMuxer"/>. Writes
/// variable-length integers, element headers and typed value payloads
/// (uint, int, float, string, binary) into a backing buffer that the
/// muxer then flushes to the output stream.
/// </summary>
internal sealed class EbmlWriter
{
    private byte[] _buffer;
    private int _length;

    public EbmlWriter(int initialCapacity = 1024)
    {
        _buffer = new byte[initialCapacity];
    }

    public int Length => _length;
    public ReadOnlySpan<byte> Written => _buffer.AsSpan(0, _length);

    private void EnsureCapacity(int extra)
    {
        if (_length + extra <= _buffer.Length) return;
        int newLen = _buffer.Length * 2;
        while (newLen < _length + extra) newLen *= 2;
        Array.Resize(ref _buffer, newLen);
    }

    public void WriteRaw(ReadOnlySpan<byte> data)
    {
        EnsureCapacity(data.Length);
        data.CopyTo(_buffer.AsSpan(_length));
        _length += data.Length;
    }

    /// <summary>Write an EBML element id (the id constants already carry the length marker).</summary>
    public void WriteId(ulong id)
    {
        Span<byte> tmp = stackalloc byte[8];
        int n = 0;
        // Determine how many bytes id occupies by finding the MSB.
        if (id <= 0xFF) n = 1;
        else if (id <= 0xFFFF) n = 2;
        else if (id <= 0xFFFFFF) n = 3;
        else if (id <= 0xFFFFFFFF) n = 4;
        else n = 8;
        for (int i = n - 1; i >= 0; i--) tmp[i] = (byte)(id >> (8 * (n - 1 - i)));
        WriteRaw(tmp[..n]);
    }

    /// <summary>Write a VINT element length. Uses the shortest representation.</summary>
    public void WriteVintLength(long value)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(value);
        int width = WidthForVint((ulong)value);
        Span<byte> tmp = stackalloc byte[width];
        ulong v = (ulong)value | (1UL << (7 * width));
        for (int i = width - 1; i >= 0; i--) { tmp[i] = (byte)v; v >>= 8; }
        WriteRaw(tmp);
    }

    /// <summary>Write a VINT of unknown size: all data bits = 1.</summary>
    public void WriteVintUnknown(int width = 8)
    {
        Span<byte> tmp = stackalloc byte[width];
        tmp[0] = width switch
        {
            1 => 0xFF,
            2 => 0x7F,
            3 => 0x3F,
            4 => 0x1F,
            5 => 0x0F,
            6 => 0x07,
            7 => 0x03,
            8 => 0x01,
            _ => throw new ArgumentOutOfRangeException(nameof(width)),
        };
        for (int i = 1; i < width; i++) tmp[i] = 0xFF;
        WriteRaw(tmp);
    }

    private static int WidthForVint(ulong v)
    {
        // Maximum representable value at width w is (1 << (7*w)) - 2 (since all-1 = unknown).
        for (int w = 1; w <= 8; w++)
        {
            ulong max = (1UL << (7 * w)) - 2;
            if (v <= max) return w;
        }
        throw new ArgumentOutOfRangeException(nameof(v));
    }

    public void WriteUInt(ulong id, ulong value)
    {
        WriteId(id);
        // Minimal-length unsigned representation.
        int n;
        if (value == 0) n = 1;
        else if (value <= 0xFF) n = 1;
        else if (value <= 0xFFFF) n = 2;
        else if (value <= 0xFFFFFF) n = 3;
        else if (value <= 0xFFFFFFFF) n = 4;
        else if (value <= 0xFFFFFFFFFFUL) n = 5;
        else if (value <= 0xFFFFFFFFFFFFUL) n = 6;
        else if (value <= 0xFFFFFFFFFFFFFFUL) n = 7;
        else n = 8;
        WriteVintLength(n);
        for (int i = n - 1; i >= 0; i--)
        {
            EnsureCapacity(1);
            _buffer[_length++] = (byte)(value >> (8 * i));
        }
    }

    public void WriteFloat64(ulong id, double value)
    {
        WriteId(id);
        WriteVintLength(8);
        EnsureCapacity(8);
        BinaryPrimitives.WriteDoubleBigEndian(_buffer.AsSpan(_length, 8), value);
        _length += 8;
    }

    public void WriteString(ulong id, string value)
    {
        WriteId(id);
        byte[] bytes = System.Text.Encoding.UTF8.GetBytes(value);
        WriteVintLength(bytes.Length);
        WriteRaw(bytes);
    }

    public void WriteBinary(ulong id, ReadOnlySpan<byte> data)
    {
        WriteId(id);
        WriteVintLength(data.Length);
        WriteRaw(data);
    }

    /// <summary>Write a master element by buffering child writes via the given action.</summary>
    public void WriteMaster(ulong id, Action<EbmlWriter> writeChildren)
    {
        var inner = new EbmlWriter();
        writeChildren(inner);
        WriteId(id);
        WriteVintLength(inner.Length);
        WriteRaw(inner.Written);
    }

    public byte[] ToArray() => _buffer.AsSpan(0, _length).ToArray();
}
