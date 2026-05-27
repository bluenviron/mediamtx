using System.Buffers.Binary;

namespace Mediar.Imaging.Png;

/// <summary>PNG color-type values from the IHDR chunk.</summary>
public enum PngColorType : byte
{
    /// <summary>Single grayscale channel.</summary>
    Grayscale = 0,
    /// <summary>RGB.</summary>
    Rgb = 2,
    /// <summary>Palette index (8-bit only).</summary>
    Indexed = 3,
    /// <summary>Grayscale + alpha.</summary>
    GrayscaleAlpha = 4,
    /// <summary>RGB + alpha.</summary>
    Rgba = 6,
}

/// <summary>Parsed PNG IHDR.</summary>
internal readonly record struct PngHeader(
    int Width,
    int Height,
    int BitDepth,
    PngColorType ColorType,
    byte Compression,
    byte Filter,
    byte Interlace)
{
    public int ChannelCount => ColorType switch
    {
        PngColorType.Grayscale or PngColorType.Indexed => 1,
        PngColorType.GrayscaleAlpha => 2,
        PngColorType.Rgb => 3,
        PngColorType.Rgba => 4,
        _ => 0,
    };

    public int BitsPerPixel => BitDepth * ChannelCount;

    public bool HasAlpha => ColorType is PngColorType.GrayscaleAlpha or PngColorType.Rgba;

    public PixelFormat PixelFormat => ColorType switch
    {
        PngColorType.Grayscale => BitDepth <= 8 ? PixelFormat.Gray8 : PixelFormat.Gray16,
        PngColorType.Rgb => BitDepth == 16 ? PixelFormat.Rgb48 : PixelFormat.Rgb24,
        PngColorType.Indexed => PixelFormat.Indexed8,
        PngColorType.GrayscaleAlpha => BitDepth == 16 ? PixelFormat.Rgba64 : PixelFormat.GrayAlpha16,
        PngColorType.Rgba => BitDepth == 16 ? PixelFormat.Rgba64 : PixelFormat.Rgba32,
        _ => PixelFormat.Unknown,
    };

    public int OutputBytesPerPixel
    {
        get
        {
            int bytesPerSample = BitDepth == 16 ? 2 : 1;
            int channels = ColorType switch
            {
                PngColorType.Grayscale or PngColorType.Indexed => 1,
                PngColorType.GrayscaleAlpha => BitDepth == 16 ? 4 : 2,
                PngColorType.Rgb => 3,
                PngColorType.Rgba => 4,
                _ => 1,
            };
            return channels * bytesPerSample;
        }
    }

    public static PngHeader Parse(ReadOnlySpan<byte> data)
    {
        if (data.Length < 13) throw new ImageFormatException("PNG IHDR too short.");
        return new PngHeader(
            Width: (int)BinaryPrimitives.ReadUInt32BigEndian(data[..4]),
            Height: (int)BinaryPrimitives.ReadUInt32BigEndian(data.Slice(4, 4)),
            BitDepth: data[8],
            ColorType: (PngColorType)data[9],
            Compression: data[10],
            Filter: data[11],
            Interlace: data[12]);
    }
}
