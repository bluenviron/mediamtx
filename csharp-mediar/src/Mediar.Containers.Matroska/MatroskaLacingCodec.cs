using System.Buffers;

namespace Mediar.Containers.Matroska;

/// <summary>
/// Standalone helpers for encoding and decoding the per-frame size tables
/// used by Matroska Block / SimpleBlock lacing. The codec is intentionally
/// agnostic of the surrounding block layout (track id, relative timestamp,
/// flags byte) so that it can be unit-tested in isolation and re-used by
/// both <see cref="MatroskaDemuxer"/> and <see cref="MatroskaMuxer"/>.
/// </summary>
/// <remarks>
/// <para>
/// Reference: Matroska Block specification — <c>https://www.matroska.org/technical/notes.html</c>
/// (Lacing section) and <c>EBMLDocTypes</c> spec for VINT encoding of signed
/// values. Lacing carries an explicit frame count byte (frame_count - 1, so
/// 1..256 frames) followed by an optional per-mode header that encodes the
/// sizes of all but the last frame; the last frame is whatever bytes remain.
/// </para>
/// </remarks>
internal static class MatroskaLacingCodec
{
    /// <summary>Maximum number of frames per laced block (frame_count byte is 1 byte).</summary>
    public const int MaxFrames = 256;

    // ---- Decode ----

    /// <summary>
    /// Decode the size table of a laced block. <paramref name="body"/> points
    /// at the byte immediately after the block flags byte — i.e. starting with
    /// the <c>frame_count - 1</c> byte. Returns the byte offset within
    /// <paramref name="body"/> at which the frame payloads start.
    /// </summary>
    /// <param name="lacing">Lacing mode (must not be <see cref="MatroskaLacing.None"/>).</param>
    /// <param name="body">Buffer covering the post-flags portion of the block.</param>
    /// <param name="sizes">On success, populated with one entry per frame.</param>
    /// <returns>Byte offset where the first frame's payload begins.</returns>
    /// <exception cref="InvalidDataException">Thrown if the size table is malformed,
    /// the encoded sizes would exceed the buffer, or a per-frame size goes negative.</exception>
    public static int DecodeSizes(MatroskaLacing lacing, ReadOnlySpan<byte> body, out int[] sizes)
    {
        if (lacing == MatroskaLacing.None)
            throw new ArgumentException("DecodeSizes is only for laced blocks.", nameof(lacing));
        if (body.Length < 1)
            throw new InvalidDataException("Laced block missing frame-count byte.");

        int frameCount = body[0] + 1;
        if (frameCount < 1 || frameCount > MaxFrames)
            throw new InvalidDataException($"Invalid laced frame count {frameCount}.");

        sizes = new int[frameCount];
        int offset = 1;

        switch (lacing)
        {
            case MatroskaLacing.Fixed:
            {
                int remaining = body.Length - offset;
                if (frameCount == 0 || (remaining % frameCount) != 0)
                    throw new InvalidDataException("Fixed-lacing payload not divisible by frame count.");
                int per = remaining / frameCount;
                for (int i = 0; i < frameCount; i++) sizes[i] = per;
                return offset;
            }
            case MatroskaLacing.Xiph:
            {
                long accumulated = 0;
                for (int i = 0; i < frameCount - 1; i++)
                {
                    int size = 0;
                    while (true)
                    {
                        if (offset >= body.Length)
                            throw new InvalidDataException("Truncated Xiph lacing size table.");
                        byte b = body[offset++];
                        size += b;
                        if (b != 0xFF) break;
                    }
                    sizes[i] = size;
                    accumulated += size;
                    if (accumulated > body.Length - offset)
                        throw new InvalidDataException("Xiph lacing size table overruns payload.");
                }
                int last = body.Length - offset - (int)accumulated;
                if (last < 0)
                    throw new InvalidDataException("Xiph lacing last-frame size is negative.");
                sizes[frameCount - 1] = last;
                return offset;
            }
            case MatroskaLacing.Ebml:
            {
                if (frameCount == 1)
                {
                    sizes[0] = body.Length - offset;
                    return offset;
                }
                // First size: unsigned VINT (length marker bit stripped).
                int first = (int)ReadUnsignedVint(body, ref offset);
                if (first < 0) throw new InvalidDataException("EBML lacing first size out of range.");
                sizes[0] = first;
                long accumulated = first;
                int prev = first;
                for (int i = 1; i < frameCount - 1; i++)
                {
                    long delta = ReadSignedVint(body, ref offset);
                    long cur = prev + delta;
                    if (cur < 0 || cur > int.MaxValue)
                        throw new InvalidDataException($"EBML lacing produced invalid size {cur}.");
                    sizes[i] = (int)cur;
                    accumulated += cur;
                    prev = (int)cur;
                    if (accumulated > body.Length - offset)
                        throw new InvalidDataException("EBML lacing size table overruns payload.");
                }
                int last = body.Length - offset - (int)accumulated;
                if (last < 0)
                    throw new InvalidDataException("EBML lacing last-frame size is negative.");
                sizes[frameCount - 1] = last;
                return offset;
            }
            default:
                throw new InvalidDataException($"Unknown lacing mode {lacing}.");
        }
    }

