namespace Mediar.Imaging.TiffRaw;

/// <summary>
/// Standard EXIF / IFD 0 metadata fields shared by every TIFF-based RAW
/// container (DNG, NEF, ARW, PEF, DCR, ORF, SRW, 3FR, NRW, SR2, MEF, MOS,
/// ERF, MRW, CRW). Per-format readers expose this record verbatim via
/// <c>TiffRawReader.Raw</c>; the only thing distinguishing one format
/// from another at the metadata layer is the value of the
/// <see cref="Make"/> tag.
/// </summary>
public sealed record TiffRawMetadata
{
    /// <summary>EXIF Make tag (camera manufacturer identifier).</summary>
    public required string? Make { get; init; }

    /// <summary>EXIF Model tag (body identifier).</summary>
    public required string? Model { get; init; }

    /// <summary>EXIF Software string (firmware version / processing tool).</summary>
    public required string? Software { get; init; }

    /// <summary>EXIF DateTime (raw ASCII as stored, typically "YYYY:MM:DD HH:MM:SS").</summary>
    public required string? DateTime { get; init; }

    /// <summary>EXIF Artist tag.</summary>
    public required string? Artist { get; init; }

    /// <summary>EXIF Copyright tag.</summary>
    public required string? Copyright { get; init; }

    /// <summary>
    /// Number of bytes occupied by the raw vendor MakerNote (EXIF tag 0x927C),
    /// or 0 if absent. The byte payload is not decoded - each vendor
    /// (Canon, Nikon, Sony, ...) uses a different proprietary layout.
    /// </summary>
    public required int MakerNoteLength { get; init; }
}

/// <summary>
/// Per-sub-image (per-IFD) descriptor exposed by every TIFF-RAW reader.
/// One instance covers IFD 0 (typically the preview / thumbnail) and one
/// per SubIFD (typically the full-resolution sensor mosaic and any
/// intermediate pyramid levels).
/// </summary>
public sealed record TiffRawSubImageInfo
{
    /// <summary>Width in pixels.</summary>
    public required int Width { get; init; }

    /// <summary>Height in pixels.</summary>
    public required int Height { get; init; }

    /// <summary>Bits per sample (channel depth in bits, e.g. 8, 12, 14, 16).</summary>
    public required int BitsPerSample { get; init; }

    /// <summary>Samples per pixel (1 for grayscale / mosaic, 3 for RGB).</summary>
    public required int SamplesPerPixel { get; init; }

    /// <summary>
    /// TIFF compression tag. 1 = uncompressed, 5 = LZW, 7 = JPEG (DCT),
    /// 8 = Adobe Deflate, 32773 = PackBits, 32946 = Deflate; any other
    /// value is typically a vendor-proprietary scheme (Nikon 34713,
    /// Sony 32770, Mamiya 65000, Leaf 34713, Epson 65535, ...).
    /// </summary>
    public required int CompressionTag { get; init; }

    /// <summary>TIFF photometric interpretation tag.</summary>
    public required int Photometric { get; init; }

    /// <summary>NewSubFileType (tag 0x00FE). 0 = primary image, 1 = reduced-res preview, 2 = single page, 3 = transparency mask.</summary>
    public required int NewSubFileType { get; init; }

    /// <summary>Pixel format Mediar will emit (<see cref="PixelFormat.Unknown"/> if not yet supported).</summary>
    public required PixelFormat PixelFormat { get; init; }

    /// <summary>0 for IFD 0, 1 for direct SubIFD children, 2 for nested SubIFDs, etc.</summary>
    public required int SubIfdLevel { get; init; }

    /// <summary>True if Mediar can decode this sub-image through the underlying TIFF reader.</summary>
    public required bool CanDecodePixels { get; init; }
}

/// <summary>
/// A single 12-byte TIFF IFD entry as parsed off the wire.
/// </summary>
public readonly record struct IfdEntry(int Tag, int Type, uint Count, uint ValueOffset);
