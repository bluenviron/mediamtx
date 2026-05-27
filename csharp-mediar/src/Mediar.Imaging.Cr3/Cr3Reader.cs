using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Jpeg;

namespace Mediar.Imaging.Cr3;

/// <summary>
/// Reader for Canon Raw v3 (CR3) files. CR3 is the ISO-BMFF based
/// successor to CR2 used by every Canon body from the EOS M50 / R / RP
/// onward. The container is a standard MP4 with major brand "crx "
/// plus a Canon-specific <c>uuid</c> box (UUID
/// 85c0b687-820f-11e0-8111-f4ce462b6a48) carrying typed sub-boxes:
/// <list type="bullet">
///   <item><description>CCTP - Canon CR3 metadata index header</description></item>
///   <item><description>CTBO - Canon thumbnail box offset table</description></item>
///   <item><description>CMT1 - main EXIF IFD (TIFF tag stream: Make / Model / Software / DateTime / Artist / Copyright)</description></item>
///   <item><description>CMT2 - EXIF sub-IFD</description></item>
///   <item><description>CMT3 - Canon MakerNote</description></item>
///   <item><description>CMT4 - GPS IFD</description></item>
///   <item><description>THMB - 160x120 / 160x107 JPEG thumbnail</description></item>
/// </list>
/// A second top-level uuid box (UUID
/// eaf42b5e-1c98-4b88-b9fb-b7dc406e4d16) carries the PRVW (preview)
/// JPEG used for camera-back review.
/// </summary>
/// <remarks>
/// The reader composes <see cref="JpegReader"/> for THMB / PRVW
/// decode and exposes the raw CRAW track as an undecodable
/// sub-image. Canon's CRAW codec is a proprietary delta + lossless
/// JPEG hybrid that requires a dedicated decoder.
/// </remarks>
public sealed class Cr3Reader : IImageReader
{
    private static readonly byte[] s_canonUuid =
    [
        0x85, 0xC0, 0xB6, 0x87, 0x82, 0x0F, 0x11, 0xE0,
        0x81, 0x11, 0xF4, 0xCE, 0x46, 0x2B, 0x6A, 0x48,
    ];

    private static readonly byte[] s_prvwUuid =
    [
        0xEA, 0xF4, 0x2B, 0x5E, 0x1C, 0x98, 0x4B, 0x88,
        0xB9, 0xFB, 0xB7, 0xDC, 0x40, 0x6E, 0x4D, 0x16,
    ];

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Cr3;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The Canon-specific metadata parsed from the file.</summary>
    public Cr3Metadata Cr3 { get; }

    /// <summary>All sub-images discovered in this CR3 file (THMB / PRVW / CRAW).</summary>
    public IReadOnlyList<Cr3SubImageInfo> SubImages { get; }

    private Cr3Reader(Stream stream, bool ownsStream, byte[] bytes,
                     ImageInfo info, ImageMetadata meta, Cr3Metadata cr3,
                     IReadOnlyList<Cr3SubImageInfo> subs, bool canDecode)
    {
        _stream = stream; _ownsStream = ownsStream; _bytes = bytes;
        Info = info; Metadata = meta; Cr3 = cr3;
        SubImages = subs; CanDecodePixels = canDecode;
    }