    // ---- Encode ----

    /// <summary>
    /// Encode the size table for the given frame sizes. The output covers the
    /// frame-count byte plus any per-mode size header — but NOT the frame
    /// payload bytes themselves.
    /// </summary>
    /// <param name="lacing">Lacing mode (must not be <see cref="MatroskaLacing.None"/>).</param>
    /// <param name="sizes">Per-frame sizes; must contain 1..256 entries.</param>
    /// <param name="header">On success, contains the encoded size table.</param>
    /// <exception cref="ArgumentException">Thrown for invalid input.</exception>
    public static void EncodeSizes(MatroskaLacing lacing, ReadOnlySpan<int> sizes, out byte[] header)
    {
        if (lacing == MatroskaLacing.None)
            throw new ArgumentException("EncodeSizes is only for laced blocks.", nameof(lacing));
        if (sizes.Length < 1 || sizes.Length > MaxFrames)
            throw new ArgumentException($"Lacing frame count must be 1..{MaxFrames}.", nameof(sizes));
        foreach (int s in sizes)
            if (s < 0) throw new ArgumentException("Frame sizes must be non-negative.", nameof(sizes));

        var bw = new ArrayBufferWriter<byte>(16 + sizes.Length * 2);
        var span = bw.GetSpan(1);
        span[0] = (byte)(sizes.Length - 1);
        bw.Advance(1);

        switch (lacing)
        {
            case MatroskaLacing.Fixed:
            {
                int first = sizes[0];
                for (int i = 1; i < sizes.Length; i++)
                    if (sizes[i] != first)
                        throw new ArgumentException("Fixed lacing requires all frames to be the same size.", nameof(sizes));
                break;
            }
            case MatroskaLacing.Xiph:
            {
                for (int i = 0; i < sizes.Length - 1; i++)
                {
                    int s = sizes[i];
                    while (s >= 0xFF)
                    {
                        var d = bw.GetSpan(1);
                        d[0] = 0xFF;
                        bw.Advance(1);
                        s -= 0xFF;
                    }
                    var last = bw.GetSpan(1);
                    last[0] = (byte)s;
                    bw.Advance(1);
                }
                break;
            }
            case MatroskaLacing.Ebml:
            {
                // For a single-frame lace there are no sizes to record — the
                // single frame's size is implicit (the whole payload). The
                // unsigned-first-size + signed-delta sequence only applies
                // when there are 2+ frames.
                if (sizes.Length >= 2)
                {
                    WriteUnsignedVint(bw, (ulong)sizes[0]);
                    int prev = sizes[0];
                    for (int i = 1; i < sizes.Length - 1; i++)
                    {
                        long delta = sizes[i] - (long)prev;
                        WriteSignedVint(bw, delta);
                        prev = sizes[i];
                    }
                }
                break;
            }
            default:
                throw new ArgumentException($"Unknown lacing mode {lacing}.", nameof(lacing));
        }

        header = bw.WrittenSpan.ToArray();
    }

