using System.Buffers.Binary;

namespace Mediar.Codecs.Alac.Decoder;

/// <summary>
/// ALAC Specific Config — the 24-byte magic cookie body Apple publishes as
/// <c>ALACSpecificConfig</c> in their ALAC Magic Cookie Description. The
/// cookie is required to initialise an ALAC decoder: it carries the encoder
/// parameters (rice modifier, history multiplier, etc.) and stream
/// properties (frame length, bit depth, channels, sample rate).
/// </summary>
/// <remarks>
/// Layout (all big-endian per the Apple reference):
/// <code>
/// uint32  frameLength       // samples per frame per channel (typical: 4096)
/// uint8   compatibleVersion // 0
/// uint8   bitDepth          // 16, 20, 24, or 32
/// uint8   pb                // tuning parameter (encoder rice "limit", default 40)
/// uint8   mb                // rice history multiplier (default 10)
/// uint8   kb                // initial rice parameter cap (default 14)
/// uint8   numChannels
/// uint16  maxRun            // typically 255
/// uint32  maxFrameBytes
/// uint32  avgBitRate
/// uint32  sampleRate
/// </code>
/// </remarks>
public sealed class AlacSpecificConfig
{
    /// <summary>Default rice <c>pb</c> ("encoder rice limit").</summary>
    public const int DefaultPb = 40;

    /// <summary>Default rice history multiplier <c>mb</c>.</summary>
    public const int DefaultMb = 10;

    /// <summary>Default initial rice parameter cap <c>kb</c>.</summary>
    public const int DefaultKb = 14;

    /// <summary>Default frames-per-packet (4096 samples per channel).</summary>
    public const int DefaultFrameLength = 4096;

    /// <summary>Default max run-length value (Apple reference).</summary>
    public const int DefaultMaxRun = 255;

    /// <summary>Size of the ALAC Specific Config body in bytes.</summary>
    public const int CookieBodyBytes = 24;

    /// <summary>Samples per frame per channel.</summary>
    public required int FrameLength { get; init; }

    /// <summary>Compatible version (0 in all known streams).</summary>
    public required byte CompatibleVersion { get; init; }

    /// <summary>Bit depth: 16, 20, 24 or 32.</summary>
    public required int BitDepth { get; init; }

    /// <summary>Encoder rice parameter <c>pb</c> ("rice limit"); default 40.</summary>
    public required int Pb { get; init; }

    /// <summary>Rice history multiplier <c>mb</c>; default 10.</summary>
    public required int Mb { get; init; }

    /// <summary>Initial rice parameter cap <c>kb</c>; default 14.</summary>
    public required int Kb { get; init; }

    /// <summary>Channel count.</summary>
    public required int NumChannels { get; init; }

    /// <summary>Maximum permitted run length (default 255).</summary>
    public required int MaxRun { get; init; }

    /// <summary>Maximum frame size in bytes.</summary>
    public required int MaxFrameBytes { get; init; }

    /// <summary>Average bit rate.</summary>
    public required int AvgBitRate { get; init; }

    /// <summary>Sample rate (Hz).</summary>
    public required int SampleRate { get; init; }

