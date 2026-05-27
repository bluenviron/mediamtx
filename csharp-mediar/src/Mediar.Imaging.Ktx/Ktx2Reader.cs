using System.Buffers.Binary;
using System.Collections.Frozen;
using System.IO.Compression;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Ktx;

/// <summary>
/// Reader for Khronos Texture v2 (.ktx2) files. KTX 2.x is the modern GPU
/// texture container used by glTF, OpenXR, and Khronos Basis Universal
/// pipelines. It prefixes a 12-byte identifier (<c>«KTX 20»\r\n\x1A\n</c>)
/// to a 68-byte fixed header (Vulkan VkFormat enum), an index pointing at
/// the DFD / KVD / SGD sections, a per-mip level index with absolute byte
/// offsets, then the mip payload data ordered smallest-to-largest. The
/// reader composes <see cref="BcnDecoder"/> / <see cref="Bc6hDecoder"/> /
/// <see cref="Bc7Decoder"/> to decode BC1-BC7 surfaces and natively unpacks
/// VK_FORMAT_R8_UNORM / R8G8B8(A)_UNORM / B8G8R8A8_UNORM layouts. Basis
/// Universal supercompression (scheme 1) and Zstd / ZLIB supercompression
/// (schemes 2 / 3) are surfaced as undecodable.
/// </summary>
public sealed class Ktx2Reader : IImageReader
{
    private const int FixedHeaderSize = 12 + 17 * 4; // 12-byte id + 7 u32 dims + 1 u32 scheme + index (4 u32 + 2 u64)
    private const int LevelIndexEntrySize = 24; // 3 × u64
    private const int MaxLevels = 64;
    private const int MaxFaces = 6;
    private const int MaxArrayLayers = 4096;

    private static readonly byte[] s_identifier =
    {
        0xAB, 0x4B, 0x54, 0x58, 0x20, 0x32, 0x30, 0xBB, 0x0D, 0x0A, 0x1A, 0x0A,
    };

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly byte[]?[]? _levelBuffers;
    private readonly BcnFormat _bcn;
    private readonly EtcFormat _etc;
    private readonly PixelFormat _uncompressedPf;
    private readonly int _uncompressedBytesPerPixel;
    private readonly uint _supercompressionScheme;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Ktx2;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>KTX 2.x header + index + key-value metadata.</summary>
    public Ktx2Metadata Ktx2 { get; }

    /// <summary>All mip / face / array entries discovered from the level index.</summary>
    public IReadOnlyList<KtxLevelInfo> Levels { get; }

