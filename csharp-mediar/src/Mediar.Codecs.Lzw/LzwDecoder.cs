using System.Buffers;

namespace Mediar.Codecs.Lzw;

/// <summary>
/// Variable-width LZW decoder shared by every Mediar imaging reader that
/// embeds LZW (GIF, TIFF, PDF, Postscript). The same kernel covers both
/// the GIF (LSB-first, in-band CLEAR / EOI) and TIFF 6.0 (MSB-first,
/// "early-change") dialects via <see cref="LzwOptions"/>.
/// </summary>
/// <remarks>
/// The dictionary is materialised as flat arenas (parent / tail / length
/// arrays of <c>1 &lt;&lt; MaxBits</c> entries) so the inner loop is
/// allocation-free. The output is collected through pooled buffers from
/// <see cref="ArrayPool{T}"/>. The codec is fully AOT-safe and contains
/// no reflection.
/// </remarks>
public static class LzwDecoder
{
    /// <summary>
    /// Decompresses an LZW byte stream and returns the inflated bytes.
    /// </summary>
    /// <param name="source">Raw LZW-encoded bytes.</param>
    /// <param name="options">Dialect parameters — use
    /// <see cref="LzwOptions.Gif"/> or <see cref="LzwOptions.Tiff"/>.</param>
    /// <param name="expectedLength">
    /// Optional hint for the inflated byte count. When &gt; 0 the result
    /// is sized exactly to this value and decoding stops once the buffer
    /// is full; this is the canonical GIF behaviour (one frame = width ×
    /// height pixels). Pass <c>-1</c> when the size is not known up front
    /// (TIFF strips, PDF streams).
    /// </param>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="options"/> has an out-of-range <c>InitialBits</c>
    /// or <c>MaxBits</c>.
    /// </exception>
    public static byte[] Decode(ReadOnlySpan<byte> source, in LzwOptions options, int expectedLength = -1)
    {
        ValidateOptions(options);

        int initialAlphabet = 1 << options.InitialBits;
        int clearCode = initialAlphabet;
        int endCode = clearCode + 1;
        int maxDictEntries = 1 << options.MaxBits;

        int[] parent = ArrayPool<int>.Shared.Rent(maxDictEntries);
        byte[] tail = ArrayPool<byte>.Shared.Rent(maxDictEntries);
        int[] length = ArrayPool<int>.Shared.Rent(maxDictEntries);
        byte[] scratch = ArrayPool<byte>.Shared.Rent(maxDictEntries);

        try
        {
            byte[] outBuf;
            int outCap;
            int outLen = 0;
            bool outOwnsRented;
            if (expectedLength >= 0)
            {
                outBuf = new byte[expectedLength];
                outCap = expectedLength;
                outOwnsRented = false;
            }
            else
            {
                outBuf = ArrayPool<byte>.Shared.Rent(Math.Max(1024, source.Length * 2));
                outCap = outBuf.Length;
                outOwnsRented = true;
            }

            ResetDictionary(parent, tail, length, initialAlphabet, out int dictCount, out int codeSize, options.InitialBits);

            int bitBuf = 0;
            int bitCount = 0;
            int srcPos = 0;
            int prevCode = -1;
            bool lsb = options.BitOrder == LzwBitOrder.LsbFirst;
            bool earlyChange = options.EarlyChange;
            int maxBits = options.MaxBits;

            while (true)
            {
                while (bitCount < codeSize && srcPos < source.Length)
                {
                    if (lsb)
                    {
                        bitBuf |= source[srcPos++] << bitCount;
                    }
                    else
                    {
                        bitBuf = (bitBuf << 8) | source[srcPos++];
                    }
                    bitCount += 8;
                }
                if (bitCount < codeSize) break;

                int code;
                if (lsb)
                {
                    code = bitBuf & ((1 << codeSize) - 1);
                    bitBuf >>>= codeSize;
                }
                else
                {
                    code = (bitBuf >> (bitCount - codeSize)) & ((1 << codeSize) - 1);
                }
                bitCount -= codeSize;

                if (code == clearCode)
                {
                    ResetDictionary(parent, tail, length, initialAlphabet, out dictCount, out codeSize, options.InitialBits);
                    prevCode = -1;
                    continue;
                }
                if (code == endCode)
                {
                    break;
                }

                int entryLen;
                if (code < dictCount)
                {
                    entryLen = MaterialiseEntry(parent, tail, length, code, scratch);
                }
                else if (code == dictCount && prevCode >= 0)
                {
                    // K + first(K) — the classic LZW edge case.
                    entryLen = MaterialiseEntry(parent, tail, length, prevCode, scratch);
                    scratch[entryLen++] = scratch[0];
                }
                else
                {
                    break; // corrupt stream — match prior tolerant behaviour
                }

                int writable = expectedLength >= 0 ? Math.Min(entryLen, outCap - outLen) : entryLen;
                if (writable < 0)
                {
                    writable = 0;
                }
                if (writable > 0)
                {
                    if (expectedLength < 0)
                    {
                        EnsureCapacity(ref outBuf, ref outCap, ref outOwnsRented, outLen + writable);
                    }
                    Buffer.BlockCopy(scratch, 0, outBuf, outLen, writable);
                    outLen += writable;
                }
                if (expectedLength >= 0 && outLen >= outCap)
                {
                    break;
                }

                if (prevCode >= 0 && dictCount < maxDictEntries)
                {
                    parent[dictCount] = prevCode;
                    tail[dictCount] = scratch[0];
                    length[dictCount] = length[prevCode] + 1;
                    dictCount++;

                    int threshold = earlyChange ? (1 << codeSize) - 1 : (1 << codeSize);
                    if (dictCount >= threshold && codeSize < maxBits)
                    {
                        codeSize++;
                    }
                }

                prevCode = code < dictCount - 1 ? code : dictCount - 1;
            }

            byte[] result;
            if (expectedLength >= 0)
            {
                result = outBuf;
            }
            else
            {
                result = new byte[outLen];
                Buffer.BlockCopy(outBuf, 0, result, 0, outLen);
                if (outOwnsRented)
                {
                    ArrayPool<byte>.Shared.Return(outBuf);
                }
            }
            return result;
        }
        finally
        {
            ArrayPool<int>.Shared.Return(parent);
            ArrayPool<byte>.Shared.Return(tail);
            ArrayPool<int>.Shared.Return(length);
            ArrayPool<byte>.Shared.Return(scratch);
        }
    }