    /// <summary>
    /// Parse a 24-byte ALAC Specific Config body. Pass the body only — NOT
    /// the enclosing <c>alac</c> box header or its 4-byte FullBox
    /// version/flags. <see cref="AlacExtraData.NormalizeCookie"/> can extract
    /// the body from common container wrappings.
    /// </summary>
    public static AlacSpecificConfig Parse(ReadOnlySpan<byte> cookieBody)
    {
        if (cookieBody.Length < CookieBodyBytes)
        {
            throw new ArgumentException(
                $"ALAC Specific Config must be at least {CookieBodyBytes} bytes; got {cookieBody.Length}.",
                nameof(cookieBody));
        }

        int frameLength = (int)BinaryPrimitives.ReadUInt32BigEndian(cookieBody[..4]);
        byte compatibleVersion = cookieBody[4];
        int bitDepth = cookieBody[5];
        int pb = cookieBody[6];
        int mb = cookieBody[7];
        int kb = cookieBody[8];
        int channels = cookieBody[9];
        int maxRun = BinaryPrimitives.ReadUInt16BigEndian(cookieBody[10..12]);
        int maxFrameBytes = (int)BinaryPrimitives.ReadUInt32BigEndian(cookieBody[12..16]);
        int avgBitRate = (int)BinaryPrimitives.ReadUInt32BigEndian(cookieBody[16..20]);
        int sampleRate = (int)BinaryPrimitives.ReadUInt32BigEndian(cookieBody[20..24]);

        if (bitDepth != 16 && bitDepth != 20 && bitDepth != 24 && bitDepth != 32)
        {
            throw new InvalidDataException($"ALAC cookie reports unsupported bit depth {bitDepth}.");
        }
        if (channels is < 1 or > 8)
        {
            throw new InvalidDataException($"ALAC cookie reports unsupported channel count {channels}.");
        }
        if (frameLength <= 0 || frameLength > 16384)
        {
            throw new InvalidDataException($"ALAC cookie reports implausible frame length {frameLength}.");
        }

        return new AlacSpecificConfig
        {
            FrameLength = frameLength,
            CompatibleVersion = compatibleVersion,
            BitDepth = bitDepth,
            Pb = pb,
            Mb = mb,
            Kb = kb,
            NumChannels = channels,
            MaxRun = maxRun,
            MaxFrameBytes = maxFrameBytes,
            AvgBitRate = avgBitRate,
            SampleRate = sampleRate,
        };
    }
}

/// <summary>
/// Helpers for extracting / normalising the ALAC magic cookie that lives in
/// various container wrappings (MP4 <c>alac</c> child box, CAF <c>kuki</c>
/// chunk, sometimes wrapped in a <c>frma</c>/<c>alac</c> atom pair).
/// </summary>
public static class AlacExtraData
{
    private const uint FourCcFrma = 0x66726D61; // 'frma'
    private const uint FourCcAlac = 0x616C6163; // 'alac'

    /// <summary>
    /// Reduce a container-supplied ALAC cookie to its 24-byte
    /// <see cref="AlacSpecificConfig"/> body. Recognises:
    /// <list type="bullet">
    /// <item>Raw 24-byte body (returned as-is).</item>
    /// <item>28-byte FullBox-prefixed body (4-byte version/flags + body) as
    /// produced by an MP4 <c>alac</c> child box.</item>
    /// <item>Apple atom-style cookie: <c>frma</c>('alac') + (optional <c>chan</c>)
    /// + <c>alac</c>(version/flags + body) blocks.</item>
    /// </list>
    /// </summary>
    /// <returns>The 24-byte cookie body, or <see cref="ReadOnlySpan{T}.Empty"/>
    /// if no recognisable cookie was found.</returns>
    public static ReadOnlySpan<byte> NormalizeCookie(ReadOnlySpan<byte> raw)
    {
        if (raw.Length == AlacSpecificConfig.CookieBodyBytes) return raw;

        if (raw.Length == AlacSpecificConfig.CookieBodyBytes + 4)
        {
            // FullBox version+flags prefix (MP4 alac child box payload shape).
            return raw[4..];
        }

        // Try scanning for an atom-style 'alac' inside the buffer.
        int pos = 0;
        while (pos + 8 <= raw.Length)
        {
            uint size = BinaryPrimitives.ReadUInt32BigEndian(raw.Slice(pos, 4));
            uint type = BinaryPrimitives.ReadUInt32BigEndian(raw.Slice(pos + 4, 4));
            if (size < 8 || pos + size > raw.Length) break;

            if (type == FourCcAlac)
            {
                int payloadStart = pos + 8;
                int payloadLen = (int)size - 8;
                var payload = raw.Slice(payloadStart, payloadLen);
                if (payload.Length == AlacSpecificConfig.CookieBodyBytes) return payload;
                if (payload.Length == AlacSpecificConfig.CookieBodyBytes + 4) return payload[4..];
            }
            else if (type == FourCcFrma)
            {
                // Just a format marker — move past it.
            }

            pos += (int)size;
        }

        return ReadOnlySpan<byte>.Empty;
    }
}
