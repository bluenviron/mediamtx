using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Tiff;

namespace Mediar.Imaging.Mrw;

/// <summary>
/// Reader for Konica Minolta RAW (MRW) files. MRW is a proprietary
/// big-endian envelope built from <c>\0MRM</c>-prefixed sub-blocks, followed
/// by a raw Bayer mosaic CFA payload. The reader walks the sub-block
/// stream, parses PRD geometry, and composes <see cref="TiffReader"/>
/// against the embedded <c>\0TTW</c> (TIFF tag wrapper) sub-block to
/// surface EXIF metadata.
/// </summary>
/// <remarks>
/// <para>
/// File layout per libopenraw / dcraw (all multi-byte fields are big-endian):
/// </para>
/// <list type="table">
///   <listheader><term>Offset</term><description>Field</description></listheader>
///   <item><term>0x00</term><description>4-byte magic <c>\0MRM</c> = 0x00 0x4D 0x52 0x4D</description></item>
///   <item><term>0x04</term><description>uint32 envelope length (total size of all sub-blocks, excludes magic + this length field)</description></item>
///   <item><term>0x08</term><description>Sub-block stream begins. Each sub-block: 4-byte tag (starts with <c>\0</c>) + uint32 payload length + payload bytes.</description></item>
///   <item><term>0x08 + envelope</term><description>Raw Bayer mosaic CFA data (Konica Minolta proprietary packed format).</description></item>
/// </list>
/// <para>
/// Known sub-block tags:
/// </para>
/// <list type="bullet">
///   <item><term><c>\0PRD</c></term><description>Picture Raw Dimensions - version, sensor / image geometry, data layout.</description></item>
///   <item><term><c>\0TTW</c></term><description>TIFF Tag Wrapper - complete embedded TIFF with EXIF tags.</description></item>
///   <item><term><c>\0WBG</c></term><description>White Balance Gains - 4 channel multipliers.</description></item>
///   <item><term><c>\0RIF</c></term><description>Raw Information File - colour profile, processing settings.</description></item>
///   <item><term><c>\0CSA</c></term><description>Colour Sensitivity Array (older firmware).</description></item>
/// </list>
/// <para>
/// Konica Minolta's raw mosaic compression (12-bit packed Bayer + per-row
/// delta predictor on later bodies) is proprietary and not yet decoded by
/// Mediar; the CFA sub-image is reported as <see cref="CanDecodePixels"/>
/// <c>false</c>. If the embedded TTW TIFF carries a strip-stored thumbnail
/// (uncompressed or standard JPEG-in-TIFF) the reader will decode that
/// instead.
/// </para>
/// </remarks>
public sealed class MrwReader : IImageReader
{
    private const int MinimumHeaderSize = 8;
    private static readonly byte[] s_magic = [0x00, (byte)'M', (byte)'R', (byte)'M'];

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _ttwOffset;
    private readonly int _ttwLength;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Mrw;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The Konica Minolta-specific metadata block parsed from the MRW envelope.</summary>
    public MrwMetadata Mrw { get; }

    /// <summary>All sub-images discovered in this MRW file (TTW + raw Bayer mosaic CFA).</summary>
    public IReadOnlyList<MrwSubImageInfo> SubImages { get; }

    private MrwReader(Stream s, bool ownsStream, byte[] bytes,
                      int ttwOffset, int ttwLength,
                      ImageInfo info, ImageMetadata meta, MrwMetadata mrw,
                      IReadOnlyList<MrwSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream; _bytes = bytes;
        _ttwOffset = ttwOffset; _ttwLength = ttwLength;
        Info = info; Metadata = meta; Mrw = mrw;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open an MRW file by path.</summary>
    public static MrwReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an MRW from a stream (the contents are buffered into memory).</summary>
    public static MrwReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < MinimumHeaderSize)
        {
            throw new ImageFormatException("Truncated MRW (header < 8 bytes).");
        }

