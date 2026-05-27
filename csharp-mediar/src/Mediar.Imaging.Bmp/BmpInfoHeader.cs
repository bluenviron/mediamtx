using System.Buffers.Binary;

namespace Mediar.Imaging.Bmp;

/// <summary>Compression method values used by the DIB header.</summary>
public enum BmpCompression
{
    /// <summary>No compression (raw BGR / BGRA).</summary>
    Rgb = 0,

    /// <summary>RLE-encoded 8 bits per pixel.</summary>
    Rle8 = 1,

    /// <summary>RLE-encoded 4 bits per pixel.</summary>
    Rle4 = 2,

    /// <summary>Per-channel bit masks follow.</summary>
    BitFields = 3,

    /// <summary>Image is a JPEG payload (rare; not decoded by Mediar).</summary>
    Jpeg = 4,

    /// <summary>Image is a PNG payload (rare; not decoded by Mediar).</summary>
    Png = 5,

    /// <summary>Per-channel bit masks including alpha (V3 alphabitfields).</summary>
    AlphaBitFields = 6,
}

/// <summary>
/// Strongly-typed view of any DIB header version supported by Mediar
/// (BITMAPCOREHEADER, BITMAPINFOHEADER, BITMAPV4HEADER, BITMAPV5HEADER).
/// </summary>
internal readonly record struct BmpInfoHeader(
    uint HeaderSize,
    int Width,
    int Height,
    ushort Planes,
    ushort BitsPerPixel,
    BmpCompression Compression,
    uint ImageSize,
    int XPelsPerMeter,
    int YPelsPerMeter,
    uint PaletteColors,
    uint ImportantColors,
    uint RedMask,
    uint GreenMask,
    uint BlueMask,
    uint AlphaMask)
{
    public static BmpInfoHeader Parse(ReadOnlySpan<byte> dib)
    {
        uint sz = BinaryPrimitives.ReadUInt32LittleEndian(dib[..4]);
        if (sz == 12)
        {
            // BITMAPCOREHEADER
            return new BmpInfoHeader(
                HeaderSize: sz,
                Width: BinaryPrimitives.ReadInt16LittleEndian(dib.Slice(4, 2)),
                Height: BinaryPrimitives.ReadInt16LittleEndian(dib.Slice(6, 2)),
                Planes: BinaryPrimitives.ReadUInt16LittleEndian(dib.Slice(8, 2)),
                BitsPerPixel: BinaryPrimitives.ReadUInt16LittleEndian(dib.Slice(10, 2)),
                Compression: BmpCompression.Rgb,
                ImageSize: 0,
                XPelsPerMeter: 0,
                YPelsPerMeter: 0,
                PaletteColors: 0,
                ImportantColors: 0,
                RedMask: 0, GreenMask: 0, BlueMask: 0, AlphaMask: 0);
        }
        // BITMAPINFOHEADER and up
        return new BmpInfoHeader(
            HeaderSize: sz,
            Width: BinaryPrimitives.ReadInt32LittleEndian(dib.Slice(4, 4)),
            Height: BinaryPrimitives.ReadInt32LittleEndian(dib.Slice(8, 4)),
            Planes: BinaryPrimitives.ReadUInt16LittleEndian(dib.Slice(12, 2)),
            BitsPerPixel: BinaryPrimitives.ReadUInt16LittleEndian(dib.Slice(14, 2)),
            Compression: (BmpCompression)BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(16, 4)),
            ImageSize: BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(20, 4)),
            XPelsPerMeter: BinaryPrimitives.ReadInt32LittleEndian(dib.Slice(24, 4)),
            YPelsPerMeter: BinaryPrimitives.ReadInt32LittleEndian(dib.Slice(28, 4)),
            PaletteColors: BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(32, 4)),
            ImportantColors: BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(36, 4)),
            RedMask: sz >= 56 ? BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(40, 4)) : 0,
            GreenMask: sz >= 56 ? BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(44, 4)) : 0,
            BlueMask: sz >= 56 ? BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(48, 4)) : 0,
            AlphaMask: sz >= 56 ? BinaryPrimitives.ReadUInt32LittleEndian(dib.Slice(52, 4)) : 0);
    }
}
