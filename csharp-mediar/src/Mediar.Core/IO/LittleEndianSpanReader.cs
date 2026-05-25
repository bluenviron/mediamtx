using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.IO;

/// <summary>
/// A cursor-tracking little-endian reader over a <see cref="ReadOnlySpan{Byte}"/>.
/// Mirrors <see cref="BigEndianSpanReader"/> for formats using little-endian byte order
/// (RIFF/WAVE, FLAC, Matroska variable-length integers excluded).
/// </summary>
public ref struct LittleEndianSpanReader
{
    private readonly ReadOnlySpan<byte> _span;
    private int _position;

    /// <summary>Create a reader positioned at the start of <paramref name="span"/>.</summary>
    public LittleEndianSpanReader(ReadOnlySpan<byte> span)
    {
        _span = span;
        _position = 0;
    }

    /// <summary>Current read position in bytes.</summary>
    public int Position
    {
        readonly get => _position;
        set
        {
            if ((uint)value > (uint)_span.Length)
            {
                ThrowOutOfRange();
            }
            _position = value;
        }
    }

    /// <summary>Total length of the underlying span.</summary>
    public readonly int Length => _span.Length;

    /// <summary>Number of bytes still readable.</summary>
    public readonly int Remaining => _span.Length - _position;

    /// <summary>Underlying span as read-only.</summary>
    public readonly ReadOnlySpan<byte> Span => _span;

    /// <summary>Bytes not yet consumed.</summary>
    public readonly ReadOnlySpan<byte> RemainingSpan => _span[_position..];

    /// <summary>Advance the cursor without reading.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void Skip(int count)
    {
        if ((uint)count > (uint)Remaining) ThrowOutOfRange();
        _position += count;
    }

    /// <summary>Read a single byte.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public byte ReadUInt8()
    {
        if (_position >= _span.Length) ThrowOutOfRange();
        return _span[_position++];
    }

    /// <summary>Read a little-endian 16-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public ushort ReadUInt16()
    {
        var v = BinaryPrimitives.ReadUInt16LittleEndian(EnsureSlice(2));
        _position += 2;
        return v;
    }

    /// <summary>Read a little-endian 16-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public short ReadInt16() => (short)ReadUInt16();

    /// <summary>Read a little-endian 24-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public uint ReadUInt24()
    {
        var s = EnsureSlice(3);
        uint v = s[0] | ((uint)s[1] << 8) | ((uint)s[2] << 16);
        _position += 3;
        return v;
    }

    /// <summary>Read a little-endian 32-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public uint ReadUInt32()
    {
        var v = BinaryPrimitives.ReadUInt32LittleEndian(EnsureSlice(4));
        _position += 4;
        return v;
    }

    /// <summary>Read a little-endian 32-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public int ReadInt32() => (int)ReadUInt32();

    /// <summary>Read a little-endian 64-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public ulong ReadUInt64()
    {
        var v = BinaryPrimitives.ReadUInt64LittleEndian(EnsureSlice(8));
        _position += 8;
        return v;
    }

    /// <summary>Read a little-endian 64-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public long ReadInt64() => (long)ReadUInt64();

    /// <summary>Read an IEEE 754 32-bit float (little-endian byte order).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public float ReadSingle()
    {
        var v = BinaryPrimitives.ReadSingleLittleEndian(EnsureSlice(4));
        _position += 4;
        return v;
    }

    /// <summary>Read <paramref name="count"/> bytes and return them as a slice (no copy).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public ReadOnlySpan<byte> ReadBytes(int count)
    {
        var slice = EnsureSlice(count);
        _position += count;
        return slice;
    }

    /// <summary>Read a fixed-length ASCII string. Stops at the first NUL.</summary>
    public string ReadAsciiString(int length)
    {
        var slice = ReadBytes(length);
        int nul = slice.IndexOf((byte)0);
        if (nul >= 0) slice = slice[..nul];
        return System.Text.Encoding.ASCII.GetString(slice);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private readonly ReadOnlySpan<byte> EnsureSlice(int count)
    {
        if ((uint)count > (uint)(_span.Length - _position))
        {
            ThrowOutOfRange();
        }
        return _span.Slice(_position, count);
    }

    private static void ThrowOutOfRange() =>
        throw new EndOfStreamException("Read past end of span.");
}
