using System.Buffers.Binary;
using System.Collections.Frozen;
using System.IO.Compression;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.PackBits;

namespace Mediar.Imaging.Psd;

/// <summary>
/// Reader for Adobe Photoshop PSD (version 1) and PSB (Photoshop Big,
/// version 2) documents. Decodes the merged-composite image stored at the
/// end of the file, which Photoshop maintains for backward-compatibility
/// preview. Supports raw (compression 0), PackBits (1), and Deflate
/// (zlib-without-prediction = 2, zlib-with-prediction = 3) channel data
/// for 8-bit and 16-bit depths in Grayscale, RGB, RGBA, and CMYK colour
/// modes.
/// </summary>
/// <remarks>
/// Layer and mask info is parsed enough to skip cleanly but individual
/// layers are not exposed yet. Bitmap (1 bpp) and Indexed images decode
/// as their native pixel format; Lab is read as four planar 8-bit
/// channels and surfaced as <see cref="PixelFormat.Cmyk32"/> for now
/// (callers can convert further if needed).
/// </remarks>
public sealed class PsdReader : IImageReader
{
    private const ushort PsdVersion = 1;
    private const ushort PsbVersion = 2;

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _imageDataStart;
    private readonly bool _isPsb;
    private readonly ushort _channels;
    private readonly ushort _depth;
    private readonly PsdColorMode _colorMode;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _isPsb ? ImageFormat.Psb : ImageFormat.Psd;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>True if the document is PSB (Photoshop Big), false for PSD.</summary>
    public bool IsPsb => _isPsb;

    /// <summary>Channel count declared in the PSD header (1..56).</summary>
    public int ChannelCount => _channels;

    /// <summary>Bit depth: 1, 8, 16, or 32.</summary>
    public int BitDepth => _depth;

    /// <summary>Colour mode declared in the PSD header.</summary>
    public PsdColorMode ColorMode => _colorMode;

