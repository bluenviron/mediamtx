namespace Mediar.Imaging;

/// <summary>
/// Pixel layout of an <see cref="ImageFrame"/>. Names are read as
/// channel-byte-order from byte offset 0 (so <see cref="Rgba32"/> means
/// the red byte comes first in memory regardless of host endian).
/// </summary>
public enum PixelFormat
{
    /// <summary>Unspecified or unknown.</summary>
    Unknown = 0,

    /// <summary>8-bit single-channel grayscale.</summary>
    Gray8,
    /// <summary>16-bit single-channel grayscale, little-endian on disk.</summary>
    Gray16,
    /// <summary>8-bit grayscale + 8-bit alpha.</summary>
    GrayAlpha16,

    /// <summary>R8 G8 B8, no alpha. 24 bits per pixel.</summary>
    Rgb24,
    /// <summary>B8 G8 R8, no alpha. 24 bits per pixel (BMP/TGA/DIB native).</summary>
    Bgr24,

    /// <summary>R8 G8 B8 A8, 32 bits per pixel.</summary>
    Rgba32,
    /// <summary>B8 G8 R8 A8, 32 bits per pixel (Windows / DDS native).</summary>
    Bgra32,
    /// <summary>A8 R8 G8 B8, 32 bits per pixel (Mac PICT / some legacy).</summary>
    Argb32,

    /// <summary>R16 G16 B16, 48 bits per pixel.</summary>
    Rgb48,
    /// <summary>R16 G16 B16 A16, 64 bits per pixel.</summary>
    Rgba64,

    /// <summary>32-bit IEEE float per channel, RGB (HDR).</summary>
    Rgb96Float,
    /// <summary>32-bit IEEE float per channel, RGBA (HDR).</summary>
    Rgba128Float,

    /// <summary>C M Y K, 8 bits per channel.</summary>
    Cmyk32,

    /// <summary>Indexed 8-bit palette + a palette stored on the frame.</summary>
    Indexed8,

    /// <summary>1 bit per pixel (Black=0, White=1).</summary>
    Indexed1,
    /// <summary>4 bits per pixel, palette stored on the frame.</summary>
    Indexed4,

    /// <summary>5/6/5 RGB packed in 16 bits (DDS / phone GPU).</summary>
    Rgb565,
    /// <summary>5/5/5 RGB + 1 bit alpha in 16 bits.</summary>
    Rgba5551,

    /// <summary>Floating-point HDR (RGBE) - 32-bit packed.</summary>
    Rgbe32,
}

/// <summary>
/// Convenience extensions for <see cref="PixelFormat"/>.
/// </summary>
public static class PixelFormatExtensions
{
    /// <summary>Returns bits per pixel for a fully-decoded buffer.</summary>
    public static int BitsPerPixel(this PixelFormat f) => f switch
    {
        PixelFormat.Indexed1 => 1,
        PixelFormat.Indexed4 => 4,
        PixelFormat.Gray8 or PixelFormat.Indexed8 => 8,
        PixelFormat.GrayAlpha16 or PixelFormat.Gray16
            or PixelFormat.Rgb565 or PixelFormat.Rgba5551 => 16,
        PixelFormat.Rgb24 or PixelFormat.Bgr24 => 24,
        PixelFormat.Rgba32 or PixelFormat.Bgra32 or PixelFormat.Argb32
            or PixelFormat.Cmyk32 or PixelFormat.Rgbe32 => 32,
        PixelFormat.Rgb48 => 48,
        PixelFormat.Rgba64 => 64,
        PixelFormat.Rgb96Float => 96,
        PixelFormat.Rgba128Float => 128,
        _ => 0,
    };

    /// <summary>Returns the number of color channels (excluding palette indirection).</summary>
    public static int ChannelCount(this PixelFormat f) => f switch
    {
        PixelFormat.Gray8 or PixelFormat.Gray16 or PixelFormat.Indexed1
            or PixelFormat.Indexed4 or PixelFormat.Indexed8 => 1,
        PixelFormat.GrayAlpha16 => 2,
        PixelFormat.Rgb24 or PixelFormat.Bgr24 or PixelFormat.Rgb48
            or PixelFormat.Rgb96Float or PixelFormat.Rgb565
            or PixelFormat.Rgbe32 => 3,
        PixelFormat.Rgba32 or PixelFormat.Bgra32 or PixelFormat.Argb32
            or PixelFormat.Rgba64 or PixelFormat.Rgba128Float
            or PixelFormat.Cmyk32 or PixelFormat.Rgba5551 => 4,
        _ => 0,
    };

    /// <summary>True if the pixel format carries an alpha channel.</summary>
    public static bool HasAlpha(this PixelFormat f) => f is
        PixelFormat.GrayAlpha16 or
        PixelFormat.Rgba32 or PixelFormat.Bgra32 or PixelFormat.Argb32 or
        PixelFormat.Rgba64 or PixelFormat.Rgba128Float or
        PixelFormat.Rgba5551;
}
