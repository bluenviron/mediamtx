using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Globalization;
using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;

namespace Mediar.Imaging.Pvr;

/// <summary>
/// Reader for Imagination PowerVR Texture v3 files (.pvr). PVR v3 is the
/// modern PowerVR Texture container; the file begins with a 52-byte fixed
/// header whose first u32 is the version word
/// 0x03525650 (<c>'PVR\x03'</c> little-endian, the canonical layout) or
/// 0x50565203 (big-endian, all multi-byte fields swapped). The fixed
/// header is followed by a variable-size metadata block (FourCC-keyed
/// entries) and then the texture payload iterating in the order
/// (mip, surface, face, depth slice) with mip 0 stored first.
/// </summary>
/// <remarks>
/// The reader composes <see cref="BcnDecoder"/> / <see cref="Bc6hDecoder"/>
/// / <see cref="Bc7Decoder"/> for BC1-BC7 (DXT1/3/5 aliases) and
/// <see cref="EtcDecoder"/> for ETC1 / ETC2 RGB / RGB+A1 / RGBA8 / EAC R11.
/// PVRTC, PVRTC II, ASTC and all 64-bit channel-descriptor variants that
/// are not in <see cref="PvrFormat.MapUncompressed"/> are detected but
/// surfaced as <see cref="CanDecodePixels"/> = <c>false</c>.
/// </remarks>
public sealed class PvrReader : IImageReader
{
    private const int HeaderSize = 52;
    private const uint VersionLittleEndian = 0x03525650u;
    private const uint VersionBigEndian = 0x50565203u;
    private const int MaxLevels = 32;
    private const int MaxFaces = 6;
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

    /// <summary>PVR v3 header + metadata block.</summary>
    public PvrMetadata Pvr { get; }

    /// <summary>All mip / surface / face entries discovered by the level walk.</summary>
    public IReadOnlyList<PvrLevelInfo> Levels { get; }

