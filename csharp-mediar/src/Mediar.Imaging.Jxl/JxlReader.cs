using System.Buffers.Binary;
using System.Collections.Immutable;
using System.Text;

namespace Mediar.Imaging.Jxl;

/// <summary>
/// Reader for JPEG XL (ISO/IEC 18181) files. Recognises both the bare
/// codestream signature (0xFF 0x0A) and the ISO-BMFF wrapped container
/// (12-byte JXL signature box followed by jxlc / jxlp data boxes).
/// Decodes the SizeHeader from the leading codestream bytes. Modular and
/// VarDCT entropy decoding are not implemented in this release.
/// </summary>
public sealed class JxlReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Jxl;
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }
    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>True for the 12-byte ISO-BMFF wrapper; false for a bare codestream.</summary>
    public bool HasContainer { get; }
    /// <summary>Top-level JXL boxes (empty for bare codestreams).</summary>
    public ImmutableArray<JxlBox> Boxes { get; }
    /// <summary>Byte offset where the JXL codestream starts.</summary>
    public int CodestreamOffset { get; }
    /// <summary>Byte length of the (first) codestream chunk.</summary>
    public int CodestreamLength { get; }

    private JxlReader(Stream s, bool owns, ImageInfo info, ImageMetadata meta,
                      bool hasContainer, ImmutableArray<JxlBox> boxes,
                      int csOffset, int csLength)
    {
        _stream = s; _ownsStream = owns;
        Info = info; Metadata = meta;
        HasContainer = hasContainer; Boxes = boxes;
        CodestreamOffset = csOffset; CodestreamLength = csLength;
    }

    private static ReadOnlySpan<byte> ContainerSig => [0x00, 0x00, 0x00, 0x0C, 0x4A, 0x58, 0x4C, 0x20, 0x0D, 0x0A, 0x87, 0x0A];

    /// <summary>Open a JXL file from a path.</summary>
    public static JxlReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a JXL file from a stream.</summary>
    public static JxlReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] b = ms.ToArray();

        bool hasContainer = b.Length >= 12 && b.AsSpan(0, 12).SequenceEqual(ContainerSig);
        var boxes = ImmutableArray<JxlBox>.Empty;
        int csOff = 0, csLen = b.Length;

        if (hasContainer)
        {
            var bb = ImmutableArray.CreateBuilder<JxlBox>();
            int p = 12;
            while (p + 8 <= b.Length)
            {
                uint sz = BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(p));
                string ty = Encoding.ASCII.GetString(b, p + 4, 4);
                int cs = p + 8;
                int total = (int)sz;
                if (sz == 1)
                {
                    if (p + 16 > b.Length) break;
                    ulong large = BinaryPrimitives.ReadUInt64BigEndian(b.AsSpan(p + 8));
                    if (large > int.MaxValue || large < 16) break;
                    total = (int)large;
                    cs = p + 16;
                }
                else if (sz == 0) { total = b.Length - p; }
                if (total < 8 || p + total > b.Length) break;
                int cl = total - (cs - p);
                bb.Add(new JxlBox(ty, p, cl));
                if (ty == "jxlc" && csLen == b.Length) { csOff = cs; csLen = cl; }
                else if (ty == "jxlp" && csLen == b.Length) { csOff = cs + 4; csLen = cl - 4; }
                p += total;
            }
            boxes = bb.ToImmutable();
        }
        else
        {
            if (b.Length < 2 || b[0] != 0xFF || b[1] != 0x0A)
                throw new ImageFormatException("Not a JPEG XL file (missing 0xFF 0x0A signature).");
            csOff = 2;
            csLen = b.Length - 2;
        }

        var (w, h) = ParseSizeHeader(b, csOff, csOff + csLen);
        var info = new ImageInfo
        {
            Width = w,
            Height = h,
            Format = ImageFormat.Jxl,
            FrameCount = 1,
        };
        return new JxlReader(stream, ownsStream, info, ImageMetadata.Empty, hasContainer, boxes, csOff, csLen);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "JPEG XL Modular / VarDCT decoding is not implemented in this Mediar release. " +
            "Container box list, codestream location, and image dimensions are exposed.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    // ----- SizeHeader -----
    private static (int W, int H) ParseSizeHeader(byte[] b, int start, int end)
    {
        // For bare codestreams the signature is already consumed (caller passes start=2).
        // For container payloads jxlc/jxlp the codestream begins with the signature.
        if (start + 2 <= end && b[start] == 0xFF && b[start + 1] == 0x0A)
            start += 2;
        if (start >= end) return (0, 0);

        var br = new JxlBitReader(b, start, end);
        bool small = br.ReadBit();
        int h, w;
        if (small)
        {
            int hRaw = br.ReadBits(5);
            h = (hRaw + 1) * 8;
            int ratio = br.ReadBits(3);
            w = ratio switch
            {
                0 => 0,  // explicit width follows; not in small branch though
                1 => h, 2 => (int)(h * 1.2), 3 => (int)(h * (4.0 / 3.0)),
                4 => (int)(h * 1.5), 5 => (int)(h * (16.0 / 9.0)),
                6 => h * 2, 7 => h * 3,
                _ => 0,
            };
        }
        else
        {
            int sizeClass = br.ReadBits(2);
            int bits = sizeClass switch { 0 => 9, 1 => 13, 2 => 18, _ => 30 };
            h = br.ReadBits(bits) + 1;
            int ratio = br.ReadBits(3);
            if (ratio == 0)
            {
                int sizeClassW = br.ReadBits(2);
                int bitsW = sizeClassW switch { 0 => 9, 1 => 13, 2 => 18, _ => 30 };
                w = br.ReadBits(bitsW) + 1;
            }
            else
            {
                w = ratio switch
                {
                    1 => h, 2 => (int)(h * 1.2), 3 => (int)(h * (4.0 / 3.0)),
                    4 => (int)(h * 1.5), 5 => (int)(h * (16.0 / 9.0)),
                    6 => h * 2, 7 => h * 3,
                    _ => 0,
                };
            }
        }
        return (w, h);
    }

    private ref struct JxlBitReader
    {
        private readonly ReadOnlySpan<byte> _data;
        private int _bitPos;

        public JxlBitReader(byte[] buf, int start, int end)
        {
            _data = buf.AsSpan(start, end - start);
            _bitPos = 0;
        }

        public bool ReadBit() => ReadBits(1) != 0;

        public int ReadBits(int n)
        {
            int v = 0;
            for (int i = 0; i < n; i++)
            {
                int byteIdx = _bitPos >> 3;
                if (byteIdx >= _data.Length) return v;
                int bit = (_data[byteIdx] >> (_bitPos & 7)) & 1;
                v |= bit << i;
                _bitPos++;
            }
            return v;
        }
    }
}

/// <summary>A box in the JPEG XL ISO-BMFF container.</summary>
public sealed record JxlBox(string Type, int Offset, int PayloadLength);
