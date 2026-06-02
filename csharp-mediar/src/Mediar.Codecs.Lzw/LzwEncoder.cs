using System.Buffers;

namespace Mediar.Codecs.Lzw;

/// <summary>
/// Variable-width LZW encoder shared by Mediar imaging writers that emit
/// LZW (GIF, TIFF). Mirrors <see cref="LzwDecoder"/>: the same kernel
/// covers both the GIF (LSB-first, in-band CLEAR / EOI) and TIFF 6.0
/// (MSB-first, "early-change") dialects via <see cref="LzwOptions"/>.
/// </summary>
public static class LzwEncoder
{
    /// <summary>
    /// Compresses <paramref name="source"/> into an LZW byte stream using
    /// the given <paramref name="options"/>. A CLEAR code is emitted up
    /// front and an END code at the end of stream; the dictionary is
    /// reset and re-grown when it saturates so output is decode-clean
    /// against <see cref="LzwDecoder"/>.
    /// </summary>
    public static byte[] Encode(ReadOnlySpan<byte> source, in LzwOptions options)
    {
        ValidateOptions(options);

        int initialAlphabet = 1 << options.InitialBits;
        int clearCode = initialAlphabet;
        int endCode = clearCode + 1;
        int maxDictEntries = 1 << options.MaxBits;
        bool lsb = options.BitOrder == LzwBitOrder.LsbFirst;
        bool earlyChange = options.EarlyChange;
        int maxBits = options.MaxBits;

        // Hash dictionary keyed by (prefix << 8 | nextByte).
        int hashSize = NextPow2(maxDictEntries * 2);
        int[] keys = ArrayPool<int>.Shared.Rent(hashSize);
        int[] values = ArrayPool<int>.Shared.Rent(hashSize);
        Array.Fill(keys, -1, 0, hashSize);
        var output = new BitPacker(lsb, Math.Max(64, source.Length / 2));
        try
        {
            int codeSize = options.InitialBits + 1;
            int dictCount = endCode + 1;
            output.Write(clearCode, codeSize);

            if (source.Length == 0)
            {
                output.Write(endCode, codeSize);
                return output.ToArray();
            }

            int prefix = source[0];
            for (int i = 1; i < source.Length; i++)
            {
                byte b = source[i];
                int key = (prefix << 8) | b;
                int slot = HashFind(keys, hashSize, key);
                if (keys[slot] == key)
                {
                    prefix = values[slot];
                    continue;
                }

                output.Write(prefix, codeSize);

                if (dictCount < maxDictEntries)
                {
                    keys[slot] = key;
                    values[slot] = dictCount;
                    dictCount++;
                    // The decoder is always exactly one code "behind" the
                    // encoder (it skips the very first post-CLEAR add because
                    // prevCode is -1), so for the bumps to land on the same
                    // emit / read, the encoder fires its bump when the
                    // post-add count *exceeds* the same threshold the decoder
                    // is checking against ("strictly greater" rather than
                    // "greater or equal").
                    int threshold = earlyChange ? (1 << codeSize) - 1 : (1 << codeSize);
                    if (dictCount > threshold && codeSize < maxBits)
                    {
                        codeSize++;
                    }
                }
                else
                {
                    // Dictionary full → CLEAR and restart.
                    output.Write(clearCode, codeSize);
                    Array.Fill(keys, -1, 0, hashSize);
                    dictCount = endCode + 1;
                    codeSize = options.InitialBits + 1;
                }

                prefix = b;
            }
            output.Write(prefix, codeSize);
            output.Write(endCode, codeSize);
            return output.ToArray();
        }
        finally
        {
            ArrayPool<int>.Shared.Return(keys);
            ArrayPool<int>.Shared.Return(values);
        }
    }

    /// <summary>Compresses a GIF frame's pixel data into LZW sub-block payload bytes.</summary>
    public static byte[] EncodeGif(ReadOnlySpan<byte> source, int lzwMinCodeSize)
        => Encode(source, LzwOptions.Gif(lzwMinCodeSize));

    /// <summary>Compresses a TIFF strip / tile.</summary>
    public static byte[] EncodeTiff(ReadOnlySpan<byte> source)
        => Encode(source, LzwOptions.Tiff());

    private static int HashFind(int[] keys, int size, int key)
    {
        uint mask = (uint)(size - 1);
        uint h = (uint)key * 2654435769u;
        int slot = (int)(h & mask);
        while (keys[slot] != -1 && keys[slot] != key)
        {
            slot = (int)((slot + 1) & mask);
        }
        return slot;
    }

    private static int NextPow2(int n)
    {
        int r = 1;
        while (r < n) r <<= 1;
        return r;
    }

    private static void ValidateOptions(in LzwOptions o)
    {
        if (o.MaxBits is < 9 or > 16)
        {
            throw new ArgumentOutOfRangeException(nameof(o), o.MaxBits, "MaxBits must be between 9 and 16.");
        }
        if (o.InitialBits < 2 || o.InitialBits >= o.MaxBits)
        {
            throw new ArgumentOutOfRangeException(nameof(o), o.InitialBits, "InitialBits must be between 2 and MaxBits - 1.");
        }
    }

    /// <summary>
    /// Tiny bit-packer that supports both LSB- and MSB-first packing, used
    /// by both GIF (LSB) and TIFF (MSB) LZW streams.
    /// </summary>
    private sealed class BitPacker
    {
        private readonly bool _lsb;
        private readonly List<byte> _bytes;
        private uint _buf;
        private int _bits;

        public BitPacker(bool lsb, int capacity)
        {
            _lsb = lsb;
            _bytes = new List<byte>(capacity);
        }

        public void Write(int code, int width)
        {
            if (_lsb)
            {
                _buf |= (uint)code << _bits;
                _bits += width;
                while (_bits >= 8)
                {
                    _bytes.Add((byte)(_buf & 0xFF));
                    _buf >>= 8;
                    _bits -= 8;
                }
            }
            else
            {
                _buf = (_buf << width) | (uint)code;
                _bits += width;
                while (_bits >= 8)
                {
                    int shift = _bits - 8;
                    _bytes.Add((byte)((_buf >> shift) & 0xFF));
                    _bits -= 8;
                    _buf &= (1u << _bits) - 1;
                }
            }
        }

        public byte[] ToArray()
        {
            if (_bits > 0)
            {
                if (_lsb)
                {
                    _bytes.Add((byte)(_buf & 0xFF));
                }
                else
                {
                    _bytes.Add((byte)((_buf << (8 - _bits)) & 0xFF));
                }
                _bits = 0;
                _buf = 0;
            }
            return [.. _bytes];
        }
    }
}
