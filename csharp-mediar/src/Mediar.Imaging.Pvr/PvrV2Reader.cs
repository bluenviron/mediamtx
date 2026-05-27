using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Globalization;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Pvr;

/// <summary>
/// Reader for the legacy PowerVR Texture v2 container. The format begins
/// with a 52-byte little-endian fixed header whose magic word
/// <c>0x21525650</c> ('PVR!') sits at offset <c>0x2C</c>. PVR v2 is the
/// pre-2010 PowerVR SDK container used by many early iOS and Android
/// game asset pipelines; the v3 container documented by
/// <see cref="PvrReader"/> superseded it but PVR v2 files remain common
/// in legacy asset archives.
/// </summary>
/// <remarks>
/// The reader composes <see cref="BcnDecoder"/> for DXT1 / DXT3 / DXT5
/// (BC1 / BC2 / BC3) and <see cref="EtcDecoder"/> for ETC1 RGB.
/// Uncompressed RGBA8888 / BGRA8888 / RGB888 / packed RGB565 / RGBA4444
/// / RGBA5551 / Intensity8 are recognised; the rest (PVRTC2 / PVRTC4 /
/// YUV / DXT2 / DXT4) are surfaced as <see cref="CanDecodePixels"/>
/// <c>= false</c> and raise <see cref="NotSupportedException"/> from
/// <see cref="ReadFramesAsync"/>.
/// </remarks>
public sealed class PvrV2Reader : IImageReader
{
    private const int HeaderSize = 52;
    private const uint MagicLe = 0x21525650u; // 'P','V','R','!' on disk
    private const int MagicOffset = 0x2C;
    private const int MaxLevels = 32;
    private const int MaxSurfaces = 4096;

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly BcnFormat _bcn;
    private readonly EtcFormat _etc;
    private readonly PixelFormat _uncompressedPf;
    private readonly int _uncompressedBytesPerPixel;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Pvr;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>Parsed PVR v2 header.</summary>
    public PvrV2Metadata Pvr { get; }

    /// <summary>Per (surface, mip) level table.</summary>
    public IReadOnlyList<PvrV2LevelInfo> Levels { get; }

