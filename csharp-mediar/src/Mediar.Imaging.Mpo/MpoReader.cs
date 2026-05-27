using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Jpeg;

namespace Mediar.Imaging.Mpo;

/// <summary>
/// Reader for Multi-Picture Object (MPO) files. MPO concatenates two
/// or more JPEG sub-images and embeds a Multi-Picture Format (MPF)
/// APP2 segment in the first image whose MP Index IFD lists every
/// sub-image as a 16-byte MPEntry record. The reader exposes each
/// sub-image as a typed <see cref="MpoSubImageInfo"/> and composes
/// <see cref="JpegReader"/> for per-sub-image pixel decode.
/// </summary>
/// <remarks>
/// <para>
/// MPO is defined by CIPA DC-007 (2009). The format is used by
/// stereoscopic-3D consumer cameras (Fujifilm FinePix Real 3D W3,
/// Nintendo 3DS), recent smartphones for depth-capture and
/// Live-Photo-style sequences, and any tool that wants to pack
/// multiple related JPEGs into a single file with a self-describing
/// index. Each sub-image is a complete, independently decodable JPEG
/// (SOI..EOI).
/// </para>
/// <para>
/// Layout of the MPF APP2 segment:
/// </para>
/// <list type="number">
///   <item><description>FF E2 marker.</description></item>
///   <item><description>2-byte big-endian segment length.</description></item>
///   <item><description>4-byte ASCII identifier "MPF\0".</description></item>
///   <item><description>MP Endian header: TIFF byte-order mark (II / MM)
///     + magic 42 + uint32 offset to the first IFD. Offsets inside the MP
///     Index IFD are relative to the first byte of the MP Endian header.</description></item>
///   <item><description>MP Index IFD with tags 0xB000 (Version),
///     0xB001 (NumberOfImages), 0xB002 (MPEntry table), and the optional
///     0xB003 (ImageUIDList).</description></item>
/// </list>
/// <para>
/// The first MPEntry's <c>DataOffset</c> is always 0 (it refers to the
/// JPEG already at file offset 0). Subsequent MPEntries store a positive
/// uint32 offset relative to the MP Endian header.
/// </para>
/// </remarks>
public sealed class MpoReader : IImageReader
{
    private static readonly byte[] s_mpfIdentifier = "MPF\0"u8.ToArray();

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Mpo;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The MPF-specific metadata (version, image count, byte-order, optional image UIDs).</summary>
    public MpoMetadata Mpo { get; }

    /// <summary>All sub-images discovered in this MPO file.</summary>
    public IReadOnlyList<MpoSubImageInfo> SubImages { get; }

    private MpoReader(Stream stream, bool ownsStream, byte[] bytes,
                     ImageInfo info, ImageMetadata meta, MpoMetadata mpo,
                     IReadOnlyList<MpoSubImageInfo> subs, bool canDecode)
    {
        _stream = stream; _ownsStream = ownsStream; _bytes = bytes;
        Info = info; Metadata = meta; Mpo = mpo;
        SubImages = subs; CanDecodePixels = canDecode;
    }

