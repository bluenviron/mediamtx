using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Jpeg;

namespace Mediar.Imaging.X3f;

/// <summary>
/// Reader for Sigma Foveon X3F RAW files. X3F is the proprietary container
/// used by Sigma's fixed-lens compacts (DP1/DP2/DP3 Merrill, dp Quattro) and
/// SD-series DSLRs. The reader composes <see cref="JpegReader"/> for the
/// embedded JPEG preview (always decodable) and surfaces the Foveon raw
/// mosaic as an undecodable sub-image - full Foveon decode requires the
/// proprietary Huffman / TRUE / Quattro pipeline, which is a separate
/// codec engine.
/// </summary>
/// <remarks>
/// <para>On-disk layout per public X3F specification:</para>
/// <list type="table">
///   <listheader><term>Offset</term><description>Field</description></listheader>
///   <item><term>0x00</term><description>4-byte ASCII "FOVb" file magic</description></item>
///   <item><term>0x04</term><description>u16 minor version, u16 major version (typical 2.0 / 2.1 / 2.3)</description></item>
///   <item><term>0x08</term><description>16-byte unique file identifier</description></item>
///   <item><term>0x18</term><description>u32 file mark</description></item>
///   <item><term>0x1C</term><description>(v >= 2.1) u32 rotation + 32-byte white-balance label</description></item>
///   <item><term>...</term><description>Section payloads back-to-back</description></item>
///   <item><term>EOF-4</term><description>u32 absolute offset of the "SECd" directory</description></item>
/// </list>
/// <para>
/// At the directory offset, layout is "SECd" + u32 version + u32 entry count,
/// followed by N 12-byte entries: (u32 section offset, u32 section length,
/// 4-byte section identifier such as "IMA2", "PROP", "CAMF"). Each referenced
/// section then starts with its own four-character header magic (e.g.
/// "SECi" for image, "SECp" for properties, "SECc" for camera metadata).
/// </para>
/// </remarks>
public sealed class X3fReader : IImageReader
{
    private const int HeaderSize = 28;
    private const uint MaxSectionsBound = 1024;

    private static readonly byte[] s_magicFovb = "FOVb"u8.ToArray();
    private static readonly byte[] s_magicSecd = "SECd"u8.ToArray();
    private static readonly byte[] s_magicSeci = "SECi"u8.ToArray();
    private static readonly byte[] s_magicSecp = "SECp"u8.ToArray();
    private static readonly byte[] s_magicSecc = "SECc"u8.ToArray();

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.X3f;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>Sigma-specific metadata parsed from the X3F header and property sections.</summary>
    public X3fMetadata X3f { get; }

    /// <summary>All section entries discovered while walking the X3F directory.</summary>
    public IReadOnlyList<X3fSubImageInfo> SubImages { get; }

    private X3fReader(Stream s, bool ownsStream, byte[] bytes,
                      ImageInfo info, ImageMetadata meta, X3fMetadata x3f,
                      IReadOnlyList<X3fSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream; _bytes = bytes;
        Info = info; Metadata = meta; X3f = x3f;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open an X3F file by path.</summary>
    public static X3fReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an X3F from a stream (the contents are buffered into memory).</summary>
    public static X3fReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < HeaderSize + 4)
        {
            throw new ImageFormatException("Truncated X3F (header + directory-offset trailer < 32 bytes).");
        }

        for (int i = 0; i < s_magicFovb.Length; i++)
        {
            if (bytes[i] != s_magicFovb[i])
            {
                throw new ImageFormatException("Not an X3F file (missing 'FOVb' signature at offset 0).");
            }
        }

        ushort verMinor = ReadU16(bytes, 4);
        ushort verMajor = ReadU16(bytes, 6);
        var fileId = bytes.AsSpan(8, 16);
        uint fileMark = ReadU32(bytes, 24);

        uint? rotation = null;
        string? wbLabel = null;
        int extEnd = HeaderSize;
        if (verMajor >= 2 && verMinor >= 1 && bytes.Length >= HeaderSize + 4 + 32)
        {
            rotation = ReadU32(bytes, 28);
            wbLabel = DecodeAsciiTrimNul(bytes.AsSpan(32, 32));
            extEnd = 64;
            if (verMajor >= 2 && verMinor >= 3 && bytes.Length >= extEnd + 32 + 32)
            {
                extEnd += 32 + 32;
            }
        }

