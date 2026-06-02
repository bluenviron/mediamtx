namespace Mediar.Codecs.Opus.Decoder;

/// <summary>
/// One compressed Opus frame extracted from a packet by <see cref="OpusFramePacker"/>.
/// <see cref="Offset"/> + <see cref="Length"/> point into the original packet
/// buffer (no copy is performed).
/// </summary>
public readonly record struct OpusFrameView
{
    /// <summary>Byte offset of the frame payload within the source packet.</summary>
    public int Offset { get; init; }

    /// <summary>Byte length of the frame payload.</summary>
    public int Length { get; init; }
}

/// <summary>
/// Splits an Opus packet into its constituent compressed frames as defined by
/// RFC 6716 §3.2. Handles all four framing codes plus the optional padding
/// envelope of code 3 and the optional self-delimited variant used by
/// multistream Opus.
/// </summary>
/// <remarks>
/// <para>
/// The packer is intentionally stateless — given a packet's bytes it returns
/// the per-frame slices, or throws <see cref="InvalidDataException"/> when
/// the packet violates the structural constraints in §3.2.1 / §3.4 (R1..R7).
/// </para>
/// </remarks>
public static class OpusFramePacker
{
    /// <summary>
    /// Maximum number of frames the spec permits in a single packet
    /// (RFC 6716 §3.2.5).
    /// </summary>
    public const int MaxFramesPerPacket = 48;

    /// <summary>
    /// Maximum compressed length of a single frame, in bytes
    /// (RFC 6716 §3.2.1 — frames are limited to 1275 bytes to keep the
    /// total packet within Opus's 120 ms / multistream addressing budget).
    /// </summary>
    public const int MaxFrameLength = 1275;

    /// <summary>
    /// Walk a packet (TOC byte at index 0) and append one
    /// <see cref="OpusFrameView"/> per contained frame to <paramref name="frames"/>.
    /// </summary>
    /// <param name="packet">Full packet including the TOC byte.</param>
    /// <param name="toc">Parsed TOC byte (see <see cref="OpusToc.Parse"/>).</param>
    /// <param name="frames">Destination list; cleared before use.</param>
    /// <exception cref="InvalidDataException">Thrown when the packet is malformed per §3.2 R1..R7.</exception>
    public static void Unpack(ReadOnlySpan<byte> packet, OpusToc toc, List<OpusFrameView> frames)
    {
        ArgumentNullException.ThrowIfNull(frames);
        frames.Clear();
        if (packet.IsEmpty) throw new InvalidDataException("Opus packet is empty (R1).");
        UnpackInternal(packet, toc, frames);
    }

    /// <summary>
    /// Convenience overload that parses the TOC byte itself.
    /// </summary>
    public static IReadOnlyList<OpusFrameView> Unpack(ReadOnlySpan<byte> packet)
    {
        if (packet.IsEmpty) throw new InvalidDataException("Opus packet is empty (R1).");
        var toc = OpusToc.Parse(packet[0]);
        var frames = new List<OpusFrameView>(capacity: 4);
        UnpackInternal(packet, toc, frames);
        return frames;
    }

    private static void UnpackInternal(ReadOnlySpan<byte> packet, OpusToc toc, List<OpusFrameView> frames)
    {
        // packet[0] is the TOC byte; the rest is the framing payload.
        int cursor = 1;
        switch (toc.FrameCountCode)
        {
            case 0:
                UnpackCode0(packet, cursor, frames);
                break;
            case 1:
                UnpackCode1(packet, cursor, frames);
                break;
            case 2:
                UnpackCode2(packet, cursor, frames);
                break;
            case 3:
                UnpackCode3(packet, toc, cursor, frames);
                break;
            default:
                throw new InvalidDataException(
                    $"Unrecognised Opus frame count code {toc.FrameCountCode}.");
        }
    }

