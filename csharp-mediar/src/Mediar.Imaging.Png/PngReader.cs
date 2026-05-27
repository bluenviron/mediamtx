using System.Buffers.Binary;
using System.Collections.Frozen;
using System.IO.Compression;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.Apng;

namespace Mediar.Imaging.Png;

/// <summary>
/// Reader for PNG (incl. APNG and the legacy PNJ alias) files. Implements the
/// full chunk grammar of <see href="https://www.w3.org/TR/PNG/">W3C PNG 2nd edition</see>
/// plus the APNG <c>acTL</c> / <c>fcTL</c> / <c>fdAT</c> extension. Pixel decoding
/// covers all 5 PNG color types at depths 1/2/4/8/16 and the 5 unfiltering rules.
/// </summary>
public sealed class PngReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly PngHeader _ihdr;
    private readonly byte[]? _palette;
    private readonly byte[]? _transparency;
    private readonly bool _isApng;
    private readonly int _numFrames;
    private readonly bool _numPlaysIsLoop;
    private readonly int _numPlays;
    private readonly List<PngFrameRecord> _frames;
    private readonly ImageMetadata _metadata;
    private readonly bool _hasFirstFrameOutsideAnim;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _isApng ? ImageFormat.Apng : ImageFormat.Png;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => _metadata;

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    private PngReader(
        Stream stream, bool ownsStream,
        PngHeader ihdr, byte[]? palette, byte[]? transparency,
        bool isApng, int numFrames, int numPlays, bool numPlaysIsLoop,
        bool hasFirstFrameOutsideAnim,
        List<PngFrameRecord> frames,
        ImageMetadata metadata,
        ImageInfo info)
    {
        _stream = stream;
        _ownsStream = ownsStream;
        _ihdr = ihdr;
        _palette = palette;
        _transparency = transparency;
        _isApng = isApng;
        _numFrames = numFrames;
        _numPlays = numPlays;
        _numPlaysIsLoop = numPlaysIsLoop;
        _hasFirstFrameOutsideAnim = hasFirstFrameOutsideAnim;
        _frames = frames;
        _metadata = metadata;
        Info = info;
    }

    /// <summary>Open a PNG / APNG file by path.</summary>
    public static PngReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            return Open(fs, ownsStream: true);
        }
        catch
        {
            fs.Dispose();
            throw;
        }
    }

    /// <summary>Open a PNG / APNG from <paramref name="stream"/>.</summary>
    public static PngReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        Span<byte> sig = stackalloc byte[8];
        ReadExactly(stream, sig);
        if (!IsPngSignature(sig))
        {
            throw new ImageFormatException("Not a PNG file.");
        }

        PngHeader? ihdr = null;
        byte[]? palette = null;
        byte[]? transparency = null;
        var frames = new List<PngFrameRecord>();
        var tagBag = new Dictionary<string, string>(StringComparer.Ordinal);
        bool isApng = false;
        int numFrames = 1, numPlays = 0;
        bool numPlaysIsLoop = false;

        bool sawFcTl = false;
        bool firstFrameOutsideAnim = false;
        var pendingIdat = new MemoryStream();
        PngFrameRecord? currentFrame = null;
        bool anyIdat = false;

        Span<byte> hdr = stackalloc byte[8];
        Span<byte> crc = stackalloc byte[4];
        while (true)
        {
            int read = ReadAtMost(stream, hdr);
            if (read == 0) break;
            if (read != 8) throw new ImageFormatException("Truncated PNG chunk header.");

            uint length = BinaryPrimitives.ReadUInt32BigEndian(hdr[..4]);
            string type = Encoding.ASCII.GetString(hdr.Slice(4, 4));
            byte[] data = length == 0 ? [] : new byte[length];
            if (length > 0)
            {
                stream.ReadExactly(data);
            }
            // skip CRC
            ReadExactly(stream, crc);

            switch (type)
            {
                case "IHDR":
                    ihdr = PngHeader.Parse(data);
                    break;
                case "PLTE":
                    palette = data;
                    break;
                case "tRNS":
                    transparency = data;
                    break;
                case "acTL":
                    isApng = true;
                    numFrames = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(0, 4));
                    numPlays = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(4, 4));
                    numPlaysIsLoop = numPlays == 0;
                    break;
                case "fcTL":
                    if (currentFrame is not null)
                    {
                        currentFrame.CompressedData = pendingIdat.ToArray();
                        frames.Add(currentFrame);
                    }
                    currentFrame = new PngFrameRecord
                    {
                        SequenceNumber = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(0, 4)),
                        Width = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(4, 4)),
                        Height = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(8, 4)),
                        XOffset = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(12, 4)),
                        YOffset = (int)BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(16, 4)),
                        DelayNum = BinaryPrimitives.ReadUInt16BigEndian(data.AsSpan(20, 2)),
                        DelayDen = BinaryPrimitives.ReadUInt16BigEndian(data.AsSpan(22, 2)),
                        DisposeOp = (PngDisposeOp)data[24],
                        BlendOp = (PngBlendOp)data[25],
                    };
                    pendingIdat = new MemoryStream();
                    if (!anyIdat) sawFcTl = true;
                    break;
                case "IDAT":
                    anyIdat = true;
                    if (!sawFcTl && isApng)
                    {
                        firstFrameOutsideAnim = true;
                    }
                    if (currentFrame is null)
                    {
                        currentFrame = new PngFrameRecord
                        {
                            SequenceNumber = -1,
                            Width = ihdr!.Value.Width,
                            Height = ihdr!.Value.Height,
                            DelayNum = 0,
                            DelayDen = 100,
                        };
                    }
                    pendingIdat.Write(data, 0, data.Length);
                    break;
                case "fdAT":
                    if (currentFrame is null)
                    {
                        throw new ImageFormatException("APNG fdAT before fcTL.");
                    }
                    pendingIdat.Write(data, 4, data.Length - 4);
                    break;
                case "tEXt":
                    DecodeTextChunk(data, tagBag);
                    break;
                case "zTXt":
                    DecodeCompressedTextChunk(data, tagBag);
                    break;
                case "iTXt":
                    DecodeInternationalTextChunk(data, tagBag);
                    break;
                case "pHYs":
                    if (data.Length >= 9)
                    {
                        uint ppuX = BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(0, 4));
                        uint ppuY = BinaryPrimitives.ReadUInt32BigEndian(data.AsSpan(4, 4));
                        if (data[8] == 1)
                        {
                            tagBag["pHYs:XDpi"] = (ppuX * 0.0254).ToString("R", System.Globalization.CultureInfo.InvariantCulture);
                            tagBag["pHYs:YDpi"] = (ppuY * 0.0254).ToString("R", System.Globalization.CultureInfo.InvariantCulture);
                        }
                    }
                    break;
                case "IEND":
                    if (currentFrame is not null)
                    {
                        currentFrame.CompressedData = pendingIdat.ToArray();
                        frames.Add(currentFrame);
                        currentFrame = null;
                    }
                    goto done;
                default:
                    // unknown / private chunk — ignore.
                    break;
            }
        }

    done:

        if (ihdr is null) throw new ImageFormatException("PNG IHDR missing.");
        if (frames.Count == 0) throw new ImageFormatException("PNG IDAT missing.");

        var info = new ImageInfo
        {
            Width = ihdr.Value.Width,
            Height = ihdr.Value.Height,
            BitsPerPixel = ihdr.Value.BitsPerPixel,
            ChannelCount = ihdr.Value.ChannelCount,
            PixelFormat = ihdr.Value.PixelFormat,
            Format = isApng ? ImageFormat.Apng : ImageFormat.Png,
            HasAlpha = ihdr.Value.HasAlpha,
            IsAnimated = isApng && numFrames > 1,
            FrameCount = isApng ? numFrames : 1,
            HorizontalDpi = ParseDouble(tagBag, "pHYs:XDpi"),
            VerticalDpi = ParseDouble(tagBag, "pHYs:YDpi"),
        };

        var meta = BuildMetadata(tagBag);

        return new PngReader(stream, ownsStream, ihdr.Value, palette, transparency,
                             isApng, numFrames, numPlays, numPlaysIsLoop,
                             firstFrameOutsideAnim, frames, meta, info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        foreach (var rec in _frames)
        {
            cancellationToken.ThrowIfCancellationRequested();
            yield return DecodeFrame(rec);
        }
        await Task.CompletedTask.ConfigureAwait(false);
    }

    /// <summary>
    /// Decodes each APNG frame and composites it onto a running canvas per the
    /// APNG spec's <c>dispose_op</c> / <c>blend_op</c> rules. Each yielded
    /// <see cref="ImageFrame"/> is the full canvas in <see cref="PixelFormat.Rgba32"/>
    /// at the IHDR dimensions, with <see cref="ImageFrame.Duration"/> set from
    /// the frame's fcTL. For non-APNG (still) PNGs this yields exactly one frame
    /// equal to the decoded image.
    /// </summary>
    /// <remarks>
    /// If the PNG has a default image (an <c>IDAT</c> that appears before the
    /// first <c>fcTL</c>), the default image is skipped and only the
    /// animation-sequence frames are composited and yielded.
    /// </remarks>
    public async IAsyncEnumerable<ImageFrame> ReadComposedFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!_isApng)
        {
            foreach (var rec in _frames)
            {
                cancellationToken.ThrowIfCancellationRequested();
                yield return DecodeFrame(rec);
            }
            await Task.CompletedTask.ConfigureAwait(false);
            yield break;
        }

        var compositor = new ApngCompositor(_ihdr.Width, _ihdr.Height);
        int startIndex = _hasFirstFrameOutsideAnim ? 1 : 0;
        for (int i = startIndex; i < _frames.Count; i++)
        {
            cancellationToken.ThrowIfCancellationRequested();
            var rec = _frames[i];
            using var subFrame = DecodeFrame(rec);
            byte[] rgba = ConvertFrameToRgba32(subFrame);
            int srcStride = rec.Width * 4;
            compositor.Render(
                rgba, srcStride,
                rec.Width, rec.Height,
                rec.XOffset, rec.YOffset,
                (ApngBlendOp)rec.BlendOp,
                (ApngDisposeOp)rec.DisposeOp);

            byte[] canvasCopy = compositor.Snapshot();
            int outStride = compositor.Stride;
            var composed = new ImageFrame(
                _ihdr.Width, _ihdr.Height, PixelFormat.Rgba32, outStride,
                canvasCopy)
            {
                Duration = (rec.DelayDen > 0)
                    ? TimeSpan.FromSeconds(rec.DelayNum / (double)rec.DelayDen)
                    : TimeSpan.Zero,
                OffsetX = 0,
                OffsetY = 0,
            };
            yield return composed;
        }
        await Task.CompletedTask.ConfigureAwait(false);
    }

    private byte[] ConvertFrameToRgba32(ImageFrame frame)
    {
        int w = frame.Width;
        int h = frame.Height;
        var dst = new byte[w * h * 4];
        var src = frame.Pixels.Span;
        int srcStride = frame.Stride;
        switch (frame.PixelFormat)
        {
            case PixelFormat.Rgba32:
                for (int y = 0; y < h; y++)
                {
                    src.Slice(y * srcStride, w * 4).CopyTo(dst.AsSpan(y * w * 4, w * 4));
                }
                break;
            case PixelFormat.Rgb24:
                for (int y = 0; y < h; y++)
                {
                    int srow = y * srcStride;
                    int drow = y * w * 4;
                    for (int x = 0; x < w; x++)
                    {
                        dst[drow + x * 4 + 0] = src[srow + x * 3 + 0];
                        dst[drow + x * 4 + 1] = src[srow + x * 3 + 1];
                        dst[drow + x * 4 + 2] = src[srow + x * 3 + 2];
                        dst[drow + x * 4 + 3] = 0xFF;
                    }
                }
                break;
            case PixelFormat.Gray8:
                for (int y = 0; y < h; y++)
                {
                    int srow = y * srcStride;
                    int drow = y * w * 4;
                    for (int x = 0; x < w; x++)
                    {
                        byte v = src[srow + x];
                        dst[drow + x * 4 + 0] = v;
                        dst[drow + x * 4 + 1] = v;
                        dst[drow + x * 4 + 2] = v;
                        dst[drow + x * 4 + 3] = 0xFF;
                    }
                }
                break;
            case PixelFormat.GrayAlpha16:
                for (int y = 0; y < h; y++)
                {
                    int srow = y * srcStride;
                    int drow = y * w * 4;
                    for (int x = 0; x < w; x++)
                    {
                        byte v = src[srow + x * 2 + 0];
                        byte a = src[srow + x * 2 + 1];
                        dst[drow + x * 4 + 0] = v;
                        dst[drow + x * 4 + 1] = v;
                        dst[drow + x * 4 + 2] = v;
                        dst[drow + x * 4 + 3] = a;
                    }
                }
                break;
            case PixelFormat.Indexed8:
                {
                    var palette = frame.Palette.Span;
                    for (int y = 0; y < h; y++)
                    {
                        int srow = y * srcStride;
                        int drow = y * w * 4;
                        for (int x = 0; x < w; x++)
                        {
                            int idx = src[srow + x];
                            uint argb = (idx < palette.Length) ? palette[idx] : 0u;
                            byte a = (byte)((argb >> 24) & 0xFF);
                            byte r = (byte)((argb >> 16) & 0xFF);
                            byte g = (byte)((argb >> 8) & 0xFF);
                            byte b = (byte)(argb & 0xFF);
                            dst[drow + x * 4 + 0] = r;
                            dst[drow + x * 4 + 1] = g;
                            dst[drow + x * 4 + 2] = b;
                            dst[drow + x * 4 + 3] = a;
                        }
                    }
                    break;
                }
            default:
                throw new NotSupportedException(
                    $"APNG composition for PixelFormat {frame.PixelFormat} not implemented.");
        }
        return dst;
    }

    private ImageFrame DecodeFrame(PngFrameRecord rec)
    {
        int width = rec.Width;
        int height = rec.Height;
        int bpp = _ihdr.BitsPerPixel;
        int bytesPerPixel = Math.Max(1, bpp / 8);
        int rowBytes = (width * bpp + 7) / 8;
        int outStride = width * (_ihdr.OutputBytesPerPixel);
        var (frame, dst) = ImageFrame.Rent(width, height, _ihdr.PixelFormat, outStride, MakePalette());

        byte[] raw = Decompress(rec.CompressedData);
        if (raw.Length < height * (rowBytes + 1))
        {
            throw new ImageFormatException("PNG payload truncated.");
        }
        byte[] prevRow = new byte[rowBytes];
        byte[] currRow = new byte[rowBytes];

        int srcOffset = 0;
        for (int y = 0; y < height; y++)
        {
            byte filter = raw[srcOffset++];
            Buffer.BlockCopy(raw, srcOffset, currRow, 0, rowBytes);
            srcOffset += rowBytes;
            UnfilterRow(filter, currRow, prevRow, bytesPerPixel);
            ExpandRowToOutput(currRow, dst.AsSpan(y * outStride, outStride), width);
            (prevRow, currRow) = (currRow, prevRow);
        }
        if (rec.DelayDen > 0 && _isApng)
        {
            double seconds = rec.DelayNum / (double)rec.DelayDen;
            return new ImageFrame(width, height, _ihdr.PixelFormat, outStride, dst, dst, MakePalette())
            {
                Duration = TimeSpan.FromSeconds(seconds),
                OffsetX = rec.XOffset,
                OffsetY = rec.YOffset,
            };
        }
        return frame;
    }

    private ReadOnlyMemory<uint> MakePalette()
    {
        if (_palette is null || _ihdr.ColorType != PngColorType.Indexed) return ReadOnlyMemory<uint>.Empty;
        int entries = _palette.Length / 3;
        var pal = new uint[entries];
        for (int i = 0; i < entries; i++)
        {
            byte r = _palette[i * 3 + 0];
            byte g = _palette[i * 3 + 1];
            byte b = _palette[i * 3 + 2];
            byte a = (_transparency is { } t && i < t.Length) ? t[i] : (byte)0xFF;
            pal[i] = ((uint)a << 24) | ((uint)r << 16) | ((uint)g << 8) | b;
        }
        return pal;
    }

    private static byte[] Decompress(byte[] zlib)
    {
        using var dst = new MemoryStream();
        using var src = new MemoryStream(zlib);
        using var zs = new ZLibStream(src, CompressionMode.Decompress);
        zs.CopyTo(dst);
        return dst.ToArray();
    }

    private static void UnfilterRow(byte filter, Span<byte> row, ReadOnlySpan<byte> prev, int bpp)
    {
        switch (filter)
        {
            case 0:
                break;
            case 1:
                for (int i = bpp; i < row.Length; i++) row[i] = (byte)(row[i] + row[i - bpp]);
                break;
            case 2:
                for (int i = 0; i < row.Length; i++) row[i] = (byte)(row[i] + prev[i]);
                break;
            case 3:
                for (int i = 0; i < row.Length; i++)
                {
                    int left = i >= bpp ? row[i - bpp] : 0;
                    int up = prev[i];
                    row[i] = (byte)(row[i] + ((left + up) / 2));
                }
                break;
            case 4:
                for (int i = 0; i < row.Length; i++)
                {
                    int a = i >= bpp ? row[i - bpp] : 0;
                    int b = prev[i];
                    int c = i >= bpp ? prev[i - bpp] : 0;
                    row[i] = (byte)(row[i] + Paeth(a, b, c));
                }
                break;
            default:
                throw new ImageFormatException($"Unknown PNG filter byte {filter}.");
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int Paeth(int a, int b, int c)
    {
        int p = a + b - c;
        int pa = Math.Abs(p - a);
        int pb = Math.Abs(p - b);
        int pc = Math.Abs(p - c);
        if (pa <= pb && pa <= pc) return a;
        if (pb <= pc) return b;
        return c;
    }

    private void ExpandRowToOutput(ReadOnlySpan<byte> row, Span<byte> dst, int width)
    {
        switch (_ihdr.ColorType)
        {
            case PngColorType.Grayscale:
                if (_ihdr.BitDepth == 8) row[..width].CopyTo(dst);
                else if (_ihdr.BitDepth == 16)
                {
                    for (int x = 0; x < width; x++)
                    {
                        dst[x * 2 + 0] = row[x * 2 + 1];
                        dst[x * 2 + 1] = row[x * 2 + 0];
                    }
                }
                else if (_ihdr.BitDepth < 8)
                {
                    int bd = _ihdr.BitDepth;
                    int max = (1 << bd) - 1;
                    int scale = 255 / max;
                    for (int x = 0; x < width; x++)
                    {
                        int bitPos = x * bd;
                        int byteIdx = bitPos / 8;
                        int bitOffset = 8 - bd - (bitPos & 7);
                        int v = (row[byteIdx] >> bitOffset) & max;
                        dst[x] = (byte)(v * scale);
                    }
                }
                break;
            case PngColorType.Rgb:
                row[..(width * (_ihdr.BitDepth == 8 ? 3 : 6))].CopyTo(dst);
                break;
            case PngColorType.Indexed:
                if (_ihdr.BitDepth == 8) row[..width].CopyTo(dst);
                else
                {
                    int bd = _ihdr.BitDepth;
                    int max = (1 << bd) - 1;
                    for (int x = 0; x < width; x++)
                    {
                        int bitPos = x * bd;
                        int byteIdx = bitPos / 8;
                        int bitOffset = 8 - bd - (bitPos & 7);
                        int v = (row[byteIdx] >> bitOffset) & max;
                        dst[x] = (byte)v;
                    }
                }
                break;
            case PngColorType.GrayscaleAlpha:
                if (_ihdr.BitDepth == 8) row[..(width * 2)].CopyTo(dst);
                else
                {
                    for (int x = 0; x < width; x++)
                    {
                        dst[x * 4 + 0] = row[x * 4 + 1];
                        dst[x * 4 + 1] = row[x * 4 + 0];
                        dst[x * 4 + 2] = row[x * 4 + 3];
                        dst[x * 4 + 3] = row[x * 4 + 2];
                    }
                }
                break;
            case PngColorType.Rgba:
                if (_ihdr.BitDepth == 8) row[..(width * 4)].CopyTo(dst);
                else
                {
                    for (int x = 0; x < width * 4; x++)
                    {
                        dst[x * 2 + 0] = row[x * 2 + 1];
                        dst[x * 2 + 1] = row[x * 2 + 0];
                    }
                }
                break;
        }
    }

    private static void DecodeTextChunk(byte[] data, Dictionary<string, string> tags)
    {
        int sep = Array.IndexOf(data, (byte)0);
        if (sep <= 0) return;
        string key = Encoding.GetEncoding(28591).GetString(data, 0, sep);
        string val = Encoding.GetEncoding(28591).GetString(data, sep + 1, data.Length - sep - 1);
        tags[key] = val;
    }

    private static void DecodeCompressedTextChunk(byte[] data, Dictionary<string, string> tags)
    {
        int sep = Array.IndexOf(data, (byte)0);
        if (sep <= 0) return;
        string key = Encoding.GetEncoding(28591).GetString(data, 0, sep);
        // data[sep+1] is the compression method (must be 0 = zlib).
        if (data.Length <= sep + 2) return;
        using var ms = new MemoryStream(data, sep + 2, data.Length - sep - 2);
        using var zs = new ZLibStream(ms, CompressionMode.Decompress);
        using var sr = new StreamReader(zs, Encoding.GetEncoding(28591));
        tags[key] = sr.ReadToEnd();
    }

    private static void DecodeInternationalTextChunk(byte[] data, Dictionary<string, string> tags)
    {
        int sep = Array.IndexOf(data, (byte)0);
        if (sep <= 0) return;
        string key = Encoding.UTF8.GetString(data, 0, sep);
        if (data.Length < sep + 3) return;
        byte compressed = data[sep + 1];
        byte compMethod = data[sep + 2];
        int langStart = sep + 3;
        int langEnd = Array.IndexOf(data, (byte)0, langStart);
        if (langEnd < 0) return;
        int transStart = langEnd + 1;
        int transEnd = Array.IndexOf(data, (byte)0, transStart);
        if (transEnd < 0) return;
        int textStart = transEnd + 1;
        int textLen = data.Length - textStart;
        if (compressed == 0)
        {
            tags[key] = Encoding.UTF8.GetString(data, textStart, textLen);
        }
        else if (compressed == 1 && compMethod == 0)
        {
            using var ms = new MemoryStream(data, textStart, textLen);
            using var zs = new ZLibStream(ms, CompressionMode.Decompress);
            using var sr = new StreamReader(zs, Encoding.UTF8);
            tags[key] = sr.ReadToEnd();
        }
    }

    private static double ParseDouble(Dictionary<string, string> tags, string key)
        => tags.TryGetValue(key, out var s)
            && double.TryParse(s, System.Globalization.NumberStyles.Float,
                               System.Globalization.CultureInfo.InvariantCulture, out var d)
            ? d
            : 0;

    private static ImageMetadata BuildMetadata(Dictionary<string, string> tags)
    {
        string? title = tags.GetValueOrDefault("Title");
        string? author = tags.GetValueOrDefault("Author") ?? tags.GetValueOrDefault("Artist");
        string? desc = tags.GetValueOrDefault("Description");
        string? copyright = tags.GetValueOrDefault("Copyright");
        string? sw = tags.GetValueOrDefault("Software");
        DateTimeOffset? createdAt = null;
        if (tags.TryGetValue("Creation Time", out var ct) &&
            DateTimeOffset.TryParse(ct, out var dto))
        {
            createdAt = dto;
        }
        return new ImageMetadata
        {
            Title = title,
            Author = author,
            Description = desc,
            Copyright = copyright,
            Software = sw,
            CapturedAt = createdAt,
            CapturedAtRaw = tags.GetValueOrDefault("Creation Time"),
            Tags = tags.ToFrozenDictionary(),
        };
    }

    private static bool IsPngSignature(ReadOnlySpan<byte> sig)
        => sig.Length >= 8 &&
           sig[0] == 0x89 && sig[1] == 0x50 && sig[2] == 0x4E && sig[3] == 0x47 &&
           sig[4] == 0x0D && sig[5] == 0x0A && sig[6] == 0x1A && sig[7] == 0x0A;

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void ReadExactly(Stream s, Span<byte> dst)
    {
        int read = 0;
        while (read < dst.Length)
        {
            int n = s.Read(dst[read..]);
            if (n <= 0) throw new EndOfStreamException();
            read += n;
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ReadAtMost(Stream s, Span<byte> dst)
    {
        int read = 0;
        while (read < dst.Length)
        {
            int n = s.Read(dst[read..]);
            if (n <= 0) break;
            read += n;
        }
        return read;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}

/// <summary>APNG fcTL disposal operation.</summary>
public enum PngDisposeOp : byte
{
    /// <summary>Leave the frame untouched after rendering.</summary>
    None = 0,
    /// <summary>Clear to fully transparent black before rendering the next frame.</summary>
    Background = 1,
    /// <summary>Revert to the previous canvas state before rendering the next frame.</summary>
    Previous = 2,
}

/// <summary>APNG fcTL blending operation.</summary>
public enum PngBlendOp : byte
{
    /// <summary>Source pixels overwrite the canvas.</summary>
    Source = 0,
    /// <summary>Source pixels are alpha-blended over the canvas.</summary>
    Over = 1,
}

internal sealed class PngFrameRecord
{
    public int SequenceNumber { get; set; }
    public int Width { get; set; }
    public int Height { get; set; }
    public int XOffset { get; set; }
    public int YOffset { get; set; }
    public ushort DelayNum { get; set; }
    public ushort DelayDen { get; set; } = 100;
    public PngDisposeOp DisposeOp { get; set; }
    public PngBlendOp BlendOp { get; set; }
    public byte[] CompressedData { get; set; } = [];
}