    private PsdReader(Stream s, bool owns, byte[] bytes, ImageInfo info, ImageMetadata meta,
                      bool canDecode, bool isPsb, ushort channels, ushort depth,
                      PsdColorMode colorMode, int imageDataStart)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes;
        Info = info; Metadata = meta; CanDecodePixels = canDecode;
        _isPsb = isPsb;
        _channels = channels;
        _depth = depth;
        _colorMode = colorMode;
        _imageDataStart = imageDataStart;
    }

    /// <summary>Open a PSD/PSB file by path.</summary>
    public static PsdReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a PSD/PSB from a stream.</summary>
    public static PsdReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 26 || bytes[0] != (byte)'8' || bytes[1] != (byte)'B'
            || bytes[2] != (byte)'P' || bytes[3] != (byte)'S')
        {
            throw new ImageFormatException("Not a PSD/PSB file (missing 8BPS signature).");
        }

        ushort version = BinaryPrimitives.ReadUInt16BigEndian(bytes.AsSpan(4));
        bool isPsb = version == PsbVersion;
        if (version != PsdVersion && !isPsb)
            throw new ImageFormatException("Unsupported PSD version " + version);

        ushort channels = BinaryPrimitives.ReadUInt16BigEndian(bytes.AsSpan(12));
        int height = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(14));
        int width = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(18));
        ushort depth = BinaryPrimitives.ReadUInt16BigEndian(bytes.AsSpan(22));
        var colorMode = (PsdColorMode)BinaryPrimitives.ReadUInt16BigEndian(bytes.AsSpan(24));

        int cursor = 26;
        // Color mode data
        uint colorModeLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(cursor));
        cursor += 4;
        int colorModeStart = cursor;
        cursor += (int)colorModeLen;

        // Image resources
        uint resourcesLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(cursor));
        cursor += 4;
        int resourcesStart = cursor;
        cursor += (int)resourcesLen;

        // Layer + Mask info: length is 4 bytes for PSD, 8 bytes for PSB
        long layerLen;
        if (isPsb)
        {
            layerLen = (long)BinaryPrimitives.ReadUInt64BigEndian(bytes.AsSpan(cursor));
            cursor += 8;
        }
        else
        {
            layerLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(cursor));
            cursor += 4;
        }
        cursor += (int)layerLen;

        int imageDataStart = cursor;

        var pf = (colorMode, depth, channels) switch
        {
            (PsdColorMode.Bitmap, 1, _) => PixelFormat.Indexed1,
            (PsdColorMode.Grayscale, 8, 1) => PixelFormat.Gray8,
            (PsdColorMode.Grayscale, 8, 2) => PixelFormat.GrayAlpha16,
            (PsdColorMode.Grayscale, 16, 1) => PixelFormat.Gray16,
            (PsdColorMode.Indexed, 8, _) => PixelFormat.Indexed8,
            (PsdColorMode.Rgb, 8, 3) => PixelFormat.Rgb24,
            (PsdColorMode.Rgb, 8, 4) => PixelFormat.Rgba32,
            (PsdColorMode.Rgb, 16, 3) => PixelFormat.Rgb48,
            (PsdColorMode.Rgb, 16, 4) => PixelFormat.Rgba64,
            (PsdColorMode.Cmyk, 8, 4) => PixelFormat.Cmyk32,
            (PsdColorMode.Lab, 8, 3 or 4) => PixelFormat.Cmyk32,
            _ => PixelFormat.Unknown,
        };
        bool supportedDepth = depth is 8 or 16;
        bool supportedMode = pf != PixelFormat.Unknown;
        bool canDecode = supportedDepth && supportedMode && imageDataStart + 2 <= bytes.Length;

        var meta = BuildMetadata(bytes, resourcesStart, (int)resourcesLen);

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = depth * channels,
            ChannelCount = channels,
            PixelFormat = pf,
            Format = isPsb ? ImageFormat.Psb : ImageFormat.Psd,
            HasAlpha = pf is PixelFormat.Rgba32 or PixelFormat.Rgba64 or PixelFormat.GrayAlpha16,
            FrameCount = 1,
        };
        _ = colorModeStart;

        return new PsdReader(stream, ownsStream, bytes, info, meta, canDecode,
                              isPsb, channels, depth, colorMode, imageDataStart);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        if (!CanDecodePixels)
            throw new NotSupportedException(
                $"PSD pixel decode for depth={_depth} mode={_colorMode} channels={_channels} is not implemented.");

        int width = Info.Width;
        int height = Info.Height;
        int channelCount = _channels;
        int bytesPerSample = _depth / 8;
        if (bytesPerSample == 0) bytesPerSample = 1;
        int rowBytesPerChannel = width * bytesPerSample;

        ushort compression = BinaryPrimitives.ReadUInt16BigEndian(_bytes.AsSpan(_imageDataStart));
        int cursor = _imageDataStart + 2;

        // Decode each channel plane into a flat width*height*bytesPerSample buffer
        var planes = new byte[channelCount][];
        switch (compression)
        {
            case 0:
                for (int c = 0; c < channelCount; c++)
                {
                    var plane = new byte[rowBytesPerChannel * height];
                    Buffer.BlockCopy(_bytes, cursor, plane, 0, plane.Length);
                    cursor += plane.Length;
                    planes[c] = plane;
                }
                break;
            case 1:
                {
                    // PackBits: row lengths follow, channels stacked
                    int totalRows = height * channelCount;
                    var rowLens = new int[totalRows];
                    if (_isPsb)
                    {
                        for (int i = 0; i < totalRows; i++)
                        {
                            rowLens[i] = (int)BinaryPrimitives.ReadUInt32BigEndian(_bytes.AsSpan(cursor));
                            cursor += 4;
                        }
                    }
                    else
                    {
                        for (int i = 0; i < totalRows; i++)
                        {
                            rowLens[i] = BinaryPrimitives.ReadUInt16BigEndian(_bytes.AsSpan(cursor));
                            cursor += 2;
                        }
                    }
                    for (int c = 0; c < channelCount; c++)
                    {
                        var plane = new byte[rowBytesPerChannel * height];
                        for (int y = 0; y < height; y++)
                        {
                            int len = rowLens[c * height + y];
                            var src = new ReadOnlySpan<byte>(_bytes, cursor, len);
                            var decoded = PackBitsCodec.Decode(src, rowBytesPerChannel);
                            Buffer.BlockCopy(decoded, 0, plane, y * rowBytesPerChannel, rowBytesPerChannel);
                            cursor += len;
                        }
                        planes[c] = plane;
                    }
                }
                break;
            case 2:
            case 3:
                {
                    int remaining = _bytes.Length - cursor;
                    int perChannelLen = remaining / channelCount;
                    for (int c = 0; c < channelCount; c++)
                    {
                        var plane = ZlibDecompress(_bytes.AsSpan(cursor, perChannelLen), rowBytesPerChannel * height);
                        cursor += perChannelLen;
                        if (compression == 3 && _depth == 8)
                            UnpackPrediction8(plane, width, height);
                        else if (compression == 3 && _depth == 16)
                            UnpackPrediction16(plane, width, height);
                        planes[c] = plane;
                    }
                }
                break;
            default:
                throw new NotSupportedException("Unknown PSD compression " + compression);
        }

        // Interleave planes into the pixel format
        yield return Interleave(planes, width, height, Info.PixelFormat, _depth);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static ImageFrame Interleave(byte[][] planes, int width, int height, PixelFormat pf, int depth)
    {
        int channels = planes.Length;
        int bps = Math.Max(1, depth / 8);
        int rowBytesPerChannel = width * bps;

        switch (pf)
        {
            case PixelFormat.Gray8:
            case PixelFormat.Gray16:
            case PixelFormat.Indexed8:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, rowBytesPerChannel);
                Buffer.BlockCopy(planes[0], 0, buf, 0, rowBytesPerChannel * height);
                return frame;
            }
            case PixelFormat.Rgb24:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, width * 3);
                for (int y = 0; y < height; y++)
                {
                    int srcRow = y * width;
                    int dstRow = y * width * 3;
                    for (int x = 0; x < width; x++)
                    {
                        buf[dstRow + x * 3 + 0] = planes[0][srcRow + x];
                        buf[dstRow + x * 3 + 1] = planes[1][srcRow + x];
                        buf[dstRow + x * 3 + 2] = planes[2][srcRow + x];
                    }
                }
                return frame;
            }
            case PixelFormat.Rgba32:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, width * 4);
                for (int y = 0; y < height; y++)
                {
                    int srcRow = y * width;
                    int dstRow = y * width * 4;
                    for (int x = 0; x < width; x++)
                    {
                        buf[dstRow + x * 4 + 0] = planes[0][srcRow + x];
                        buf[dstRow + x * 4 + 1] = planes[1][srcRow + x];
                        buf[dstRow + x * 4 + 2] = planes[2][srcRow + x];
                        buf[dstRow + x * 4 + 3] = planes[3][srcRow + x];
                    }
                }
                return frame;
            }
            case PixelFormat.Cmyk32:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, width * 4);
                int planeCount = Math.Min(4, planes.Length);
                for (int y = 0; y < height; y++)
                {
                    int srcRow = y * width;
                    int dstRow = y * width * 4;
                    for (int x = 0; x < width; x++)
                    {
                        for (int c = 0; c < planeCount; c++)
                        {
                            // Photoshop stores CMYK inverted (255 = no ink).
                            buf[dstRow + x * 4 + c] = (byte)(255 - planes[c][srcRow + x]);
                        }
                    }
                }
                return frame;
            }
            case PixelFormat.Rgb48:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, width * 6);
                for (int y = 0; y < height; y++)
                {
                    int dstRow = y * width * 6;
                    int srcRow = y * width * 2;
                    for (int x = 0; x < width; x++)
                    {
                        for (int c = 0; c < 3; c++)
                        {
                            ushort v = BinaryPrimitives.ReadUInt16BigEndian(planes[c].AsSpan(srcRow + x * 2));
                            BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(dstRow + (x * 3 + c) * 2), v);
                        }
                    }
                }
                return frame;
            }
            case PixelFormat.Rgba64:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, width * 8);
                for (int y = 0; y < height; y++)
                {
                    int dstRow = y * width * 8;
                    int srcRow = y * width * 2;
                    for (int x = 0; x < width; x++)
                    {
                        for (int c = 0; c < 4; c++)
                        {
                            ushort v = BinaryPrimitives.ReadUInt16BigEndian(planes[c].AsSpan(srcRow + x * 2));
                            BinaryPrimitives.WriteUInt16LittleEndian(buf.AsSpan(dstRow + (x * 4 + c) * 2), v);
                        }
                    }
                }
                return frame;
            }
            case PixelFormat.GrayAlpha16:
            {
                var (frame, buf) = ImageFrame.Rent(width, height, pf, width * 2);
                for (int y = 0; y < height; y++)
                {
                    int srcRow = y * width;
                    int dstRow = y * width * 2;
                    for (int x = 0; x < width; x++)
                    {
                        buf[dstRow + x * 2 + 0] = planes[0][srcRow + x];
                        buf[dstRow + x * 2 + 1] = planes[1][srcRow + x];
                    }
                }
                return frame;
            }
            default:
                throw new NotSupportedException("PSD interleave not implemented for " + pf);
        }
    }

    private static byte[] ZlibDecompress(ReadOnlySpan<byte> src, int expectedLength)
    {
        var output = new byte[expectedLength];
        // Skip 2-byte zlib header (CMF + FLG); .NET DeflateStream needs raw deflate.
        if (src.Length < 2) return output;
        using var ms = new MemoryStream(src[2..].ToArray(), writable: false);
        using var ds = new DeflateStream(ms, CompressionMode.Decompress);
        int total = 0;
        while (total < expectedLength)
        {
            int read = ds.Read(output, total, expectedLength - total);
            if (read == 0) break;
            total += read;
        }
        return output;
    }

    private static void UnpackPrediction8(byte[] plane, int width, int height)
    {
        for (int y = 0; y < height; y++)
        {
            int rowStart = y * width;
            for (int x = 1; x < width; x++)
                plane[rowStart + x] = (byte)(plane[rowStart + x] + plane[rowStart + x - 1]);
        }
    }

    private static void UnpackPrediction16(byte[] plane, int width, int height)
    {
        for (int y = 0; y < height; y++)
        {
            int rowStart = y * width * 2;
            for (int x = 1; x < width; x++)
            {
                int offset = rowStart + x * 2;
                ushort prev = BinaryPrimitives.ReadUInt16BigEndian(plane.AsSpan(offset - 2));
                ushort cur = BinaryPrimitives.ReadUInt16BigEndian(plane.AsSpan(offset));
                BinaryPrimitives.WriteUInt16BigEndian(plane.AsSpan(offset), (ushort)(prev + cur));
            }
        }
    }

    private static ImageMetadata BuildMetadata(byte[] bytes, int start, int len)
    {
        if (len <= 0) return ImageMetadata.Empty;
        int end = Math.Min(start + len, bytes.Length);
        var tags = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        string? title = null, author = null, description = null, copyright = null, software = null;
        DateTimeOffset? captured = null;
        string? capturedRaw = null;

        int cursor = start;
        while (cursor + 12 <= end)
        {
            if (bytes[cursor] != (byte)'8' || bytes[cursor + 1] != (byte)'B'
                || bytes[cursor + 2] != (byte)'I' || bytes[cursor + 3] != (byte)'M')
                break;
            ushort id = BinaryPrimitives.ReadUInt16BigEndian(bytes.AsSpan(cursor + 4));
            int nameStart = cursor + 6;
            int nameLen = bytes[nameStart];
            int nameTotal = 1 + nameLen;
            if ((nameTotal & 1) == 1) nameTotal++; // pad to even
            int dataStart = nameStart + nameTotal;
            if (dataStart + 4 > end) break;
            uint dataLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart));
            int data = dataStart + 4;
            if (data + dataLen > end) break;

            switch (id)
            {
                case 1028: // IPTC NAA
                    IngestIptc(bytes.AsSpan(data, (int)dataLen), tags, ref title, ref author, ref description, ref copyright, ref capturedRaw, ref captured);
                    break;
                case 1060: // XMP
                    tags["XMP"] = Encoding.UTF8.GetString(bytes, data, (int)dataLen);
                    break;
                case 1058: // EXIF
                    tags["EXIF"] = "(present, " + dataLen + " bytes)";
                    break;
                case 1039: // ICC profile
                    tags["ICCProfile"] = "(present, " + dataLen + " bytes)";
                    break;
                case 1036: // Thumbnail
                    tags["Thumbnail"] = "(present, " + dataLen + " bytes)";
                    break;
                case 1037: // Global lighting angle (caption)
                    break;
                case 1057: // Version info
                    if (dataLen >= 6)
                    {
                        uint v = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(data));
                        software = "Adobe Photoshop (writer version " + v + ")";
                    }
                    break;
            }

            int padded = (int)dataLen;
            if ((padded & 1) == 1) padded++;
            cursor = data + padded;
        }

        return new ImageMetadata
        {
            Title = title,
            Author = author,
            Description = description,
            Copyright = copyright,
            Software = software,
            CapturedAt = captured,
            CapturedAtRaw = capturedRaw,
            Tags = tags.ToFrozenDictionary(StringComparer.OrdinalIgnoreCase),
        };
    }

    private static void IngestIptc(ReadOnlySpan<byte> data, Dictionary<string, string> tags,
                                    ref string? title, ref string? author, ref string? description,
                                    ref string? copyright, ref string? capturedRaw, ref DateTimeOffset? captured)
    {
        int p = 0;
        while (p + 5 <= data.Length)
        {
            if (data[p] != 0x1C) { p++; continue; }
            byte recordNumber = data[p + 1];
            byte dataset = data[p + 2];
            int dlen = (data[p + 3] << 8) | data[p + 4];
            int payload = p + 5;
            if (payload + dlen > data.Length) break;
            var v = Encoding.UTF8.GetString(data[payload..(payload + dlen)]);
            if (recordNumber == 2)
            {
                switch (dataset)
                {
                    case 5: title = v; tags["IPTC:ObjectName"] = v; break;
                    case 25: tags["IPTC:Keywords"] = string.IsNullOrEmpty(tags.GetValueOrDefault("IPTC:Keywords")) ? v : tags["IPTC:Keywords"] + ", " + v; break;
                    case 55: capturedRaw = v; tags["IPTC:DateCreated"] = v; break;
                    case 80: author = v; tags["IPTC:By-line"] = v; break;
                    case 116: copyright = v; tags["IPTC:CopyrightNotice"] = v; break;
                    case 120: description = v; tags["IPTC:Caption"] = v; break;
                    default: tags["IPTC:" + dataset] = v; break;
                }
                if (dataset == 55 && DateTime.TryParse(v, out var dt)) captured = new DateTimeOffset(dt);
            }
            p = payload + dlen;
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Min(int a, int b) => a < b ? a : b;
}

/// <summary>Colour mode declared in a PSD/PSB header.</summary>
public enum PsdColorMode : ushort
{
    /// <summary>1-bit black-and-white.</summary>
    Bitmap = 0,
    /// <summary>Grayscale (1, 8, 16 bpp).</summary>
    Grayscale = 1,
    /// <summary>Indexed colour with a palette stored in colour-mode data.</summary>
    Indexed = 2,
    /// <summary>RGB / RGBA.</summary>
    Rgb = 3,
    /// <summary>Subtractive CMYK.</summary>
    Cmyk = 4,
    /// <summary>Multichannel (multiple spot channels, no composite).</summary>
    Multichannel = 7,
    /// <summary>Duotone (grayscale + ink table).</summary>
    Duotone = 8,
    /// <summary>L*a*b* colour.</summary>
    Lab = 9,
}