    // ---- Code 0: a single frame whose length spans the rest of the packet. ----
    private static void UnpackCode0(ReadOnlySpan<byte> packet, int cursor, List<OpusFrameView> frames)
    {
        int length = packet.Length - cursor;
        if (length < 0) throw new InvalidDataException("Opus code 0 packet truncated (R1).");
        if (length > MaxFrameLength)
            throw new InvalidDataException($"Opus code 0 frame too large ({length} > {MaxFrameLength}).");
        frames.Add(new OpusFrameView { Offset = cursor, Length = length });
    }

    // ---- Code 1: two frames of identical size, no length byte. ----
    private static void UnpackCode1(ReadOnlySpan<byte> packet, int cursor, List<OpusFrameView> frames)
    {
        int total = packet.Length - cursor;
        if (total < 0 || (total & 1) != 0)
        {
            // R3: code-1 packets must hold two frames of equal compressed size,
            // so the remaining length after the TOC must be even.
            throw new InvalidDataException("Opus code 1 packet has an odd payload length (R3).");
        }
        int per = total / 2;
        if (per > MaxFrameLength)
            throw new InvalidDataException($"Opus code 1 frame too large ({per} > {MaxFrameLength}).");
        frames.Add(new OpusFrameView { Offset = cursor, Length = per });
        frames.Add(new OpusFrameView { Offset = cursor + per, Length = per });
    }

    // ---- Code 2: two frames of independent sizes; the first size is encoded. ----
    private static void UnpackCode2(ReadOnlySpan<byte> packet, int cursor, List<OpusFrameView> frames)
    {
        if (cursor >= packet.Length)
            throw new InvalidDataException("Opus code 2 packet missing frame-length byte (R4).");
        int len1 = ReadFrameLength(packet, ref cursor);
        if (len1 > MaxFrameLength)
            throw new InvalidDataException($"Opus code 2 first frame too large ({len1} > {MaxFrameLength}).");
        int len2 = packet.Length - cursor - len1;
        if (len2 < 0)
            throw new InvalidDataException("Opus code 2 first frame extends past packet end (R4).");
        if (len2 > MaxFrameLength)
            throw new InvalidDataException($"Opus code 2 second frame too large ({len2} > {MaxFrameLength}).");
        frames.Add(new OpusFrameView { Offset = cursor, Length = len1 });
        frames.Add(new OpusFrameView { Offset = cursor + len1, Length = len2 });
    }

