using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Ktx;

/// <summary>
/// Reader for Khronos Texture v1 (.ktx) files. KTX 1.x is the legacy GPU
/// texture container that prefixes a 12-byte identifier
/// (<c>«KTX 11»\r\n\x1A\n</c>) to a 64-byte fixed header of 13 little-endian
/// u32 fields, followed by an optional key-value metadata pool and the mip
/// pyramid from largest to smallest. The reader composes
/// <see cref="BcnDecoder"/> / <see cref="Bc6hDecoder"/> / <see cref="Bc7Decoder"/>
/// to decode BC1-BC7 surfaces and natively unpacks GL_R8 / GL_RGB8 /
/// GL_RGBA8 layouts; ETC/ASTC/PVRTC and most non-BCn compressed formats
/// are surfaced as undecodable.
/// </summary>
public sealed class KtxReader : IImageReader
{
    private const int HeaderSize = 12 + 13 * 4; // 12-byte identifier + 13 u32s
    private const uint EndiannessNative = 0x04030201u;
    private const uint EndiannessSwapped = 0x01020304u;
    private const int MaxLevels = 64;
    private const int MaxFaces = 6;
    private const int MaxArrayLayers = 4096;

    private static readonly byte[] s_identifier =
    {
        0xAB, 0x4B, 0x54, 0x58, 0x20, 0x31, 0x31, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A,
    };

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly BcnFormat _bcn;
    private readonly EtcFormat _etc;
    private readonly PixelFormat _uncompressedPf;
    private readonly int _uncompressedBytesPerPixel;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Ktx;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>KTX 1.x header + key-value metadata.</summary>
    public KtxMetadata Ktx { get; }

    /// <summary>All mip / face / array entries discovered by the level walk.</summary>
    public IReadOnlyList<KtxLevelInfo> Levels { get; }