    private PvrReader(Stream s, bool owns, byte[] bytes, BcnFormat bcn, EtcFormat etc,
                     PixelFormat uncompressedPf, int uncompressedBpp,
                     ImageInfo info, ImageMetadata meta, PvrMetadata pvr,
                     IReadOnlyList<PvrLevelInfo> levels, bool canDecode)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes;
        _bcn = bcn; _etc = etc; _uncompressedPf = uncompressedPf;
        _uncompressedBytesPerPixel = uncompressedBpp;
        Info = info; Metadata = meta; Pvr = pvr;
        Levels = levels; CanDecodePixels = canDecode;
    }

    /// <summary>Open a PVR file by path.</summary>
    public static PvrReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a PVR file from a stream. The contents are buffered into memory.</summary>
    public static PvrReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < HeaderSize)
        {
            throw new ImageFormatException("Truncated PVR (header < 52 bytes).");
        }

        uint version = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(0, 4));
        bool le;
        if (version == VersionLittleEndian) le = true;
        else if (version == VersionBigEndian) le = false;
        else throw new ImageFormatException(
            $"Not a PVR v3 file (version word 0x{version:X8} is neither 0x03525650 nor 0x50565203).");

        uint flags = ReadU32(bytes, 4, le);
        ulong pfWord = ReadU64(bytes, 8, le);
        uint colourSpace = ReadU32(bytes, 16, le);
        uint channelType = ReadU32(bytes, 20, le);
        uint height = ReadU32(bytes, 24, le);
        uint width = ReadU32(bytes, 28, le);
        uint depth = ReadU32(bytes, 32, le);
        uint numSurfaces = ReadU32(bytes, 36, le);
        uint numFaces = ReadU32(bytes, 40, le);
        uint numMipMaps = ReadU32(bytes, 44, le);
        uint metaDataSize = ReadU32(bytes, 48, le);

        if (width == 0 || height == 0)
        {
            throw new ImageFormatException("PVR width/height must be > 0.");
        }
        uint d = depth == 0 ? 1u : depth;
        uint surfaces = numSurfaces == 0 ? 1u : numSurfaces;
        uint faces = numFaces == 0 ? 1u : numFaces;
        uint levels = numMipMaps == 0 ? 1u : numMipMaps;
        if (levels > MaxLevels || faces > MaxFaces || surfaces > MaxSurfaces)
        {
            throw new ImageFormatException(
                $"PVR mip/face/surface counts out of bounds ({levels}/{faces}/{surfaces}).");
        }
        if (faces is not (1u or 6u))
        {
            throw new ImageFormatException($"PVR numFaces must be 1 or 6 (was {faces}).");
        }

        long metaOffset = HeaderSize;
        long pixelsStart = metaOffset + metaDataSize;
        if (pixelsStart > bytes.Length)
        {
            throw new ImageFormatException("PVR metaDataSize exceeds file length.");
        }

        var (entries, byKey) = ParseMetaBlock(bytes, (int)metaOffset, (int)metaDataSize, le);

        PvrFormatId fmtId = (pfWord >> 32) == 0
            ? (PvrFormatId)pfWord
            : PvrFormatId.None;
        BcnFormat bcn = PvrFormat.MapBcn(fmtId);
        EtcFormat etc = bcn == BcnFormat.None ? PvrFormat.MapEtc(fmtId, channelType) : EtcFormat.None;
        PixelFormat uncompressedPf = bcn == BcnFormat.None && etc == EtcFormat.None
            ? PvrFormat.MapUncompressed(pfWord)
            : PixelFormat.Unknown;
        int uncompressedBpp = PvrFormat.UncompressedBytesPerPixel(uncompressedPf);

        var levelInfos = new List<PvrLevelInfo>();
        long cursor = pixelsStart;
        for (int level = 0; level < levels; level++)
        {
            int lw = Math.Max(1, (int)(width >> level));
            int lh = Math.Max(1, (int)(height >> level));
            int ld = Math.Max(1, (int)(d >> level));

            long perFaceLen = ComputeFacePayloadLength(fmtId, uncompressedBpp, lw, lh, ld);

            for (int surf = 0; surf < surfaces; surf++)
            {
                for (int face = 0; face < faces; face++)
                {
                    if (perFaceLen > 0 && cursor + perFaceLen > bytes.Length)
                    {
                        throw new ImageFormatException(
                            $"PVR mip {level} surface {surf} face {face} payload exceeds file length.");
                    }
                    levelInfos.Add(new PvrLevelInfo
                    {
                        Level = level,
                        Surface = surf,
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
                         || (etc != EtcFormat.None && CanDecodeEtc(etc))
                         || uncompressedPf != PixelFormat.Unknown;

        var pvrMeta = new PvrMetadata
        {
            LittleEndian = le,
            Flags = flags,
            PixelFormat = pfWord,
            FormatId = fmtId,
            ColourSpace = colourSpace,
            ChannelType = channelType,
            Height = height,
            Width = width,
            Depth = depth,
            NumSurfaces = numSurfaces,
            NumFaces = numFaces,
            NumMipMaps = numMipMaps,
            MetaDataSize = metaDataSize,
            Bcn = bcn,
            Etc = etc,
            MetaEntries = entries,
            MetaByFourCcKey = byKey,
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
                PixelFormat.Gray16 => 16,
                PixelFormat.Rgb24 or PixelFormat.Bgr24 => 24,
                PixelFormat.Rgba32 or PixelFormat.Bgra32 => 32,
                PixelFormat.Rgb96Float => 96,
                _ => 0,
            },
            ChannelCount = infoPf switch
            {
                PixelFormat.Gray8 or PixelFormat.Gray16 => 1,
                PixelFormat.Rgb24 or PixelFormat.Bgr24 or PixelFormat.Rgb96Float => 3,
                PixelFormat.Rgba32 or PixelFormat.Bgra32 => 4,
                _ => 0,
            },
            PixelFormat = infoPf,
            Format = ImageFormat.Pvr,
            HasAlpha = infoPf is PixelFormat.Rgba32 or PixelFormat.Bgra32,
            FrameCount = canDecode ? levelInfos.Count : 0,
            ColorSpace = bcn != BcnFormat.None ? $"BCn:{bcn}"
                       : etc != EtcFormat.None ? $"ETC:{etc}"
                       : fmtId != PvrFormatId.None ? $"PVR:{fmtId}"
                       : "PVR",
        };

        var meta = BuildImageMetadata(pvrMeta);
        return new PvrReader(stream, ownsStream, bytes, bcn, etc, uncompressedPf,
                             uncompressedBpp, info, meta, pvrMeta, levelInfos, canDecode);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            string what = Pvr.FormatId != PvrFormatId.None
                ? $"PVR format id {Pvr.FormatId} (0x{(uint)Pvr.FormatId:X})"
                : $"PVR channel descriptor 0x{Pvr.PixelFormat:X16}";
            throw new NotSupportedException($"{what}; pixel decode is not implemented.");
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
            else if (CanDecodeEtc(_etc))
            {
                var (pixels, stride, pf) = DecodeEtc(_etc, payload, lv.Width, lv.Height);
                yield return new ImageFrame(lv.Width, lv.Height, pf, stride, pixels);
            }
            else
            {
                int stride = lv.Width * _uncompressedBytesPerPixel;
                int totalBytes = stride * lv.Height;
                if (lv.Length < totalBytes)
                {
                    throw new ImageFormatException(
                        $"PVR uncompressed mip {lv.Level} payload {lv.Length} smaller than {totalBytes} bytes.");
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

    private static long ComputeFacePayloadLength(PvrFormatId id, int uncompressedBpp, int w, int h, int d)
    {
        int bitsPerBlock = PvrFormat.BitsPerBlock(id);
        if (bitsPerBlock > 0)
        {
            var (bw, bh) = PvrFormat.BlockDimensions(id);
            int blocksX = (w + bw - 1) / bw;
            int blocksY = (h + bh - 1) / bh;
            return (long)blocksX * blocksY * d * (bitsPerBlock / 8);
        }
        if (uncompressedBpp > 0)
        {
            return (long)w * h * d * uncompressedBpp;
        }
        return 0;
    }

    private static (IReadOnlyList<PvrMetaEntry> Entries, FrozenDictionary<ulong, byte[]> ByKey)
        ParseMetaBlock(byte[] bytes, int offset, int length, bool le)
    {
        var entries = new List<PvrMetaEntry>();
        var byKey = new Dictionary<ulong, byte[]>();
        int cursor = offset;
        int end = offset + length;
        while (cursor + 12 <= end)
        {
            uint fourCc = ReadU32(bytes, cursor, le);
            uint key = ReadU32(bytes, cursor + 4, le);
            uint dataSize = ReadU32(bytes, cursor + 8, le);
            cursor += 12;
            if (cursor + dataSize > end) break;
            var data = bytes.AsSpan(cursor, (int)dataSize).ToArray();
            cursor += (int)dataSize;
            var entry = new PvrMetaEntry { FourCc = fourCc, Key = key, Data = data };
            entries.Add(entry);
            byKey[((ulong)fourCc << 32) | key] = data;
        }
        return (entries, byKey.ToFrozenDictionary());
    }

    private static ImageMetadata BuildImageMetadata(PvrMetadata p)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["PVR:Endian"] = p.LittleEndian ? "LE" : "BE",
            ["PVR:Flags"] = $"0x{p.Flags:X8}",
            ["PVR:PixelFormat"] = $"0x{p.PixelFormat:X16}",
            ["PVR:ColourSpace"] = p.ColourSpace == 1 ? "sRGB" : "Linear",
            ["PVR:ChannelType"] = p.ChannelType.ToString(CultureInfo.InvariantCulture),
            ["PVR:NumFaces"] = p.NumFaces.ToString(CultureInfo.InvariantCulture),
            ["PVR:NumSurfaces"] = p.NumSurfaces.ToString(CultureInfo.InvariantCulture),
            ["PVR:NumMipMaps"] = p.NumMipMaps.ToString(CultureInfo.InvariantCulture),
            ["PVR:MetaDataSize"] = p.MetaDataSize.ToString(CultureInfo.InvariantCulture),
        };
        if (p.FormatId != PvrFormatId.None)
        {
            tags["PVR:FormatId"] = p.FormatId.ToString();
        }
        if (p.Bcn != BcnFormat.None) tags["PVR:Bcn"] = p.Bcn.ToString();
        if (p.Etc != EtcFormat.None) tags["PVR:Etc"] = p.Etc.ToString();
        return new ImageMetadata { Tags = tags.ToFrozenDictionary(StringComparer.Ordinal) };
    }

    private static bool CanDecodeEtc(EtcFormat f) => f is
        EtcFormat.Etc1Rgb or EtcFormat.Etc2Rgb or EtcFormat.Etc2RgbA1
        or EtcFormat.Etc2Rgba8 or EtcFormat.EacR11Unorm or EtcFormat.EacR11Snorm;

    private static PixelFormat BcnToDecodedPf(BcnFormat f) => f switch
    {
        BcnFormat.Bc1 or BcnFormat.Bc2 or BcnFormat.Bc3 or BcnFormat.Bc7 => PixelFormat.Bgra32,
        BcnFormat.Bc4 => PixelFormat.Gray8,
        BcnFormat.Bc5 => PixelFormat.Rgb24,
        BcnFormat.Bc6hUf16 or BcnFormat.Bc6hSf16 => PixelFormat.Rgb96Float,
        _ => PixelFormat.Unknown,
    };

    private static PixelFormat EtcToDecodedPf(EtcFormat f) => f switch
    {
        EtcFormat.Etc1Rgb or EtcFormat.Etc2Rgb or EtcFormat.Etc2RgbA1
            or EtcFormat.Etc2Rgba8 => PixelFormat.Rgba32,
        EtcFormat.EacR11Unorm or EtcFormat.EacR11Snorm => PixelFormat.Gray16,
        _ => PixelFormat.Unknown,
    };

    private static (byte[] Pixels, int Stride, PixelFormat Format) DecodeBcn(
        BcnFormat f, ReadOnlySpan<byte> payload, int w, int h)
    {
        return f switch
        {
            BcnFormat.Bc1 => (BcnDecoder.DecodeBc1(payload, w, h), w * 4, PixelFormat.Bgra32),
            BcnFormat.Bc2 => (BcnDecoder.DecodeBc2(payload, w, h), w * 4, PixelFormat.Bgra32),
            BcnFormat.Bc3 => (BcnDecoder.DecodeBc3(payload, w, h), w * 4, PixelFormat.Bgra32),
            BcnFormat.Bc4 => (BcnDecoder.DecodeBc4(payload, w, h), w, PixelFormat.Gray8),
            BcnFormat.Bc5 => (BcnDecoder.DecodeBc5(payload, w, h), w * 3, PixelFormat.Rgb24),
            BcnFormat.Bc7 => (Bc7Decoder.DecodeBc7(payload, w, h), w * 4, PixelFormat.Bgra32),
            BcnFormat.Bc6hUf16 => (Bc6hDecoder.DecodeBc6h(payload, w, h, isSigned: false), w * 12, PixelFormat.Rgb96Float),
            BcnFormat.Bc6hSf16 => (Bc6hDecoder.DecodeBc6h(payload, w, h, isSigned: true), w * 12, PixelFormat.Rgb96Float),
            _ => throw new NotSupportedException($"PVR BCn format {f} cannot be decoded."),
        };
    }

    private static (byte[] Pixels, int Stride, PixelFormat Format) DecodeEtc(
        EtcFormat f, ReadOnlySpan<byte> payload, int w, int h)
    {
        return f switch
        {
            EtcFormat.Etc1Rgb => (EtcDecoder.DecodeEtc1(payload, w, h), w * 4, PixelFormat.Rgba32),
            EtcFormat.Etc2Rgb => (EtcDecoder.DecodeEtc2Rgb(payload, w, h), w * 4, PixelFormat.Rgba32),
            EtcFormat.Etc2RgbA1 => (EtcDecoder.DecodeEtc2RgbA1(payload, w, h), w * 4, PixelFormat.Rgba32),
            EtcFormat.Etc2Rgba8 => (EtcDecoder.DecodeEtc2Rgba8(payload, w, h), w * 4, PixelFormat.Rgba32),
            EtcFormat.EacR11Unorm => (EtcDecoder.DecodeEacR11Unorm(payload, w, h), w * 2, PixelFormat.Gray16),
            EtcFormat.EacR11Snorm => (EtcDecoder.DecodeEacR11Snorm(payload, w, h), w * 2, PixelFormat.Gray16),
            _ => throw new NotSupportedException($"PVR ETC format {f} cannot be decoded."),
        };
    }

    private static uint ReadU32(byte[] b, int offset, bool le) => le
        ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(offset, 4))
        : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(offset, 4));

    private static ulong ReadU64(byte[] b, int offset, bool le) => le
        ? BinaryPrimitives.ReadUInt64LittleEndian(b.AsSpan(offset, 8))
        : BinaryPrimitives.ReadUInt64BigEndian(b.AsSpan(offset, 8));
}