    // ---- Code 3: variable frame count + optional padding + CBR/VBR encoding. ----
    private static void UnpackCode3(ReadOnlySpan<byte> packet, OpusToc toc, int cursor, List<OpusFrameView> frames)
    {
        if (cursor >= packet.Length)
            throw new InvalidDataException("Opus code 3 packet missing frame-count byte (R5).");
        byte fc = packet[cursor++];
        bool vbr = (fc & 0x80) != 0;
        bool padding = (fc & 0x40) != 0;
        int m = fc & 0x3F;
        if (m < 1 || m > MaxFramesPerPacket)
            throw new InvalidDataException($"Opus code 3 frame count {m} out of range [1, {MaxFramesPerPacket}] (R5).");

        // Total audio duration must not exceed 120 ms (R5).
        long totalUs = (long)m * toc.FrameSizeMicroseconds;
        if (totalUs > 120_000)
        {
            throw new InvalidDataException(
                $"Opus code 3 packet duration {totalUs} µs exceeds 120 000 µs limit (R5).");
        }

        // Optional padding length. The byte(s) following fc encode the count
        // of padding bytes appended to the packet; their content is ignored
        // by the decoder. The encoding is identical to libopus: each padding
        // length byte adds either 254 (when the byte is 255) and consumes
        // another byte, or its own value.
        int paddingBytes = 0;
        if (padding)
        {
            while (true)
            {
                if (cursor >= packet.Length)
                    throw new InvalidDataException("Opus code 3 padding-length byte missing (R6).");
                int p = packet[cursor++];
                if (p == 255)
                {
                    paddingBytes += 254;
                }
                else
                {
                    paddingBytes += p;
                    break;
                }
            }
        }

        int paddingStart = packet.Length - paddingBytes;
        if (paddingStart < cursor)
            throw new InvalidDataException("Opus code 3 padding-length exceeds remaining packet (R6).");

        if (vbr)
        {
            // M-1 frame lengths follow; the M-th frame's length is the
            // remainder of the payload (between the last length byte and
            // the start of the padding region).
            int[] lengths = new int[m];
            int sum = 0;
            for (int i = 0; i < m - 1; i++)
            {
                int len = ReadFrameLength(packet, ref cursor);
                if (len > MaxFrameLength)
                    throw new InvalidDataException($"Opus code 3 VBR frame {i} too large ({len} > {MaxFrameLength}).");
                lengths[i] = len;
                sum += len;
                if (cursor + sum > paddingStart)
                    throw new InvalidDataException("Opus code 3 VBR frame lengths exceed packet (R6).");
            }
            int last = paddingStart - (cursor + sum);
            if (last < 0)
                throw new InvalidDataException("Opus code 3 VBR final frame length negative (R6).");
            if (last > MaxFrameLength)
                throw new InvalidDataException($"Opus code 3 VBR last frame too large ({last} > {MaxFrameLength}).");
            lengths[m - 1] = last;
            int p = cursor;
            for (int i = 0; i < m; i++)
            {
                frames.Add(new OpusFrameView { Offset = p, Length = lengths[i] });
                p += lengths[i];
            }
        }
        else
        {
            // CBR: every frame has the same length. The remaining payload
            // after padding must be divisible by M.
            int payload = paddingStart - cursor;
            if (payload < 0 || payload % m != 0)
                throw new InvalidDataException("Opus code 3 CBR payload not divisible by frame count (R6).");
            int per = payload / m;
            if (per > MaxFrameLength)
                throw new InvalidDataException($"Opus code 3 CBR frame too large ({per} > {MaxFrameLength}).");
            int p = cursor;
            for (int i = 0; i < m; i++)
            {
                frames.Add(new OpusFrameView { Offset = p, Length = per });
                p += per;
            }
        }
    }

    /// <summary>
    /// Decode the variable-length frame size used by codes 2 and 3
    /// (RFC 6716 §3.2.1). Sizes 0..251 are one byte; sizes 252..1275 are
    /// two bytes, where the first byte is 252..255 and the size is
    /// <c>first + 4 * second</c> (yielding the encoded value
    /// <c>first - 252 + 252 = first</c>, i.e. the lookup
    /// <c>252 + 4*(first-252) + 4*second</c> ... see implementation).
    /// </summary>
    internal static int ReadFrameLength(ReadOnlySpan<byte> packet, ref int cursor)
    {
        int b0 = packet[cursor++];
        if (b0 < 252) return b0;
        if (cursor >= packet.Length)
            throw new InvalidDataException("Opus frame-length 2nd byte missing (R4/R6).");
        int b1 = packet[cursor++];
        // Encoded: size = b1*4 + b0; reading the spec exactly.
        return b1 * 4 + b0;
    }

    /// <summary>
    /// Encode a frame length using the 1- or 2-byte form. Used by the test
    /// suite to construct synthetic packets; exposed via
    /// <c>InternalsVisibleTo</c>.
    /// </summary>
    internal static int WriteFrameLength(int length, Span<byte> dest)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(length);
        if (length > MaxFrameLength)
            throw new ArgumentOutOfRangeException(nameof(length),
                $"Frame length {length} exceeds the {MaxFrameLength}-byte ceiling.");
        if (length < 252)
        {
            dest[0] = (byte)length;
            return 1;
        }
        // Two-byte form: size = b1*4 + b0 with b0 ∈ [252, 255].
        int b1 = (length - 252) / 4;
        int b0 = length - b1 * 4;
        dest[0] = (byte)b0;
        dest[1] = (byte)b1;
        return 2;
    }
}