    private Ktx2Reader(Stream s, bool owns, byte[] bytes, byte[]?[]? levelBuffers,
                      BcnFormat bcn, EtcFormat etc,
                      PixelFormat uncompressedPf, int uncompressedBytesPerPixel,
                      uint supercompressionScheme,
                      ImageInfo info, ImageMetadata meta, Ktx2Metadata ktx2,
                      IReadOnlyList<KtxLevelInfo> levels, bool canDecode)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes;
        _levelBuffers = levelBuffers;
        _bcn = bcn; _etc = etc; _uncompressedPf = uncompressedPf;
        _uncompressedBytesPerPixel = uncompressedBytesPerPixel;
        _supercompressionScheme = supercompressionScheme;
        Info = info; Metadata = meta; Ktx2 = ktx2;
        Levels = levels; CanDecodePixels = canDecode;
    }

    /// <summary>Open a KTX2 file by path.</summary>
    public static Ktx2Reader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a KTX2 from a stream (the contents are buffered into memory).</summary>
    public static Ktx2Reader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < FixedHeaderSize)
        {
            throw new ImageFormatException($"Truncated KTX2 (header < {FixedHeaderSize} bytes).");
        }
        for (int i = 0; i < s_identifier.Length; i++)
        {
            if (bytes[i] != s_identifier[i])
            {
                throw new ImageFormatException("Not a KTX2 file (missing 12-byte Khronos identifier).");
            }
        }

        uint vkFormat = ReadU32(bytes, 12);
        uint typeSize = ReadU32(bytes, 16);
        uint pixelWidth = ReadU32(bytes, 20);
        uint pixelHeight = ReadU32(bytes, 24);
        uint pixelDepth = ReadU32(bytes, 28);
        uint layerCount = ReadU32(bytes, 32);
        uint faceCount = ReadU32(bytes, 36);
        uint levelCount = ReadU32(bytes, 40);
        uint supercompressionScheme = ReadU32(bytes, 44);

        uint dfdByteOffset = ReadU32(bytes, 48);
        uint dfdByteLength = ReadU32(bytes, 52);
        uint kvdByteOffset = ReadU32(bytes, 56);
        uint kvdByteLength = ReadU32(bytes, 60);
        // sgdByteOffset + sgdByteLength at 64..79 are u64 (ignored for now).
        KtxDfd? parsedDfd = DfdParser.Parse(bytes, (int)dfdByteOffset, (int)dfdByteLength);

        if (pixelWidth == 0) throw new ImageFormatException("KTX2 pixelWidth is 0 (invalid).");
        uint h = pixelHeight == 0 ? 1u : pixelHeight;
        uint d = pixelDepth == 0 ? 1u : pixelDepth;
        uint layers = layerCount == 0 ? 1u : layerCount;
        uint faces = faceCount == 0 ? 1u : faceCount;
        uint levels = levelCount == 0 ? 1u : levelCount;
        if (levels > MaxLevels || faces > MaxFaces || layers > MaxArrayLayers)
        {
            throw new ImageFormatException($"KTX2 level/face/layer counts out of bounds ({levels}/{faces}/{layers}).");
        }
        if (faces is not (1u or 6u))
        {
            throw new ImageFormatException($"KTX2 faceCount must be 1 or 6 (was {faces}).");
        }

        long levelIndexOffset = FixedHeaderSize;
        long levelIndexEnd = levelIndexOffset + (long)levels * LevelIndexEntrySize;
        if (levelIndexEnd > bytes.Length)
        {
            throw new ImageFormatException("KTX2 level index exceeds file length.");
        }

        var keyValues = ParseKeyValuePool(bytes, (int)kvdByteOffset, (int)kvdByteLength);

        BcnFormat bcn = KtxFormat.MapVkFormat(vkFormat);
        EtcFormat etc = bcn == BcnFormat.None ? KtxFormat.MapVkFormatEtc(vkFormat) : EtcFormat.None;
        // For ZLIB supercompression, we can decompress on read; for Basis/Zstd we
        // cannot. Treat bcn/etc as authoritative only when scheme is None or ZLIB.
        bool schemeOk = supercompressionScheme == 0 || supercompressionScheme == 3;
        if (!schemeOk)
        {
            bcn = BcnFormat.None;
            etc = EtcFormat.None;
        }
        PixelFormat uncompressedPf = schemeOk && bcn == BcnFormat.None && etc == EtcFormat.None
            ? KtxFormat.MapVkUncompressed(vkFormat)
            : PixelFormat.Unknown;
        int uncompressedBpp = uncompressedPf switch
        {
            PixelFormat.Gray8 => 1,
            PixelFormat.Gray16 or PixelFormat.GrayAlpha16 => 2,
            PixelFormat.Rgb24 or PixelFormat.Bgr24 => 3,
            PixelFormat.Rg32 or PixelFormat.Rgba32 or PixelFormat.Bgra32 => 4,
            PixelFormat.Rgb48 or PixelFormat.Rgb48Float => 6,
            PixelFormat.Rgba64 or PixelFormat.Rgba64Float => 8,
            PixelFormat.Gray16Float => 2,
            PixelFormat.Rg32Float => 4,
            PixelFormat.Gray32Float => 4,
            PixelFormat.Gray32UInt or PixelFormat.Gray32SInt => 4,
            PixelFormat.Rg64Float => 8,
            PixelFormat.Rg64UInt or PixelFormat.Rg64SInt => 8,
            PixelFormat.Rgb96Float => 12,
            PixelFormat.Rgb96UInt or PixelFormat.Rgb96SInt => 12,
            PixelFormat.Rgba128Float => 16,
            PixelFormat.Rgba128UInt or PixelFormat.Rgba128SInt => 16,
            _ => 0,
        };

        var levelInfos = new List<KtxLevelInfo>();
        byte[]?[]? levelBuffers = supercompressionScheme == 3 ? new byte[]?[levels] : null;
        for (int level = 0; level < levels; level++)
        {
            long entry = levelIndexOffset + level * LevelIndexEntrySize;
            ulong byteOffset = ReadU64(bytes, (int)entry);
            ulong byteLength = ReadU64(bytes, (int)entry + 8);
            ulong uncompressedLen = ReadU64(bytes, (int)entry + 16);

            if (byteLength == 0 || (long)(byteOffset + byteLength) > bytes.Length)
            {
                throw new ImageFormatException($"KTX2 level {level} payload out of bounds.");
            }

            int lw = Math.Max(1, (int)(pixelWidth >> level));
            int lh = Math.Max(1, (int)(h >> level));
            int ld = Math.Max(1, (int)(d >> level));

            long payloadLen;
            long basePayloadOffset;
            if (supercompressionScheme == 3)
            {
                if (uncompressedLen == 0)
                {
                    throw new ImageFormatException($"KTX2 ZLIB level {level} declares uncompressedLength = 0.");
                }
                var compressed = bytes.AsSpan((int)byteOffset, (int)byteLength);
                var decompressed = new byte[(int)uncompressedLen];
                using (var inMs = new MemoryStream(compressed.ToArray(), writable: false))
                using (var zls = new ZLibStream(inMs, CompressionMode.Decompress))
                {
                    int total = 0;
                    while (total < decompressed.Length)
                    {
                        int read = zls.Read(decompressed, total, decompressed.Length - total);
                        if (read <= 0) break;
                        total += read;
                    }
                    if (total != decompressed.Length)
                    {
                        throw new ImageFormatException(
                            $"KTX2 ZLIB level {level} decompressed {total} bytes, expected {decompressed.Length}.");
                    }
                }
                levelBuffers![level] = decompressed;
                payloadLen = decompressed.Length;
                basePayloadOffset = 0;
            }
            else
            {
                payloadLen = (long)byteLength;
                basePayloadOffset = (long)byteOffset;
            }

            long perFaceLen = payloadLen / Math.Max(1L, (long)layers * faces);
            long cursor = basePayloadOffset;
            for (int layer = 0; layer < layers; layer++)
            {
                for (int face = 0; face < faces; face++)
                {
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
                }
            }
        }

        bool canDecode = bcn is BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3
                         or BcnFormat.Bc4 or BcnFormat.Bc5
                         or BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16
                         or BcnFormat.Bc7
                         || KtxFormat.CanDecodeEtc(etc)
                         || uncompressedPf != PixelFormat.Unknown;

        var ktx2Meta = new Ktx2Metadata
        {
            VkFormat = vkFormat,
            TypeSize = typeSize,
            LayerCount = layerCount,
            FaceCount = faceCount,
            SupercompressionScheme = supercompressionScheme,
            Bcn = bcn,
            Etc = etc,
            KeyValues = keyValues,
            Dfd = parsedDfd,
        };

        PixelFormat infoPf = canDecode
            ? (bcn != BcnFormat.None ? KtxFormat.BcnToDecodedPixelFormat(bcn)
               : etc != EtcFormat.None ? KtxFormat.EtcToDecodedPixelFormat(etc)
               : uncompressedPf)
            : PixelFormat.Unknown;
        int infoBpp = infoPf switch
        {
            PixelFormat.Gray8 => 8,
            PixelFormat.Gray16 or PixelFormat.GrayAlpha16 or PixelFormat.Gray16Float => 16,
            PixelFormat.Rgb24 or PixelFormat.Bgr24 => 24,
            PixelFormat.Rg32 or PixelFormat.Rg32Float
                or PixelFormat.Rgba32 or PixelFormat.Bgra32 => 32,
            PixelFormat.Rgb48 or PixelFormat.Rgb48Float => 48,
            PixelFormat.Rgba64 or PixelFormat.Rgba64Float or PixelFormat.Rg64Float
                or PixelFormat.Rg64UInt or PixelFormat.Rg64SInt => 64,
            PixelFormat.Gray32Float or PixelFormat.Gray32UInt or PixelFormat.Gray32SInt => 32,
            PixelFormat.Rgb96Float or PixelFormat.Rgb96UInt or PixelFormat.Rgb96SInt => 96,
            PixelFormat.Rgba128Float or PixelFormat.Rgba128UInt or PixelFormat.Rgba128SInt => 128,
            _ => 0,
        };

        var info = new ImageInfo
        {
            Width = (int)pixelWidth,
            Height = (int)h,
            BitsPerPixel = infoBpp,
            ChannelCount = infoPf switch
            {
                PixelFormat.Gray8 or PixelFormat.Gray16 or PixelFormat.Gray16Float
                    or PixelFormat.Gray32Float or PixelFormat.Gray32UInt
                    or PixelFormat.Gray32SInt => 1,
                PixelFormat.Rg32 or PixelFormat.GrayAlpha16 or PixelFormat.Rg32Float
                    or PixelFormat.Rg64Float or PixelFormat.Rg64UInt
                    or PixelFormat.Rg64SInt => 2,
                PixelFormat.Rgb24 or PixelFormat.Bgr24 or PixelFormat.Rgb48
                    or PixelFormat.Rgb48Float or PixelFormat.Rgb96Float
                    or PixelFormat.Rgb96UInt or PixelFormat.Rgb96SInt => 3,
                PixelFormat.Rgba32 or PixelFormat.Bgra32 or PixelFormat.Rgba64
                    or PixelFormat.Rgba64Float or PixelFormat.Rgba128Float
                    or PixelFormat.Rgba128UInt or PixelFormat.Rgba128SInt => 4,
                _ => 0,
            },
            PixelFormat = infoPf,
            Format = ImageFormat.Ktx2,
            HasAlpha = infoPf is PixelFormat.Rgba32 or PixelFormat.Bgra32
                                or PixelFormat.Rgba64 or PixelFormat.Rgba64Float
                                or PixelFormat.GrayAlpha16
                                or PixelFormat.Rgba128Float,
            FrameCount = canDecode ? levelInfos.Count : 0,
            ColorSpace = DfdColorSpace.Describe(parsedDfd)
                       ?? (KtxFormat.IsSrgbVkFormat(vkFormat) ? "sRGB" : null)
                       ?? (bcn != BcnFormat.None ? $"BCn:{bcn}"
                            : etc != EtcFormat.None ? $"ETC:{etc}"
                            : supercompressionScheme != 0 && supercompressionScheme != 3
                                ? $"Supercompressed:{SchemeName(supercompressionScheme)}"
                                : "Vk"),
        };

        var meta = BuildImageMetadata(ktx2Meta);
        return new Ktx2Reader(stream, ownsStream, bytes, levelBuffers, bcn, etc, uncompressedPf,
                              uncompressedBpp, supercompressionScheme,
                              info, meta, ktx2Meta, levelInfos, canDecode);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            string reason = _supercompressionScheme != 0
                ? $"KTX2 uses supercompression scheme {SchemeName(_supercompressionScheme)} which is not implemented."
                : $"KTX2 uses VkFormat {Ktx2.VkFormat} which is not implemented for pixel decode.";
            throw new NotSupportedException(reason);
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
            ReadOnlySpan<byte> payload;
            if (_levelBuffers != null)
            {
                var buf = _levelBuffers[lv.Level]
                    ?? throw new InvalidOperationException($"KTX2 level {lv.Level} buffer missing.");
                payload = buf.AsSpan((int)lv.Offset, (int)lv.Length);
            }
            else
            {
                payload = _bytes.AsSpan((int)lv.Offset, (int)lv.Length);
            }

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
                        $"KTX2 uncompressed mip {lv.Level} payload {lv.Length} smaller than {totalBytes} bytes.");
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

    private static FrozenDictionary<string, string> ParseKeyValuePool(byte[] bytes, int offset, int length)
    {
        var dict = new Dictionary<string, string>(StringComparer.Ordinal);
        if (length <= 0 || offset <= 0 || offset + length > bytes.Length) return dict.ToFrozenDictionary(StringComparer.Ordinal);
        int cursor = offset;
        int end = offset + length;
        while (cursor + 4 <= end)
        {
            uint kvLen = ReadU32(bytes, cursor);
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
            // KTX2 KV pool aligns each entry to a 4-byte boundary after the value.
            int pad = (-cursor) & 3;
            cursor += pad;
        }
        return dict.ToFrozenDictionary(StringComparer.Ordinal);
    }

    private static ImageMetadata BuildImageMetadata(Ktx2Metadata k)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["KTX2:VkFormat"] = k.VkFormat.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["KTX2:TypeSize"] = k.TypeSize.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["KTX2:FaceCount"] = k.FaceCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["KTX2:LayerCount"] = k.LayerCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["KTX2:Supercompression"] = SchemeName(k.SupercompressionScheme),
            ["KTX2:Bcn"] = k.Bcn.ToString(),
            ["KTX2:Etc"] = k.Etc.ToString(),
        };
        foreach (var kv in k.KeyValues)
        {
            tags[$"KTX2:KV:{kv.Key}"] = kv.Value;
        }

        if (k.Dfd?.Basic is { } basic)
        {
            tags["KTX2:DFD:ColorModel"] = basic.ColorModel.ToString();
            tags["KTX2:DFD:ColorPrimaries"] = basic.ColorPrimaries.ToString();
            tags["KTX2:DFD:TransferFunction"] = basic.TransferFunction.ToString();
            tags["KTX2:DFD:Flags"] = basic.Flags.ToString();
            tags["KTX2:DFD:SampleCount"] = basic.Samples.Count.ToString(
                System.Globalization.CultureInfo.InvariantCulture);
            if (basic.BytesPlanes.Count > 0)
            {
                tags["KTX2:DFD:BytesPerTexelBlock"] = basic.BytesPlanes[0].ToString(
                    System.Globalization.CultureInfo.InvariantCulture);
            }
        }

        return new ImageMetadata { Tags = tags.ToFrozenDictionary(StringComparer.Ordinal) };
    }

    internal static string SchemeName(uint scheme) => scheme switch
    {
        0 => "None",
        1 => "BasisLZ",
        2 => "Zstd",
        3 => "ZLIB",
        _ => $"Unknown({scheme})",
    };

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] bytes, int offset)
        => BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(offset, 4));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ulong ReadU64(byte[] bytes, int offset)
        => BinaryPrimitives.ReadUInt64LittleEndian(bytes.AsSpan(offset, 8));
}
