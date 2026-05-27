using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using Mediar.Imaging.Tiff;
using Mediar.Imaging.TiffRaw;
using static Mediar.Imaging.TiffRaw.TiffRawHelpers;

namespace Mediar.Imaging.Dng;

/// <summary>
/// Reader for Adobe Digital Negative (DNG) files. DNG is a TIFF-based
/// camera-RAW container; this reader composes <see cref="TiffReader"/> for
/// the underlying container walk and parses the DNG-specific tag set
/// (defined in the DNG 1.7 specification) into a structured
/// <see cref="DngMetadata"/> record.
/// </summary>
/// <remarks>
/// <para>
/// DNG layout differs from a plain TIFF in two important ways:
/// </para>
/// <list type="bullet">
///   <item>The IFD chain typically holds a small preview / thumbnail in IFD 0
///   and the full-resolution raw sensor data inside a <em>SubIFD</em>
///   pointed at by tag 0x014A. <see cref="SubImages"/> exposes every
///   discovered SubIFD as a separate inspectable page so callers can pick
///   the right one by <see cref="DngSubImageInfo.NewSubFileType"/> and
///   dimensions.</item>
///   <item>RAW pixel payloads use lossless JPEG (SOF3), packed-integer or
///   Adobe Deflate. Where the underlying compression is supported by
///   <see cref="TiffReader"/>, this reader delegates to it; otherwise
///   <see cref="ReadFramesAsync"/> throws
///   <see cref="NotSupportedException"/> with the compression code in the
///   error message.</item>
/// </list>
/// </remarks>
public sealed class DngReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly bool _littleEndian;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Dng;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The DNG-specific metadata block parsed from IFD 0.</summary>
    public DngMetadata Dng { get; }

    /// <summary>
    /// All sub-images discovered in this DNG file, in tree-walk order.
    /// Index 0 is IFD 0 (typically a thumbnail / preview); subsequent
    /// entries are the contents of every SubIFD pointed at by tag 0x014A
    /// from any IFD. Callers can pick the full-resolution raw by
    /// scanning for the entry whose
    /// <see cref="DngSubImageInfo.NewSubFileType"/> equals 0 (primary
    /// image) or whose dimensions match
    /// <see cref="DngMetadata.DefaultCropSize"/>.
    /// </summary>
    public IReadOnlyList<DngSubImageInfo> SubImages { get; }

    private DngReader(Stream s, bool ownsStream, byte[] bytes, bool le,
                     ImageInfo info, ImageMetadata meta, DngMetadata dng,
                     IReadOnlyList<DngSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes; _littleEndian = le;
        Info = info; Metadata = meta; Dng = dng;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open a DNG file by path.</summary>
    public static DngReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a DNG from a stream (the contents are buffered into memory).</summary>
    public static DngReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8) throw new ImageFormatException("Truncated DNG.");

        bool le = bytes[0] == 'I' && bytes[1] == 'I';
        bool be = bytes[0] == 'M' && bytes[1] == 'M';
        if (!le && !be) throw new ImageFormatException("Bad DNG byte-order mark (expected II or MM).");
        int magic = ReadU16(bytes, 2, le);
        if (magic != 42) throw new ImageFormatException("Unsupported DNG/TIFF magic " + magic + ".");

        uint ifd0Offset = ReadU32(bytes, 4, le);
        if (ifd0Offset == 0) throw new ImageFormatException("DNG file has no IFDs.");

        var ifd0 = ParseIfd(bytes, le, (int)ifd0Offset);
        var dng = ParseDngMetadata(ifd0, bytes, le);
        if (dng.DngVersion.IsEmpty)
        {
            throw new ImageFormatException(
                "Not a DNG file (missing required DNGVersion tag 0xC612).");
        }

        var subs = new List<DngSubImageInfo>();
        var visitedIfds = new HashSet<uint>();
        WalkIfdsRecursive(bytes, le, ifd0Offset, parentSubIfdLevel: 0, subs, visitedIfds);

        // Pick the "primary" sub-image: prefer NewSubFileType == 0
        // (primary), else the largest one by pixel count.
        DngSubImageInfo primary = SelectPrimary(subs);

        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = primary.BitsPerSample * primary.SamplesPerPixel,
            ChannelCount = primary.SamplesPerPixel,
            PixelFormat = primary.PixelFormat,
            Format = ImageFormat.Dng,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(ifd0, dng, bytes, le);
        bool canDecode = primary.CanDecodePixels;
        return new DngReader(stream, ownsStream, bytes, le, info, meta, dng,
                              subs, canDecode);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This DNG file's primary image uses an unsupported compression scheme.");
        }
        cancellationToken.ThrowIfCancellationRequested();

        // Delegate the full container decode to TiffReader; it already
        // handles every DNG-supported compression Mediar ships today
        // (uncompressed, LZW, Deflate, JPEG-in-TIFF including SOF3
        // lossless). For DNG the raw sensor image is typically a SubIFD,
        // so we let TiffReader pick the IFD whose `NewSubFileType == 0`
        // by emitting all of its pages and selecting the matching one.
        using var ms = new MemoryStream(_bytes, writable: false);
        using var tiff = TiffReader.Open(ms, ownsStream: false);

        // Pick the primary frame from the TIFF stream by matching dims.
        DngSubImageInfo primary = SelectPrimary(SubImages);
        bool yielded = false;
        await foreach (var frame in tiff.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            if (frame.Width == primary.Width && frame.Height == primary.Height)
            {
                yielded = true;
                yield return frame;
                yield break;
            }
            frame.Dispose();
        }
        if (!yielded)
        {
            throw new NotSupportedException(
                "DNG primary image was not produced by the underlying TIFF decoder.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static DngSubImageInfo SelectPrimary(IReadOnlyList<DngSubImageInfo> subs)
    {
        DngSubImageInfo? best = null;
        long bestPixels = 0;
        foreach (var s in subs)
        {
            if (s.NewSubFileType == 0)
            {
                long px = (long)s.Width * s.Height;
                if (best is null || px > bestPixels)
                {
                    best = s; bestPixels = px;
                }
            }
        }
        if (best is not null) return best;

        // Fallback: largest dimensions of any sub-image.
        foreach (var s in subs)
        {
            long px = (long)s.Width * s.Height;
            if (best is null || px > bestPixels)
            {
                best = s; bestPixels = px;
            }
        }
        return best ?? throw new ImageFormatException("DNG file has no inspectable sub-images.");
    }

    private static void WalkIfdsRecursive(byte[] bytes, bool le, uint ifdOffset,
                                          int parentSubIfdLevel,
                                          List<DngSubImageInfo> sink,
                                          HashSet<uint> visited)
    {
        while (ifdOffset != 0)
        {
            if (!visited.Add(ifdOffset)) return;
            if (ifdOffset + 2 > bytes.Length) return;
            var entries = ParseIfd(bytes, le, (int)ifdOffset);
            sink.Add(BuildSubImageInfo(entries, bytes, le, parentSubIfdLevel));

            // Recurse into any SubIFD pointers (tag 0x014A).
            foreach (var e in entries)
            {
                if (e.Tag != 0x014A) continue;
                var subOffsets = ReadLongArray(e, bytes, le);
                foreach (uint sub in subOffsets)
                {
                    WalkIfdsRecursive(bytes, le, sub, parentSubIfdLevel + 1, sink, visited);
                }
            }

            // Walk the IFD chain only at the top level. SubIFDs themselves
            // don't have a meaningful next-IFD pointer per the DNG spec.
            if (parentSubIfdLevel != 0) return;

            int n = entries.Length;
            int nextSlot = (int)ifdOffset + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) return;
            ifdOffset = ReadU32(bytes, nextSlot, le);
        }
    }

    private static DngSubImageInfo BuildSubImageInfo(IfdEntry[] entries, byte[] bytes, bool le,
                                                      int subIfdLevel)
    {
        int width = (int)GetScalar(entries, 0x0100, def: 0);
        int height = (int)GetScalar(entries, 0x0101, def: 0);
        ushort[] bps = GetShortArray(entries, 0x0102, bytes, le);
        int bitsPerSample = bps.Length == 0 ? 1 : bps[0];
        int samplesPerPixel = (int)GetScalar(entries, 0x0115, def: 1);
        int compression = (int)GetScalar(entries, 0x0103, def: 1);
        int photometric = (int)GetScalar(entries, 0x0106, def: 0);
        int newSubFileType = (int)GetScalar(entries, 0x00FE, def: 0);
        bool hasTw = false, hasTl = false;
        foreach (var e in entries)
        {
            if (e.Tag == 0x0142) hasTw = true;
            else if (e.Tag == 0x0143) hasTl = true;
        }
        bool isTiled = hasTw && hasTl;

        // Mediar.Imaging.Tiff supports compression 1/5/7/8/2/3/4/32773/32946
        // at bps 1/8/16. DNG raw typically uses 1 (uncompressed) or 7
        // (lossless JPEG SOF3).
        bool canDecode = compression is 1 or 5 or 7 or 8 or 32773 or 32946
                         && bitsPerSample is 1 or 8 or 16;

        var pf = samplesPerPixel switch
        {
            1 when bitsPerSample == 8 => PixelFormat.Gray8,
            1 when bitsPerSample == 16 => PixelFormat.Gray16,
            3 => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        return new DngSubImageInfo
        {
            Width = width,
            Height = height,
            BitsPerSample = bitsPerSample,
            SamplesPerPixel = samplesPerPixel,
            CompressionTag = compression,
            Photometric = photometric,
            NewSubFileType = newSubFileType,
            IsTiled = isTiled,
            PixelFormat = pf,
            SubIfdLevel = subIfdLevel,
            CanDecodePixels = canDecode,
        };
    }

    private static DngMetadata ParseDngMetadata(IfdEntry[] entries, byte[] bytes, bool le)
    {
        var version = ReadByteTag(entries, 0xC612, bytes, le);
        var backVersion = ReadByteTag(entries, 0xC613, bytes, le);
        var uniqueModel = ReadAsciiTag(entries, 0xC614, bytes, le);
        var localizedModel = ReadAsciiTag(entries, 0xC615, bytes, le);
        var make = ReadAsciiTag(entries, 0x010F, bytes, le);
        var model = ReadAsciiTag(entries, 0x0110, bytes, le);
        var software = ReadAsciiTag(entries, 0x0131, bytes, le);
        var dateTime = ReadAsciiTag(entries, 0x0132, bytes, le);
        var artist = ReadAsciiTag(entries, 0x013B, bytes, le);
        var copyright = ReadAsciiTag(entries, 0x8298, bytes, le);

        var cfaPattern = ReadByteTag(entries, 0x828E, bytes, le);
        ushort[] cfaRepeatPatternDim = GetShortArray(entries, 0x828D, bytes, le);
        var cfaPlaneColor = ReadByteTag(entries, 0xC616, bytes, le);
        var cfaLayout = (int)GetScalar(entries, 0xC617, def: 0);

        uint[] blackLevel = ReadAnyNumericArray(entries, 0xC61A, bytes, le);
        uint[] whiteLevel = ReadAnyNumericArray(entries, 0xC61D, bytes, le);
        double[] defaultCropOrigin = ReadRationalArrayAsDouble(entries, 0xC61F, bytes, le);
        double[] defaultCropSize = ReadRationalArrayAsDouble(entries, 0xC620, bytes, le);
        double[] activeArea = ReadAnyNumericArrayAsDouble(entries, 0xC68D, bytes, le);

        double[] asShotNeutral = ReadRationalArrayAsDouble(entries, 0xC628, bytes, le);
        double[] asShotWhiteXY = ReadRationalArrayAsDouble(entries, 0xC629, bytes, le);
        double[] colorMatrix1 = ReadSRationalArrayAsDouble(entries, 0xC621, bytes, le);
        double[] colorMatrix2 = ReadSRationalArrayAsDouble(entries, 0xC622, bytes, le);

        return new DngMetadata
        {
            DngVersion = version,
            DngBackwardVersion = backVersion,
            UniqueCameraModel = uniqueModel,
            LocalizedCameraModel = localizedModel,
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            Artist = artist,
            Copyright = copyright,
            CfaPattern = cfaPattern,
            CfaRepeatPatternDim = cfaRepeatPatternDim,
            CfaPlaneColor = cfaPlaneColor,
            CfaLayout = cfaLayout,
            BlackLevel = blackLevel,
            WhiteLevel = whiteLevel,
            DefaultCropOrigin = defaultCropOrigin,
            DefaultCropSize = defaultCropSize,
            ActiveArea = activeArea,
            AsShotNeutral = asShotNeutral,
            AsShotWhiteXY = asShotWhiteXY,
            ColorMatrix1 = colorMatrix1,
            ColorMatrix2 = colorMatrix2,
        };
    }

    private static ImageMetadata BuildImageMetadata(IfdEntry[] ifd, DngMetadata dng,
                                                     byte[] bytes, bool le)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        if (!dng.DngVersion.IsEmpty)
        {
            var v = dng.DngVersion.Span;
            tags["DNG:Version"] = $"{v[0]}.{(v.Length > 1 ? v[1] : 0)}.{(v.Length > 2 ? v[2] : 0)}.{(v.Length > 3 ? v[3] : 0)}";
        }
        if (dng.UniqueCameraModel is not null) tags["DNG:UniqueCameraModel"] = dng.UniqueCameraModel;
        if (dng.LocalizedCameraModel is not null) tags["DNG:LocalizedCameraModel"] = dng.LocalizedCameraModel;
        if (dng.CfaLayout != 0) tags["DNG:CfaLayout"] = dng.CfaLayout.ToString(System.Globalization.CultureInfo.InvariantCulture);
        if (dng.BlackLevel.Length > 0) tags["DNG:BlackLevel"] = string.Join(",", dng.BlackLevel);
        if (dng.WhiteLevel.Length > 0) tags["DNG:WhiteLevel"] = string.Join(",", dng.WhiteLevel);

        return new ImageMetadata
        {
            CameraMake = dng.Make,
            CameraModel = dng.Model,
            Software = dng.Software,
            CapturedAtRaw = dng.DateTime,
            Author = dng.Artist,
            Copyright = dng.Copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static ReadOnlyMemory<byte> ReadByteTag(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n <= 4)
            {
                var inline = new byte[n];
                Span<byte> tmp = stackalloc byte[4];
                if (le) BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                else BinaryPrimitives.WriteUInt32BigEndian(tmp, e.ValueOffset);
                tmp[..n].CopyTo(inline);
                return inline;
            }
            if (e.ValueOffset + n > b.Length) return ReadOnlyMemory<byte>.Empty;
            return new ReadOnlyMemory<byte>(b, (int)e.ValueOffset, n);
        }
        return ReadOnlyMemory<byte>.Empty;
    }

    private static ushort[] GetShortArrayInline(IfdEntry e, byte[] b, bool le, int n)
    {
        var arr = new ushort[n];
        if (n * 2 <= 4)
        {
            Span<byte> tmp = stackalloc byte[4];
            if (le) BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
            else BinaryPrimitives.WriteUInt32BigEndian(tmp, e.ValueOffset);
            for (int k = 0; k < n; k++)
            {
                arr[k] = le
                    ? BinaryPrimitives.ReadUInt16LittleEndian(tmp[(k * 2)..])
                    : BinaryPrimitives.ReadUInt16BigEndian(tmp[(k * 2)..]);
            }
        }
        else
        {
            for (int k = 0; k < n; k++)
            {
                arr[k] = ReadU16(b, (int)e.ValueOffset + k * 2, le);
            }
        }
        return arr;
    }

    private static uint[] ReadAnyNumericArray(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n == 0) return [];
            return e.Type switch
            {
                3 => ToUIntArray(GetShortArrayInline(e, b, le, n)),
                4 => ReadLongArray(e, b, le),
                _ => [],
            };
        }
        return [];
    }

    private static uint[] ToUIntArray(ushort[] src)
    {
        var dst = new uint[src.Length];
        for (int i = 0; i < src.Length; i++) dst[i] = src[i];
        return dst;
    }

    private static double[] ReadAnyNumericArrayAsDouble(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        var ints = ReadAnyNumericArray(ifd, tag, b, le);
        var dbl = new double[ints.Length];
        for (int i = 0; i < ints.Length; i++) dbl[i] = ints[i];
        return dbl;
    }

    private static double[] ReadRationalArrayAsDouble(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n == 0) return [];
            if (e.ValueOffset + n * 8 > b.Length) return [];
            var arr = new double[n];
            for (int k = 0; k < n; k++)
            {
                uint num = ReadU32(b, (int)e.ValueOffset + k * 8, le);
                uint den = ReadU32(b, (int)e.ValueOffset + k * 8 + 4, le);
                arr[k] = den == 0 ? 0.0 : (double)num / den;
            }
            return arr;
        }
        return [];
    }

    private static double[] ReadSRationalArrayAsDouble(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n == 0) return [];
            if (e.ValueOffset + n * 8 > b.Length) return [];
            var arr = new double[n];
            for (int k = 0; k < n; k++)
            {
                int num = (int)ReadU32(b, (int)e.ValueOffset + k * 8, le);
                int den = (int)ReadU32(b, (int)e.ValueOffset + k * 8 + 4, le);
                arr[k] = den == 0 ? 0.0 : (double)num / den;
            }
            return arr;
        }
        return [];
    }
}
