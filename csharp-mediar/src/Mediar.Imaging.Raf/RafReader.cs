using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Jpeg;
using Mediar.Imaging.Tiff;

namespace Mediar.Imaging.Raf;

/// <summary>
/// Reader for Fujifilm RAW (RAF) files. RAF is a proprietary big-endian
/// container that wraps an EXIF/JFIF JPEG preview and a TIFF-encapsulated
/// CFA (Color Filter Array) raw sensor payload. Mediar.Imaging.Raf parses
/// the container header and composes <see cref="JpegReader"/> for the
/// embedded preview, the only sub-image whose codec is freely decodable
/// across the entire Fujifilm body lineup.
/// </summary>
/// <remarks>
/// <para>
/// Layout (offsets are byte-exact and the multi-byte fields are big-endian
/// per libopenraw):
/// </para>
/// <list type="table">
///   <listheader><term>Offset</term><description>Field</description></listheader>
///   <item><term>0x00</term><description>15-byte ASCII magic "FUJIFILMCCD-RAW" + 0x00 pad</description></item>
///   <item><term>0x10</term><description>4-byte ASCII format version (e.g. "0201")</description></item>
///   <item><term>0x14</term><description>8 bytes reserved/identifier</description></item>
///   <item><term>0x1C</term><description>32-byte null-terminated ASCII camera model string</description></item>
///   <item><term>0x3C</term><description>4-byte ASCII directory version</description></item>
///   <item><term>0x40</term><description>20 bytes unknown</description></item>
///   <item><term>0x54</term><description>uint32 JPEG offset</description></item>
///   <item><term>0x58</term><description>uint32 JPEG length</description></item>
///   <item><term>0x5C</term><description>uint32 Meta-container offset</description></item>
///   <item><term>0x60</term><description>uint32 Meta-container length</description></item>
///   <item><term>0x64</term><description>uint32 CFA TIFF offset</description></item>
///   <item><term>0x68</term><description>uint32 CFA TIFF length</description></item>
/// </list>
/// <para>
/// The CFA payload is itself a TIFF container, so the reader inspects its
/// IFD 0 (when present) to surface the raw sensor width/height. Modern
/// X-series bodies (X-Pro2, X-T3, X-T4, X-H2, X-T5) ship lossless or lossy
/// Fujifilm-proprietary compression in the CFA strip; the bitstream
/// requires a dedicated decoder and is reported as
/// <c>CanDecodePixels = false</c>.
/// </para>
/// </remarks>
public sealed class RafReader : IImageReader
{
    private const int HeaderSize = 0x6C;
    private static readonly byte[] s_magic = "FUJIFILMCCD-RAW"u8.ToArray();

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Raf;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The Fujifilm-specific metadata parsed from the RAF header.</summary>
    public RafMetadata Raf { get; }

    /// <summary>All sub-images discovered in this RAF file (JPEG preview + CFA).</summary>
    public IReadOnlyList<RafSubImageInfo> SubImages { get; }

    private RafReader(Stream s, bool ownsStream, byte[] bytes,
                      ImageInfo info, ImageMetadata meta, RafMetadata raf,
                      IReadOnlyList<RafSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream; _bytes = bytes;
        Info = info; Metadata = meta; Raf = raf;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open a RAF file by path.</summary>
    public static RafReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a RAF from a stream (the contents are buffered into memory).</summary>
    public static RafReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < HeaderSize)
        {
            throw new ImageFormatException("Truncated RAF (header < 108 bytes).");
        }

        for (int i = 0; i < s_magic.Length; i++)
        {
            if (bytes[i] != s_magic[i])
            {
                throw new ImageFormatException("Not a RAF file (missing FUJIFILMCCD-RAW magic).");
            }
        }

        string formatVersion = ReadAsciiFixed(bytes, 0x10, 4);
        string cameraModel = ReadAsciiZeroTerminated(bytes, 0x1C, 32);
        string directoryVersion = ReadAsciiFixed(bytes, 0x3C, 4);

        uint jpegOffset = ReadU32Be(bytes, 0x54);
        uint jpegLength = ReadU32Be(bytes, 0x58);
        uint metaOffset = ReadU32Be(bytes, 0x5C);
        uint metaLength = ReadU32Be(bytes, 0x60);
        uint cfaOffset = ReadU32Be(bytes, 0x64);
        uint cfaLength = ReadU32Be(bytes, 0x68);

        ValidateSlice(bytes.Length, jpegOffset, jpegLength, "JPEG preview");
        if (metaLength != 0) ValidateSlice(bytes.Length, metaOffset, metaLength, "Meta container");
        if (cfaLength != 0) ValidateSlice(bytes.Length, cfaOffset, cfaLength, "CFA");

