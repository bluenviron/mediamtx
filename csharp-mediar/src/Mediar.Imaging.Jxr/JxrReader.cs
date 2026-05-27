using System.Buffers.Binary;
using System.Collections.Immutable;

namespace Mediar.Imaging.Jxr;

/// <summary>
/// Reader for JPEG XR (<c>.jxr</c>, <c>.wdp</c>, <c>.hdp</c>) files. Parses the
/// TIFF-IFD outer container with the JPEG XR tag set defined in ITU-T T.832
/// (also published as ISO/IEC 29199-2). The bitstream decoder is not
/// implemented; <see cref="ReadFramesAsync"/> throws.
/// </summary>
public sealed class JxrReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Jxr;
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }
    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>Decoded IFD entries.</summary>
    public ImmutableArray<JxrTag> Tags { get; }

    /// <summary>GUID-coded pixel format (tag 0xBC00), or all-zero if missing.</summary>
    public Guid PixelFormatGuid { get; }

    /// <summary>Offset into the source where the JPEG XR codestream begins (tag 0xBCC0).</summary>
    public uint ImageOffset { get; }

    /// <summary>Byte length of the JPEG XR codestream (tag 0xBCC1).</summary>
    public uint ImageByteCount { get; }

    private JxrReader(Stream s, bool owns, ImageInfo info, ImageMetadata meta,
                      ImmutableArray<JxrTag> tags, Guid pixelFormatGuid,
                      uint imageOffset, uint imageByteCount)
    {
        _stream = s; _ownsStream = owns;
        Info = info; Metadata = meta;
        Tags = tags; PixelFormatGuid = pixelFormatGuid;
        ImageOffset = imageOffset; ImageByteCount = imageByteCount;
    }

    /// <summary>Open a JXR/WDP/HDP file from a path.</summary>
    public static JxrReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a JXR/WDP/HDP file from a stream.</summary>
    public static JxrReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] b = ms.ToArray();

        if (b.Length < 8 || b[0] != (byte)'I' || b[1] != (byte)'I' || b[2] != 0xBC)
            throw new ImageFormatException("Not a JPEG XR file (expected II 0xBC signature).");

        uint ifdOffset = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(4));
        var tagsBuilder = ImmutableArray.CreateBuilder<JxrTag>();
        Guid pf = Guid.Empty;
        uint imgOff = 0, imgLen = 0;
        int width = 0, height = 0, bitsPerPixel = 0;

        if (ifdOffset + 2 <= b.Length)
        {
            int n = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan((int)ifdOffset));
            int p = (int)ifdOffset + 2;
            for (int i = 0; i < n && p + 12 <= b.Length; i++, p += 12)
            {
                ushort tag = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(p));
                ushort type = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(p + 2));
                uint count = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(p + 4));
                uint valueOrOff = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(p + 8));
                tagsBuilder.Add(new JxrTag(tag, type, count, valueOrOff));
                switch (tag)
                {
                    case 0xBC00 when count == 16 && valueOrOff + 16 <= b.Length:
                        pf = new Guid(b.AsSpan((int)valueOrOff, 16));
                        break;
                    case 0xBC80: width = (int)valueOrOff; break;
                    case 0xBC81: height = (int)valueOrOff; break;
                    case 0xBC83: bitsPerPixel = (int)valueOrOff; break;
                    case 0xBCC0: imgOff = valueOrOff; break;
                    case 0xBCC1: imgLen = valueOrOff; break;
                }
            }
        }

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = bitsPerPixel,
            Format = ImageFormat.Jxr,
            FrameCount = 1,
        };
        return new JxrReader(stream, ownsStream, info, ImageMetadata.Empty,
                              tagsBuilder.ToImmutable(), pf, imgOff, imgLen);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "JPEG XR bitstream decoding is not implemented in this Mediar release. " +
            "Container metadata, pixel-format GUID, and codestream offset/length are exposed.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}

/// <summary>A raw TIFF-style IFD entry from the JPEG XR file.</summary>
public readonly record struct JxrTag(ushort Tag, ushort Type, uint Count, uint ValueOrOffset);