    /// <summary>Open an MPO file by path.</summary>
    public static MpoReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an MPO from a stream (the contents are buffered into memory).</summary>
    public static MpoReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 4 || bytes[0] != 0xFF || bytes[1] != 0xD8)
        {
            throw new ImageFormatException("Not an MPO file (missing JPEG SOI marker).");
        }

        if (!TryFindMpfSegment(bytes, out int mpfPayloadStart, out int mpfPayloadLength))
        {
            throw new ImageFormatException("Not an MPO file (no MPF APP2 segment found in first image).");
        }

        // MP Endian header begins immediately after "MPF\0".
        int mpEndianBase = mpfPayloadStart + s_mpfIdentifier.Length;
        int mpEndianEnd = mpfPayloadStart + mpfPayloadLength;
        if (mpEndianBase + 8 > mpEndianEnd)
        {
            throw new ImageFormatException("MPO MPF segment truncated before MP Endian header.");
        }

        bool littleEndian = ReadByteOrder(bytes, mpEndianBase, out string byteOrder);
        ushort magic = ReadU16(bytes, mpEndianBase + 2, littleEndian);
        if (magic != 42)
        {
            throw new ImageFormatException(
                "MPO MP Endian header has bad TIFF magic (expected 42, got " + magic + ").");
        }

        uint firstIfdOffset = ReadU32(bytes, mpEndianBase + 4, littleEndian);
        if (firstIfdOffset == 0 || mpEndianBase + (int)firstIfdOffset + 2 > mpEndianEnd)
        {
            throw new ImageFormatException("MPO MP Index IFD offset out of range.");
        }

        int ifdAbs = mpEndianBase + (int)firstIfdOffset;
        ushort tagCount = ReadU16(bytes, ifdAbs, littleEndian);
        if (ifdAbs + 2 + tagCount * 12 > mpEndianEnd)
        {
            throw new ImageFormatException("MPO MP Index IFD entry table overruns MPF segment.");
        }

        string version = "0100";
        uint numberOfImages = 0;
        int mpEntryOffsetRel = -1;
        int mpEntryLength = 0;
        int uidsOffsetRel = -1;
        int uidsLength = 0;

        for (int i = 0; i < tagCount; i++)
        {
            int entry = ifdAbs + 2 + i * 12;
            ushort tag = ReadU16(bytes, entry, littleEndian);
            ushort type = ReadU16(bytes, entry + 2, littleEndian);
            uint count = ReadU32(bytes, entry + 4, littleEndian);
            int valueAt = entry + 8;
            int byteCount = TypeByteSize(type) * (int)count;

            switch (tag)
            {
                case 0xB000: // MPFVersion (UNDEFINED 4)
                    if (byteCount == 4)
                    {
                        version = Encoding.ASCII.GetString(bytes, valueAt, 4).TrimEnd('\0');
                    }
                    break;

                case 0xB001: // NumberOfImages (LONG)
                    if (type == 4 && count == 1)
                    {
                        numberOfImages = ReadU32(bytes, valueAt, littleEndian);
                    }
                    break;

                case 0xB002: // MPEntry (UNDEFINED 16*N)
                    mpEntryLength = byteCount;
                    if (byteCount <= 4)
                    {
                        mpEntryOffsetRel = valueAt - mpEndianBase;
                    }
                    else
                    {
                        mpEntryOffsetRel = (int)ReadU32(bytes, valueAt, littleEndian);
                    }
                    break;

                case 0xB003: // ImageUIDList (UNDEFINED 33*N)
                    uidsLength = byteCount;
                    if (byteCount <= 4)
                    {
                        uidsOffsetRel = valueAt - mpEndianBase;
                    }
                    else
                    {
                        uidsOffsetRel = (int)ReadU32(bytes, valueAt, littleEndian);
                    }
                    break;
            }
        }

        if (mpEntryOffsetRel < 0 || mpEntryLength == 0)
        {
            throw new ImageFormatException("MPO MP Index IFD missing MPEntry table (tag 0xB002).");
        }
        if (numberOfImages == 0 || mpEntryLength != (int)numberOfImages * 16)
        {
            throw new ImageFormatException(
                "MPO MPEntry table length " + mpEntryLength + " disagrees with NumberOfImages "
                + numberOfImages + " (expected " + numberOfImages * 16 + " bytes).");
        }

        int mpEntryAbs = mpEndianBase + mpEntryOffsetRel;
        if (mpEntryAbs < 0 || mpEntryAbs + mpEntryLength > bytes.Length)
        {
            throw new ImageFormatException("MPO MPEntry table extends past end of file.");
        }

        var subs = new List<MpoSubImageInfo>((int)numberOfImages);
        for (int i = 0; i < (int)numberOfImages; i++)
        {
            int rec = mpEntryAbs + i * 16;
            uint attr = ReadU32(bytes, rec, littleEndian);
            uint size = ReadU32(bytes, rec + 4, littleEndian);
            uint dataOffsetRel = ReadU32(bytes, rec + 8, littleEndian);
            ushort dep1 = ReadU16(bytes, rec + 12, littleEndian);
            ushort dep2 = ReadU16(bytes, rec + 14, littleEndian);

            long absoluteOffset = i == 0 ? 0 : mpEndianBase + (long)dataOffsetRel;
            if (absoluteOffset < 0 || size == 0 || absoluteOffset + size > bytes.Length)
            {
                throw new ImageFormatException(
                    "MPO MPEntry[" + i + "] points outside file bounds: offset="
                    + absoluteOffset + " size=" + size + " file=" + bytes.Length + ".");
            }

            var (w, h, can) = ProbeJpegDimensions(bytes, (int)absoluteOffset, (int)size);

            subs.Add(new MpoSubImageInfo
            {
                Index = i,
                Kind = (MpoImageKind)(attr & 0x00FFFFFFu),
                IsDependentParent = (attr & 0x80000000u) != 0,
                IsDependentChild = (attr & 0x40000000u) != 0,
                IsRepresentative = (attr & 0x20000000u) != 0,
                RawAttribute = attr,
                Length = size,
                Offset = absoluteOffset,
                DependentImage1 = dep1,
                DependentImage2 = dep2,
                Width = w,
                Height = h,
                CanDecodePixels = can,
            });
        }

        var uids = new List<string>();
        if (uidsOffsetRel >= 0 && uidsLength > 0)
        {
            int uidAbs = mpEndianBase + uidsOffsetRel;
            if (uidAbs + uidsLength <= mpEndianEnd && uidsLength % 33 == 0)
            {
                int n = uidsLength / 33;
                for (int i = 0; i < n; i++)
                {
                    uids.Add(Encoding.ASCII.GetString(bytes, uidAbs + i * 33, 33).TrimEnd('\0'));
                }
            }
        }

        var mpoMeta = new MpoMetadata
        {
            Version = version,
            NumberOfImages = numberOfImages,
            ByteOrder = byteOrder,
            ImageUids = uids,
        };

        var primary = subs[0];
        bool canDecode = primary.CanDecodePixels;

        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = 24,
            ChannelCount = 3,
            PixelFormat = PixelFormat.Rgb24,
            Format = ImageFormat.Mpo,
            HasAlpha = false,
            FrameCount = (int)numberOfImages,
            ColorSpace = "YCbCr",
        };

        var meta = BuildImageMetadata(mpoMeta);
        return new MpoReader(stream, ownsStream, bytes, info, meta, mpoMeta, subs, canDecode);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        foreach (var sub in SubImages)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (!sub.CanDecodePixels)
            {
                throw new NotSupportedException(
                    "MPO sub-image " + sub.Index + " cannot be decoded (JpegReader rejected its header).");
            }

            using var ms = new MemoryStream(_bytes, (int)sub.Offset, (int)sub.Length, writable: false);
            using var jpeg = JpegReader.Open(ms, ImageFormat.Jpeg, ownsStream: false);
            await foreach (var frame in jpeg.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
            {
                yield return frame;
                break;
            }
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    /// <summary>
    /// Walk the first JPEG image's marker chain to find the APP2
    /// segment whose payload begins with "MPF\0". Returns the absolute
    /// file offset of the payload (the byte after "MPF\0" is the MP
    /// Endian header start) and the payload length.
    /// </summary>
    private static bool TryFindMpfSegment(byte[] b, out int payloadStart, out int payloadLength)
    {
        payloadStart = 0;
        payloadLength = 0;
        int i = 2; // skip SOI

        while (i + 4 <= b.Length)
        {
            if (b[i] != 0xFF) return false;
            byte marker = b[i + 1];
            // Skip fill bytes (FF FF ...).
            if (marker == 0xFF) { i++; continue; }
            // SOS or EOI ends the marker chain.
            if (marker == 0xDA || marker == 0xD9) return false;
            // Standalone markers without payload (RSTn / SOI / EOI / TEM).
            if (marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7))
            {
                i += 2;
                continue;
            }

            // Segment length (big-endian, including the 2 length bytes).
            int segLen = (b[i + 2] << 8) | b[i + 3];
            if (segLen < 2 || i + 2 + segLen > b.Length) return false;

            int dataAt = i + 4;
            int dataLen = segLen - 2;

            if (marker == 0xE2 && dataLen >= s_mpfIdentifier.Length)
            {
                bool match = true;
                for (int k = 0; k < s_mpfIdentifier.Length; k++)
                {
                    if (b[dataAt + k] != s_mpfIdentifier[k]) { match = false; break; }
                }
                if (match)
                {
                    payloadStart = dataAt;
                    payloadLength = dataLen;
                    return true;
                }
            }

            i = dataAt + dataLen;
        }

        return false;
    }

    private static (int Width, int Height, bool CanDecode) ProbeJpegDimensions(byte[] bytes, int offset, int length)
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

    private static ImageMetadata BuildImageMetadata(MpoMetadata mpo)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal)
        {
            ["MPF:Version"] = mpo.Version,
            ["MPF:NumberOfImages"] = mpo.NumberOfImages.ToString(System.Globalization.CultureInfo.InvariantCulture),
            ["MPF:ByteOrder"] = mpo.ByteOrder,
        };
        for (int i = 0; i < mpo.ImageUids.Count; i++)
        {
            tags["MPF:ImageUID[" + i + "]"] = mpo.ImageUids[i];
        }
        return new ImageMetadata
        {
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static bool ReadByteOrder(byte[] b, int offset, out string byteOrder)
    {
        if (b[offset] == 0x49 && b[offset + 1] == 0x49) { byteOrder = "II"; return true; }
        if (b[offset] == 0x4D && b[offset + 1] == 0x4D) { byteOrder = "MM"; return false; }
        throw new ImageFormatException("MPO MP Endian header has invalid byte-order mark.");
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(byte[] b, int o, bool le) => le
        ? BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o))
        : BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] b, int o, bool le) => le
        ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o))
        : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));

    private static int TypeByteSize(ushort type) => type switch
    {
        1 => 1,  // BYTE
        2 => 1,  // ASCII
        3 => 2,  // SHORT
        4 => 4,  // LONG
        5 => 8,  // RATIONAL
        7 => 1,  // UNDEFINED
        9 => 4,  // SLONG
        10 => 8, // SRATIONAL
        _ => 1,
    };
}
