using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Globalization;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Dicom;

/// <summary>
/// Reader for DICOM Part-10 files (PS 3.10) covering the uncompressed
/// Implicit VR Little Endian (1.2.840.10008.1.2) and Explicit VR Little
/// Endian (1.2.840.10008.1.2.1) transfer syntaxes — the format used by
/// virtually every CT / MR / CR / ultrasound scanner for storage and the
/// only encoding required for DICOM conformance.
/// </summary>
/// <remarks>
/// <para>
/// Compressed transfer syntaxes (JPEG baseline / lossless, JPEG-LS,
/// JPEG 2000, RLE Lossless) are detected and exposed through
/// <see cref="ImageInfo"/> + <see cref="ImageMetadata"/>, but
/// <see cref="ReadFramesAsync"/> will throw
/// <see cref="NotSupportedException"/> because they require dispatch to
/// the JPEG / JPEG-2000 / RLE codecs which are not yet wired through the
/// DICOM encapsulated pixel-data parser.
/// </para>
/// <para>
/// Supported photometric interpretations: <c>MONOCHROME1</c>,
/// <c>MONOCHROME2</c>, <c>RGB</c>. Supported bits-allocated values: 8 and 16.
/// Multi-frame studies (NumberOfFrames &gt; 1) are decoded frame-by-frame.
/// </para>
/// </remarks>
public sealed class DicomReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _pixelDataOffset;
    private readonly int _pixelDataLength;
    private readonly bool _encapsulated;
    private readonly string _transferSyntax;
    private readonly int _numberOfFrames;
    private readonly bool _invertGrayscale;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Dicom;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    private DicomReader(
        Stream stream, bool ownsStream, byte[] bytes,
        ImageInfo info, ImageMetadata metadata,
        int pixelDataOffset, int pixelDataLength,
        bool encapsulated, string transferSyntax,
        int numberOfFrames, bool invertGrayscale, bool canDecode)
    {
        _stream = stream;
        _ownsStream = ownsStream;
        _bytes = bytes;
        Info = info;
        Metadata = metadata;
        _pixelDataOffset = pixelDataOffset;
        _pixelDataLength = pixelDataLength;
        _encapsulated = encapsulated;
        _transferSyntax = transferSyntax;
        _numberOfFrames = numberOfFrames;
        _invertGrayscale = invertGrayscale;
        CanDecodePixels = canDecode;
    }

    /// <summary>Open a DICOM file by path.</summary>
    public static DicomReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a DICOM stream.</summary>
    public static DicomReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        return OpenBytes(stream, ownsStream, ms.ToArray());
    }

    private static DicomReader OpenBytes(Stream stream, bool ownsStream, byte[] bytes)
    {
        int cursor = 0;
        bool hasPreamble = bytes.Length >= 132
            && bytes[128] == 'D' && bytes[129] == 'I' && bytes[130] == 'C' && bytes[131] == 'M';
        if (hasPreamble)
        {
            cursor = 132;
        }

        // PS 3.10: the File Meta Information group (0002) is always Explicit VR LE.
        string transferSyntax = "1.2.840.10008.1.2"; // implicit VR LE default per PS 3.5
        if (hasPreamble)
        {
            cursor = ParseMetaGroup(bytes, cursor, ref transferSyntax);
        }

        var ds = ParseDataset(bytes, cursor, transferSyntax);

        bool encapsulated = IsEncapsulated(transferSyntax);
        int bitsAllocated = ds.BitsAllocated;
        int samplesPerPixel = ds.SamplesPerPixel <= 0 ? 1 : ds.SamplesPerPixel;
        int width = ds.Columns;
        int height = ds.Rows;
        int frames = ds.NumberOfFrames <= 0 ? 1 : ds.NumberOfFrames;

        bool invertGrayscale = string.Equals(ds.PhotometricInterpretation, "MONOCHROME1", StringComparison.Ordinal);

        var pf = (samplesPerPixel, bitsAllocated, ds.PhotometricInterpretation) switch
        {
            (1, 8, "MONOCHROME1" or "MONOCHROME2") => PixelFormat.Gray8,
            (1, 16, "MONOCHROME1" or "MONOCHROME2") => PixelFormat.Gray16,
            (3, 8, "RGB" or "YBR_FULL" or "YBR_FULL_422" or "YBR_RCT" or "YBR_ICT") => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        bool canDecode = !encapsulated
                         && pf != PixelFormat.Unknown
                         && ds.PixelDataOffset > 0
                         && width > 0 && height > 0;

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = bitsAllocated * samplesPerPixel,
            ChannelCount = samplesPerPixel,
            PixelFormat = pf,
            Format = ImageFormat.Dicom,
            FrameCount = frames,
            IsAnimated = frames > 1,
            ColorSpace = ds.PhotometricInterpretation,
        };

        var metadata = BuildMetadata(ds, transferSyntax);
        return new DicomReader(
            stream, ownsStream, bytes,
            info, metadata,
            ds.PixelDataOffset, ds.PixelDataLength,
            encapsulated, transferSyntax,
            frames, invertGrayscale, canDecode);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        if (!CanDecodePixels)
        {
            throw _encapsulated
                ? new NotSupportedException(
                    $"DICOM transfer syntax {_transferSyntax} requires codec dispatch through encapsulated pixel data, which is not yet implemented.")
                : new NotSupportedException(
                    $"DICOM photometric/bit configuration is not supported: bits={Info.BitsPerPixel}, channels={Info.ChannelCount}, space={Info.ColorSpace}.");
        }

        int width = Info.Width;
        int height = Info.Height;
        var pf = Info.PixelFormat;
        int bytesPerPixel = pf switch
        {
            PixelFormat.Gray8 => 1,
            PixelFormat.Gray16 => 2,
            PixelFormat.Rgb24 => 3,
            _ => throw new InvalidOperationException("Unreachable."),
        };
        int stride = width * bytesPerPixel;
        int frameSize = stride * height;

        for (int f = 0; f < _numberOfFrames; f++)
        {
            cancellationToken.ThrowIfCancellationRequested();
            int offset = _pixelDataOffset + f * frameSize;
            if (offset + frameSize > _pixelDataOffset + _pixelDataLength)
            {
                throw new ImageFormatException("DICOM pixel data shorter than declared.");
            }

            var (frame, buf) = ImageFrame.Rent(width, height, pf, stride);
            Buffer.BlockCopy(_bytes, offset, buf, 0, frameSize);

            if (_invertGrayscale)
            {
                InvertGrayscaleInPlace(buf, frameSize, pf);
            }

            yield return frame;
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    // ── parsing internals ───────────────────────────────────────────────────────

    private readonly struct DatasetSummary
    {
        public int Rows { get; init; }
        public int Columns { get; init; }
        public int BitsAllocated { get; init; }
        public int BitsStored { get; init; }
        public int HighBit { get; init; }
        public int PixelRepresentation { get; init; }
        public int SamplesPerPixel { get; init; }
        public string PhotometricInterpretation { get; init; }
        public int NumberOfFrames { get; init; }
        public string? PatientName { get; init; }
        public string? PatientId { get; init; }
        public string? StudyDate { get; init; }
        public string? StudyDescription { get; init; }
        public string? SeriesDescription { get; init; }
        public string? Modality { get; init; }
        public string? Manufacturer { get; init; }
        public string? ManufacturerModelName { get; init; }
        public string? SoftwareVersions { get; init; }
        public string? InstitutionName { get; init; }
        public string? BodyPartExamined { get; init; }
        public int PixelDataOffset { get; init; }
        public int PixelDataLength { get; init; }
        public Dictionary<string, string> AllTags { get; init; }
    }

    private static int ParseMetaGroup(byte[] b, int cursor, ref string transferSyntax)
    {
        // The File Meta Information group is always Explicit VR LE.
        while (cursor + 8 <= b.Length)
        {
            ushort group = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(cursor));
            if (group != 0x0002)
            {
                return cursor;
            }
            ushort element = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(cursor + 2));
            string vr = Encoding.ASCII.GetString(b, cursor + 4, 2);
            int valOff, valLen;
            if (vr is "OB" or "OW" or "OF" or "SQ" or "UT" or "UN")
            {
                if (cursor + 12 > b.Length) return cursor;
                valLen = (int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(cursor + 8));
                valOff = cursor + 12;
            }
            else
            {
                valLen = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(cursor + 6));
                valOff = cursor + 8;
            }
            if (valOff + valLen > b.Length || valLen < 0) return cursor;

            if (element == 0x0010)
            {
                transferSyntax = Encoding.ASCII.GetString(b, valOff, valLen).TrimEnd('\0', ' ');
            }
            cursor = valOff + valLen;
        }
        return cursor;
    }

    private static DatasetSummary ParseDataset(byte[] b, int cursor, string transferSyntax)
    {
        bool explicitVr = IsExplicitVr(transferSyntax);

        int rows = 0, columns = 0;
        int bitsAllocated = 0, bitsStored = 0, highBit = 0, pixelRepresentation = 0;
        int samplesPerPixel = 1;
        string photometric = string.Empty;
        int numberOfFrames = 0;
        int pixelDataOffset = 0, pixelDataLength = 0;

        string? patientName = null, patientId = null, studyDate = null;
        string? studyDescription = null, seriesDescription = null;
        string? modality = null, manufacturer = null, modelName = null;
        string? softwareVersions = null, institutionName = null, bodyPart = null;
        var allTags = new Dictionary<string, string>(StringComparer.Ordinal);

        while (cursor + 8 <= b.Length)
        {
            ushort group = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(cursor));
            ushort element = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(cursor + 2));
            int valOff, valLen;
            string vr;

            if (explicitVr)
            {
                if (cursor + 8 > b.Length) break;
                vr = Encoding.ASCII.GetString(b, cursor + 4, 2);
                if (vr is "OB" or "OW" or "OF" or "OD" or "OL" or "SQ" or "UT" or "UN")
                {
                    if (cursor + 12 > b.Length) break;
                    valLen = (int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(cursor + 8));
                    valOff = cursor + 12;
                }
                else
                {
                    valLen = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(cursor + 6));
                    valOff = cursor + 8;
                }
            }
            else
            {
                vr = string.Empty;
                if (cursor + 8 > b.Length) break;
                valLen = (int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(cursor + 4));
                valOff = cursor + 8;
            }

            // Undefined length (0xFFFFFFFF) is used for encapsulated PixelData
            // and SQ items; we stop reading at the PixelData tag and let the
            // caller handle encapsulation.
            bool undefinedLength = valLen == unchecked((int)0xFFFFFFFFu);

            if (group == 0x7FE0 && element == 0x0010)
            {
                pixelDataOffset = valOff;
                pixelDataLength = undefinedLength ? Math.Max(0, b.Length - valOff) : valLen;
                break;
            }

            if (undefinedLength || valOff + valLen > b.Length || valLen < 0)
            {
                break;
            }

            switch (((uint)group << 16) | element)
            {
                case 0x00280010u: rows = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280011u: columns = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280100u: bitsAllocated = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280101u: bitsStored = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280102u: highBit = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280103u: pixelRepresentation = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280002u: samplesPerPixel = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00280004u: photometric = ReadStringValue(b, valOff, valLen); break;
                case 0x00280008u: numberOfFrames = ReadIntegerValue(b, valOff, valLen, vr); break;
                case 0x00100010u: patientName = ReadStringValue(b, valOff, valLen); break;
                case 0x00100020u: patientId = ReadStringValue(b, valOff, valLen); break;
                case 0x00080020u: studyDate = ReadStringValue(b, valOff, valLen); break;
                case 0x00081030u: studyDescription = ReadStringValue(b, valOff, valLen); break;
                case 0x0008103Eu: seriesDescription = ReadStringValue(b, valOff, valLen); break;
                case 0x00080060u: modality = ReadStringValue(b, valOff, valLen); break;
                case 0x00080070u: manufacturer = ReadStringValue(b, valOff, valLen); break;
                case 0x00081090u: modelName = ReadStringValue(b, valOff, valLen); break;
                case 0x00181020u: softwareVersions = ReadStringValue(b, valOff, valLen); break;
                case 0x00080080u: institutionName = ReadStringValue(b, valOff, valLen); break;
                case 0x00180015u: bodyPart = ReadStringValue(b, valOff, valLen); break;
                default:
                    if (group == 0x0010 || group == 0x0008 || group == 0x0018 || group == 0x0020)
                    {
                        if (vr is not "SQ" && valLen <= 256)
                        {
                            string key = string.Format(CultureInfo.InvariantCulture, "DICOM:({0:X4},{1:X4})", group, element);
                            allTags[key] = ReadStringValue(b, valOff, valLen);
                        }
                    }
                    break;
            }

            cursor = valOff + valLen;
        }

        return new DatasetSummary
        {
            Rows = rows,
            Columns = columns,
            BitsAllocated = bitsAllocated,
            BitsStored = bitsStored,
            HighBit = highBit,
            PixelRepresentation = pixelRepresentation,
            SamplesPerPixel = samplesPerPixel,
            PhotometricInterpretation = photometric,
            NumberOfFrames = numberOfFrames,
            PatientName = patientName,
            PatientId = patientId,
            StudyDate = studyDate,
            StudyDescription = studyDescription,
            SeriesDescription = seriesDescription,
            Modality = modality,
            Manufacturer = manufacturer,
            ManufacturerModelName = modelName,
            SoftwareVersions = softwareVersions,
            InstitutionName = institutionName,
            BodyPartExamined = bodyPart,
            PixelDataOffset = pixelDataOffset,
            PixelDataLength = pixelDataLength,
            AllTags = allTags,
        };
    }

    private static bool IsExplicitVr(string transferSyntax) => transferSyntax switch
    {
        "1.2.840.10008.1.2" => false,
        _ => true, // every other transfer syntax in PS 3.5 is Explicit VR
    };

    private static bool IsEncapsulated(string transferSyntax) => transferSyntax switch
    {
        "1.2.840.10008.1.2" or "1.2.840.10008.1.2.1" or "1.2.840.10008.1.2.2" => false,
        _ => true,
    };

    private static int ReadIntegerValue(byte[] b, int off, int len, string vr)
    {
        return vr switch
        {
            "US" or "OW" when len >= 2 => BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(off)),
            "SS" when len >= 2 => BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(off)),
            "UL" when len >= 4 => unchecked((int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(off))),
            "SL" when len >= 4 => BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(off)),
            "IS" => ParseDecimalString(b, off, len),
            _ when len == 2 => BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(off)),
            _ when len == 4 => unchecked((int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(off))),
            _ => ParseDecimalString(b, off, len),
        };
    }

    private static int ParseDecimalString(byte[] b, int off, int len)
    {
        if (len <= 0) return 0;
        var s = Encoding.ASCII.GetString(b, off, len).Trim().TrimEnd('\0');
        return int.TryParse(s, NumberStyles.Integer, CultureInfo.InvariantCulture, out int v) ? v : 0;
    }

    private static string ReadStringValue(byte[] b, int off, int len)
    {
        if (len <= 0) return string.Empty;
        return Encoding.ASCII.GetString(b, off, len).TrimEnd('\0', ' ');
    }

    private static ImageMetadata BuildMetadata(DatasetSummary ds, string transferSyntax)
    {
        DateTimeOffset? capturedAt = null;
        if (!string.IsNullOrEmpty(ds.StudyDate) && ds.StudyDate.Length == 8 &&
            int.TryParse(ds.StudyDate.AsSpan(0, 4), NumberStyles.Integer, CultureInfo.InvariantCulture, out int yyyy) &&
            int.TryParse(ds.StudyDate.AsSpan(4, 2), NumberStyles.Integer, CultureInfo.InvariantCulture, out int mm) &&
            int.TryParse(ds.StudyDate.AsSpan(6, 2), NumberStyles.Integer, CultureInfo.InvariantCulture, out int dd))
        {
            try
            {
                capturedAt = new DateTimeOffset(yyyy, mm, dd, 0, 0, 0, TimeSpan.Zero);
            }
            catch (ArgumentOutOfRangeException)
            {
                // leave null
            }
        }

        var tags = new Dictionary<string, string>(ds.AllTags, StringComparer.Ordinal)
        {
            ["DICOM:TransferSyntaxUID"] = transferSyntax,
        };
        if (!string.IsNullOrEmpty(ds.Modality)) tags["DICOM:Modality"] = ds.Modality;
        if (!string.IsNullOrEmpty(ds.BodyPartExamined)) tags["DICOM:BodyPartExamined"] = ds.BodyPartExamined;
        if (!string.IsNullOrEmpty(ds.InstitutionName)) tags["DICOM:InstitutionName"] = ds.InstitutionName;
        if (!string.IsNullOrEmpty(ds.PatientId)) tags["DICOM:PatientID"] = ds.PatientId;

        return new ImageMetadata
        {
            Title = string.IsNullOrEmpty(ds.SeriesDescription) ? ds.StudyDescription : ds.SeriesDescription,
            Description = ds.StudyDescription,
            Author = NormalisePersonName(ds.PatientName),
            CameraMake = ds.Manufacturer,
            CameraModel = ds.ManufacturerModelName,
            Software = ds.SoftwareVersions,
            CapturedAt = capturedAt,
            CapturedAtRaw = ds.StudyDate,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static string? NormalisePersonName(string? raw)
    {
        if (string.IsNullOrWhiteSpace(raw)) return null;
        // DICOM PN: components separated by '^' as Family^Given^Middle^Prefix^Suffix.
        var parts = raw.Split('^');
        if (parts.Length >= 2)
        {
            string family = parts[0].Trim();
            string given = parts[1].Trim();
            if (family.Length == 0) return given;
            if (given.Length == 0) return family;
            return $"{given} {family}";
        }
        return raw.Replace('^', ' ').Trim();
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void InvertGrayscaleInPlace(byte[] buf, int frameSize, PixelFormat pf)
    {
        if (pf == PixelFormat.Gray8)
        {
            for (int i = 0; i < frameSize; i++) buf[i] = (byte)(0xFF - buf[i]);
            return;
        }
        if (pf == PixelFormat.Gray16)
        {
            var span = buf.AsSpan(0, frameSize);
            for (int i = 0; i + 1 < frameSize; i += 2)
            {
                ushort v = BinaryPrimitives.ReadUInt16LittleEndian(span[i..]);
                BinaryPrimitives.WriteUInt16LittleEndian(span[i..], (ushort)(0xFFFF - v));
            }
        }
    }
}