        uint directoryOffset = ReadU32(bytes, bytes.Length - 4);
        if (directoryOffset < (uint)extEnd || (long)directoryOffset + 12 > bytes.Length)
        {
            throw new ImageFormatException($"X3F directory offset 0x{directoryOffset:X} is out of bounds (file size {bytes.Length}).");
        }

        for (int i = 0; i < s_magicSecd.Length; i++)
        {
            if (bytes[directoryOffset + i] != s_magicSecd[i])
            {
                throw new ImageFormatException("Not an X3F directory (missing 'SECd' magic at directory offset).");
            }
        }

        uint dirEntryCount = ReadU32(bytes, (int)directoryOffset + 8);
        if (dirEntryCount > MaxSectionsBound)
        {
            throw new ImageFormatException($"X3F directory entry count {dirEntryCount} exceeds bound {MaxSectionsBound}.");
        }
        long entriesStart = directoryOffset + 12;
        if (entriesStart + (long)dirEntryCount * 12 > bytes.Length)
        {
            throw new ImageFormatException($"X3F directory has {dirEntryCount} entries but the file is truncated.");
        }

        var subImages = new List<X3fSubImageInfo>((int)dirEntryCount);
        string? make = null, model = null, software = null, dateTime = null;

        for (int i = 0; i < dirEntryCount; i++)
        {
            int e = (int)entriesStart + i * 12;
            uint secOff = ReadU32(bytes, e);
            uint secLen = ReadU32(bytes, e + 4);
            string secId = Encoding.ASCII.GetString(bytes, e + 8, 4);
            if (secOff > bytes.Length || secOff + secLen > bytes.Length)
            {
                continue;
            }

            X3fSubImageKind kind = ClassifySection(secId, bytes, secOff, secLen);
            int w = 0, h = 0;
            uint rowStride = 0, imgType = 0, dataFormat = 0;
            bool canDecode = false;

            if (kind is X3fSubImageKind.JpegPreview or X3fSubImageKind.RawMosaic)
            {
                ParseImageHeader(bytes, secOff, secLen,
                                 out imgType, out dataFormat, out w, out h, out rowStride);
                if (kind == X3fSubImageKind.JpegPreview)
                {
                    canDecode = ProbeJpegDecodable(bytes, secOff, secLen);
                }
            }
            else if (kind == X3fSubImageKind.Properties)
            {
                ParseProperties(bytes, secOff, secLen,
                                ref make, ref model, ref software, ref dateTime);
            }

            subImages.Add(new X3fSubImageInfo
            {
                Kind = kind,
                SectionId = secId,
                Offset = secOff,
                Length = secLen,
                Width = w,
                Height = h,
                RowStride = rowStride,
                ImageType = imgType,
                DataFormat = dataFormat,
                CanDecodePixels = canDecode,
            });
        }

        X3fSubImageInfo? primary = null;
        foreach (var s2 in subImages)
        {
            if (s2.CanDecodePixels && (primary is null || (long)s2.Width * s2.Height > (long)primary.Width * primary.Height))
            {
                primary = s2;
            }
        }

        bool canDecodeAny = primary is not null;
        int infoW = primary?.Width ?? 0;
        int infoH = primary?.Height ?? 0;
        var infoPf = canDecodeAny ? PixelFormat.Rgb24 : PixelFormat.Unknown;
        int infoBpp = canDecodeAny ? 24 : 0;

        var x3f = new X3fMetadata
        {
            VersionMajor = verMajor,
            VersionMinor = verMinor,
            FileIdHex = Convert.ToHexString(fileId).ToLowerInvariant(),
            FileMark = fileMark,
            Rotation = rotation,
            WhiteBalanceLabel = wbLabel,
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            EntryCount = subImages.Count,
        };