    /// <summary>Open a CR3 file by path.</summary>
    public static Cr3Reader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a CR3 from a stream (the contents are buffered into memory).</summary>
    public static Cr3Reader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8)
        {
            throw new ImageFormatException("Truncated CR3 (file < 8 bytes).");
        }

        // ftyp must be the first top-level box.
        if (!TryReadBoxHeader(bytes, 0, out string firstType, out int firstContentStart, out int firstContentEnd, out _))
        {
            throw new ImageFormatException("CR3 missing initial ftyp box.");
        }
        if (firstType != "ftyp")
        {
            throw new ImageFormatException("CR3 first box is '" + firstType + "', expected 'ftyp'.");
        }

        string majorBrand = ReadAsciiFourCc(bytes, firstContentStart);
        if (majorBrand != "crx ")
        {
            throw new ImageFormatException("Not a CR3 file: ftyp major brand is '" + majorBrand + "', expected 'crx '.");
        }

        uint minorVersion = ReadU32Be(bytes, firstContentStart + 4);
        var compatBrands = new List<string>();
        for (int p = firstContentStart + 8; p + 4 <= firstContentEnd; p += 4)
        {
            compatBrands.Add(ReadAsciiFourCc(bytes, p));
        }

        // Walk top-level boxes looking for moov + uuid (PRVW).
        Cr3CmtFields cmt = default;
        bool hasCanonUuid = false;
        var subs = new List<Cr3SubImageInfo>();

        int pos = 0;
        while (TryReadBoxHeader(bytes, pos, out string type, out int cs, out int ce, out int totalLen))
        {
            switch (type)
            {
                case "moov":
                    WalkMoov(bytes, cs, ce, ref cmt, ref hasCanonUuid, subs);
                    break;
                case "uuid":
                    if (ce - cs >= 16)
                    {
                        if (BytesEqual(bytes, cs, s_prvwUuid))
                        {
                            CollectPrvw(bytes, cs + 16, ce, subs);
                        }
                    }
                    break;
                case "mdat":
                    // The raw track payload usually lives here. Capture
                    // its bounds as a non-decodable sub-image so callers
                    // can see the file does carry a raw mosaic.
                    subs.Add(new Cr3SubImageInfo
                    {
                        Kind = Cr3SubImageKind.RawMosaic,
                        Offset = cs,
                        Length = ce - cs,
                        CanDecodePixels = false,
                    });
                    break;
            }
            pos += totalLen;
            if (totalLen <= 0) break;
        }

        var cr3 = new Cr3Metadata
        {
            MajorBrand = majorBrand,
            MinorVersion = minorVersion,
            CompatibleBrands = compatBrands,
            Make = cmt.Make,
            Model = cmt.Model,
            Software = cmt.Software,
            DateTime = cmt.DateTime,
            Artist = cmt.Artist,
            Copyright = cmt.Copyright,
            HasCanonUuid = hasCanonUuid,
            HasCmt1 = cmt.HasCmt1,
            HasCmt2 = cmt.HasCmt2,
            HasCmt3 = cmt.HasCmt3,
            HasCmt4 = cmt.HasCmt4,
            Cmt3ByteLength = cmt.Cmt3ByteLength,
            Exif = cmt.HasCmt2 ? new Cr3ExifMetadata
            {
                ExposureTimeSeconds = cmt.ExposureTimeSeconds,
                FNumber = cmt.FNumber,
                IsoSpeedRatings = cmt.IsoSpeedRatings,
                DateTimeOriginal = cmt.DateTimeOriginal,
                DateTimeDigitized = cmt.DateTimeDigitized,
                ExposureBiasValue = cmt.ExposureBiasValue,
                FocalLengthMm = cmt.FocalLengthMm,
                LensModel = cmt.LensModel,
                LensMake = cmt.LensMake,
                Flash = cmt.Flash,
                MeteringMode = cmt.MeteringMode,
                ExposureProgram = cmt.ExposureProgram,
                WhiteBalance = cmt.WhiteBalance,
            } : null,
            Gps = cmt.HasCmt4 ? new Cr3GpsMetadata
            {
                LatitudeDegrees = cmt.GpsLatitudeDegrees,
                LongitudeDegrees = cmt.GpsLongitudeDegrees,
                AltitudeMeters = cmt.GpsAltitudeMeters,
                LatitudeRef = cmt.GpsLatitudeRef,
                LongitudeRef = cmt.GpsLongitudeRef,
                TimeStampUtc = cmt.GpsTimeStampUtc,
                DateStamp = cmt.GpsDateStamp,
            } : null,
        };

        // Choose the largest JPEG sub-image as primary (preview > thumbnail).
        Cr3SubImageInfo? primary = null;
        long primaryPixels = -1;
        foreach (var sub in subs)
        {
            if (!sub.CanDecodePixels) continue;
            long pixels = (long)sub.Width * sub.Height;
            if (pixels > primaryPixels) { primary = sub; primaryPixels = pixels; }
        }

        int infoWidth = primary?.Width ?? 0;
        int infoHeight = primary?.Height ?? 0;
        bool canDecode = primary is not null;

        var info = new ImageInfo
        {
            Width = infoWidth,
            Height = infoHeight,
            BitsPerPixel = canDecode ? 24 : 0,
            ChannelCount = canDecode ? 3 : 0,
            PixelFormat = canDecode ? PixelFormat.Rgb24 : PixelFormat.Unknown,
            Format = ImageFormat.Cr3,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "YCbCr",
        };

        var meta = BuildImageMetadata(cr3);
        return new Cr3Reader(stream, ownsStream, bytes, info, meta, cr3, subs, canDecode);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();

        Cr3SubImageInfo? primary = null;
        long primaryPixels = -1;
        foreach (var sub in SubImages)
        {
            if (!sub.CanDecodePixels) continue;
            long pixels = (long)sub.Width * sub.Height;
            if (pixels > primaryPixels) { primary = sub; primaryPixels = pixels; }
        }
        if (primary is null)
        {
            throw new NotSupportedException(
                "CR3 file contains no decodable JPEG sub-image (THMB / PRVW). Raw CRAW decode is not supported.");
        }

        using var ms = new MemoryStream(_bytes, (int)primary.Offset, (int)primary.Length, writable: false);
        using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
        await foreach (var frame in jpeg.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            yield return frame;
            yield break;
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    // ---- box-tree walkers ----

    private struct Cr3CmtFields
    {
        // CMT1 (main IFD) - already populated.
        public string? Make;
        public string? Model;
        public string? Software;
        public string? DateTime;
        public string? Artist;
        public string? Copyright;
        public bool HasCmt1;

        // CMT2 (EXIF sub-IFD).
        public bool HasCmt2;
        public double? ExposureTimeSeconds;
        public double? FNumber;
        public ushort? IsoSpeedRatings;
        public string? DateTimeOriginal;
        public string? DateTimeDigitized;
        public double? ExposureBiasValue;
        public double? FocalLengthMm;
        public string? LensModel;
        public string? LensMake;
        public ushort? Flash;
        public ushort? MeteringMode;
        public ushort? ExposureProgram;
        public ushort? WhiteBalance;

        // CMT3 (Canon MakerNote) - capture raw byte length only.
        public bool HasCmt3;
        public int Cmt3ByteLength;

        // CMT4 (GPS IFD).
        public bool HasCmt4;
        public double? GpsLatitudeDegrees;
        public double? GpsLongitudeDegrees;
        public double? GpsAltitudeMeters;
        public string? GpsLatitudeRef;
        public string? GpsLongitudeRef;
        public string? GpsTimeStampUtc;
        public string? GpsDateStamp;
    }

    private static void WalkMoov(
        byte[] bytes, int start, int end,
        ref Cr3CmtFields cmt, ref bool hasCanonUuid,
        List<Cr3SubImageInfo> subs)
    {
        int p = start;
        while (TryReadBoxHeader(bytes, p, out string type, out int cs, out int ce, out int tot))
        {
            if (type == "uuid" && ce - cs >= 16 && BytesEqual(bytes, cs, s_canonUuid))
            {
                hasCanonUuid = true;
                WalkCanonUuid(bytes, cs + 16, ce, ref cmt, subs);
            }
            p += tot;
            if (p >= end || tot <= 0) break;
        }
    }

    private static void WalkCanonUuid(
        byte[] bytes, int start, int end,
        ref Cr3CmtFields cmt, List<Cr3SubImageInfo> subs)
    {
        int p = start;
        while (TryReadBoxHeader(bytes, p, out string type, out int cs, out int ce, out int tot))
        {
            switch (type)
            {
                case "CMT1":
                    ParseCmt1(bytes, cs, ce, ref cmt);
                    cmt.HasCmt1 = true;
                    break;
                case "CMT2":
                    ParseCmt2(bytes, cs, ce, ref cmt);
                    cmt.HasCmt2 = true;
                    break;
                case "CMT3":
                    cmt.HasCmt3 = true;
                    cmt.Cmt3ByteLength = ce - cs;
                    break;
                case "CMT4":
                    ParseCmt4(bytes, cs, ce, ref cmt);
                    cmt.HasCmt4 = true;
                    break;
                case "THMB":
                    CollectThmb(bytes, cs, ce, subs);
                    break;
            }
            p += tot;
            if (p >= end || tot <= 0) break;
        }
    }

    /// <summary>
    /// Parse the CMT1 sub-box payload as a TIFF stream (II/MM + magic 42
    /// + IFD0 offset + IFD0 entries). Extract the Make / Model /
    /// Software / DateTime / Artist / Copyright ASCII tags. Anything
    /// malformed is silently ignored - CR3 files without one of these
    /// tags are common.
    /// </summary>
    private static void ParseCmt1(byte[] b, int s, int e, ref Cr3CmtFields cmt)
    {
        if (e - s < 8) return;
        bool littleEndian;
        if (b[s] == 0x49 && b[s + 1] == 0x49) littleEndian = true;
        else if (b[s] == 0x4D && b[s + 1] == 0x4D) littleEndian = false;
        else return;
        ushort magic = ReadU16(b, s + 2, littleEndian);
        if (magic != 42) return;

        uint ifdOffset = ReadU32(b, s + 4, littleEndian);
        if (ifdOffset == 0 || s + ifdOffset + 2 > e) return;

        int ifdPos = s + (int)ifdOffset;
        ushort tagCount = ReadU16(b, ifdPos, littleEndian);
        if (ifdPos + 2 + tagCount * 12 > e) return;

        for (int i = 0; i < tagCount; i++)
        {
            int entry = ifdPos + 2 + i * 12;
            ushort tag = ReadU16(b, entry, littleEndian);
            ushort type = ReadU16(b, entry + 2, littleEndian);
            uint count = ReadU32(b, entry + 4, littleEndian);
            int valueAt = entry + 8;

            if (type != 2 || count == 0) continue; // ASCII only

            int dataLen = (int)count;
            int dataAt = dataLen <= 4 ? valueAt : s + (int)ReadU32(b, valueAt, littleEndian);
            if (dataAt < s || dataAt + dataLen > e) continue;

            string value = ReadAsciiString(b, dataAt, dataLen);
            switch (tag)
            {
                case 0x010F: cmt.Make = value; break;
                case 0x0110: cmt.Model = value; break;
                case 0x0131: cmt.Software = value; break;
                case 0x0132: cmt.DateTime = value; break;
                case 0x013B: cmt.Artist = value; break;
                case 0x8298: cmt.Copyright = value; break;
            }
        }
    }

    /// <summary>
    /// Parse the CMT2 sub-box payload (EXIF sub-IFD). Decodes the
    /// commonly-requested EXIF tags (exposure, aperture, ISO, capture
    /// timestamps, focal length, lens make / model, flash / metering).
    /// Malformed entries are silently ignored.
    /// </summary>
    private static void ParseCmt2(byte[] b, int s, int e, ref Cr3CmtFields cmt)
    {
        if (!TryReadTiffHeader(b, s, e, out bool le, out int ifdPos)) return;
        if (!TryReadTagCount(b, e, ifdPos, le, out int tagCount, out int tagsStart)) return;

        for (int i = 0; i < tagCount; i++)
        {
            int entry = tagsStart + i * 12;
            ushort tag = ReadU16(b, entry, le);
            ushort type = ReadU16(b, entry + 2, le);
            uint count = ReadU32(b, entry + 4, le);
            int valueAt = entry + 8;
            switch (tag)
            {
                case 0x829A: // ExposureTime, RATIONAL
                    if (TryReadRational(b, s, e, valueAt, type, count, le, out double et))
                        cmt.ExposureTimeSeconds = et;
                    break;
                case 0x829D: // FNumber, RATIONAL
                    if (TryReadRational(b, s, e, valueAt, type, count, le, out double fn))
                        cmt.FNumber = fn;
                    break;
                case 0x8827: // ISOSpeedRatings, SHORT (count >= 1)
                    if (TryReadShort(b, valueAt, type, le, out ushort iso))
                        cmt.IsoSpeedRatings = iso;
                    break;
                case 0x9003: // DateTimeOriginal, ASCII
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string dto))
                        cmt.DateTimeOriginal = dto;
                    break;
                case 0x9004: // DateTimeDigitized, ASCII
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string dtd))
                        cmt.DateTimeDigitized = dtd;
                    break;
                case 0x9204: // ExposureBiasValue, SRATIONAL
                    if (TryReadSRational(b, s, e, valueAt, type, count, le, out double ev))
                        cmt.ExposureBiasValue = ev;
                    break;
                case 0x9207: // MeteringMode, SHORT
                    if (TryReadShort(b, valueAt, type, le, out ushort mm))
                        cmt.MeteringMode = mm;
                    break;
                case 0x9209: // Flash, SHORT
                    if (TryReadShort(b, valueAt, type, le, out ushort fl))
                        cmt.Flash = fl;
                    break;
                case 0x920A: // FocalLength, RATIONAL (in mm)
                    if (TryReadRational(b, s, e, valueAt, type, count, le, out double focal))
                        cmt.FocalLengthMm = focal;
                    break;
                case 0x8822: // ExposureProgram, SHORT
                    if (TryReadShort(b, valueAt, type, le, out ushort ep))
                        cmt.ExposureProgram = ep;
                    break;
                case 0xA403: // WhiteBalance, SHORT
                    if (TryReadShort(b, valueAt, type, le, out ushort wb))
                        cmt.WhiteBalance = wb;
                    break;
                case 0xA433: // LensMake, ASCII
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string lmake))
                        cmt.LensMake = lmake;
                    break;
                case 0xA434: // LensModel, ASCII
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string lmodel))
                        cmt.LensModel = lmodel;
                    break;
            }
        }
    }

    /// <summary>
    /// Parse the CMT4 sub-box payload (GPS IFD). Decodes the standard
    /// GPS reference / coordinate / altitude / timestamp tags, converting
    /// the three-RATIONAL (deg, min, sec) DMS tuples into signed decimal
    /// degrees per the EXIF 2.32 spec.
    /// </summary>
    private static void ParseCmt4(byte[] b, int s, int e, ref Cr3CmtFields cmt)
    {
        if (!TryReadTiffHeader(b, s, e, out bool le, out int ifdPos)) return;
        if (!TryReadTagCount(b, e, ifdPos, le, out int tagCount, out int tagsStart)) return;

        double? lat = null, lon = null;
        string? latRef = null, lonRef = null;
        double? altMag = null;
        byte altRef = 0;
        var timestamps = new double[3];
        bool hasTimestamps = false;

        for (int i = 0; i < tagCount; i++)
        {
            int entry = tagsStart + i * 12;
            ushort tag = ReadU16(b, entry, le);
            ushort type = ReadU16(b, entry + 2, le);
            uint count = ReadU32(b, entry + 4, le);
            int valueAt = entry + 8;
            switch (tag)
            {
                case 0x0001: // GPSLatitudeRef, ASCII ('N' or 'S')
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string lref))
                        latRef = lref;
                    break;
                case 0x0002: // GPSLatitude, RATIONAL[3]
                    if (TryReadDmsRationals(b, s, e, valueAt, type, count, le, out double latDeg))
                        lat = latDeg;
                    break;
                case 0x0003: // GPSLongitudeRef, ASCII ('E' or 'W')
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string lonRefStr))
                        lonRef = lonRefStr;
                    break;
                case 0x0004: // GPSLongitude, RATIONAL[3]
                    if (TryReadDmsRationals(b, s, e, valueAt, type, count, le, out double lonDeg))
                        lon = lonDeg;
                    break;
                case 0x0005: // GPSAltitudeRef, BYTE (0=above sea level, 1=below)
                    if (type == 1 && count >= 1) altRef = b[valueAt];
                    break;
                case 0x0006: // GPSAltitude, RATIONAL (metres above sea level)
                    if (TryReadRational(b, s, e, valueAt, type, count, le, out double alt))
                        altMag = alt;
                    break;
                case 0x0007: // GPSTimeStamp, RATIONAL[3] (h, m, s UTC)
                    if (TryReadThreeRationals(b, s, e, valueAt, type, count, le, timestamps))
                        hasTimestamps = true;
                    break;
                case 0x001D: // GPSDateStamp, ASCII "YYYY:MM:DD"
                    if (TryReadAscii(b, s, e, valueAt, type, count, le, out string ds))
                        cmt.GpsDateStamp = ds;
                    break;
            }
        }

        if (lat is double latVal && latRef is { Length: >= 1 })
        {
            cmt.GpsLatitudeDegrees = (latRef[0] is 'S' or 's') ? -latVal : latVal;
            cmt.GpsLatitudeRef = latRef;
        }
        if (lon is double lonVal && lonRef is { Length: >= 1 })
        {
            cmt.GpsLongitudeDegrees = (lonRef[0] is 'W' or 'w') ? -lonVal : lonVal;
            cmt.GpsLongitudeRef = lonRef;
        }
        if (altMag is double altVal)
        {
            cmt.GpsAltitudeMeters = altRef == 1 ? -altVal : altVal;
        }
        if (hasTimestamps)
        {
            int h = (int)timestamps[0];
            int m = (int)timestamps[1];
            double sec = timestamps[2];
            cmt.GpsTimeStampUtc = $"{h:D2}:{m:D2}:{sec:00.###}";
        }
    }

    // ---- TIFF tag helpers (CMT2 / CMT4) ----

    private static bool TryReadTiffHeader(byte[] b, int s, int e, out bool le, out int ifdPos)
    {
        le = false; ifdPos = 0;
        if (e - s < 8) return false;
        if (b[s] == 0x49 && b[s + 1] == 0x49) le = true;
        else if (b[s] == 0x4D && b[s + 1] == 0x4D) le = false;
        else return false;
        ushort magic = ReadU16(b, s + 2, le);
        if (magic != 42) return false;
        uint ifdOffset = ReadU32(b, s + 4, le);
        if (ifdOffset == 0 || s + ifdOffset + 2 > e) return false;
        ifdPos = s + (int)ifdOffset;
        return true;
    }

    private static bool TryReadTagCount(byte[] b, int e, int ifdPos, bool le, out int tagCount, out int tagsStart)
    {
        tagCount = ReadU16(b, ifdPos, le);
        tagsStart = ifdPos + 2;
        return tagsStart + tagCount * 12 <= e;
    }

    private static bool TryReadShort(byte[] b, int valueAt, ushort type, bool le, out ushort value)
    {
        value = 0;
        if (type == 3) // SHORT
        {
            value = ReadU16(b, valueAt, le);
            return true;
        }
        if (type == 4) // LONG (some encoders use this for ISO)
        {
            uint v = ReadU32(b, valueAt, le);
            value = (ushort)Math.Min(v, ushort.MaxValue);
            return true;
        }
        return false;
    }

    private static bool TryReadAscii(byte[] b, int s, int e, int valueAt, ushort type, uint count, bool le, out string value)
    {
        value = string.Empty;
        if (type != 2 || count == 0) return false;
        int dataLen = (int)count;
        int dataAt = dataLen <= 4 ? valueAt : s + (int)ReadU32(b, valueAt, le);
        if (dataAt < s || dataAt + dataLen > e) return false;
        value = ReadAsciiString(b, dataAt, dataLen);
        return value.Length > 0;
    }

    private static bool TryReadRational(byte[] b, int s, int e, int valueAt, ushort type, uint count, bool le, out double value)
    {
        value = 0;
        if (type != 5 || count == 0) return false;
        int dataAt = s + (int)ReadU32(b, valueAt, le);
        if (dataAt < s || dataAt + 8 > e) return false;
        uint num = ReadU32(b, dataAt, le);
        uint den = ReadU32(b, dataAt + 4, le);
        if (den == 0) return false;
        value = (double)num / den;
        return true;
    }

    private static bool TryReadSRational(byte[] b, int s, int e, int valueAt, ushort type, uint count, bool le, out double value)
    {
        value = 0;
        if (type != 10 || count == 0) return false;
        int dataAt = s + (int)ReadU32(b, valueAt, le);
        if (dataAt < s || dataAt + 8 > e) return false;
        int num = (int)ReadU32(b, dataAt, le);
        int den = (int)ReadU32(b, dataAt + 4, le);
        if (den == 0) return false;
        value = (double)num / den;
        return true;
    }

    private static bool TryReadDmsRationals(byte[] b, int s, int e, int valueAt, ushort type, uint count, bool le, out double degrees)
    {
        degrees = 0;
        var trio = new double[3];
        if (!TryReadThreeRationals(b, s, e, valueAt, type, count, le, trio)) return false;
        degrees = trio[0] + (trio[1] / 60.0) + (trio[2] / 3600.0);
        return true;
    }

    private static bool TryReadThreeRationals(byte[] b, int s, int e, int valueAt, ushort type, uint count, bool le, double[] outValues)
    {
        if (type != 5 || count < 3) return false;
        int dataAt = s + (int)ReadU32(b, valueAt, le);
        if (dataAt < s || dataAt + 24 > e) return false;
        for (int i = 0; i < 3; i++)
        {
            uint num = ReadU32(b, dataAt + i * 8, le);
            uint den = ReadU32(b, dataAt + i * 8 + 4, le);
            if (den == 0) return false;
            outValues[i] = (double)num / den;
        }
        return true;
    }

    /// <summary>
    /// Extract the embedded JPEG from a Canon THMB sub-box. The THMB
    /// payload layout is:
    /// <code>
    /// 4 bytes version / flags
    /// 2 bytes width (BE u16)
    /// 2 bytes height (BE u16)
    /// 4 bytes JPEG size (BE u32)
    /// JPEG bytes (size bytes)
    /// </code>
    /// </summary>
    private static void CollectThmb(byte[] b, int s, int e, List<Cr3SubImageInfo> subs)
    {
        if (e - s < 12) return;
        int width = ReadU16Be(b, s + 4);
        int height = ReadU16Be(b, s + 6);
        uint size = ReadU32Be(b, s + 8);
        int jpegStart = s + 12;
        if (size == 0 || jpegStart + size > e) return;

        var (probedW, probedH, can) = ProbeJpegDimensions(b, jpegStart, (int)size);
        subs.Add(new Cr3SubImageInfo
        {
            Kind = Cr3SubImageKind.Thumbnail,
            Width = probedW > 0 ? probedW : width,
            Height = probedH > 0 ? probedH : height,
            Offset = jpegStart,
            Length = size,
            CanDecodePixels = can,
        });
    }

    /// <summary>
    /// Walk the PRVW uuid box (UUID eaf42b5e-1c98-4b88-b9fb-b7dc406e4d16)
    /// looking for the embedded "PRVW" sub-box. The PRVW payload layout
    /// matches THMB but with a slightly different prelude:
    /// <code>
    /// 4 bytes version / flags
    /// 2 bytes reserved
    /// 2 bytes width (BE u16)
    /// 2 bytes height (BE u16)
    /// 2 bytes reserved
    /// 4 bytes JPEG size (BE u32)
    /// JPEG bytes (size bytes)
    /// </code>
    /// </summary>
    private static void CollectPrvw(byte[] b, int start, int end, List<Cr3SubImageInfo> subs)
    {
        int p = start;
        while (TryReadBoxHeader(b, p, out string type, out int cs, out int ce, out int tot))
        {
            if (type == "PRVW" && ce - cs >= 16)
            {
                int width = ReadU16Be(b, cs + 6);
                int height = ReadU16Be(b, cs + 8);
                uint size = ReadU32Be(b, cs + 12);
                int jpegStart = cs + 16;
                if (size > 0 && jpegStart + size <= ce)
                {
                    var (probedW, probedH, can) = ProbeJpegDimensions(b, jpegStart, (int)size);
                    subs.Add(new Cr3SubImageInfo
                    {
                        Kind = Cr3SubImageKind.Preview,
                        Width = probedW > 0 ? probedW : width,
                        Height = probedH > 0 ? probedH : height,
                        Offset = jpegStart,
                        Length = size,
                        CanDecodePixels = can,
                    });
                }
            }
            p += tot;
            if (p >= end || tot <= 0) break;
        }
    }

    // ---- helpers ----

    private static (int Width, int Height, bool Can) ProbeJpegDimensions(byte[] bytes, int offset, int length)
    {
        try
        {
            using var ms = new MemoryStream(bytes, offset, length, writable: false);
            using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
            return (jpeg.Info.Width, jpeg.Info.Height, jpeg.CanDecodePixels);
        }
        catch (ImageFormatException)
        {
            return (0, 0, false);
        }
    }

    private static ImageMetadata BuildImageMetadata(Cr3Metadata cr3)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["CR3:MajorBrand"] = cr3.MajorBrand,
        };
        if (cr3.HasCanonUuid) tags["CR3:HasCanonUuid"] = "1";
        if (cr3.HasCmt1) tags["CR3:HasCmt1"] = "1";
        if (cr3.HasCmt2) tags["CR3:HasCmt2"] = "1";
        if (cr3.HasCmt3)
        {
            tags["CR3:HasCmt3"] = "1";
            tags["CR3:MakerNoteLength"] = cr3.Cmt3ByteLength.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }
        if (cr3.HasCmt4) tags["CR3:HasCmt4"] = "1";

        if (cr3.Exif is { } exif)
        {
            var inv = System.Globalization.CultureInfo.InvariantCulture;
            if (exif.ExposureTimeSeconds is double et) tags["Exif:ExposureTime"] = et.ToString("0.######", inv);
            if (exif.FNumber is double fn) tags["Exif:FNumber"] = fn.ToString("0.##", inv);
            if (exif.IsoSpeedRatings is ushort iso) tags["Exif:ISOSpeedRatings"] = iso.ToString(inv);
            if (exif.DateTimeOriginal is string dto) tags["Exif:DateTimeOriginal"] = dto;
            if (exif.DateTimeDigitized is string dtd) tags["Exif:DateTimeDigitized"] = dtd;
            if (exif.ExposureBiasValue is double ev) tags["Exif:ExposureBiasValue"] = ev.ToString("0.##", inv);
            if (exif.FocalLengthMm is double focal) tags["Exif:FocalLength"] = focal.ToString("0.##", inv);
            if (exif.LensModel is string lm) tags["Exif:LensModel"] = lm;
            if (exif.LensMake is string lk) tags["Exif:LensMake"] = lk;
            if (exif.Flash is ushort fl) tags["Exif:Flash"] = fl.ToString(inv);
            if (exif.MeteringMode is ushort mm) tags["Exif:MeteringMode"] = mm.ToString(inv);
            if (exif.ExposureProgram is ushort ep) tags["Exif:ExposureProgram"] = ep.ToString(inv);
            if (exif.WhiteBalance is ushort wb) tags["Exif:WhiteBalance"] = wb.ToString(inv);
        }

        if (cr3.Gps is { } gps)
        {
            var inv = System.Globalization.CultureInfo.InvariantCulture;
            if (gps.LatitudeDegrees is double lat) tags["Gps:Latitude"] = lat.ToString("0.######", inv);
            if (gps.LongitudeDegrees is double lon) tags["Gps:Longitude"] = lon.ToString("0.######", inv);
            if (gps.AltitudeMeters is double alt) tags["Gps:Altitude"] = alt.ToString("0.##", inv);
            if (gps.LatitudeRef is string lr) tags["Gps:LatitudeRef"] = lr;
            if (gps.LongitudeRef is string lor) tags["Gps:LongitudeRef"] = lor;
            if (gps.TimeStampUtc is string ts) tags["Gps:TimeStamp"] = ts;
            if (gps.DateStamp is string ds) tags["Gps:DateStamp"] = ds;
        }

        return new ImageMetadata
        {
            CameraMake = cr3.Make,
            CameraModel = cr3.Model,
            Software = cr3.Software,
            CapturedAtRaw = cr3.Exif?.DateTimeOriginal ?? cr3.DateTime,
            Author = cr3.Artist,
            Copyright = cr3.Copyright,
            ExposureTimeSeconds = cr3.Exif?.ExposureTimeSeconds,
            FNumber = cr3.Exif?.FNumber,
            IsoSpeed = cr3.Exif?.IsoSpeedRatings,
            FocalLengthMm = cr3.Exif?.FocalLengthMm,
            LensModel = cr3.Exif?.LensModel,
            GpsLatitude = cr3.Gps?.LatitudeDegrees,
            GpsLongitude = cr3.Gps?.LongitudeDegrees,
            GpsAltitudeMeters = cr3.Gps?.AltitudeMeters,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static bool TryReadBoxHeader(byte[] b, int offset, out string type, out int contentStart, out int contentEnd, out int totalLen)
    {
        type = string.Empty; contentStart = 0; contentEnd = 0; totalLen = 0;
        if (offset + 8 > b.Length) return false;
        uint size = ReadU32Be(b, offset);
        type = ReadAsciiFourCc(b, offset + 4);
        int headerLen = 8;
        long actualSize = size;
        if (size == 1)
        {
            if (offset + 16 > b.Length) return false;
            actualSize = (long)ReadU64Be(b, offset + 8);
            headerLen = 16;
        }
        else if (size == 0)
        {
            actualSize = b.Length - offset;
        }
        if (actualSize < headerLen || offset + actualSize > b.Length) return false;
        contentStart = offset + headerLen;
        contentEnd = offset + (int)actualSize;
        totalLen = (int)actualSize;
        return true;
    }

    private static bool BytesEqual(byte[] b, int offset, byte[] needle)
    {
        if (offset + needle.Length > b.Length) return false;
        for (int i = 0; i < needle.Length; i++)
        {
            if (b[offset + i] != needle[i]) return false;
        }
        return true;
    }

    private static string ReadAsciiFourCc(byte[] b, int o) =>
        Encoding.ASCII.GetString(b, o, 4);

    private static string ReadAsciiString(byte[] b, int offset, int length)
    {
        // Trim trailing NUL if present (TIFF ASCII tags always carry one).
        int n = length;
        while (n > 0 && b[offset + n - 1] == 0) n--;
        return Encoding.ASCII.GetString(b, offset, n);
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16Be(byte[] b, int o) =>
        BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32Be(byte[] b, int o) =>
        BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ulong ReadU64Be(byte[] b, int o) =>
        BinaryPrimitives.ReadUInt64BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(byte[] b, int o, bool le) => le
        ? BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o))
        : BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] b, int o, bool le) => le
        ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o))
        : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));
}