        for (int i = 0; i < s_magic.Length; i++)
        {
            if (bytes[i] != s_magic[i])
            {
                throw new ImageFormatException("Not an MRW file (missing \"\\0MRM\" magic).");
            }
        }

        uint envelopeLength = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(4, 4));
        long envelopeEnd = 8L + envelopeLength;
        if (envelopeEnd > bytes.Length)
        {
            throw new ImageFormatException(
                "MRW envelope length " + envelopeLength + " exceeds file size " + bytes.Length + ".");
        }

        int prdOffset = -1, prdLength = 0;
        int ttwOffset = -1, ttwLength = 0;
        int wbgLength = 0, rifLength = 0;

        int p = 8;
        while (p < envelopeEnd)
        {
            if (p + 8 > envelopeEnd)
            {
                throw new ImageFormatException("MRW sub-block header truncated at offset " + p + ".");
            }
            // Tag is 4 bytes; first byte must be \0 per spec.
            if (bytes[p] != 0x00)
            {
                throw new ImageFormatException(
                    "MRW sub-block tag at offset " + p + " does not start with the expected \\0 byte.");
            }
            uint blockLen = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(p + 4, 4));
            int payloadOffset = p + 8;
            long payloadEnd = (long)payloadOffset + blockLen;
            if (payloadEnd > envelopeEnd)
            {
                throw new ImageFormatException(
                    "MRW sub-block at offset " + p + " (length " + blockLen + ") overruns envelope.");
            }

            // Identify by tag bytes 1..3 (byte 0 already verified as \0).
            byte t1 = bytes[p + 1], t2 = bytes[p + 2], t3 = bytes[p + 3];
            if (t1 == (byte)'P' && t2 == (byte)'R' && t3 == (byte)'D')
            {
                prdOffset = payloadOffset;
                prdLength = (int)blockLen;
            }
            else if (t1 == (byte)'T' && t2 == (byte)'T' && t3 == (byte)'W')
            {
                ttwOffset = payloadOffset;
                ttwLength = (int)blockLen;
            }
            else if (t1 == (byte)'W' && t2 == (byte)'B' && t3 == (byte)'G')
            {
                wbgLength = (int)blockLen;
            }
            else if (t1 == (byte)'R' && t2 == (byte)'I' && t3 == (byte)'F')
            {
                rifLength = (int)blockLen;
            }
            // Other tags (CSA, PAD, etc.) are tolerated and skipped.

            p = (int)payloadEnd;
        }

        if (prdOffset < 0)
        {
            throw new ImageFormatException("MRW envelope is missing the required \\0PRD sub-block.");
        }

        var (version, sensorH, sensorW, imageH, imageW, dataSize, pixelSize, storage, bayer) =
            ParsePrd(bytes, prdOffset, prdLength);

        string? make = null, model = null, software = null, dateTime = null, artist = null, copyright = null;
        int ttwWidth = 0, ttwHeight = 0;
        bool ttwCanDecode = false;
        var ttwPixelFormat = PixelFormat.Unknown;

        if (ttwOffset > 0 && ttwLength > 0)
        {
            ProbeTtwTiff(bytes, ttwOffset, ttwLength,
                         out make, out model, out software, out dateTime, out artist, out copyright,
                         out ttwWidth, out ttwHeight, out ttwPixelFormat, out ttwCanDecode);
        }

        if (string.IsNullOrEmpty(make) || !IsKonicaMinoltaMake(make))
        {
            // The MRW envelope is structurally correct, but the embedded TTW does not identify
            // a Konica Minolta camera. This is a strict guard against mistaking other "\0MRM"
            // sequences for camera RAW.
            throw new ImageFormatException(
                "Not an MRW file (embedded TTW TIFF Make tag does not identify a Konica Minolta camera).");
        }

        var mrw = new MrwMetadata
        {
            VersionNumber = version,
            SensorHeight = sensorH,
            SensorWidth = sensorW,
            ImageHeight = imageH,
            ImageWidth = imageW,
            DataSize = dataSize,
            PixelSize = pixelSize,
            StorageMethod = storage,
            BayerPattern = bayer,
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            Artist = artist,
            Copyright = copyright,
            WhiteBalanceGainsLength = wbgLength,
            RawInformationFileLength = rifLength,
        };

        var subs = new List<MrwSubImageInfo>();
        if (ttwOffset > 0 && ttwLength > 0)
        {
            subs.Add(new MrwSubImageInfo
            {
                Kind = MrwSubImageKind.TiffTagWrapper,
                Width = ttwWidth,
                Height = ttwHeight,
                Offset = (uint)ttwOffset,
                Length = (uint)ttwLength,
                PixelFormat = ttwPixelFormat,
                CanDecodePixels = ttwCanDecode,
            });
        }

        long cfaOffset = envelopeEnd;
        long cfaLength = bytes.Length - cfaOffset;
        if (cfaLength > 0)
        {
            subs.Add(new MrwSubImageInfo
            {
                Kind = MrwSubImageKind.Cfa,
                Width = imageW,
                Height = imageH,
                Offset = (uint)cfaOffset,
                Length = (uint)cfaLength,
                PixelFormat = PixelFormat.Unknown,
                CanDecodePixels = false,
            });
        }

        // Public Info: width/height from the TTW if it carries decodable pixels,
        // otherwise from the PRD geometry so callers still get the sensor dimensions.
        int reportedW = ttwCanDecode && ttwWidth > 0 ? ttwWidth : imageW;
        int reportedH = ttwCanDecode && ttwHeight > 0 ? ttwHeight : imageH;
        var reportedPf = ttwCanDecode ? ttwPixelFormat : PixelFormat.Unknown;

        var info = new ImageInfo
        {
            Width = reportedW,
            Height = reportedH,
            BitsPerPixel = ttwCanDecode ? 24 : pixelSize,
            ChannelCount = ttwCanDecode ? 3 : 1,
            PixelFormat = reportedPf,
            Format = ImageFormat.Mrw,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(mrw);
        return new MrwReader(stream, ownsStream, bytes, ttwOffset, ttwLength,
                             info, meta, mrw, subs, canDecode: ttwCanDecode);
    }

    private static bool IsKonicaMinoltaMake(string make) =>
        make.StartsWith("MINOLTA", StringComparison.Ordinal) ||
        make.StartsWith("Minolta", StringComparison.Ordinal) ||
        make.StartsWith("KONICA MINOLTA", StringComparison.Ordinal) ||
        make.StartsWith("Konica Minolta", StringComparison.Ordinal);

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This MRW file's embedded TTW TIFF cannot be decoded with the bundled codecs " +
                "(no strip-stored thumbnail or unsupported compression); the proprietary Konica " +
                "Minolta raw CFA mosaic is not yet implemented.");
        }
        cancellationToken.ThrowIfCancellationRequested();

        using var ms = new MemoryStream(_bytes, _ttwOffset, _ttwLength, writable: false);
        using var tiff = TiffReader.Open(ms, ownsStream: false);

        bool yielded = false;
        await foreach (var frame in tiff.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
        {
            yielded = true;
            yield return frame;
            yield break;
        }
        if (!yielded)
        {
            throw new ImageFormatException("MRW embedded TTW TIFF produced no frames.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static (string Version, int SensorH, int SensorW, int ImageH, int ImageW,
                    int DataSize, int PixelSize, int Storage, int Bayer)
        ParsePrd(byte[] bytes, int offset, int length)
    {
        // PRD layout (24 bytes nominal, older firmware emits 16):
        //   [0..7]   ASCII version string
        //   [8..9]   BE u16 sensor height
        //   [10..11] BE u16 sensor width
        //   [12..13] BE u16 image height
        //   [14..15] BE u16 image width
        //   [16]     u8  data size (bits stored per pixel)
        //   [17]     u8  pixel size (bits as captured)
        //   [18]     u8  storage method
        //   [19..22] 4 bytes unknown / reserved
        //   [23]     u8  bayer pattern
        if (length < 16)
        {
            throw new ImageFormatException("MRW \\0PRD sub-block is too small (" + length + " bytes, need >= 16).");
        }

        var span = bytes.AsSpan(offset, length);
        int versionEnd = 0;
        while (versionEnd < 8 && span[versionEnd] != 0) versionEnd++;
        string version = Encoding.ASCII.GetString(span[..versionEnd]);

        int sensorH = BinaryPrimitives.ReadUInt16BigEndian(span.Slice(8, 2));
        int sensorW = BinaryPrimitives.ReadUInt16BigEndian(span.Slice(10, 2));
        int imageH = BinaryPrimitives.ReadUInt16BigEndian(span.Slice(12, 2));
        int imageW = BinaryPrimitives.ReadUInt16BigEndian(span.Slice(14, 2));

        int dataSize = length > 16 ? span[16] : 0;
        int pixelSize = length > 17 ? span[17] : 0;
        int storage = length > 18 ? span[18] : 0;
        int bayer = length > 23 ? span[23] : 0;

        return (version, sensorH, sensorW, imageH, imageW, dataSize, pixelSize, storage, bayer);
    }

    private static void ProbeTtwTiff(byte[] bytes, int offset, int length,
                                     out string? make, out string? model,
                                     out string? software, out string? dateTime,
                                     out string? artist, out string? copyright,
                                     out int width, out int height,
                                     out PixelFormat pixelFormat, out bool canDecode)
    {
        make = model = software = dateTime = artist = copyright = null;
        width = 0; height = 0; pixelFormat = PixelFormat.Unknown; canDecode = false;
        if (length < 8) return;

        ParseTtwTiffEnvelope(bytes.AsSpan(offset, length), out var le, out var ifd0Off);
        if (ifd0Off < 0)
        {
            return;
        }

        var ttwSpan = bytes.AsSpan(offset, length);
        ParseTtwIfd0(ttwSpan, le, ifd0Off,
                     ref make, ref model, ref software, ref dateTime, ref artist, ref copyright);

        try
        {
            using var ms = new MemoryStream(bytes, offset, length, writable: false);
            using var tiff = TiffReader.Open(ms, ownsStream: false);
            width = tiff.Info.Width;
            height = tiff.Info.Height;
            pixelFormat = tiff.Info.PixelFormat;
            // Genuine decodability requires positive dimensions in addition to
            // a supported compression+pixel-format. Real MRW TTWs frequently
            // carry metadata-only IFDs with zero geometry; treat those as
            // undecodable so callers fall through to the proprietary CFA path.
            canDecode = tiff.CanDecodePixels && width > 0 && height > 0;
        }
        catch (ImageFormatException)
        {
            // TTW carries metadata-only IFDs (no Strip/Tile pixel data) - that's common
            // for MRW and not a hard failure. We've already extracted the strings.
            canDecode = false;
        }
    }

    private static void ParseTtwTiffEnvelope(ReadOnlySpan<byte> ttw, out bool le, out int ifd0Offset)
    {
        le = false; ifd0Offset = -1;
        if (ttw.Length < 8) return;
        if (ttw[0] == 'I' && ttw[1] == 'I') le = true;
        else if (ttw[0] == 'M' && ttw[1] == 'M') le = false;
        else return;

        int magic = le
            ? BinaryPrimitives.ReadUInt16LittleEndian(ttw.Slice(2, 2))
            : BinaryPrimitives.ReadUInt16BigEndian(ttw.Slice(2, 2));
        if (magic != 42) return;

        uint off = le
            ? BinaryPrimitives.ReadUInt32LittleEndian(ttw.Slice(4, 4))
            : BinaryPrimitives.ReadUInt32BigEndian(ttw.Slice(4, 4));
        if (off == 0 || off + 2 > ttw.Length) return;
        ifd0Offset = (int)off;
    }

    private static void ParseTtwIfd0(ReadOnlySpan<byte> ttw, bool le, int ifd0Off,
                                     ref string? make, ref string? model,
                                     ref string? software, ref string? dateTime,
                                     ref string? artist, ref string? copyright)
    {
        if (ifd0Off + 2 > ttw.Length) return;
        int n = le
            ? BinaryPrimitives.ReadUInt16LittleEndian(ttw.Slice(ifd0Off, 2))
            : BinaryPrimitives.ReadUInt16BigEndian(ttw.Slice(ifd0Off, 2));
        if (ifd0Off + 2 + n * 12 > ttw.Length) return;

        for (int i = 0; i < n; i++)
        {
            int o = ifd0Off + 2 + i * 12;
            int tag = ReadU16Span(ttw, o, le);
            int type = ReadU16Span(ttw, o + 2, le);
            uint count = ReadU32Span(ttw, o + 4, le);
            uint valueOff = ReadU32Span(ttw, o + 8, le);
            if (type != 2) continue;

            string? value = ReadAscii(ttw, count, valueOff, o + 8, le);
            switch (tag)
            {
                case 0x010F: make = value; break;
                case 0x0110: model = value; break;
                case 0x0131: software = value; break;
                case 0x0132: dateTime = value; break;
                case 0x013B: artist = value; break;
                case 0x8298: copyright = value; break;
            }
        }
    }

    private static string? ReadAscii(ReadOnlySpan<byte> b, uint count, uint valueOff, int inlineSlot, bool le)
    {
        int n = (int)count;
        if (n == 0) return string.Empty;
        if (n <= 4)
        {
            // TIFF stores inline ASCII values verbatim in the 4-byte value slot
            // regardless of byte order, so we read straight from the buffer.
            Span<byte> tmp = stackalloc byte[4];
            b.Slice(inlineSlot, 4).CopyTo(tmp);
            _ = valueOff;
            _ = le;
            while (n > 0 && tmp[n - 1] == 0) n--;
            return Encoding.ASCII.GetString(tmp[..n]);
        }
        else
        {
            if (valueOff + n > b.Length) return null;
            while (n > 0 && b[(int)valueOff + n - 1] == 0) n--;
            return Encoding.ASCII.GetString(b.Slice((int)valueOff, n));
        }
    }

    private static ImageMetadata BuildImageMetadata(MrwMetadata mrw)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["MRW:VersionNumber"] = mrw.VersionNumber,
            ["MRW:SensorWidth"] = mrw.SensorWidth.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:SensorHeight"] = mrw.SensorHeight.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:ImageWidth"] = mrw.ImageWidth.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:ImageHeight"] = mrw.ImageHeight.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:DataSize"] = mrw.DataSize.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:PixelSize"] = mrw.PixelSize.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:StorageMethod"] = "0x" + mrw.StorageMethod.ToString("X2", System.Globalization.CultureInfo.InvariantCulture),
            ["MRW:BayerPattern"] = mrw.BayerPattern.ToString(System.Globalization.CultureInfo.InvariantCulture),
        };

        return new ImageMetadata
        {
            CameraMake = mrw.Make,
            CameraModel = mrw.Model,
            Software = mrw.Software,
            CapturedAtRaw = mrw.DateTime,
            Author = mrw.Artist,
            Copyright = mrw.Copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16Span(ReadOnlySpan<byte> b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt16LittleEndian(b.Slice(o, 2))
           : BinaryPrimitives.ReadUInt16BigEndian(b.Slice(o, 2));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32Span(ReadOnlySpan<byte> b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt32LittleEndian(b.Slice(o, 4))
           : BinaryPrimitives.ReadUInt32BigEndian(b.Slice(o, 4));
}