        var info = new ImageInfo
        {
            Width = infoW,
            Height = infoH,
            BitsPerPixel = infoBpp,
            ChannelCount = canDecodeAny ? 3 : 1,
            PixelFormat = infoPf,
            Format = ImageFormat.X3f,
            HasAlpha = false,
            FrameCount = canDecodeAny ? 1 : 0,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(x3f);
        return new X3fReader(stream, ownsStream, bytes, info, meta, x3f, subImages, canDecodeAny);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "X3F pixel decode requires a decodable embedded JPEG preview (image type 1, data format 3). " +
                "None was found in this file. Foveon raw mosaic decode (data format 11/18) requires the " +
                "Sigma proprietary X3 / TRUE / Quattro codec, which is a separate codec engine.");
        }

        X3fSubImageInfo? primary = null;
        foreach (var s in SubImages)
        {
            if (s.CanDecodePixels && s.Kind == X3fSubImageKind.JpegPreview &&
                (primary is null || (long)s.Width * s.Height > (long)primary.Width * primary.Height))
            {
                primary = s;
            }
        }
        if (primary is null)
        {
            throw new InvalidOperationException("X3fReader.CanDecodePixels = true but no decodable JPEG preview was found.");
        }

        int payloadOffset = (int)primary.Offset + 28;
        int payloadLength = (int)primary.Length - 28;
        if (payloadOffset + payloadLength > _bytes.Length || payloadLength < 4)
        {
            throw new InvalidOperationException("X3F JPEG preview payload is out of bounds.");
        }

        using var ms = new MemoryStream(_bytes, payloadOffset, payloadLength, writable: false);
        using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
        bool yielded = false;
        await foreach (var frame in jpeg.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            yielded = true;
            yield return frame;
            break;
        }
        if (!yielded)
        {
            throw new InvalidOperationException("Embedded X3F JPEG preview produced no frames.");
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

    private static X3fSubImageKind ClassifySection(string secId, byte[] bytes, uint off, uint len)
    {
        switch (secId)
        {
            case "PROP":
                if (len >= 4 && HasMagic(bytes, off, s_magicSecp)) return X3fSubImageKind.Properties;
                return X3fSubImageKind.Unknown;
            case "CAMF":
                if (len >= 4 && HasMagic(bytes, off, s_magicSecc)) return X3fSubImageKind.CameraMetadata;
                return X3fSubImageKind.Unknown;
            case "IMA2":
            case "IMAG":
                if (len < 28 || !HasMagic(bytes, off, s_magicSeci)) return X3fSubImageKind.Unknown;
                uint dataFormat = ReadU32(bytes, (int)off + 12);
                return dataFormat == 3 ? X3fSubImageKind.JpegPreview : X3fSubImageKind.RawMosaic;
            default:
                return X3fSubImageKind.Unknown;
        }
    }

    private static void ParseImageHeader(byte[] bytes, uint off, uint len,
                                         out uint imageType, out uint dataFormat,
                                         out int width, out int height, out uint rowStride)
    {
        imageType = 0; dataFormat = 0; width = 0; height = 0; rowStride = 0;
        if (len < 28) return;
        imageType = ReadU32(bytes, (int)off + 8);
        dataFormat = ReadU32(bytes, (int)off + 12);
        uint w = ReadU32(bytes, (int)off + 16);
        uint h = ReadU32(bytes, (int)off + 20);
        rowStride = ReadU32(bytes, (int)off + 24);
        width = (int)Math.Min(w, int.MaxValue);
        height = (int)Math.Min(h, int.MaxValue);
    }

    private static void ParseProperties(byte[] bytes, uint off, uint len,
                                        ref string? make, ref string? model,
                                        ref string? software, ref string? dateTime)
    {
        if (len < 24 || !HasMagic(bytes, off, s_magicSecp)) return;
        uint entryCount = ReadU32(bytes, (int)off + 8);
        uint charFormat = ReadU32(bytes, (int)off + 12);
        uint poolChars = ReadU32(bytes, (int)off + 20);
        if (entryCount == 0 || entryCount > 4096) return;
        long entriesStart = off + 24;
        long entriesEnd = entriesStart + (long)entryCount * 8;
        long poolStart = entriesEnd;
        long poolByteEnd = poolStart + (long)poolChars * 2;
        if (poolByteEnd > off + len) return;

        for (int i = 0; i < entryCount; i++)
        {
            int e = (int)(entriesStart + i * 8);
            uint nameCharOff = ReadU32(bytes, e);
            uint valueCharOff = ReadU32(bytes, e + 4);
            string? name = DecodePoolString(bytes, (int)poolStart, (int)poolChars, (int)nameCharOff, charFormat);
            string? value = DecodePoolString(bytes, (int)poolStart, (int)poolChars, (int)valueCharOff, charFormat);
            if (name is null || value is null) continue;

            switch (name)
            {
                case "CAMMANUF": make ??= value; break;
                case "CAMMODEL": model ??= value; break;
                case "FIRMVERS": software ??= value; break;
                case "TIME": dateTime ??= value; break;
            }
        }
    }

    private static string? DecodePoolString(byte[] bytes, int poolStart, int poolChars,
                                            int charOffset, uint charFormat)
    {
        if (charOffset < 0 || charOffset >= poolChars) return null;
        int byteStart = poolStart + charOffset * 2;
        int byteMax = poolStart + poolChars * 2;
        int byteEnd = byteStart;
        while (byteEnd + 1 < byteMax)
        {
            if (bytes[byteEnd] == 0 && bytes[byteEnd + 1] == 0) break;
            byteEnd += 2;
        }
        int len = byteEnd - byteStart;
        if (len <= 0) return string.Empty;
        return charFormat == 0
            ? Encoding.Unicode.GetString(bytes, byteStart, len)
            : Encoding.ASCII.GetString(bytes, byteStart, len).TrimEnd('\0');
    }

    private static bool ProbeJpegDecodable(byte[] bytes, uint sectionOff, uint sectionLen)
    {
        if (sectionLen < 28 + 4) return false;
        int payloadOff = (int)sectionOff + 28;
        int payloadLen = (int)sectionLen - 28;
        if (bytes[payloadOff] != 0xFF || bytes[payloadOff + 1] != 0xD8) return false;
        try
        {
            using var ms = new MemoryStream(bytes, payloadOff, payloadLen, writable: false);
            using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
            return jpeg.CanDecodePixels;
        }
        catch (Exception ex) when (ex is ImageFormatException or InvalidOperationException or NotSupportedException)
        {
            return false;
        }
    }

    private static ImageMetadata BuildImageMetadata(X3fMetadata x3f)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["X3F:Version"] = $"{x3f.VersionMajor}.{x3f.VersionMinor}",
            ["X3F:FileId"] = x3f.FileIdHex,
            ["X3F:FileMark"] = $"0x{x3f.FileMark:X8}",
            ["X3F:EntryCount"] = x3f.EntryCount.ToString(System.Globalization.CultureInfo.InvariantCulture),
        };
        if (x3f.Rotation is uint r) tags["X3F:Rotation"] = r.ToString(System.Globalization.CultureInfo.InvariantCulture);
        if (x3f.WhiteBalanceLabel is not null) tags["X3F:WhiteBalanceLabel"] = x3f.WhiteBalanceLabel;
        if (x3f.Make is not null) tags["EXIF:Make"] = x3f.Make;
        if (x3f.Model is not null) tags["EXIF:Model"] = x3f.Model;
        if (x3f.Software is not null) tags["EXIF:Software"] = x3f.Software;
        if (x3f.DateTime is not null) tags["EXIF:DateTime"] = x3f.DateTime;

        return new ImageMetadata
        {
            CameraMake = x3f.Make,
            CameraModel = x3f.Model,
            Software = x3f.Software,
            CapturedAtRaw = x3f.DateTime,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static bool HasMagic(byte[] bytes, uint off, byte[] magic)
    {
        if (off + magic.Length > bytes.Length) return false;
        for (int i = 0; i < magic.Length; i++)
        {
            if (bytes[off + i] != magic[i]) return false;
        }
        return true;
    }

    private static string DecodeAsciiTrimNul(ReadOnlySpan<byte> span)
    {
        int len = span.IndexOf((byte)0);
        if (len < 0) len = span.Length;
        return len > 0 ? Encoding.ASCII.GetString(span[..len]) : string.Empty;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(byte[] bytes, int offset)
        => BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(offset, 2));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] bytes, int offset)
        => BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(offset, 4));
}
