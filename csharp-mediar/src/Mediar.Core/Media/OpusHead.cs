using System.Buffers.Binary;

namespace Mediar;

/// <summary>
/// Codec-initialization data for an Opus track. Models the shared payload
/// between the Ogg <c>OpusHead</c> packet (RFC 7845 §5.1) and the ISO BMFF
/// <c>OpusSpecificBox</c> (<c>dOps</c>, per Opus-in-ISOBMFF specification).
/// </summary>
/// <remarks>
/// <para>
/// The two on-the-wire representations differ in three ways:
/// </para>
/// <list type="bullet">
///   <item><description>Ogg form is prefixed with the 8-byte ASCII magic <c>"OpusHead"</c>; <c>dOps</c> is not.</description></item>
///   <item><description>Ogg form encodes <see cref="PreSkip"/>, <see cref="InputSampleRate"/> and <see cref="OutputGain"/> in <b>little-endian</b>; <c>dOps</c> uses <b>big-endian</b> (ISOBMFF convention).</description></item>
///   <item><description>Ogg form's leading version byte is <c>1</c>; <c>dOps</c>'s leading version byte is <c>0</c>. This struct therefore does NOT model "version" as a settable field — the writers hard-code the correct value for each format.</description></item>
/// </list>
/// <para>
/// Throughout the Mediar codebase, <see cref="CodecParameters.ExtraData"/> for an Opus track holds the
/// Ogg-form bytes (magic + LE body), matching <c>OggDemuxer</c>'s output and <c>OggMuxer</c>'s input.
/// Container shims (e.g. MP4 <c>dOps</c>, CAF <c>kuki</c>) convert in and out of this canonical form.
/// </para>
/// </remarks>
public readonly record struct OpusHead
{
    /// <summary>Ogg encapsulation magic (always written by <see cref="WriteOgg(in OpusHead, Span{byte})"/>).</summary>
    public static ReadOnlySpan<byte> Magic => "OpusHead"u8;

    /// <summary>Length of the Ogg <c>OpusHead</c> packet for <see cref="ChannelMappingFamily"/> 0.</summary>
    public const int OggMinimumLength = 19;

    /// <summary>Length of the ISOBMFF <c>dOps</c> body for <see cref="ChannelMappingFamily"/> 0.</summary>
    public const int IsobmffMinimumLength = 11;

    /// <summary>Number of output channels (1..255). Must be 1 or 2 when <see cref="ChannelMappingFamily"/> is 0.</summary>
    public byte ChannelCount { get; init; }

    /// <summary>
    /// Number of samples (at 48 kHz) to discard from the decoder output at the
    /// start of playback to compensate for encoder-side look-ahead.
    /// </summary>
    public ushort PreSkip { get; init; }

    /// <summary>Original input sample rate (informational). May be 0 if unknown.</summary>
    public uint InputSampleRate { get; init; }

    /// <summary>Per-stream output gain, signed Q7.8 dB (i.e. value/256.0 dB).</summary>
    public short OutputGain { get; init; }

    /// <summary>
    /// Channel mapping family. 0 = mono/stereo (RTP layout, no mapping table).
    /// 1 = Vorbis surround layout (up to 8 channels). 255 = no defined layout
    /// (application-specific). 2..254 reserved.
    /// </summary>
    public byte ChannelMappingFamily { get; init; }

    /// <summary>Number of Opus streams encoded in each packet. Only valid when <see cref="ChannelMappingFamily"/> != 0.</summary>
    public byte StreamCount { get; init; }

    /// <summary>Number of streams whose first decoded channel is paired with a second decoded channel. Only valid when <see cref="ChannelMappingFamily"/> != 0.</summary>
    public byte CoupledCount { get; init; }

    /// <summary>
    /// Per-output-channel stream index, length <see cref="ChannelCount"/>. Only present when <see cref="ChannelMappingFamily"/> != 0.
    /// Each byte is either an index into the per-packet stream list, or <c>0xFF</c>
    /// to mark a silent channel.
    /// </summary>
    public ReadOnlyMemory<byte> ChannelMapping { get; init; }

    /// <summary>Compute the number of bytes <see cref="WriteOgg(in OpusHead, Span{byte})"/> will emit.</summary>
    public int OggByteCount =>
        ChannelMappingFamily == 0 ? OggMinimumLength : OggMinimumLength + 2 + ChannelCount;

    /// <summary>Compute the number of bytes <see cref="WriteIsobmff(in OpusHead, Span{byte})"/> will emit.</summary>
    public int IsobmffByteCount =>
        ChannelMappingFamily == 0 ? IsobmffMinimumLength : IsobmffMinimumLength + 2 + ChannelCount;

    /// <summary>
    /// Parse an Ogg-form <c>OpusHead</c> packet (magic + LE body). Returns
    /// <c>false</c> if the magic, version, length or channel-mapping table is malformed.
    /// </summary>
    public static bool TryReadOgg(ReadOnlySpan<byte> bytes, out OpusHead head)
    {
        head = default;
        if (bytes.Length < OggMinimumLength) return false;
        if (!bytes[..8].SequenceEqual(Magic)) return false;
        // The spec lists version 1 as current; readers should accept the major
        // version (high nibble) being 0 — we only require the high nibble to be 0
        // so that the stream is recognizable as "Opus version 0.x".
        if ((bytes[8] & 0xF0) != 0) return false;
        byte channels = bytes[9];
        if (channels == 0) return false;
        ushort preSkip = BinaryPrimitives.ReadUInt16LittleEndian(bytes[10..12]);
        uint inputSampleRate = BinaryPrimitives.ReadUInt32LittleEndian(bytes[12..16]);
        short outputGain = BinaryPrimitives.ReadInt16LittleEndian(bytes[16..18]);
        byte family = bytes[18];

        byte streams = 0, coupled = 0;
        ReadOnlyMemory<byte> mapping = default;
        if (family != 0)
        {
            int required = OggMinimumLength + 2 + channels;
            if (bytes.Length < required) return false;
            streams = bytes[19];
            coupled = bytes[20];
            if (streams == 0 || coupled > streams) return false;
            byte[] map = bytes.Slice(21, channels).ToArray();
            int max = streams + coupled;
            for (int i = 0; i < map.Length; i++)
            {
                if (map[i] != 0xFF && map[i] >= max) return false;
            }
            mapping = map;
        }
        else
        {
            if (channels != 1 && channels != 2) return false;
        }

        head = new OpusHead
        {
            ChannelCount = channels,
            PreSkip = preSkip,
            InputSampleRate = inputSampleRate,
            OutputGain = outputGain,
            ChannelMappingFamily = family,
            StreamCount = streams,
            CoupledCount = coupled,
            ChannelMapping = mapping,
        };
        return true;
    }

    /// <summary>
    /// Parse an ISOBMFF <c>OpusSpecificBox</c> body (BE, no magic) into an <see cref="OpusHead"/>.
    /// Returns <c>false</c> if the version, length or channel-mapping table is malformed.
    /// </summary>
    public static bool TryReadIsobmff(ReadOnlySpan<byte> body, out OpusHead head)
    {
        head = default;
        if (body.Length < IsobmffMinimumLength) return false;
        // dOps version is currently fixed at 0.
        if (body[0] != 0) return false;
        byte channels = body[1];
        if (channels == 0) return false;
        ushort preSkip = BinaryPrimitives.ReadUInt16BigEndian(body[2..4]);
        uint inputSampleRate = BinaryPrimitives.ReadUInt32BigEndian(body[4..8]);
        short outputGain = BinaryPrimitives.ReadInt16BigEndian(body[8..10]);
        byte family = body[10];

        byte streams = 0, coupled = 0;
        ReadOnlyMemory<byte> mapping = default;
        if (family != 0)
        {
            int required = IsobmffMinimumLength + 2 + channels;
            if (body.Length < required) return false;
            streams = body[11];
            coupled = body[12];
            if (streams == 0 || coupled > streams) return false;
            byte[] map = body.Slice(13, channels).ToArray();
            int max = streams + coupled;
            for (int i = 0; i < map.Length; i++)
            {
                if (map[i] != 0xFF && map[i] >= max) return false;
            }
            mapping = map;
        }
        else
        {
            if (channels != 1 && channels != 2) return false;
        }

        head = new OpusHead
        {
            ChannelCount = channels,
            PreSkip = preSkip,
            InputSampleRate = inputSampleRate,
            OutputGain = outputGain,
            ChannelMappingFamily = family,
            StreamCount = streams,
            CoupledCount = coupled,
            ChannelMapping = mapping,
        };
        return true;
    }

    /// <summary>Serialize this header as Ogg-form bytes ("OpusHead" magic + LE body).</summary>
    public static int WriteOgg(in OpusHead head, Span<byte> destination)
    {
        int total = head.OggByteCount;
        if (destination.Length < total) throw new ArgumentException("Destination too small.", nameof(destination));
        Magic.CopyTo(destination);
        destination[8] = 1; // Ogg OpusHead version
        destination[9] = head.ChannelCount;
        BinaryPrimitives.WriteUInt16LittleEndian(destination[10..12], head.PreSkip);
        BinaryPrimitives.WriteUInt32LittleEndian(destination[12..16], head.InputSampleRate);
        BinaryPrimitives.WriteInt16LittleEndian(destination[16..18], head.OutputGain);
        destination[18] = head.ChannelMappingFamily;
        if (head.ChannelMappingFamily != 0)
        {
            destination[19] = head.StreamCount;
            destination[20] = head.CoupledCount;
            head.ChannelMapping.Span.Slice(0, head.ChannelCount).CopyTo(destination[21..]);
        }
        return total;
    }

    /// <summary>Serialize this header as Ogg-form bytes ("OpusHead" magic + LE body).</summary>
    public static byte[] WriteOgg(in OpusHead head)
    {
        byte[] bytes = new byte[head.OggByteCount];
        WriteOgg(head, bytes);
        return bytes;
    }

    /// <summary>Serialize this header as an ISOBMFF <c>dOps</c> body (BE, no magic).</summary>
    public static int WriteIsobmff(in OpusHead head, Span<byte> destination)
    {
        int total = head.IsobmffByteCount;
        if (destination.Length < total) throw new ArgumentException("Destination too small.", nameof(destination));
        destination[0] = 0; // dOps version (fixed)
        destination[1] = head.ChannelCount;
        BinaryPrimitives.WriteUInt16BigEndian(destination[2..4], head.PreSkip);
        BinaryPrimitives.WriteUInt32BigEndian(destination[4..8], head.InputSampleRate);
        BinaryPrimitives.WriteInt16BigEndian(destination[8..10], head.OutputGain);
        destination[10] = head.ChannelMappingFamily;
        if (head.ChannelMappingFamily != 0)
        {
            destination[11] = head.StreamCount;
            destination[12] = head.CoupledCount;
            head.ChannelMapping.Span.Slice(0, head.ChannelCount).CopyTo(destination[13..]);
        }
        return total;
    }

    /// <summary>Serialize this header as an ISOBMFF <c>dOps</c> body (BE, no magic).</summary>
    public static byte[] WriteIsobmff(in OpusHead head)
    {
        byte[] bytes = new byte[head.IsobmffByteCount];
        WriteIsobmff(head, bytes);
        return bytes;
    }
}
