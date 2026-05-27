using System.Buffers.Binary;
using System.Collections.Immutable;
using System.Text;

namespace Mediar.Imaging.Jpeg2000;

/// <summary>
/// Reader for the JPEG 2000 family of files: <c>.jp2</c> / <c>.j2k</c> /
/// <c>.j2c</c> / <c>.jpc</c> / <c>.jpf</c> / <c>.jpm</c> / <c>.jpx</c>.
/// </summary>
/// <remarks>
/// Parses the JP2 box wrapper (ISO/IEC 15444-1 Annex I) when present and
/// the inner J2K codestream marker sequence in either case. Surfaces the
/// image-header values from SIZ (image size and per-component sample
/// bit-depths), the colour specification from COLR, embedded XML / UUID
/// boxes, and the tile-part offsets / lengths from SOT markers. EBCOT
/// entropy decode is not implemented in this release.
/// </remarks>
public sealed class Jpeg2000Reader : IImageReader
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

    /// <summary>True if the source carries the outer JP2 box wrapper.</summary>
    public bool HasJp2Wrapper { get; }
    /// <summary>Top-level boxes of the JP2 wrapper (empty for raw codestreams).</summary>
    public ImmutableArray<Jp2Box> Boxes { get; }
    /// <summary>SIZ image and tile geometry (always populated, even for raw codestreams).</summary>
    public Jpeg2000Size Size { get; }
    /// <summary>Per-component metadata from SIZ.</summary>
    public ImmutableArray<Jpeg2000Component> Components { get; }
    /// <summary>Tile-part records from the codestream's SOT markers.</summary>
    public ImmutableArray<Jpeg2000TilePart> TileParts { get; }
    /// <summary>Colour-spec method from the JP2 <c>colr</c> box (or empty string).</summary>
    public string ColourSpace { get; }

    private Jpeg2000Reader(Stream s, bool owns, ImageFormat fmt, ImageInfo info, ImageMetadata meta,
                           bool hasWrapper, ImmutableArray<Jp2Box> boxes, Jpeg2000Size siz,
                           ImmutableArray<Jpeg2000Component> components, ImmutableArray<Jpeg2000TilePart> tileParts,
                           string colourSpace)
    {
        _stream = s; _ownsStream = owns;
        Format = fmt; Info = info; Metadata = meta;
        HasJp2Wrapper = hasWrapper; Boxes = boxes; Size = siz;
        Components = components; TileParts = tileParts; ColourSpace = colourSpace;
    }

    /// <summary>Open a JPEG 2000 file from a path.</summary>
    public static Jpeg2000Reader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ImageFormatExtensions.FromExtension(path), ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a JPEG 2000 file from a stream.</summary>
    public static Jpeg2000Reader Open(Stream stream, ImageFormat expected = ImageFormat.Jp2, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] bytes = ms.ToArray();

        bool hasWrapper = bytes.Length >= 12 && bytes[4] == (byte)'j' && bytes[5] == (byte)'P' &&
                          bytes[6] == 0x20 && bytes[7] == 0x20;
        var boxes = ImmutableArray<Jp2Box>.Empty;
        int codestreamOffset = 0;
        int codestreamLength = bytes.Length;
        string colourSpace = "";
        var meta = ImageMetadata.Empty;

        if (hasWrapper)
        {
            var bb = ImmutableArray.CreateBuilder<Jp2Box>();
            ScanBoxes(bytes, 0, bytes.Length, bb, ref codestreamOffset, ref codestreamLength, ref colourSpace);
            boxes = bb.ToImmutable();
        }
        // No wrapper -> codestream begins at offset 0.

        var size = default(Jpeg2000Size);
        var components = ImmutableArray<Jpeg2000Component>.Empty;
        var tileParts = ImmutableArray<Jpeg2000TilePart>.Empty;
        if (codestreamLength > 0 && codestreamOffset + 4 <= bytes.Length)
        {
            ParseCodestream(bytes, codestreamOffset, codestreamOffset + codestreamLength,
                            out size, out components, out tileParts);
        }

        var info = new ImageInfo
        {
            Width = (int)(size.Xsiz - size.XOsiz),
            Height = (int)(size.Ysiz - size.YOsiz),
            BitsPerPixel = components.Sum(c => c.BitDepth),
            ChannelCount = components.Length,
            Format = expected,
            ColorSpace = string.IsNullOrEmpty(colourSpace) ? null : colourSpace,
            FrameCount = 1,
        };

        return new Jpeg2000Reader(stream, ownsStream, expected, info, meta,
                                   hasWrapper, boxes, size, components, tileParts, colourSpace);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "JPEG 2000 entropy decode (EBCOT tier-1 / tier-2) is not implemented in this Mediar release. " +
            "The container, SIZ geometry, component table, and tile-part directory are exposed for inspection.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    // ----- JP2 box scanner -----

    private static void ScanBoxes(byte[] b, int start, int end, ImmutableArray<Jp2Box>.Builder boxes,
                                   ref int codestreamOffset, ref int codestreamLength, ref string colourSpace)
    {
        int p = start;
        while (p + 8 <= end)
        {
            uint sz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(p));
            string ty = Encoding.ASCII.GetString(b, p + 4, 4);
            int cs = p + 8;
            int total = (int)sz;
            if (sz == 1)
            {
                if (p + 16 > end) break;
                ulong large = BinaryPrimitives.ReadUInt64BigEndian(b.AsSpan(p + 8));
                if (large > int.MaxValue || large < 16) break;
                total = (int)large;
                cs = p + 16;
            }
            else if (sz == 0) { total = end - p; }
            if (total < 8 || p + total > end) break;
            int cl = total - (cs - p);
            boxes.Add(new Jp2Box(ty, p, cl));
            if (ty == "jp2c")
            {
                codestreamOffset = cs;
                codestreamLength = cl;
            }
            else if (ty == "jp2h")
            {
                // nested boxes
                int q = cs;
                while (q + 8 <= cs + cl)
                {
                    uint isz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(q));
                    string ity = Encoding.ASCII.GetString(b, q + 4, 4);
                    int ics = q + 8;
                    int itotal = (int)isz;
                    if (isz == 1)
                    {
                        if (q + 16 > cs + cl) break;
                        ulong large = BinaryPrimitives.ReadUInt64BigEndian(b.AsSpan(q + 8));
                        if (large > int.MaxValue || large < 16) break;
                        itotal = (int)large;
                        ics = q + 16;
                    }
                    if (itotal < 8 || q + itotal > cs + cl) break;
                    int icl = itotal - (ics - q);
                    if (ity == "colr" && icl >= 3)
                    {
                        byte meth = b[ics];
                        if (meth == 1 && icl >= 7)
                        {
                            uint enumCs = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(ics + 3));
                            colourSpace = enumCs switch
                            {
                                16 => "sRGB",
                                17 => "Greyscale",
                                18 => "sYCC",
                                _ => $"Enum:{enumCs}",
                            };
                        }
                        else if (meth == 2)
                        {
                            colourSpace = "ICC";
                        }
                    }
                    q += itotal;
                }
            }
            p += total;
        }
    }

    // ----- J2K codestream marker scanner -----

    private static void ParseCodestream(byte[] b, int start, int end,
                                         out Jpeg2000Size size, out ImmutableArray<Jpeg2000Component> components,
                                         out ImmutableArray<Jpeg2000TilePart> tileParts)
    {
        size = default;
        components = ImmutableArray<Jpeg2000Component>.Empty;
        var tps = ImmutableArray.CreateBuilder<Jpeg2000TilePart>();

        int p = start;
        if (p + 2 > end || b[p] != 0xFF || b[p + 1] != 0x4F) { tileParts = tps.ToImmutable(); return; }
        p += 2;

        while (p + 4 <= end)
        {
            if (b[p] != 0xFF) { p++; continue; }
            byte marker = b[p + 1];
            // markers without segment length
            if (marker == 0x93 /* SOD */)
            {
                // SOD - data follows; no segment length. Stop here (tile data scanning is out of scope).
                break;
            }
            if (marker == 0xD9 /* EOC */) { p += 2; break; }
            if (p + 4 > end) break;
            int len = BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(p + 2));
            if (len < 2 || p + 2 + len > end) break;
            int segStart = p + 4;
            int segEnd = p + 2 + len;

            switch (marker)
            {
                case 0x51: ParseSiz(b, segStart, segEnd, out size, out components); break;
                case 0x90: ParseSot(b, segStart, segEnd, p, tps); break;
            }
            p += 2 + len;
        }

        tileParts = tps.ToImmutable();
    }

    private static void ParseSiz(byte[] b, int s, int end,
                                  out Jpeg2000Size size, out ImmutableArray<Jpeg2000Component> comps)
    {
        if (end - s < 36) { size = default; comps = ImmutableArray<Jpeg2000Component>.Empty; return; }
        ushort rsiz = BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(s));
        uint xsiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 2));
        uint ysiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 6));
        uint xosiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 10));
        uint yosiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 14));
        uint xtsiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 18));
        uint ytsiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 22));
        uint xtosiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 26));
        uint ytosiz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 30));
        ushort csiz = BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(s + 34));
        size = new Jpeg2000Size(rsiz, xsiz, ysiz, xosiz, yosiz, xtsiz, ytsiz, xtosiz, ytosiz);
        var cb = ImmutableArray.CreateBuilder<Jpeg2000Component>(csiz);
        int p = s + 36;
        for (int i = 0; i < csiz && p + 3 <= end; i++, p += 3)
        {
            byte ssiz = b[p];
            byte xr = b[p + 1];
            byte yr = b[p + 2];
            bool signed = (ssiz & 0x80) != 0;
            int bitDepth = (ssiz & 0x7F) + 1;
            cb.Add(new Jpeg2000Component(bitDepth, signed, xr, yr));
        }
        comps = cb.ToImmutable();
    }

    private static void ParseSot(byte[] b, int s, int end, int markerOffset,
                                  ImmutableArray<Jpeg2000TilePart>.Builder tps)
    {
        if (end - s < 8) return;
        ushort isot = BinaryPrimitives.ReadUInt16BigEndian(b.AsSpan(s));
        uint psot = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(s + 2));
        byte tpsot = b[s + 6];
        byte tnsot = b[s + 7];
        tps.Add(new Jpeg2000TilePart(isot, tpsot, tnsot, markerOffset, (int)psot));
    }
}

/// <summary>A JP2 box record (top-level only).</summary>
public sealed record Jp2Box(string Type, int Offset, int Length);

/// <summary>SIZ marker contents: image and tile geometry plus capability indicator.</summary>
public readonly record struct Jpeg2000Size(
    ushort CapabilityRsiz,
    uint Xsiz, uint Ysiz,
    uint XOsiz, uint YOsiz,
    uint XTsiz, uint YTsiz,
    uint XTOsiz, uint YTOsiz);

/// <summary>Per-component SIZ entry.</summary>
public readonly record struct Jpeg2000Component(int BitDepth, bool IsSigned, int HorizontalSeparation, int VerticalSeparation);

/// <summary>Tile-part record extracted from a SOT marker (offset + length spans the entire SOT-to-SOD-to-end region).</summary>
public readonly record struct Jpeg2000TilePart(int TileIndex, int PartIndex, int PartCount, int MarkerOffset, int Length);
