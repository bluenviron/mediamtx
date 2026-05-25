using Mediar.IO;

namespace Mediar.Containers.Matroska;

/// <summary>
/// Reads EBML (RFC 8794) variable-length integers and primitive values from an
/// <see cref="IRandomAccessSource"/>. Used by the Matroska demuxer to walk the
/// element tree without buffering the whole file.
/// </summary>
internal sealed class EbmlReader
{
    private readonly IRandomAccessSource _source;
    private long _position;

    public EbmlReader(IRandomAccessSource source, long position = 0)
    {
        _source = source;
        _position = position;
    }

    public long Position
    {
        get => _position;
        set => _position = value;
    }

    public long Length => _source.Length;

    public bool Eof => _position >= _source.Length;

    /// <summary>
    /// Read a variable-length unsigned EBML integer at the current position.
    /// Returns the value, the encoded length in bytes, and whether the value
    /// is the "unknown" sentinel (all ones).
    /// </summary>
    public ulong ReadVarInt(out int bytesRead, out bool isUnknown, bool keepLeadingBit = false)
    {
        Span<byte> first = stackalloc byte[1];
        if (_source.Read(_position, first) != 1) throw new EndOfStreamException("EBML EOF.");
        byte b0 = first[0];
        if (b0 == 0) throw new InvalidDataException("Invalid EBML var-int (leading zero byte).");

        int len = 1;
        byte mask = 0x80;
        while ((b0 & mask) == 0)
        {
            len++;
            mask >>= 1;
            if (len > 8) throw new InvalidDataException("EBML var-int too long.");
        }

        Span<byte> buf = stackalloc byte[8];
        buf[0] = b0;
        if (len > 1)
        {
            if (_source.Read(_position + 1, buf.Slice(1, len - 1)) != len - 1)
                throw new EndOfStreamException("Truncated EBML var-int.");
        }

        ulong value;
        if (keepLeadingBit)
        {
            value = 0;
            for (int i = 0; i < len; i++) value = (value << 8) | buf[i];
        }
        else
        {
            // Strip the length marker bit from the first byte.
            value = (ulong)(buf[0] & (mask - 1));
            for (int i = 1; i < len; i++) value = (value << 8) | buf[i];
        }
        _position += len;
        bytesRead = len;

        // "All data bits set" is the "unknown size" marker.
        ulong allOnes = (1UL << (7 * len)) - 1;
        isUnknown = !keepLeadingBit && value == allOnes;
        return value;
    }

    public ulong ReadElementId(out int bytesRead)
    {
        return ReadVarInt(out bytesRead, out _, keepLeadingBit: true);
    }

    public ulong ReadUInt(int length)
    {
        if (length <= 0) return 0;
        if (length > 8) throw new InvalidDataException("EBML uint > 8 bytes.");
        Span<byte> buf = stackalloc byte[8];
        var slice = buf[..length];
        if (_source.Read(_position, slice) != length)
            throw new EndOfStreamException("Truncated EBML uint.");
        ulong v = 0;
        for (int i = 0; i < length; i++) v = (v << 8) | slice[i];
        _position += length;
        return v;
    }

    public long ReadInt(int length)
    {
        if (length <= 0) return 0;
        if (length > 8) throw new InvalidDataException("EBML int > 8 bytes.");
        Span<byte> buf = stackalloc byte[8];
        var slice = buf[..length];
        if (_source.Read(_position, slice) != length)
            throw new EndOfStreamException("Truncated EBML int.");
        long v = (slice[0] & 0x80) != 0 ? -1L : 0L;
        for (int i = 0; i < length; i++) v = (v << 8) | slice[i];
        _position += length;
        return v;
    }

    public double ReadFloat(int length)
    {
        Span<byte> buf = stackalloc byte[8];
        if (length == 4)
        {
            if (_source.Read(_position, buf[..4]) != 4) throw new EndOfStreamException();
            uint asInt = ((uint)buf[0] << 24) | ((uint)buf[1] << 16) | ((uint)buf[2] << 8) | buf[3];
            _position += 4;
            return BitConverter.UInt32BitsToSingle(asInt);
        }
        if (length == 8)
        {
            if (_source.Read(_position, buf[..8]) != 8) throw new EndOfStreamException();
            ulong asInt = 0;
            for (int i = 0; i < 8; i++) asInt = (asInt << 8) | buf[i];
            _position += 8;
            return BitConverter.UInt64BitsToDouble(asInt);
        }
        if (length == 0) return 0;
        throw new InvalidDataException($"Unsupported EBML float length {length}.");
    }

    public byte[] ReadBytes(long length)
    {
        if (length < 0 || length > int.MaxValue) throw new InvalidDataException("EBML bytes too large.");
        byte[] data = new byte[(int)length];
        if (_source.Read(_position, data) != length)
            throw new EndOfStreamException("Truncated EBML bytes.");
        _position += length;
        return data;
    }

    public string ReadString(long length)
    {
        byte[] data = ReadBytes(length);
        int end = data.Length;
        while (end > 0 && data[end - 1] == 0) end--;
        return System.Text.Encoding.UTF8.GetString(data, 0, end);
    }

    public void Skip(long length)
    {
        _position += length;
    }
}