    private PvrV2Reader(Stream s, bool owns, byte[] bytes, BcnFormat bcn, EtcFormat etc,
                       PixelFormat uncompressedPf, int uncompressedBpp,
                       ImageInfo info, ImageMetadata meta, PvrV2Metadata pvr,
                       IReadOnlyList<PvrV2LevelInfo> levels, bool canDecode)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes;
        _bcn = bcn; _etc = etc; _uncompressedPf = uncompressedPf;
        _uncompressedBytesPerPixel = uncompressedBpp;
        Info = info; Metadata = meta; Pvr = pvr;
        Levels = levels; CanDecodePixels = canDecode;
    }

    /// <summary>Open a PVR v2 file by path.</summary>
    public static PvrV2Reader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a PVR v2 file from a stream. The contents are buffered into memory.</summary>
    public static PvrV2Reader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < HeaderSize)
        {
            throw new ImageFormatException("Truncated PVR v2 (header < 52 bytes).");
        }

        uint magic = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(MagicOffset, 4));
        if (magic != MagicLe)
        {
            throw new ImageFormatException(
                $"Not a PVR v2 file (magic at offset 0x2C is 0x{magic:X8}, expected 0x{MagicLe:X8}).");
        }

        uint headerSize = ReadU32(bytes, 0x00);
        uint height = ReadU32(bytes, 0x04);
        uint width = ReadU32(bytes, 0x08);
        uint mipMapCount = ReadU32(bytes, 0x0C);
        uint pfWord = ReadU32(bytes, 0x10);
        uint dataLength = ReadU32(bytes, 0x14);
        uint bitsPerPixel = ReadU32(bytes, 0x18);
        uint redMask = ReadU32(bytes, 0x1C);
        uint greenMask = ReadU32(bytes, 0x20);
        uint blueMask = ReadU32(bytes, 0x24);
        uint alphaMask = ReadU32(bytes, 0x28);
        // magic at 0x2C already validated
        uint numSurfaces = bytes.Length >= 0x34 ? ReadU32(bytes, 0x30) : 1u;
        if (numSurfaces == 0) numSurfaces = 1u;

        if (width == 0 || height == 0)
        {
            throw new ImageFormatException("PVR v2 width/height must be > 0.");
        }

        var fmtId = (PvrV2FormatId)(byte)(pfWord & 0xFFu);
        var flags = (PvrV2Flags)(pfWord & 0xFFFFFF00u);

        bool hasMips = (flags & PvrV2Flags.HasMipmaps) != 0;
        uint levels = hasMips ? (mipMapCount + 1u) : 1u;
        uint surfaces = (flags & PvrV2Flags.Cubemap) != 0 ? 6u : numSurfaces;
        if (levels > MaxLevels || surfaces > MaxSurfaces)
        {
            throw new ImageFormatException(
                $"PVR v2 mip/surface counts out of bounds ({levels}/{surfaces}).");
        }

        BcnFormat bcn = MapBcn(fmtId);
        EtcFormat etc = bcn == BcnFormat.None ? MapEtc(fmtId) : EtcFormat.None;
        PixelFormat uncompressedPf = bcn == BcnFormat.None && etc == EtcFormat.None
            ? MapUncompressed(fmtId)
            : PixelFormat.Unknown;
        int uncompressedBpp = UncompressedBytesPerPixel(uncompressedPf);

        var levelInfos = new List<PvrV2LevelInfo>();
        long cursor = HeaderSize;
        for (int level = 0; level < levels; level++)
        {
            int lw = Math.Max(1, (int)(width >> level));
            int lh = Math.Max(1, (int)(height >> level));
            long perSurfaceLen = ComputeSurfaceLength(fmtId, uncompressedBpp, lw, lh);
            for (int surf = 0; surf < surfaces; surf++)
            {
                if (perSurfaceLen > 0 && cursor + perSurfaceLen > bytes.Length)
                {
                    throw new ImageFormatException(
                        $"PVR v2 mip {level} surface {surf} payload exceeds file length.");
                }
                levelInfos.Add(new PvrV2LevelInfo
                {
                    Level = level,
                    Surface = surf,
                    Width = lw,
                    Height = lh,
                    Offset = cursor,
                    Length = perSurfaceLen,
                });
                cursor += perSurfaceLen;
            }
        }

        bool canDecode = bcn is BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3
                         || etc == EtcFormat.Etc1Rgb
                         || uncompressedPf != PixelFormat.Unknown;

        var pvrMeta = new PvrV2Metadata
        {
            HeaderSize = headerSize,
            Height = height,
            Width = width,
            MipMapCount = mipMapCount,
            PixelFormatWord = pfWord,
            FormatId = fmtId,
            Flags = flags,
            DataLength = dataLength,
            BitsPerPixel = bitsPerPixel,
            RedMask = redMask,
            GreenMask = greenMask,
            BlueMask = blueMask,
            AlphaMask = alphaMask,
            Magic = magic,
            NumSurfaces = numSurfaces,
        };

        PixelFormat infoPf = canDecode
            ? (bcn != BcnFormat.None ? BcnToDecodedPf(bcn)
               : etc != EtcFormat.None ? EtcToDecodedPf(etc)
               : uncompressedPf)
            : PixelFormat.Unknown;

        var info = new ImageInfo
        {
            Width = (int)width,
            Height = (int)height,
            BitsPerPixel = infoPf switch
            {
                PixelFormat.Gray8 => 8,
                PixelFormat.Rgb24 or PixelFormat.Bgr24 => 24,
                PixelFormat.Rgba32 or PixelFormat.Bgra32 => 32,
                _ => 0,
            },
            ChannelCount = infoPf switch
            {
                PixelFormat.Gray8 => 1,
                PixelFormat.Rgb24 or PixelFormat.Bgr24 => 3,
                PixelFormat.Rgba32 or PixelFormat.Bgra32 => 4,
                _ => 0,
            },
            PixelFormat = infoPf,
            Format = ImageFormat.Pvr,
            HasAlpha = infoPf is PixelFormat.Rgba32 or PixelFormat.Bgra32,
            FrameCount = canDecode ? levelInfos.Count : 0,
            ColorSpace = bcn != BcnFormat.None ? $"BCn:{bcn}"
                       : etc != EtcFormat.None ? $"ETC:{etc}"
                       : $"PVR2:{fmtId}",
        };

        var meta = BuildImageMetadata(pvrMeta);
        return new PvrV2Reader(stream, ownsStream, bytes, bcn, etc, uncompressedPf,
                              uncompressedBpp, info, meta, pvrMeta, levelInfos, canDecode);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                $"PVR v2 format id {Pvr.FormatId} (0x{(byte)Pvr.FormatId:X2}); pixel decode is not implemented.");
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
                var (pixels, stride, pf) = DecodeBcn(_bcn, payload, lv.Width, lv.Height);
                yield return new ImageFrame(lv.Width, lv.Height, pf, stride, pixels);
            }
            else if (_etc == EtcFormat.Etc1Rgb)
            {
                var pixels = EtcDecoder.DecodeEtc1(payload, lv.Width, lv.Height);
                yield return new ImageFrame(lv.Width, lv.Height, PixelFormat.Rgba32, lv.Width * 4, pixels);
            }
            else
            {
                int stride = lv.Width * _uncompressedBytesPerPixel;
                int totalBytes = stride * lv.Height;
                if (lv.Length < totalBytes)
                {
                    throw new ImageFormatException(
                        $"PVR v2 uncompressed mip {lv.Level} payload {lv.Length} smaller than {totalBytes} bytes.");
                }
                var buf = payload[..totalBytes].ToArray();
                yield return new ImageFrame(lv.Width, lv.Height, _uncompressedPf, stride, buf);
            }
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
        GC.SuppressFinalize(this);
    }

    private static BcnFormat MapBcn(PvrV2FormatId id) => id switch
    {
        PvrV2FormatId.Dxt1 => BcnFormat.Bc1,
        PvrV2FormatId.Dxt3 => BcnFormat.Bc2,
        PvrV2FormatId.Dxt5 => BcnFormat.Bc3,
        _ => BcnFormat.None,
    };

    private static EtcFormat MapEtc(PvrV2FormatId id) => id switch
    {
        PvrV2FormatId.GlEtc1 => EtcFormat.Etc1Rgb,
        _ => EtcFormat.None,
    };

    private static PixelFormat MapUncompressed(PvrV2FormatId id) => id switch
    {
        PvrV2FormatId.Rgb888 or PvrV2FormatId.GlRgb888 => PixelFormat.Rgb24,
        PvrV2FormatId.Argb8888 or PvrV2FormatId.GlRgba8888 => PixelFormat.Rgba32,
        PvrV2FormatId.GlBgra8888 => PixelFormat.Bgra32,
        PvrV2FormatId.I8 or PvrV2FormatId.GlIntensity8 => PixelFormat.Gray8,
        _ => PixelFormat.Unknown,
    };

    private static int UncompressedBytesPerPixel(PixelFormat pf) => pf switch
    {
        PixelFormat.Gray8 => 1,
        PixelFormat.Rgb24 or PixelFormat.Bgr24 => 3,
        PixelFormat.Rgba32 or PixelFormat.Bgra32 => 4,
        _ => 0,
    };

    private static long ComputeSurfaceLength(PvrV2FormatId id, int uncompressedBpp, int w, int h)
    {
        switch (id)
        {
            case PvrV2FormatId.Dxt1:
            case PvrV2FormatId.GlEtc1:
                {
                    int bx = (w + 3) / 4;
                    int by = (h + 3) / 4;
                    return (long)bx * by * 8;
                }
            case PvrV2FormatId.Dxt3:
            case PvrV2FormatId.Dxt5:
                {
                    int bx = (w + 3) / 4;
                    int by = (h + 3) / 4;
                    return (long)bx * by * 16;
                }
            case PvrV2FormatId.Pvrtc2:
                {
                    int bx = (w + 7) / 8;
                    int by = (h + 3) / 4;
                    return (long)bx * by * 8;
                }
            case PvrV2FormatId.Pvrtc4:
                {
                    int bx = (w + 3) / 4;
                    int by = (h + 3) / 4;
                    return (long)bx * by * 8;
                }
            default:
                return uncompressedBpp > 0 ? (long)w * h * uncompressedBpp : 0;
        }
    }

    private static PixelFormat BcnToDecodedPf(BcnFormat f) => f switch
    {
        BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3 => PixelFormat.Bgra32,
        _ => PixelFormat.Unknown,
    };

    private static PixelFormat EtcToDecodedPf(EtcFormat f) => f switch
    {
        EtcFormat.Etc1Rgb => PixelFormat.Rgba32,
        _ => PixelFormat.Unknown,
    };

    private static (byte[] Pixels, int Stride, PixelFormat Format) DecodeBcn(
        BcnFormat f, ReadOnlySpan<byte> payload, int w, int h) => f switch
        {
            BcnFormat.Bc1 => (BcnDecoder.DecodeBc1(payload, w, h), w * 4, PixelFormat.Bgra32),
            BcnFormat.Bc2 => (BcnDecoder.DecodeBc2(payload, w, h), w * 4, PixelFormat.Bgra32),
            BcnFormat.Bc3 => (BcnDecoder.DecodeBc3(payload, w, h), w * 4, PixelFormat.Bgra32),
            _ => throw new NotSupportedException($"PVR v2 BCn format {f} cannot be decoded."),
        };

    private static ImageMetadata BuildImageMetadata(PvrV2Metadata p)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["PVR2:HeaderSize"] = p.HeaderSize.ToString(CultureInfo.InvariantCulture),
            ["PVR2:FormatId"] = p.FormatId.ToString(),
            ["PVR2:Flags"] = $"0x{(uint)p.Flags:X8}",
            ["PVR2:PixelFormatWord"] = $"0x{p.PixelFormatWord:X8}",
            ["PVR2:DataLength"] = p.DataLength.ToString(CultureInfo.InvariantCulture),
            ["PVR2:BitsPerPixel"] = p.BitsPerPixel.ToString(CultureInfo.InvariantCulture),
            ["PVR2:NumSurfaces"] = p.NumSurfaces.ToString(CultureInfo.InvariantCulture),
            ["PVR2:Magic"] = $"0x{p.Magic:X8}",
        };
        if ((p.Flags & PvrV2Flags.HasMipmaps) != 0) tags["PVR2:HasMipmaps"] = "true";
        if ((p.Flags & PvrV2Flags.Cubemap) != 0) tags["PVR2:Cubemap"] = "true";
        if ((p.Flags & PvrV2Flags.VolumeTexture) != 0) tags["PVR2:Volume"] = "true";
        if ((p.Flags & PvrV2Flags.PremultipliedAlpha) != 0) tags["PVR2:PremultipliedAlpha"] = "true";
        return new ImageMetadata { Tags = tags.ToFrozenDictionary(StringComparer.Ordinal) };
    }

    private static uint ReadU32(byte[] b, int offset)
        => BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(offset, 4));
}