        var raf = new RafMetadata
        {
            FormatVersion = formatVersion,
            CameraModel = cameraModel,
            DirectoryVersion = directoryVersion,
            JpegOffset = jpegOffset,
            JpegLength = jpegLength,
            MetaOffset = metaOffset,
            MetaLength = metaLength,
            CfaOffset = cfaOffset,
            CfaLength = cfaLength,
        };

        var (jpegW, jpegH) = ProbeJpegDimensions(bytes, (int)jpegOffset, (int)jpegLength);
        var (cfaW, cfaH, cfaPf) = ProbeCfaDimensions(bytes, (int)cfaOffset, (int)cfaLength);

        var subs = new List<RafSubImageInfo>
        {
            new()
            {
                Kind = RafSubImageKind.JpegPreview,
                Width = jpegW,
                Height = jpegH,
                Offset = jpegOffset,
                Length = jpegLength,
                PixelFormat = PixelFormat.Rgb24,
                CanDecodePixels = true,
            },
        };
        if (cfaLength != 0)
        {
            subs.Add(new RafSubImageInfo
            {
                Kind = RafSubImageKind.Cfa,
                Width = cfaW,
                Height = cfaH,
                Offset = cfaOffset,
                Length = cfaLength,
                PixelFormat = cfaPf,
                CanDecodePixels = false,
            });
        }

        var info = new ImageInfo
        {
            Width = jpegW,
            Height = jpegH,
            BitsPerPixel = 24,
            ChannelCount = 3,
            PixelFormat = PixelFormat.Rgb24,
            Format = ImageFormat.Raf,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(raf);
        return new RafReader(stream, ownsStream, bytes, info, meta, raf, subs, canDecode: true);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        var primary = SubImages[0];
        using var ms = new MemoryStream(_bytes, (int)primary.Offset, (int)primary.Length, writable: false);
        using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);

        bool yielded = false;
        await foreach (var frame in jpeg.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            yielded = true;
            yield return frame;
            yield break;
        }
        if (!yielded)
        {
            throw new ImageFormatException("RAF embedded JPEG preview produced no frames.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static (int Width, int Height) ProbeJpegDimensions(byte[] bytes, int offset, int length)
    {
        try
        {
            using var ms = new MemoryStream(bytes, offset, length, writable: false);
            using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
            return (jpeg.Info.Width, jpeg.Info.Height);
        }
        catch (ImageFormatException ex)
        {
            throw new ImageFormatException("RAF embedded JPEG preview is malformed: " + ex.Message, ex);
        }
    }

    private static (int Width, int Height, PixelFormat Pf) ProbeCfaDimensions(byte[] bytes, int offset, int length)
    {
        if (length < 8) return (0, 0, PixelFormat.Unknown);
        try
        {
            using var ms = new MemoryStream(bytes, offset, length, writable: false);
            using var tiff = TiffReader.Open(ms, ownsStream: false);
            return (tiff.Info.Width, tiff.Info.Height, tiff.Info.PixelFormat);
        }
        catch (ImageFormatException)
        {
            return (0, 0, PixelFormat.Unknown);
        }
    }

    private static ImageMetadata BuildImageMetadata(RafMetadata raf)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["RAF:FormatVersion"] = raf.FormatVersion,
            ["RAF:DirectoryVersion"] = raf.DirectoryVersion,
        };

        return new ImageMetadata
        {
            CameraMake = "FUJIFILM",
            CameraModel = string.IsNullOrEmpty(raf.CameraModel) ? null : raf.CameraModel,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static void ValidateSlice(int totalLength, uint offset, uint length, string name)
    {
        if (offset == 0)
        {
            throw new ImageFormatException("RAF " + name + " offset is zero.");
        }
        long end = (long)offset + length;
        if (end > totalLength)
        {
            throw new ImageFormatException(
                "RAF " + name + " slice [" + offset + "+" + length + "] exceeds file size " + totalLength + ".");
        }
    }

    private static string ReadAsciiFixed(byte[] b, int offset, int length)
    {
        if (offset + length > b.Length) return string.Empty;
        return Encoding.ASCII.GetString(b, offset, length);
    }

    private static string ReadAsciiZeroTerminated(byte[] b, int offset, int maxLength)
    {
        if (offset + maxLength > b.Length) return string.Empty;
        int n = 0;
        while (n < maxLength && b[offset + n] != 0) n++;
        return Encoding.ASCII.GetString(b, offset, n);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32Be(byte[] b, int o) =>
        BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));
}
