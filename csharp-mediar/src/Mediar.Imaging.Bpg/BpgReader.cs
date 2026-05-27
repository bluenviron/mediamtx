using System.Collections.Immutable;

namespace Mediar.Imaging.Bpg;

/// <summary>
/// Reader for BPG (Better Portable Graphics) files. Parses the complete BPG
/// header — pixel format, alpha flag, bit depth, colour space, EXIF / ICC /
/// XMP / thumbnail / animation extension records — and exposes the offset of
/// the embedded HEVC codestream. HEVC decoding is not implemented;
/// <see cref="ReadFramesAsync"/> throws.
/// </summary>
public sealed class BpgReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Bpg;
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }
    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>Pixel format code (0=Grayscale, 1=4:2:0, 2=4:2:2, 3=4:4:4, etc).</summary>
    public int PixelFormatCode { get; }
    /// <summary>True if the image carries an alpha channel.</summary>
    public bool HasAlphaChannel { get; }
    /// <summary>Bit depth (8/10/12/14).</summary>
    public int BitDepth { get; }
    /// <summary>Colour space (0=YCbCr 601, 1=RGB, 2=YCgCo, 3=YCbCr 709, 4=YCbCr-CL, 5=BT.2020 NCL).</summary>
    public int ColorSpaceCode { get; }
    /// <summary>True if the file declares an animation control extension.</summary>
    public bool IsAnimated { get; }
    /// <summary>Decoded list of header extension records.</summary>
    public ImmutableArray<BpgExtension> Extensions { get; }
    /// <summary>Offset where the HEVC bitstream payload starts in the file.</summary>
    public int HevcCodestreamOffset { get; }

    private BpgReader(Stream s, bool owns, ImageInfo info, ImageMetadata meta,
                      int pf, bool alpha, int bd, int cs, bool animated,
                      ImmutableArray<BpgExtension> ext, int csOff)
    {
        _stream = s; _ownsStream = owns;
        Info = info; Metadata = meta;
        PixelFormatCode = pf; HasAlphaChannel = alpha; BitDepth = bd;
        ColorSpaceCode = cs; IsAnimated = animated; Extensions = ext;
        HevcCodestreamOffset = csOff;
    }

    /// <summary>Open a BPG file from a path.</summary>
    public static BpgReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a BPG file from a stream.</summary>
    public static BpgReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] b = ms.ToArray();

        if (b.Length < 6 || b[0] != (byte)'B' || b[1] != (byte)'P' || b[2] != (byte)'G' || b[3] != 0xFB)
            throw new ImageFormatException("Not a BPG file (missing 'BPG\\xFB' signature).");

        int pf = (b[4] >> 5) & 7;
        bool alpha1 = (b[4] & 0x10) != 0;
        int bd = (b[4] & 0xF) + 8;
        int cs = (b[5] >> 4) & 0xF;
        bool hasExt = (b[5] & 0x08) != 0;
        bool alpha2 = (b[5] & 0x04) != 0;
        bool limitedRange = (b[5] & 0x02) != 0;
        bool animated = (b[5] & 0x01) != 0;
        _ = limitedRange;

        int p = 6;
        int w = ReadUe7(b, ref p);
        int h = ReadUe7(b, ref p);
        _ = ReadUe7(b, ref p);  // picture_data_length

        var exts = ImmutableArray.CreateBuilder<BpgExtension>();
        if (hasExt)
        {
            int extLen = ReadUe7(b, ref p);
            int extEnd = p + extLen;
            while (p < extEnd && p < b.Length)
            {
                int extTag = ReadUe7(b, ref p);
                int dataLen = ReadUe7(b, ref p);
                if (p + dataLen > b.Length) break;
                string type = extTag switch
                {
                    1 => "Exif",
                    2 => "ICC",
                    3 => "Xmp",
                    4 => "Thumbnail",
                    5 => "Animation",
                    _ => $"Tag{extTag}",
                };
                exts.Add(new BpgExtension(type, extTag, b.AsSpan(p, dataLen).ToArray()));
                p += dataLen;
            }
            p = extEnd;
        }

        var info = new ImageInfo
        {
            Width = w,
            Height = h,
            BitsPerPixel = bd * (pf == 0 ? 1 : 3) + (alpha1 || alpha2 ? bd : 0),
            ChannelCount = pf == 0 ? 1 : 3,
            HasAlpha = alpha1 || alpha2,
            IsAnimated = animated,
            Format = ImageFormat.Bpg,
            FrameCount = animated ? 0 : 1,
        };
        return new BpgReader(stream, ownsStream, info, ImageMetadata.Empty,
                              pf, alpha1 || alpha2, bd, cs, animated, exts.ToImmutable(), p);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "BPG pixel decoding requires an HEVC decoder which is not implemented in this Mediar release. " +
            "Header, dimensions, bit depth, colour space, extension records, and codestream offset are exposed.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static int ReadUe7(byte[] b, ref int p)
    {
        int v = 0;
        while (p < b.Length)
        {
            byte by = b[p++];
            v = (v << 7) | (by & 0x7F);
            if ((by & 0x80) == 0) break;
        }
        return v;
    }
}

/// <summary>A BPG header extension record.</summary>
public sealed record BpgExtension(string Type, int Tag, byte[] Data);
