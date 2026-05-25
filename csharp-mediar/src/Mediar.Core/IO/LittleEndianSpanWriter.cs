using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.IO;

/// <summary>
/// A cursor-tracking little-endian writer that fills a caller-owned <see cref="Span{Byte}"/>.
/// </summary>
public ref struct LittleEndianSpanWriter
{
    private readonly Span<byte> _span;
    private int _position;

    /// <summary>Wrap <paramref name="span"/> as a writable little-endian sink.</summary>
    public LittleEndianSpanWriter(Span<byte> span)
    {
        _span = span;
        _position = 0;
    }

    /// <summary>Bytes written so far.</summary>
    public readonly int Position => _position;

    /// <summary>Remaining capacity.</summary>
    public readonly int Remaining => _span.Length - _position;

    /// <summary>Underlying span.</summary>
    public readonly Span<byte> Span => _span;

    /// <summary>Bytes written so far as a read-only view.</summary>
    public readonly ReadOnlySpan<byte> WrittenSpan => _span[.._position];

    /// <summary>Write a single byte.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt8(byte value)
    {
        if (_position >= _span.Length) ThrowFull();
        _span[_position++] = value;
    }

    /// <summary>Write a little-endian 16-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt16(ushort value)
    {
        BinaryPrimitives.WriteUInt16LittleEndian(EnsureSlice(2), value);
        _position += 2;
    }

    /// <summary>Write a little-endian 24-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt24(uint value)
    {
        var slice = EnsureSlice(3);
        slice[0] = (byte)value;
        slice[1] = (byte)(value >> 8);
        slice[2] = (byte)(value >> 16);
        _position += 3;
    }

    /// <summary>Write a little-endian 32-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt32(uint value)
    {
        BinaryPrimitives.WriteUInt32LittleEndian(EnsureSlice(4), value);
        _position += 4;
    }

    /// <summary>Write a little-endian 32-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteInt32(int value) => WriteUInt32((uint)value);

    /// <summary>Write a little-endian 64-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteUInt64(ulong value)
    {
        BinaryPrimitives.WriteUInt64LittleEndian(EnsureSlice(8), value);
        _position += 8;
    }

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
