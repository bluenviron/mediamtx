using System.Buffers;

namespace Mediar.Codecs.PackBits;

/// <summary>
/// Apple PackBits run-length encoder / decoder. PackBits is byte-oriented
/// and works on any opaque byte stream: TIFF strips (compression 32773),
/// PSD raster layers, MacPaint scanlines, Adobe Illustrator previews.
/// </summary>
/// <remarks>
/// <para>
/// Encoding (Apple TN1023): each chunk starts with a control byte
/// <c>n</c> stored as <see cref="sbyte"/>:
/// </para>
/// <list type="bullet">
///   <item><description><c>n</c> = 0..127 → next <c>n + 1</c> bytes are literal.</description></item>
///   <item><description><c>n</c> = -127..-1 → next byte is repeated <c>-n + 1</c> times.</description></item>
///   <item><description><c>n</c> = -128 → no-op (skip).</description></item>
/// </list>
/// </remarks>
public static class PackBitsCodec
{
    /// <summary>
    /// Decompresses a PackBits byte stream.
    /// </summary>
    /// <param name="source">Raw PackBits-encoded bytes.</param>
    /// <param name="expectedLength">
    /// Optional inflated byte count. When &gt; 0 the result is sized
    /// exactly and decoding stops once the buffer is full; pass <c>-1</c>
    /// when the size is not known up front.
    /// </param>
    public static byte[] Decode(ReadOnlySpan<byte> source, int expectedLength = -1)
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
            outBuf = ArrayPool<byte>.Shared.Rent(Math.Max(64, source.Length * 2));
            outCap = outBuf.Length;
            outOwnsRented = true;
        }

        int i = 0;
        while (i < source.Length)
        {
            sbyte n = (sbyte)source[i++];
            if (n >= 0)
            {
                int count = n + 1;
                if (i + count > source.Length)
                {
                    break;
                }
                int writable = expectedLength >= 0 ? Math.Min(count, outCap - outLen) : count;
                if (writable <= 0)
                {
                    break;
                }
                if (expectedLength < 0)
                {
                    EnsureCapacity(ref outBuf, ref outCap, ref outOwnsRented, outLen + writable);
                }
                source.Slice(i, writable).CopyTo(outBuf.AsSpan(outLen, writable));
                outLen += writable;
                i += count;
            }
            else if (n != -128)
            {
                int count = -n + 1;
                if (i >= source.Length)
                {
                    break;
                }
                byte b = source[i++];
                int writable = expectedLength >= 0 ? Math.Min(count, outCap - outLen) : count;
                if (writable <= 0)
                {
                    break;
                }
                if (expectedLength < 0)
                {
                    EnsureCapacity(ref outBuf, ref outCap, ref outOwnsRented, outLen + writable);
                }
                outBuf.AsSpan(outLen, writable).Fill(b);
                outLen += writable;
            }
            // n == -128: no-op
            if (expectedLength >= 0 && outLen >= outCap)
            {
                break;
            }
        }

        if (expectedLength >= 0)
        {
            return outBuf;
        }
        byte[] result = new byte[outLen];
        Buffer.BlockCopy(outBuf, 0, result, 0, outLen);
        if (outOwnsRented)
        {
            ArrayPool<byte>.Shared.Return(outBuf);
        }
        return result;
    }

    /// <summary>
    /// Compresses bytes using the canonical PackBits encoding.
    /// </summary>
    /// <remarks>
    /// Produces the shortest legal encoding: runs of two or more equal
    /// bytes become RLE chunks (when the saving justifies the control
    /// byte); everything else becomes literal chunks of at most 128 bytes.
    /// </remarks>
    public static byte[] Encode(ReadOnlySpan<byte> source)
    {
        // Worst-case expansion: every input byte becomes (control + byte) = +1 byte.
        byte[] outBuf = new byte[(source.Length * 2) + 1];
        int outLen = 0;
        int i = 0;
        while (i < source.Length)
        {
            int runStart = i;
            byte runByte = source[i];
            int runLen = 1;
            while (i + runLen < source.Length && source[i + runLen] == runByte && runLen < 128)
            {
                runLen++;
            }

            if (runLen >= 3)
            {
                outBuf[outLen++] = (byte)(sbyte)-(runLen - 1);
                outBuf[outLen++] = runByte;
                i += runLen;
                continue;
            }

            // Otherwise accumulate a literal run up to 128 bytes, but stop
            // early if we encounter a run of >= 3 identical bytes.
            int litStart = i;
            int litLen = 0;
            while (litLen < 128 && i < source.Length)
            {
                int look = 1;
                while (i + look < source.Length && look < 3 && source[i + look] == source[i])
                {
                    look++;
                }
                if (look >= 3)
                {
                    break;
                }
                litLen++;
                i++;
            }
            outBuf[outLen++] = (byte)(litLen - 1);
            source.Slice(litStart, litLen).CopyTo(outBuf.AsSpan(outLen, litLen));
            outLen += litLen;
        }
        byte[] result = new byte[outLen];
        Buffer.BlockCopy(outBuf, 0, result, 0, outLen);
        return result;
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
