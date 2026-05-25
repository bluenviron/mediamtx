using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.IO;

/// <summary>
/// A cursor-tracking big-endian reader over a <see cref="ReadOnlySpan{Byte}"/>.
/// </summary>
/// <remarks>
/// Designed for parsing network-byte-order container formats (ISO BMFF, MPEG, FLAC, ...).
/// Methods throw <see cref="EndOfStreamException"/> when the requested number of bytes
/// is not available – callers should validate buffer length once at the boundary and
/// rely on bounds-checked slicing inside hot loops.
/// </remarks>
public ref struct BigEndianSpanReader
{
    private readonly ReadOnlySpan<byte> _span;
    private int _position;

    /// <summary>Create a reader positioned at the start of <paramref name="span"/>.</summary>
    public BigEndianSpanReader(ReadOnlySpan<byte> span)
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

    /// <summary>Returns the bytes already consumed.</summary>
    public readonly ReadOnlySpan<byte> ConsumedSpan => _span[.._position];

    /// <summary>Returns the bytes not yet consumed.</summary>
    public readonly ReadOnlySpan<byte> RemainingSpan => _span[_position..];

    /// <summary>Advance the cursor without reading.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void Skip(int count)
    {
        if ((uint)count > (uint)Remaining)
        {
            ThrowOutOfRange();
        }
        _position += count;
    }

    /// <summary>Read a single byte.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public byte ReadUInt8()
    {
        if (_position >= _span.Length) ThrowOutOfRange();
        return _span[_position++];
    }

    /// <summary>Read a signed 8-bit integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public sbyte ReadInt8() => (sbyte)ReadUInt8();

    /// <summary>Read a big-endian 16-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public ushort ReadUInt16()
    {
        var v = BinaryPrimitives.ReadUInt16BigEndian(EnsureSlice(2));
        _position += 2;
        return v;
    }

    /// <summary>Read a big-endian 16-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public short ReadInt16() => (short)ReadUInt16();

    /// <summary>Read a big-endian 24-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public uint ReadUInt24()
    {
        var s = EnsureSlice(3);
        var v = ((uint)s[0] << 16) | ((uint)s[1] << 8) | s[2];
        _position += 3;
        return v;
    }

    /// <summary>Read a big-endian 32-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public uint ReadUInt32()
    {
        var v = BinaryPrimitives.ReadUInt32BigEndian(EnsureSlice(4));
        _position += 4;
        return v;
    }

    /// <summary>Read a big-endian 32-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public int ReadInt32() => (int)ReadUInt32();

    /// <summary>Read a big-endian 64-bit unsigned integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public ulong ReadUInt64()
    {
        var v = BinaryPrimitives.ReadUInt64BigEndian(EnsureSlice(8));
        _position += 8;
        return v;
    }

    /// <summary>Read a big-endian 64-bit signed integer.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public long ReadInt64() => (long)ReadUInt64();

    /// <summary>Read an IEEE 754 32-bit float (big-endian byte order).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public float ReadSingle()
    {
        var v = BinaryPrimitives.ReadSingleBigEndian(EnsureSlice(4));
        _position += 4;
        return v;
    }

    /// <summary>Read a 16.16 fixed-point as a double (common in ISO BMFF).</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public double ReadFixed16Dot16()
    {
        int v = ReadInt32();
        return v / 65536.0;
    }

    /// <summary>Read an 8.8 fixed-point as a double.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public double ReadFixed8Dot8()
    {
        short v = ReadInt16();
        return v / 256.0;
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

    /// <summary>Peek <paramref name="count"/> bytes ahead without consuming.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public readonly ReadOnlySpan<byte> Peek(int count)
    {
        if ((uint)count > (uint)Remaining) ThrowOutOfRange();
        return _span.Slice(_position, count);
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
