using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.IO;

/// <summary>
/// A cursor-tracking big-endian writer that fills a caller-owned <see cref="Span{Byte}"/>.
/// Throws <see cref="ArgumentException"/> when the destination is too small.
/// </summary>
public ref struct BigEndianSpanWriter
{
    private readonly Span<byte> _span;
    private int _position;

    /// <summary>Wrap <paramref name="span"/> as a writable big-endian sink.</summary>
    public BigEndianSpanWriter(Span<byte> span)
    {
        _span = span;
        _position = 0;
    }

    /// <summary>Number of bytes written so far.</summary>
    public readonly int Position => _position;

    /// <summary>Remaining capacity.</summary>
    public readonly int Remaining => _span.Length - _position;

    /// <summary>Underlying span.</summary>
    public readonly Span<byte> Span => _span;

    /// <summary>Bytes written so far.</summary>
    public readonly ReadOnlySpan<byte> WrittenSpan => _span[.._position];

    /// <summary>Write a single byte.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt8(byte value)
    {
        if (_position >= _span.Length) ThrowFull();
        _span[_position++] = value;
    }

    /// <summary>Write a big-endian 16-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt16(ushort value)
    {
        BinaryPrimitives.WriteUInt16BigEndian(EnsureSlice(2), value);
        _position += 2;
    }

    /// <summary>Write a big-endian 24-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt24(uint value)
    {
        var slice = EnsureSlice(3);
        slice[0] = (byte)(value >> 16);
        slice[1] = (byte)(value >> 8);
        slice[2] = (byte)value;
        _position += 3;
    }

    /// <summary>Write a big-endian 32-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt32(uint value)
    {
        BinaryPrimitives.WriteUInt32BigEndian(EnsureSlice(4), value);
        _position += 4;
    }

    /// <summary>Write a big-endian 32-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteInt32(int value) => WriteUInt32((uint)value);

    /// <summary>Write a big-endian 64-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt64(ulong value)
    {
        BinaryPrimitives.WriteUInt64BigEndian(EnsureSlice(8), value);
        _position += 8;
    }

    /// <summary>Write a big-endian 64-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteInt64(long value) => WriteUInt64((ulong)value);

    /// <summary>Write a sequence of bytes.</summary>
    public void WriteBytes(ReadOnlySpan<byte> bytes)
    {
        bytes.CopyTo(EnsureSlice(bytes.Length));
        _position += bytes.Length;
    }

    /// <summary>Write an ASCII string. Caller is responsible for padding.</summary>
    public void WriteAscii(string value)
    {
        var slice = EnsureSlice(value.Length);
        System.Text.Encoding.ASCII.GetBytes(value, slice);
        _position += value.Length;
    }

    /// <summary>
    /// Reserve <paramref name="count"/> bytes for later back-patching and return a writable slice.
    /// Advances the cursor by <paramref name="count"/>.
    /// </summary>
    public Span<byte> Reserve(int count)
    {
        var slice = EnsureSlice(count);
        _position += count;
        return slice;
    }

    /// <summary>Set the position (used together with <see cref="Reserve"/>).</summary>
    public void Seek(int position)
    {
        if ((uint)position > (uint)_span.Length) ThrowFull();
        _position = position;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private readonly Span<byte> EnsureSlice(int count)
    {
        if ((uint)count > (uint)(_span.Length - _position))
        {
            ThrowFull();
        }
        return _span.Slice(_position, count);
    }

    private static void ThrowFull() =>
        throw new ArgumentException("Destination span is too small.");
}