    // ---- VINT helpers ----

    /// <summary>Read an unsigned EBML VINT (data bits only) and advance offset.</summary>
    internal static ulong ReadUnsignedVint(ReadOnlySpan<byte> body, ref int offset)
    {
        if (offset >= body.Length)
            throw new InvalidDataException("Truncated VINT.");
        byte b0 = body[offset];
        if (b0 == 0) throw new InvalidDataException("Invalid VINT leading byte.");
        int len = 1;
        byte mask = 0x80;
        while ((b0 & mask) == 0)
        {
            len++;
            mask >>= 1;
            if (len > 8) throw new InvalidDataException("VINT too long.");
        }
        if (offset + len > body.Length)
            throw new InvalidDataException("Truncated VINT.");
        ulong value = (ulong)(b0 & (mask - 1));
        for (int i = 1; i < len; i++) value = (value << 8) | body[offset + i];
        offset += len;
        return value;
    }

    /// <summary>
    /// Read a signed EBML VINT and apply the centred bias. EBML lacing uses
    /// bias <c>(1 &lt;&lt; (7L - 1)) - 1</c> at width L, so the all-ones encoded
    /// value is a legitimate maximum positive delta (e.g. +64 at L=1).
    /// </summary>
    internal static long ReadSignedVint(ReadOnlySpan<byte> body, ref int offset)
    {
        if (offset >= body.Length)
            throw new InvalidDataException("Truncated signed VINT.");
        byte b0 = body[offset];
        if (b0 == 0) throw new InvalidDataException("Invalid signed VINT leading byte.");
        int len = 1;
        byte mask = 0x80;
        while ((b0 & mask) == 0)
        {
            len++;
            mask >>= 1;
            if (len > 8) throw new InvalidDataException("Signed VINT too long.");
        }
        if (offset + len > body.Length)
            throw new InvalidDataException("Truncated signed VINT.");
        ulong value = (ulong)(b0 & (mask - 1));
        for (int i = 1; i < len; i++) value = (value << 8) | body[offset + i];
        offset += len;
        long bias = (1L << (7 * len - 1)) - 1;
        return (long)value - bias;
    }

    /// <summary>Write an unsigned VINT using the shortest representation that is NOT all-ones.</summary>
    internal static void WriteUnsignedVint(ArrayBufferWriter<byte> bw, ulong value)
    {
        int width = 0;
        for (int w = 1; w <= 8; w++)
        {
            // All-ones is reserved as "unknown size" sentinel for normal EBML data VINTs.
            ulong max = (1UL << (7 * w)) - 2;
            if (value <= max) { width = w; break; }
        }
        if (width == 0) throw new ArgumentOutOfRangeException(nameof(value));
        var dst = bw.GetSpan(width);
        ulong combined = value | (1UL << (7 * width));
        for (int i = width - 1; i >= 0; i--) { dst[i] = (byte)combined; combined >>= 8; }
        bw.Advance(width);
    }

    /// <summary>
    /// Write a signed VINT for EBML lacing deltas. Width is chosen from the
    /// signed range first, then the biased value is encoded in EXACTLY that
    /// width — including the all-ones representation (which means "max +ve
    /// delta", not "unknown size", in the lacing context).
    /// </summary>
    internal static void WriteSignedVint(ArrayBufferWriter<byte> bw, long value)
    {
        int width = 0;
        for (int w = 1; w <= 8; w++)
        {
            long bias = (1L << (7 * w - 1)) - 1;
            long min = -bias;
            long max = bias + 1;
            if (value >= min && value <= max) { width = w; break; }
        }
        if (width == 0) throw new ArgumentOutOfRangeException(nameof(value));
        long biasW = (1L << (7 * width - 1)) - 1;
        ulong biased = (ulong)(value + biasW);
        var dst = bw.GetSpan(width);
        ulong combined = biased | (1UL << (7 * width));
        for (int i = width - 1; i >= 0; i--) { dst[i] = (byte)combined; combined >>= 8; }
        bw.Advance(width);
    }
}
