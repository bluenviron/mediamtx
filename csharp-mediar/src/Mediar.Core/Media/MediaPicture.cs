using System.Buffers.Binary;
using System.Text;

namespace Mediar;

/// <summary>
/// Picture-type enumeration shared by ID3v2 (APIC frame) and FLAC's
/// PICTURE metadata block (RFC 9639 § 8.8). Values are deliberately
/// numerically identical to the on-wire IDs so a container can copy
/// them verbatim into <see cref="MediaPicture.Type"/>.
/// </summary>
public enum MediaPictureType
{
    /// <summary>Other (no specific role).</summary>
    Other = 0,
    /// <summary>32x32 PNG file icon.</summary>
    FileIcon32x32 = 1,
    /// <summary>Other file icon.</summary>
    OtherFileIcon = 2,
    /// <summary>Cover (front).</summary>
    CoverFront = 3,
    /// <summary>Cover (back).</summary>
    CoverBack = 4,
    /// <summary>Leaflet page.</summary>
    LeafletPage = 5,
    /// <summary>Media (e.g. label side of CD).</summary>
    Media = 6,
    /// <summary>Lead artist / lead performer / soloist.</summary>
    LeadArtist = 7,
    /// <summary>Artist / performer.</summary>
    Artist = 8,
    /// <summary>Conductor.</summary>
    Conductor = 9,
    /// <summary>Band / orchestra.</summary>
    Band = 10,
    /// <summary>Composer.</summary>
    Composer = 11,
    /// <summary>Lyricist / text writer.</summary>
    Lyricist = 12,
    /// <summary>Recording location.</summary>
    RecordingLocation = 13,
    /// <summary>During recording.</summary>
    DuringRecording = 14,
    /// <summary>During performance.</summary>
    DuringPerformance = 15,
    /// <summary>Movie / video screen capture.</summary>
    ScreenCapture = 16,
    /// <summary>A bright coloured fish.</summary>
    BrightColouredFish = 17,
    /// <summary>Illustration.</summary>
    Illustration = 18,
    /// <summary>Band / artist logotype.</summary>
    BandLogo = 19,
    /// <summary>Publisher / studio logotype.</summary>
    PublisherLogo = 20,
}

/// <summary>
/// An embedded picture attached to a media file: cover art, artist
/// photo, label scan, etc. The raw <see cref="Data"/> is the encoded
/// image bytes (JPEG, PNG, or whatever <see cref="MimeType"/> says);
/// it has not been decoded into pixels. Callers can hand it to any
/// Mediar imaging reader to decode.
/// </summary>
public sealed record MediaPicture
{
    /// <summary>Picture role (cover front, artist photo, etc.).</summary>
    public MediaPictureType Type { get; init; }

    /// <summary>
    /// MIME type of <see cref="Data"/>, e.g. <c>image/jpeg</c>,
    /// <c>image/png</c>. May be the special string <c>-->"</c> in
    /// ID3v2 APIC, which means <see cref="Data"/> is a URL instead of
    /// an image; Mediar surfaces that as-is.
    /// </summary>
    public string MimeType { get; init; } = string.Empty;

    /// <summary>Human-readable description (often empty in practice).</summary>
    public string Description { get; init; } = string.Empty;

    /// <summary>
    /// Pixel width as declared by the source. Zero when the source
    /// does not encode it (ID3v2 APIC, MP4 covr) - decoders that
    /// probe the actual image can fill it later.
    /// </summary>
    public int Width { get; init; }

    /// <summary>Pixel height as declared by the source. Zero when unknown.</summary>
    public int Height { get; init; }

    /// <summary>
    /// Colour depth in bits-per-pixel as declared by the source. Zero
    /// when the source does not encode it.
    /// </summary>
    public int ColorDepth { get; init; }

    /// <summary>
    /// Number of colours in the palette (for indexed images). Zero
    /// when the source does not encode it or the image is direct-colour.
    /// </summary>
    public int IndexedColors { get; init; }

    /// <summary>Raw encoded image bytes (not pixel data).</summary>
    public ReadOnlyMemory<byte> Data { get; init; }
}

/// <summary>
/// Parser for the FLAC PICTURE metadata block (RFC 9639 § 8.8). The
/// same byte layout is reused by Vorbis Comments via the
/// <c>METADATA_BLOCK_PICTURE</c> field (base64-encoded).
/// </summary>
public static class FlacPictureBlock
{
    /// <summary>
    /// Decode a FLAC PICTURE block / <c>METADATA_BLOCK_PICTURE</c>
    /// payload into a <see cref="MediaPicture"/>. Returns
    /// <see langword="null"/> when the payload is malformed.
    /// </summary>
    public static MediaPicture? TryParse(ReadOnlySpan<byte> payload)
    {
        // 4  picture type (BE u32)
        // 4  mime length
        // N  mime ASCII
        // 4  desc length
        // N  desc UTF-8
        // 4  width
        // 4  height
        // 4  colour depth
        // 4  palette size
        // 4  data length
        // N  data
        if (payload.Length < 4 + 4) return null;
        int p = 0;

        uint pictureType = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        if (p + 4 > payload.Length) return null;

        uint mimeLen = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        if (mimeLen > (uint)(payload.Length - p)) return null;
        string mime = mimeLen == 0 ? string.Empty : Encoding.ASCII.GetString(payload.Slice(p, (int)mimeLen));
        p += (int)mimeLen;
        if (p + 4 > payload.Length) return null;

        uint descLen = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        if (descLen > (uint)(payload.Length - p)) return null;
        string desc = descLen == 0 ? string.Empty : Encoding.UTF8.GetString(payload.Slice(p, (int)descLen));
        p += (int)descLen;
        if (p + 20 > payload.Length) return null;

        uint width = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        uint height = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        uint depth = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        uint palette = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        uint dataLen = BinaryPrimitives.ReadUInt32BigEndian(payload[p..]); p += 4;
        if (dataLen > (uint)(payload.Length - p)) return null;

        byte[] data = payload.Slice(p, (int)dataLen).ToArray();
        MediaPictureType safeType = pictureType <= (uint)MediaPictureType.PublisherLogo
            ? (MediaPictureType)pictureType
            : MediaPictureType.Other;

        return new MediaPicture
        {
            Type = safeType,
            MimeType = mime,
            Description = desc,
            Width = (int)width,
            Height = (int)height,
            ColorDepth = (int)depth,
            IndexedColors = (int)palette,
            Data = data,
        };
    }
}
