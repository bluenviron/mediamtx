using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Tiff;

namespace Mediar.Imaging.ThreeFr;

/// <summary>
/// Reader for Hasselblad RAW (3FR) files. 3FR is TIFF-based, identified at
/// parse time by the EXIF <c>Make</c> tag value beginning with
/// "Hasselblad" or "HASSELBLAD" (Hasselblad medium-format cameras and
/// digital backs - H-System, X-System, V-System with digital backs).
/// The reader composes <see cref="TiffReader"/> for pixel decode and
/// exposes the parsed Hasselblad-specific metadata block.
/// </summary>
/// <remarks>
/// <para>
/// Like NEF / ARW / PEF / DCR / SRW, 3FR places a small JPEG preview /
/// thumbnail in IFD 0 and the full-resolution raw sensor data inside a
/// SubIFD pointed at by tag 0x014A. SubIFDs are walked recursively to
/// populate <see cref="SubImages"/>.
/// </para>
/// <para>
/// Hasselblad-compressed RAW (TIFF compression 8 with CFA photometric
/// 32803, or proprietary 16-bit packing in some H-System bodies) uses a
/// vendor-specific predictor + bit-packing scheme that Mediar does not
/// yet ship - sub-images using that compression are reported as
/// <c>CanDecodePixels = false</c>. Uncompressed (tag 1) and standard
/// JPEG-in-TIFF (tag 7) raw mosaics decode through the existing TIFF stack.
/// </para>
/// </remarks>
public sealed class ThreeFrReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly bool _littleEndian;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.ThreeFr;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>The Hasselblad-specific metadata block parsed from IFD 0.</summary>
    public ThreeFrMetadata ThreeFr { get; }

    /// <summary>All sub-images discovered in this 3FR file (IFD 0 plus SubIFDs in walk order).</summary>
    public IReadOnlyList<ThreeFrSubImageInfo> SubImages { get; }

    private ThreeFrReader(Stream s, bool ownsStream, byte[] bytes, bool le,
                          ImageInfo info, ImageMetadata meta, ThreeFrMetadata tfr,
                          IReadOnlyList<ThreeFrSubImageInfo> subImages, bool canDecode)
    {
        _stream = s; _ownsStream = ownsStream;
        _bytes = bytes; _littleEndian = le;
        Info = info; Metadata = meta; ThreeFr = tfr;
        SubImages = subImages; CanDecodePixels = canDecode;
    }

    /// <summary>Open a 3FR file by path.</summary>
    public static ThreeFrReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a 3FR from a stream (the contents are buffered into memory).</summary>
    public static ThreeFrReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8) throw new ImageFormatException("Truncated 3FR.");

        bool le = bytes[0] == 'I' && bytes[1] == 'I';
        bool be = bytes[0] == 'M' && bytes[1] == 'M';
        if (!le && !be) throw new ImageFormatException("Bad 3FR byte-order mark (expected II or MM).");
        int magic = ReadU16(bytes, 2, le);
        if (magic != 42) throw new ImageFormatException("Unsupported 3FR/TIFF magic " + magic + ".");

        uint ifd0Offset = ReadU32(bytes, 4, le);
        if (ifd0Offset == 0) throw new ImageFormatException("3FR file has no IFDs.");

        var ifd0 = ParseIfd(bytes, le, (int)ifd0Offset);
        var tfr = ParseThreeFrMetadata(ifd0, bytes, le);

        if (string.IsNullOrEmpty(tfr.Make) || !IsHasselbladMake(tfr.Make))
        {
            throw new ImageFormatException(
                "Not a 3FR file (EXIF Make tag does not identify a Hasselblad camera).");
        }

        var subs = new List<ThreeFrSubImageInfo>();
        var visited = new HashSet<uint>();
        WalkIfdsRecursive(bytes, le, ifd0Offset, parentSubIfdLevel: 0, subs, visited);

        var primary = SelectPrimary(subs);
        var info = new ImageInfo
        {
            Width = primary.Width,
            Height = primary.Height,
            BitsPerPixel = primary.BitsPerSample * primary.SamplesPerPixel,
            ChannelCount = primary.SamplesPerPixel,
            PixelFormat = primary.PixelFormat,
            Format = ImageFormat.ThreeFr,
            HasAlpha = false,
            FrameCount = 1,
            ColorSpace = "RAW",
        };

        var meta = BuildImageMetadata(tfr);
        return new ThreeFrReader(stream, ownsStream, bytes, le, info, meta, tfr,
                                 subs, primary.CanDecodePixels);
    }

    private static bool IsHasselbladMake(string make) =>
        make.StartsWith("Hasselblad", StringComparison.Ordinal) ||
        make.StartsWith("HASSELBLAD", StringComparison.Ordinal);

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        if (!CanDecodePixels)
        {
            throw new NotSupportedException(
                "This 3FR file's primary image uses an unsupported compression scheme " +
                "(Hasselblad-compressed RAW is not yet implemented).");
        }
        cancellationToken.ThrowIfCancellationRequested();

        using var ms = new MemoryStream(_bytes, writable: false);
        using var tiff = TiffReader.Open(ms, ownsStream: false);

        var primary = SelectPrimary(SubImages);
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
                "3FR primary image was not produced by the underlying TIFF decoder.");
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static ThreeFrSubImageInfo SelectPrimary(IReadOnlyList<ThreeFrSubImageInfo> subs)
    {
        ThreeFrSubImageInfo? best = null;
        long bestPixels = 0;
        foreach (var s in subs)
        {
            if (s.NewSubFileType == 0)
            {
                long px = (long)s.Width * s.Height;
                if (best is null || px > bestPixels) { best = s; bestPixels = px; }
            }
        }
        if (best is not null) return best;

        foreach (var s in subs)
        {
            long px = (long)s.Width * s.Height;
            if (best is null || px > bestPixels) { best = s; bestPixels = px; }
        }
        return best ?? throw new ImageFormatException("3FR file has no inspectable sub-images.");
    }

    private static void WalkIfdsRecursive(byte[] bytes, bool le, uint ifdOffset,
                                          int parentSubIfdLevel,
                                          List<ThreeFrSubImageInfo> sink,
                                          HashSet<uint> visited)
    {
        while (ifdOffset != 0)
        {
            if (!visited.Add(ifdOffset)) return;
            if (ifdOffset + 2 > bytes.Length) return;
            var entries = ParseIfd(bytes, le, (int)ifdOffset);
            sink.Add(BuildSubImageInfo(entries, parentSubIfdLevel, bytes, le));

            foreach (var e in entries)
            {
                if (e.Tag != 0x014A) continue;
                var subOffsets = ReadLongArray(e, bytes, le);
                foreach (uint sub in subOffsets)
                {
                    WalkIfdsRecursive(bytes, le, sub, parentSubIfdLevel + 1, sink, visited);
                }
            }

            if (parentSubIfdLevel != 0) return;

            int n = entries.Length;
            int nextSlot = (int)ifdOffset + 2 + n * 12;
            if (nextSlot + 4 > bytes.Length) return;
            ifdOffset = ReadU32(bytes, nextSlot, le);
        }
    }

    private static ThreeFrSubImageInfo BuildSubImageInfo(IfdEntry[] entries, int subIfdLevel, byte[] bytes, bool le)
    {
        int width = (int)GetScalar(entries, 0x0100, def: 0);
        int height = (int)GetScalar(entries, 0x0101, def: 0);
        ushort[] bps = GetShortArray(entries, 0x0102, bytes, le);
        int bitsPerSample = bps.Length == 0 ? 8 : bps[0];
        int samplesPerPixel = (int)GetScalar(entries, 0x0115, def: 1);
        int compression = (int)GetScalar(entries, 0x0103, def: 1);
        int photometric = (int)GetScalar(entries, 0x0106, def: 0);
        int newSubFileType = (int)GetScalar(entries, 0x00FE, def: 0);

        // Hasselblad-compressed RAW: compression 8 with CFA photometric (32803)
        // and bps != 8/16 indicates the proprietary 12/14-bit packing. We mark
        // any CFA + non-uncompressed combination as undecodable for safety.
        bool isHasselbladRaw = photometric == 32803 && compression != 1;
        bool canDecode = !isHasselbladRaw
                         && compression is 1 or 5 or 7 or 8 or 32773 or 32946
                         && bitsPerSample is 1 or 8 or 16;

        var pf = (samplesPerPixel, bitsPerSample) switch
        {
            (1, 8) => PixelFormat.Gray8,
            (1, 16) => PixelFormat.Gray16,
            (3, _) => PixelFormat.Rgb24,
            _ => PixelFormat.Unknown,
        };

        return new ThreeFrSubImageInfo
        {
            Width = width,
            Height = height,
            BitsPerSample = bitsPerSample,
            SamplesPerPixel = samplesPerPixel,
            CompressionTag = compression,
            Photometric = photometric,
            NewSubFileType = newSubFileType,
            PixelFormat = pf,
            SubIfdLevel = subIfdLevel,
            CanDecodePixels = canDecode,
        };
    }

    private static ThreeFrMetadata ParseThreeFrMetadata(IfdEntry[] entries, byte[] bytes, bool le)
    {
        var make = ReadAsciiTag(entries, 0x010F, bytes);
        var model = ReadAsciiTag(entries, 0x0110, bytes);
        var software = ReadAsciiTag(entries, 0x0131, bytes);
        var dateTime = ReadAsciiTag(entries, 0x0132, bytes);
        var artist = ReadAsciiTag(entries, 0x013B, bytes);
        var copyright = ReadAsciiTag(entries, 0x8298, bytes);
        int makerNoteLen = GetTagByteLength(entries, 0x927C);
        _ = le;

        return new ThreeFrMetadata
        {
            Make = make,
            Model = model,
            Software = software,
            DateTime = dateTime,
            Artist = artist,
            Copyright = copyright,
            MakerNoteLength = makerNoteLen,
        };
    }

    private static ImageMetadata BuildImageMetadata(ThreeFrMetadata tfr)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        if (tfr.MakerNoteLength > 0)
        {
            tags["3FR:MakerNoteLength"] = tfr.MakerNoteLength.ToString(System.Globalization.CultureInfo.InvariantCulture);
        }

        return new ImageMetadata
        {
            CameraMake = tfr.Make,
            CameraModel = tfr.Model,
            Software = tfr.Software,
            CapturedAtRaw = tfr.DateTime,
            Author = tfr.Artist,
            Copyright = tfr.Copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    private static IfdEntry[] ParseIfd(byte[] b, bool le, int offset)
    {
        if (offset < 0 || offset + 2 > b.Length) throw new ImageFormatException("Bad IFD offset.");
        int n = ReadU16(b, offset, le);
        if (offset + 2 + n * 12 > b.Length) throw new ImageFormatException("IFD truncated.");
        var arr = new IfdEntry[n];
        for (int i = 0; i < n; i++)
        {
            int o = offset + 2 + i * 12;
            arr[i] = new IfdEntry(
                ReadU16(b, o, le),
                ReadU16(b, o + 2, le),
                ReadU32(b, o + 4, le),
                ReadU32(b, o + 8, le));
        }
        return arr;
    }

    private static uint GetScalar(IfdEntry[] ifd, int tag, uint def = 0)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            if (e.Type == 3) return e.ValueOffset & 0xFFFF;
            return e.ValueOffset;
        }
        return def;
    }

    private static int GetTagByteLength(IfdEntry[] ifd, int tag)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            return (int)e.Count;
        }
        return 0;
    }

    private static uint[] ReadLongArray(IfdEntry e, byte[] b, bool le)
    {
        int n = (int)e.Count;
        if (n == 0) return [];
        if (n == 1) return [e.ValueOffset];
        var arr = new uint[n];
        for (int k = 0; k < n; k++)
        {
            arr[k] = ReadU32(b, (int)e.ValueOffset + k * 4, le);
        }
        return arr;
    }

    private static ushort[] GetShortArray(IfdEntry[] ifd, int tag, byte[] b, bool le)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            var arr = new ushort[n];
            if (n == 0) return arr;
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
        return [];
    }

    private static string? ReadAsciiTag(IfdEntry[] ifd, int tag, byte[] b)
    {
        foreach (var e in ifd)
        {
            if (e.Tag != tag) continue;
            int n = (int)e.Count;
            if (n == 0) return string.Empty;
            string raw;
            if (n <= 4)
            {
                Span<byte> tmp = stackalloc byte[4];
                BinaryPrimitives.WriteUInt32LittleEndian(tmp, e.ValueOffset);
                while (n > 0 && tmp[n - 1] == 0) n--;
                raw = Encoding.ASCII.GetString(tmp[..n]);
            }
            else
            {
                if (e.ValueOffset + n > b.Length) return null;
                while (n > 0 && b[e.ValueOffset + n - 1] == 0) n--;
                raw = Encoding.ASCII.GetString(b, (int)e.ValueOffset, n);
            }
            return raw;
        }
        return null;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static ushort ReadU16(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o))
           : BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(o));

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static uint ReadU32(byte[] b, int o, bool le) =>
        le ? BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o))
           : BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));

    internal readonly record struct IfdEntry(int Tag, int Type, uint Count, uint ValueOffset);
}
