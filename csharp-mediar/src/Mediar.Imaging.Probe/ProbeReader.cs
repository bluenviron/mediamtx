using System.Buffers.Binary;
using System.Collections.Frozen;
using System.IO.Compression;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Probe;

/// <summary>
/// Reader that handles the long tail of image formats that Mediar
/// recognises but for which a full pixel-level decoder is not practical
/// in a single library (HEIF / AVIF / BPG / JXL / JXR / JP2 family,
/// camera RAW, DJVU / ECW / DICOM / SVS / BIF, vector + metafile
/// formats, and the various proprietary editor formats).
/// </summary>
public sealed class ProbeReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format { get; }

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    private ProbeReader(Stream s, bool owns, ImageFormat fmt, ImageInfo info, ImageMetadata meta)
    {
        _stream = s; _ownsStream = owns;
        Format = fmt; Info = info; Metadata = meta;
    }

    /// <summary>Open any header-only / probe-only format from a path.</summary>
    public static ProbeReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ImageFormatExtensions.FromExtension(path), ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open any header-only / probe-only format from a stream.</summary>
    public static ProbeReader Open(Stream stream, ImageFormat expected = ImageFormat.Unknown, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        var fmt = expected;
        if (fmt == ImageFormat.Unknown)
        {
            fmt = ImageFormatDetector.Detect(bytes);
        }

        (int W, int H, int Bpp, bool Hdr, int Frames, string? ColorSpace, ImageMetadata Meta) hdr = fmt switch
        {
            ImageFormat.Heic or ImageFormat.Heif or ImageFormat.Avif or ImageFormat.Cr3 => ProbeIsoBmff(bytes, fmt),
            ImageFormat.Jp2 or ImageFormat.J2k or ImageFormat.J2c or ImageFormat.Jpc
              or ImageFormat.Jpf or ImageFormat.Jpm or ImageFormat.Jpx => ProbeJpeg2000(bytes),
            ImageFormat.Jxl => ProbeJxl(bytes),
            ImageFormat.Jxr => ProbeJxr(bytes),
            ImageFormat.Bpg => ProbeBpg(bytes),
            ImageFormat.Flif => ProbeFlif(bytes),
            ImageFormat.Mng => ProbeMng(bytes),
            ImageFormat.Emf => ProbeEmf(bytes),
            ImageFormat.Wmf or ImageFormat.Apm => ProbeWmf(bytes),
            ImageFormat.Emz or ImageFormat.Wmz or ImageFormat.Svgz => ProbeGzipped(bytes),
            ImageFormat.Dicom => ProbeDicom(bytes),
            ImageFormat.Djvu => ProbeDjvu(bytes),
            ImageFormat.Svs => ProbeSvs(bytes),
            _ => (0, 0, 0, false, 0, null, ImageMetadata.Empty),
        };

        var info = new ImageInfo
        {
            Width = hdr.W,
            Height = hdr.H,
            BitsPerPixel = hdr.Bpp,
            ChannelCount = 0,
            PixelFormat = PixelFormat.Unknown,
            Format = fmt,
            IsHdr = hdr.Hdr,
            FrameCount = hdr.Frames,
            ColorSpace = hdr.ColorSpace,
        };
        return new ProbeReader(stream, ownsStream, fmt, info, hdr.Meta);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default)
    {
        throw new NotSupportedException(
            $"Pixel decoding for {Format} is not implemented in this Mediar release. " +
            $"Inspect Info / Metadata for dimensions and tags.");
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeIsoBmff(byte[] b, ImageFormat fmt)
    {
        int w = 0, h = 0;
        string? colorSpace = null;
        Scan(b, 0, b.Length);
        void Scan(byte[] buf, int start, int len)
        {
            int end = start + len;
            int q = start;
            while (q + 8 <= end)
            {
                uint sz = BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(q));
                string ty = Encoding.ASCII.GetString(buf, q + 4, 4);
                int contentStart = q + 8;
                int contentLen = (int)sz - 8;
                if (sz == 1 && q + 16 <= end)
                {
                    ulong large = BinaryPrimitives.ReadUInt64BigEndian(buf.AsSpan(q + 8));
                    sz = (uint)large;
                    contentStart = q + 16;
                    contentLen = (int)sz - 16;
                }
                if (sz < 8 || q + sz > end) break;
                if (ty == "ispe" && contentLen >= 12 && w == 0)
                {
                    w = (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(contentStart + 4));
                    h = (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(contentStart + 8));
                }
                else if (ty == "colr" && contentLen >= 4)
                {
                    string ct = Encoding.ASCII.GetString(buf, contentStart, 4);
                    if (ct == "nclx") colorSpace = "nclx";
                    else if (ct == "prof") colorSpace = "icc";
                }
                else if (ty is "meta" or "iprp" or "ipco" or "moov" or "trak" or "mdia" or "minf" or "stbl")
                {
                    int skipPrefix = ty == "meta" ? 4 : 0;
                    Scan(buf, contentStart + skipPrefix, contentLen - skipPrefix);
                }
                q += (int)sz;
            }
        }
        _ = fmt;
        return (w, h, 0, false, 1, colorSpace, ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeJpeg2000(byte[] b)
    {
        if (b.Length >= 4 && b[0] == 0xFF && b[1] == 0x4F)
        {
            int p = 2;
            while (p + 4 <= b.Length)
            {
                if (b[p] != 0xFF) { p++; continue; }
                int marker = (b[p] << 8) | b[p + 1];
                if (marker == 0xFF51 && p + 16 <= b.Length)
                {
                    int Xsiz = ReadI32Be(b, p + 8);
                    int Ysiz = ReadI32Be(b, p + 12);
                    return (Xsiz, Ysiz, 0, false, 1, "J2K", ImageMetadata.Empty);
                }
                int len = (b[p + 2] << 8) | b[p + 3];
                if (len < 2) break;
                p += 2 + len;
            }
            return (0, 0, 0, false, 1, "J2K", ImageMetadata.Empty);
        }
        int idx = IndexOf(b, "ihdr"u8);
        if (idx > 4 && idx + 12 <= b.Length)
        {
            int h = ReadI32Be(b, idx + 4);
            int w = ReadI32Be(b, idx + 8);
            return (w, h, 0, false, 1, "JP2", ImageMetadata.Empty);
        }
        return (0, 0, 0, false, 1, "JP2", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeJxl(byte[] b)
    {
        _ = b;
        return (0, 0, 0, false, 1, "JXL", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeJxr(byte[] b)
    {
        if (b.Length < 8 || b[0] != (byte)'I' || b[1] != (byte)'I')
            return (0, 0, 0, false, 1, "JXR", ImageMetadata.Empty);
        uint ifdOffset = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(4));
        if (ifdOffset == 0 || ifdOffset + 2 > b.Length)
            return (0, 0, 0, false, 1, "JXR", ImageMetadata.Empty);
        int n = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan((int)ifdOffset));
        int w = 0, h = 0;
        for (int i = 0; i < n; i++)
        {
            int o = (int)ifdOffset + 2 + i * 12;
            if (o + 12 > b.Length) break;
            int tag = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o));
            uint val = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o + 8));
            if (tag == 0xBC80) w = (int)val;
            else if (tag == 0xBC81) h = (int)val;
        }
        return (w, h, 0, false, 1, "JXR", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeBpg(byte[] b)
    {
        if (b.Length < 5 || b[0] != (byte)'B' || b[1] != (byte)'P' || b[2] != (byte)'G' || b[3] != 0xFB)
            return (0, 0, 0, false, 1, "BPG", ImageMetadata.Empty);
        int p = 5;
        int w = ReadUe7(b, ref p);
        int h = ReadUe7(b, ref p);
        return (w, h, 0, false, 1, "BPG", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeFlif(byte[] b)
    {
        if (b.Length < 6) return (0, 0, 0, false, 1, "FLIF", ImageMetadata.Empty);
        int p = 6;
        int w = ReadVarLen(b, ref p) + 1;
        int h = ReadVarLen(b, ref p) + 1;
        return (w, h, 0, false, 1, "FLIF", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeMng(byte[] b)
    {
        if (b.Length < 28) return (0, 0, 0, false, 0, "MNG", ImageMetadata.Empty);
        int w = ReadI32Be(b, 16);
        int h = ReadI32Be(b, 20);
        return (w, h, 0, false, 0, "MNG", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeEmf(byte[] b)
    {
        if (b.Length < 88) return (0, 0, 0, false, 1, "EMF", ImageMetadata.Empty);
        int left = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(8));
        int top = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(12));
        int right = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(16));
        int bottom = BinaryPrimitives.ReadInt32LittleEndian(b.AsSpan(20));
        return (right - left, bottom - top, 0, false, 1, "EMF", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeWmf(byte[] b)
    {
        if (b.Length < 22) return (0, 0, 0, false, 1, "WMF", ImageMetadata.Empty);
        if (b[0] == 0xD7 && b[1] == 0xCD && b[2] == 0xC6 && b[3] == 0x9A)
        {
            int left = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(6));
            int top = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(8));
            int right = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(10));
            int bottom = BinaryPrimitives.ReadInt16LittleEndian(b.AsSpan(12));
            return (right - left, bottom - top, 0, false, 1, "WMF", ImageMetadata.Empty);
        }
        return (0, 0, 0, false, 1, "WMF", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeGzipped(byte[] b)
    {
        try
        {
            using var ms = new MemoryStream(b);
            using var gz = new GZipStream(ms, CompressionMode.Decompress);
            using var output = new MemoryStream();
            gz.CopyTo(output);
            var inner = output.ToArray();
            var fmt = ImageFormatDetector.Detect(inner);
            if (fmt == ImageFormat.Emf) return ProbeEmf(inner);
            if (fmt == ImageFormat.Wmf || fmt == ImageFormat.Apm) return ProbeWmf(inner);
            return (0, 0, 0, false, 1, fmt.ToString(), ImageMetadata.Empty);
        }
        catch
        {
            return (0, 0, 0, false, 1, null, ImageMetadata.Empty);
        }
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeDicom(byte[] b)
    {
        if (b.Length < 132 || b[128] != (byte)'D' || b[129] != (byte)'I' ||
            b[130] != (byte)'C' || b[131] != (byte)'M')
        {
            return (0, 0, 0, false, 1, "DICOM", ImageMetadata.Empty);
        }
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        int p = 132;
        int width = 0, height = 0, bitsAllocated = 0;
        while (p + 8 <= b.Length)
        {
            ushort group = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(p));
            ushort element = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(p + 2));
            string vr = Encoding.ASCII.GetString(b, p + 4, 2);
            int valOff, valLen;
            if (vr is "OB" or "OW" or "OF" or "SQ" or "UT" or "UN")
            {
                if (p + 12 > b.Length) break;
                valLen = (int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(p + 8));
                valOff = p + 12;
            }
            else
            {
                valLen = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(p + 6));
                valOff = p + 8;
            }
            if (valLen < 0 || valOff + valLen > b.Length) break;

            if (group == 0x0028)
            {
                switch (element)
                {
                    case 0x0010: height = ReadDicomInt(b, valOff, valLen); break;
                    case 0x0011: width = ReadDicomInt(b, valOff, valLen); break;
                    case 0x0100: bitsAllocated = ReadDicomInt(b, valOff, valLen); break;
                }
            }
            if (group == 0x0010 || group == 0x0008)
            {
                string key = $"DICOM:({group:X4},{element:X4})";
                tags[key] = Encoding.ASCII.GetString(b, valOff, valLen).TrimEnd('\0', ' ');
            }
            p = valOff + valLen;
            if (group == 0x7FE0 && element == 0x0010) break;
        }
        var meta = new ImageMetadata
        {
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
        return (width, height, bitsAllocated, false, 1, "DICOM", meta);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeDjvu(byte[] b)
    {
        if (b.Length < 32) return (0, 0, 0, false, 1, "DJVU", ImageMetadata.Empty);
        int idx = IndexOf(b, "INFO"u8);
        if (idx > 0 && idx + 12 < b.Length)
        {
            int w = BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(idx + 8));
            int h = BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(idx + 10));
            return (w, h, 0, false, 1, "DJVU", ImageMetadata.Empty);
        }
        return (0, 0, 0, false, 1, "DJVU", ImageMetadata.Empty);
    }

    private static (int, int, int, bool, int, string?, ImageMetadata) ProbeSvs(byte[] b)
    {
        if (b.Length < 8 || b[0] != (byte)'I' || b[1] != (byte)'I') return (0, 0, 0, false, 1, "SVS", ImageMetadata.Empty);
        uint ifdOff = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(4));
        if (ifdOff + 2 > b.Length) return (0, 0, 0, false, 1, "SVS", ImageMetadata.Empty);
        int n = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan((int)ifdOff));
        int w = 0, h = 0;
        for (int i = 0; i < n; i++)
        {
            int o = (int)ifdOff + 2 + i * 12;
            if (o + 12 > b.Length) break;
            int tag = BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(o));
            uint val = BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(o + 8));
            if (tag == 0x0100) w = (int)val;
            else if (tag == 0x0101) h = (int)val;
        }
        return (w, h, 0, false, 1, "SVS", ImageMetadata.Empty);
    }

    private static int ReadDicomInt(byte[] b, int off, int len)
    {
        if (len == 2) return BinaryPrimitives.ReadUInt16LittleEndian(b.AsSpan(off));
        if (len == 4) return (int)BinaryPrimitives.ReadUInt32LittleEndian(b.AsSpan(off));
        return int.TryParse(Encoding.ASCII.GetString(b, off, len), out var v) ? v : 0;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ReadI32Be(byte[] b, int o) => BinaryPrimitives.ReadInt32BigEndian(b.AsSpan(o));

    private static int ReadUe7(byte[] b, ref int p)
    {
        int v = 0;
        while (p < b.Length)
        {
            byte by = b[p++];
            v = (v << 7) | (by & 0x7F);
            if ((by & 0x80) == 0) break;
        }
        return v;
    }

    private static int ReadVarLen(byte[] b, ref int p)
    {
        int v = 0;
        for (int i = 0; i < 4 && p < b.Length; i++)
        {
            byte by = b[p++];
            v = (v << 7) | (by & 0x7F);
            if ((by & 0x80) == 0) break;
        }
        return v;
    }

    private static int IndexOf(byte[] hay, ReadOnlySpan<byte> needle)
    {
        for (int i = 0; i + needle.Length <= hay.Length; i++)
        {
            int j = 0;
            for (; j < needle.Length; j++) if (hay[i + j] != needle[j]) break;
            if (j == needle.Length) return i;
        }
        return -1;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