    /// <summary>Decompresses a GIF LZW frame.</summary>
    /// <param name="source">Concatenated LZW sub-block payload from a GIF frame.</param>
    /// <param name="lzwMinCodeSize">The byte stored at the start of the image data block.</param>
    /// <param name="pixelCount">Frame width × height; output is sized to this exactly.</param>
    public static byte[] DecodeGif(ReadOnlySpan<byte> source, int lzwMinCodeSize, int pixelCount)
        => Decode(source, LzwOptions.Gif(lzwMinCodeSize), pixelCount);

    /// <summary>Decompresses a TIFF 6.0 LZW strip / tile.</summary>
    public static byte[] DecodeTiff(ReadOnlySpan<byte> source)
        => Decode(source, LzwOptions.Tiff(), expectedLength: -1);

    private static int MaterialiseEntry(int[] parent, byte[] tail, int[] length, int code, byte[] scratch)
    {
        int len = length[code];
        int cur = code;
        for (int i = len - 1; i >= 0; i--)
        {
            scratch[i] = tail[cur];
            cur = parent[cur];
            if (cur < 0)
            {
                break;
            }
        }
        return len;
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

    private static void ResetDictionary(
        int[] parent,
        byte[] tail,
        int[] length,
        int alphabet,
        out int dictCount,
        out int codeSize,
        int initialBits)
    {
        for (int i = 0; i < alphabet; i++)
        {
            parent[i] = -1;
            tail[i] = (byte)i;
            length[i] = 1;
        }
        parent[alphabet] = -1;
        length[alphabet] = 0;
        parent[alphabet + 1] = -1;
        length[alphabet + 1] = 0;
        dictCount = alphabet + 2;
        codeSize = initialBits + 1;
    }

    private static void EnsureCapacity(ref byte[] buf, ref int cap, ref bool owns, int required)
    {
        if (required <= cap)
        {
            return;
        }
        int newCap = cap;
        while (newCap < required)
        {
            newCap *= 2;
        }
        byte[] grown = ArrayPool<byte>.Shared.Rent(newCap);
        Buffer.BlockCopy(buf, 0, grown, 0, cap);
        if (owns)
        {
            ArrayPool<byte>.Shared.Return(buf);
        }
        buf = grown;
        cap = grown.Length;
        owns = true;
    }
}