    private KtxReader(Stream s, bool owns, byte[] bytes, BcnFormat bcn, EtcFormat etc,
                     PixelFormat uncompressedPf, int uncompressedBytesPerPixel,
                     ImageInfo info, ImageMetadata meta, KtxMetadata ktx,
                     IReadOnlyList<KtxLevelInfo> levels, bool canDecode)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes;
        _bcn = bcn; _etc = etc; _uncompressedPf = uncompressedPf;
        _uncompressedBytesPerPixel = uncompressedBytesPerPixel;
        Info = info; Metadata = meta; Ktx = ktx;
        Levels = levels; CanDecodePixels = canDecode;
    }

    /// <summary>Open a KTX file by path.</summary>
    public static KtxReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a KTX from a stream (the contents are buffered into memory).</summary>
    public static KtxReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < HeaderSize)
        {
            throw new ImageFormatException("Truncated KTX (header < 64 bytes).");
        }
        for (int i = 0; i < s_identifier.Length; i++)
        {
            if (bytes[i] != s_identifier[i])
            {
                throw new ImageFormatException("Not a KTX 1.x file (missing 12-byte Khronos identifier).");
            }
        }

        uint endianness = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(12, 4));
        bool le;
        if (endianness == EndiannessNative) le = true;
        else if (endianness == EndiannessSwapped) le = false;
        else throw new ImageFormatException($"KTX endianness field 0x{endianness:X8} is not 0x04030201 or 0x01020304.");

        uint glType = ReadU32(bytes, 16, le);
        uint glTypeSize = ReadU32(bytes, 20, le);
        uint glFormat = ReadU32(bytes, 24, le);
        uint glInternalFormat = ReadU32(bytes, 28, le);
        uint glBaseInternalFormat = ReadU32(bytes, 32, le);
        uint pixelWidth = ReadU32(bytes, 36, le);
        uint pixelHeight = ReadU32(bytes, 40, le);
        uint pixelDepth = ReadU32(bytes, 44, le);
        uint arrayElems = ReadU32(bytes, 48, le);
        uint faceCount = ReadU32(bytes, 52, le);
        uint mipLevels = ReadU32(bytes, 56, le);
        uint kvdBytes = ReadU32(bytes, 60, le);

        if (pixelWidth == 0) throw new ImageFormatException("KTX pixelWidth is 0 (invalid).");
        uint h = pixelHeight == 0 ? 1u : pixelHeight;
        uint d = pixelDepth == 0 ? 1u : pixelDepth;
        uint layers = arrayElems == 0 ? 1u : arrayElems;
        uint faces = faceCount == 0 ? 1u : faceCount;
        uint levels = mipLevels == 0 ? 1u : mipLevels;
        if (levels > MaxLevels || faces > MaxFaces || layers > MaxArrayLayers)
        {
            throw new ImageFormatException($"KTX level/face/layer counts out of bounds ({levels}/{faces}/{layers}).");
        }
        if (faces is not (1u or 6u))
        {
            throw new ImageFormatException($"KTX faceCount must be 1 or 6 (was {faces}).");
        }

        long kvdOffset = HeaderSize;
        long pixelsStart = kvdOffset + kvdBytes;
        if (pixelsStart > bytes.Length)
        {
            throw new ImageFormatException("KTX bytesOfKeyValueData exceeds file length.");
        }

        var keyValues = ParseKeyValuePool(bytes, (int)kvdOffset, (int)kvdBytes, le);

        BcnFormat bcn = KtxFormat.MapGlInternalFormat(glInternalFormat);
        EtcFormat etc = bcn == BcnFormat.None ? KtxFormat.MapGlInternalFormatEtc(glInternalFormat) : EtcFormat.None;
        PixelFormat uncompressedPf = bcn == BcnFormat.None && etc == EtcFormat.None
            ? KtxFormat.MapGlUncompressed(glInternalFormat)
            : PixelFormat.Unknown;
        int uncompressedBpp = uncompressedPf switch
        {
            PixelFormat.Gray8 => 1,
            PixelFormat.Gray16 => 2,
            PixelFormat.GrayAlpha16 => 2,
            PixelFormat.Rgb24 => 3,
            PixelFormat.Bgr24 => 3,
            PixelFormat.Rg32 => 4,
            PixelFormat.Rgba32 => 4,
            PixelFormat.Bgra32 => 4,
            PixelFormat.Rgb48 => 6,
            PixelFormat.Rgba64 => 8,
            _ => 0,
        };

        var levelInfos = new List<KtxLevelInfo>();
        long cursor = pixelsStart;
        for (int level = 0; level < levels; level++)
        {
            int lw = Math.Max(1, (int)(pixelWidth >> level));
            int lh = Math.Max(1, (int)(h >> level));
            int ld = Math.Max(1, (int)(d >> level));

            if (cursor + 4 > bytes.Length)
            {
                throw new ImageFormatException($"KTX truncated at mip {level} imageSize field.");
            }
            uint imageSize = ReadU32(bytes, (int)cursor, le);
            cursor += 4;

            // Per-face byte length: the imageSize for non-cubemap layouts spans
            // all faces / layers / depth slices; for cubemaps, KTX 1.x stores
            // imageSize as the byte length of ONE face and pads to 4 between
            // faces. We surface every face/layer/slice as a separate entry.
            long perFaceLen;
            if (faces == 6 && layers == 1)
            {
                perFaceLen = imageSize;
            }
            else
            {
                long total = (long)imageSize;
                perFaceLen = total / Math.Max(1L, (long)layers * faces);
            }

            for (int layer = 0; layer < layers; layer++)
            {
                for (int face = 0; face < faces; face++)
                {
                    if (cursor + perFaceLen > bytes.Length)
                    {
                        throw new ImageFormatException(
                            $"KTX mip {level} face {face} layer {layer} payload exceeds file length.");
                    }
                    levelInfos.Add(new KtxLevelInfo
                    {
                        Level = level,
                        ArrayLayer = layer,
                        Face = face,
                        Width = lw,
                        Height = lh,
                        Depth = ld,
                        Offset = cursor,
                        Length = perFaceLen,
                    });
                    cursor += perFaceLen;
                    // cube padding (0..3 bytes to next 4-byte boundary)
                    if (faces == 6)
                    {
                        long pad = (-cursor) & 3;
                        cursor += pad;
                    }
                }
            }
            // mip padding (0..3 bytes to next 4-byte boundary)
            long mipPad = (-cursor) & 3;
            cursor += mipPad;
        }

        bool canDecode = bcn is BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3
                         or BcnFormat.Bc4 or BcnFormat.Bc5
                         or BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16
                         or BcnFormat.Bc7
                         || KtxFormat.CanDecodeEtc(etc)
                         || uncompressedPf != PixelFormat.Unknown;

        var ktxMeta = new KtxMetadata
        {
            LittleEndian = le,
            GlType = glType,
            GlTypeSize = glTypeSize,
            GlFormat = glFormat,
            GlInternalFormat = glInternalFormat,
            GlBaseInternalFormat = glBaseInternalFormat,
            ArrayElementCount = arrayElems,
            FaceCount = faceCount,
            Bcn = bcn,
            Etc = etc,
            KeyValues = keyValues,
        };

        PixelFormat infoPf = canDecode
            ? (bcn != BcnFormat.None ? KtxFormat.BcnToDecodedPixelFormat(bcn)
               : etc != EtcFormat.None ? KtxFormat.EtcToDecodedPixelFormat(etc)
               : uncompressedPf)
            : PixelFormat.Unknown;
        int infoBpp = infoPf switch
        {
            PixelFormat.Gray8 => 8,
            PixelFormat.Gray16 or PixelFormat.GrayAlpha16 => 16,
            PixelFormat.Rgb24 or PixelFormat.Bgr24 => 24,
            PixelFormat.Rg32 or PixelFormat.Rgba32 or PixelFormat.Bgra32 => 32,
            PixelFormat.Rgb48 => 48,
            PixelFormat.Rgba64 => 64,
            PixelFormat.Rgb96Float => 96,
            _ => 0,
        };

        var info = new ImageInfo
        {
            Width = (int)pixelWidth,
            Height = (int)h,
            BitsPerPixel = infoBpp,
            ChannelCount = infoPf switch
            {
                PixelFormat.Gray8 or PixelFormat.Gray16 => 1,
                PixelFormat.Rg32 or PixelFormat.GrayAlpha16 => 2,
                PixelFormat.Rgb24 or PixelFormat.Bgr24 or PixelFormat.Rgb48
                    or PixelFormat.Rgb96Float => 3,
                PixelFormat.Rgba32 or PixelFormat.Bgra32 or PixelFormat.Rgba64 => 4,
                _ => 0,
            },
            PixelFormat = infoPf,
            Format = ImageFormat.Ktx,
            HasAlpha = infoPf is PixelFormat.Rgba32 or PixelFormat.Bgra32
                                or PixelFormat.Rgba64 or PixelFormat.GrayAlpha16,
            FrameCount = canDecode ? levelInfos.Count : 0,
            ColorSpace = KtxFormat.IsSrgbGlInternalFormat(glInternalFormat) ? "sRGB"
                       : bcn != BcnFormat.None ? $"BCn:{bcn}"
                       : etc != EtcFormat.None ? $"ETC:{etc}"
                       : "GL",
        };

        var meta = BuildImageMetadata(ktxMeta);
        return new KtxReader(stream, ownsStream, bytes, bcn, etc, uncompressedPf,
                             uncompressedBpp, info, meta, ktxMeta, levelInfos, canDecode);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                $"KTX uses GL internal format 0x{Ktx.GlInternalFormat:X4}; pixel decode is not implemented.");
        }
        return DecodeAsync(cancellationToken);
    }

    private async IAsyncEnumerable<ImageFrame> DecodeAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken ct)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        ct.ThrowIfCancellationRequested();
        foreach (var lv in Levels)
        {
            var payload = _bytes.AsSpan((int)lv.Offset, (int)lv.Length);
            if (_bcn != BcnFormat.None)
            {
                var (pixels, stride, pf) = KtxFormat.DecodeBcn(_bcn, payload, lv.Width, lv.Height);
                yield return new ImageFrame(lv.Width, lv.Height, pf, stride, pixels);
            }
            else if (KtxFormat.CanDecodeEtc(_etc))
            {
                var (pixels, stride, pf) = KtxFormat.DecodeEtc(_etc, payload, lv.Width, lv.Height);
                yield return new ImageFrame(lv.Width, lv.Height, pf, stride, pixels);
            }
            else
            {
                int stride = lv.Width * _uncompressedBytesPerPixel;
                int totalBytes = stride * lv.Height;
                if (lv.Length < totalBytes)
                {
                    throw new ImageFormatException(
                        $"KTX uncompressed mip {lv.Level} payload {lv.Length} smaller than {totalBytes} bytes.");
                }
                var buf = payload[..totalBytes].ToArray();
                yield return new ImageFrame(lv.Width, lv.Height, _uncompressedPf, stride, buf);
            }
        }
    }

    /// <inheritdoc/>
    public ValueTask DisposeAsync() { Dispose(); return ValueTask.CompletedTask; }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
        GC.SuppressFinalize(this);
    }

    private static FrozenDictionary<string, string> ParseKeyValuePool(byte[] bytes, int offset, int length, bool le)
    {
        var dict = new Dictionary<string, string>(StringComparer.Ordinal);
        int cursor = offset;
        int end = offset + length;
        while (cursor + 4 <= end)
        {
            uint kvLen = ReadU32(bytes, cursor, le);
            cursor += 4;
            if (cursor + kvLen > end) break;
            var slice = bytes.AsSpan(cursor, (int)kvLen);
            int nul = slice.IndexOf((byte)0);
            if (nul > 0 && nul < slice.Length - 1)
            {
                string key = Encoding.UTF8.GetString(slice[..nul]);
                var valueBytes = slice[(nul + 1)..];
                int valNul = valueBytes.IndexOf((byte)0);
                if (valNul < 0) valNul = valueBytes.Length;
                string value = Encoding.UTF8.GetString(valueBytes[..valNul]);
                dict[key] = value;
            }
            cursor += (int)kvLen;
            int pad = (-cursor) & 3;
            cursor += pad;
        }
        return dict.ToFrozenDictionary(StringComparer.Ordinal);
    }

    private static ImageMetadata BuildImageMetadata(KtxMetadata k)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["KTX:Endian"] = k.LittleEndian ? "LE" : "BE",
            ["KTX:GlType"] = $"0x{k.GlType:X4}",
            ["KTX:GlFormat"] = $"0x{k.GlFormat:X4}",
            ["KTX:GlInternalFormat"] = $"0x{k.GlInternalFormat:X4}",
            ["KTX:GlBaseInternalFormat"] = $"0x{k.GlBaseInternalFormat:X4}",
            ["KTX:FaceCount"] = k.FaceCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["KTX:ArrayElements"] = k.ArrayElementCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["KTX:Bcn"] = k.Bcn.ToString(),
            ["KTX:Etc"] = k.Etc.ToString(),
        };
        foreach (var kv in k.KeyValues)
        {
            tags[$"KTX:KV:{kv.Key}"] = kv.Value;
        }

        return new ImageMetadata { Tags = tags.ToFrozenDictionary(StringComparer.Ordinal) };
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] bytes, int offset, bool le)
    {
        var s = bytes.AsSpan(offset, 4);
        return le ? BinaryPrimitives.ReadUInt32LittleEndian(s)
                  : BinaryPrimitives.ReadUInt32BigEndian(s);
    }
}
