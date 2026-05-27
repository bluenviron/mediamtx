using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using Mediar.Imaging.Metadata;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Reader for JPEG (JFIF / EXIF) files, including the multi-image MPO
/// container and digital-camera THM thumbnails (which are simply JPEGs).
/// </summary>
/// <remarks>
/// The reader parses every JPEG marker segment up to <c>SOS</c>, then
/// stops. Image dimensions, sampling factors and any embedded EXIF /
/// XMP metadata are exposed via <see cref="ImageInfo"/> and
/// <see cref="ImageMetadata"/>. Pixel decoding for SOF0 (baseline DCT)
/// is performed by <see cref="JpegBaselineDecoder"/>.
/// </remarks>
public sealed class JpegReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly JpegFrame _frame;
    private readonly JpegDecoderState _state;
    private readonly byte[] _scanData;
    private readonly List<JpegScan> _scans;
    private readonly ImageMetadata _metadata;
    private readonly ImageFormat _format;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _format;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => _metadata;

    /// <inheritdoc/>
    public bool CanDecodePixels =>
        (_frame.IsBaseline || _frame.IsProgressive || _frame.IsLossless) && _frame.NumberOfComponents is 1 or 3;

    /// <summary>
    /// Returns the EXIF / TIFF tag dictionary verbatim (key prefixes:
    /// <c>IFD0:</c>, <c>Exif:</c>, <c>GPS:</c>).
    /// </summary>
    public IReadOnlyDictionary<string, string> ExifTags => _metadata.Tags;

    private JpegReader(
        Stream stream, bool ownsStream, JpegFrame frame, JpegDecoderState state,
        byte[] scanData, List<JpegScan> scans,
        ImageMetadata metadata, ImageFormat format, ImageInfo info)
    {
        _stream = stream;
        _ownsStream = ownsStream;
        _frame = frame;
        _state = state;
        _scanData = scanData;
        _scans = scans;
        _metadata = metadata;
        _format = format;
        Info = info;
    }

    /// <summary>Open a JPEG file by path.</summary>
    public static JpegReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            var fmt = ImageFormat.Jpeg;
            if (path.EndsWith(".thm", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.Thm;
            else if (path.EndsWith(".mpo", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.Mpo;
            else if (path.EndsWith(".jfif", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.Jfif;
            else if (path.EndsWith(".jpg_large", StringComparison.OrdinalIgnoreCase)) fmt = ImageFormat.JpgLarge;
            return Open(fs, fmt, ownsStream: true);
        }
        catch
        {
            fs.Dispose();
            throw;
        }
    }

    /// <summary>Open a JPEG from a stream.</summary>
    public static JpegReader Open(
        Stream stream, ImageFormat format = ImageFormat.Jpeg, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        Span<byte> two = stackalloc byte[2];
        ReadExactly(stream, two);
        if (two[0] != 0xFF || two[1] != 0xD8)
        {
            throw new ImageFormatException("Not a JPEG file (missing SOI marker).");
        }

        ImageMetadata metadata = ImageMetadata.Empty;
        var frame = new JpegFrame();
        var state = new JpegDecoderState();
        var scans = new List<JpegScan>();
        byte[] scanBytes = [];
        Span<byte> lengthBuf = stackalloc byte[2];

        // Track a "pending" marker that was read by ReadEntropyData but not
        // yet processed by the main marker loop.
        byte pendingMarker = 0;
        bool hasPending = false;

        while (true)
        {
            byte marker;
            if (hasPending)
            {
                marker = pendingMarker;
                hasPending = false;
            }
            else
            {
                byte ff;
                do
                {
                    int r = stream.ReadByte();
                    if (r < 0) throw new ImageFormatException("Truncated JPEG (looking for marker).");
                    ff = (byte)r;
                } while (ff != 0xFF);

                do
                {
                    int r = stream.ReadByte();
                    if (r < 0) throw new ImageFormatException("Truncated JPEG (looking for marker).");
                    marker = (byte)r;
                } while (marker == 0xFF);
            }

            if (marker == 0xD9) break; // EOI
            if (marker == 0xD8) continue; // duplicate SOI
            if (marker is >= 0xD0 and <= 0xD7) continue; // restart (unexpected outside scan)

            Span<byte> _length = lengthBuf;
            ReadExactly(stream, _length);
            int segLen = (_length[0] << 8) | _length[1];
            if (segLen < 2) throw new ImageFormatException("Bad JPEG segment length.");
            byte[] segment = new byte[segLen - 2];
            if (segment.Length > 0) stream.ReadExactly(segment);

            switch (marker)
            {
                case 0xC0: // SOF0 baseline
                case 0xC1: // SOF1 extended sequential
                case 0xC2: // SOF2 progressive
                case 0xC3: // SOF3 lossless
                    ParseSof(segment, frame);
                    frame.IsBaseline = marker == 0xC0;
                    frame.IsProgressive = marker == 0xC2;
                    frame.IsLossless = marker == 0xC3;
                    break;

                case 0xDB: // DQT
                    ParseDqt(segment, state);
                    break;

                case 0xC4: // DHT
                    ParseDht(segment, state);
                    break;

                case 0xDD: // DRI
                    if (segment.Length >= 2)
                    {
                        state.RestartInterval = (segment[0] << 8) | segment[1];
                    }
                    break;

                case 0xE0: // APP0 (JFIF)
                    break;

                case 0xE1: // APP1 (EXIF / XMP)
                    if (segment.Length > 6 &&
                        segment[0] == (byte)'E' && segment[1] == (byte)'x' &&
                        segment[2] == (byte)'i' && segment[3] == (byte)'f' &&
                        segment[4] == 0x00 && segment[5] == 0x00)
                    {
                        metadata = ExifParser.Parse(segment.AsSpan(6));
                    }
                    break;

                case 0xE2: // APP2 (MPO multi-image / ICC profile)
                    if (segment.Length > 4 &&
                        segment[0] == (byte)'M' && segment[1] == (byte)'P' &&
                        segment[2] == (byte)'F' && segment[3] == 0x00)
                    {
                        format = ImageFormat.Mpo;
                    }
                    break;

                case 0xDA: // SOS — scan header followed by entropy-coded segment.
                    {
                        ParseSos(segment, state, frame);
                        // For baseline / lossless scans, capture entropy bytes simply.
                        // For progressive, capture per-scan info AND entropy bytes that stop
                        // at the next non-restart marker so we can process more segments.
                        if (frame.IsProgressive)
                        {
                            var scanInfo = new JpegScan
                            {
                                ComponentIds = (byte[])state.ScanComponentIds.Clone(),
                                DcTables = (byte[])state.ScanDcTables.Clone(),
                                AcTables = (byte[])state.ScanAcTables.Clone(),
                                Ss = state.ScanSs,
                                Se = state.ScanSe,
                                Ah = state.ScanAh,
                                Al = state.ScanAl,
                                RestartInterval = state.RestartInterval,
                            };
                            Array.Copy(state.DcHuffman, scanInfo.DcHuffmanSnapshot, 4);
                            Array.Copy(state.AcHuffman, scanInfo.AcHuffmanSnapshot, 4);
                            scanInfo.EntropyData = ReadEntropyUntilNonRestartMarker(stream, out pendingMarker);
                            hasPending = pendingMarker != 0;
                            scans.Add(scanInfo);
                        }
                        else
                        {
                            scanBytes = ReadRestOfStreamUntilEoi(stream);
                            goto done;
                        }
                    }
                    break;

                default:
                    break;
            }
        }
    done:

        PixelFormat pf;
        if (frame.NumberOfComponents == 1)
        {
            pf = frame.BitsPerSample > 8 ? PixelFormat.Gray16 : PixelFormat.Gray8;
        }
        else
        {
            pf = frame.BitsPerSample > 8 ? PixelFormat.Rgb48 : PixelFormat.Rgb24;
        }
        var info = new ImageInfo
        {
            Width = frame.Width,
            Height = frame.Height,
            BitsPerPixel = frame.BitsPerSample * frame.NumberOfComponents,
            ChannelCount = frame.NumberOfComponents,
            PixelFormat = pf,
            Format = format,
            HasAlpha = false,
            HorizontalDpi = ParseDouble(metadata.Tags, "IFD0:XResolution"),
            VerticalDpi = ParseDouble(metadata.Tags, "IFD0:YResolution"),
            FrameCount = 1,
        };

        return new JpegReader(stream, ownsStream, frame, state, scanBytes, scans, metadata, format, info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        ImageFrame frame;
        if (_frame.IsBaseline)
        {
            frame = JpegBaselineDecoder.Decode(_frame, _state, _scanData);
        }
        else if (_frame.IsProgressive)
        {
            frame = JpegProgressiveDecoder.Decode(_frame, _state, _scans);
        }
        else if (_frame.IsLossless)
        {
            frame = JpegLosslessDecoder.Decode(_frame, _state, _scanData);
        }
        else
        {
            throw new NotSupportedException(
                "JPEG arithmetic-coded decoding is not implemented in this version of Mediar.");
        }
        await Task.CompletedTask.ConfigureAwait(false);
        yield return frame;
    }

    private static void ParseSof(ReadOnlySpan<byte> seg, JpegFrame frame)
    {
        frame.BitsPerSample = seg[0];
        frame.Height = (seg[1] << 8) | seg[2];
        frame.Width = (seg[3] << 8) | seg[4];
        int nf = seg[5];
        frame.NumberOfComponents = nf;
        frame.Components = new JpegComponent[nf];
        int p = 6;
        for (int i = 0; i < nf; i++)
        {
            byte id = seg[p];
            byte sampling = seg[p + 1];
            byte qtab = seg[p + 2];
            frame.Components[i] = new JpegComponent
            {
                Id = id,
                HSampling = sampling >> 4,
                VSampling = sampling & 0x0F,
                QuantTableId = qtab,
            };
            p += 3;
        }
    }

    private static void ParseDqt(ReadOnlySpan<byte> seg, JpegDecoderState state)
    {
        // One or more quant-table definitions back-to-back inside the segment.
        int p = 0;
        while (p < seg.Length)
        {
            byte pq = (byte)(seg[p] >> 4);   // 0 = 8-bit, 1 = 16-bit
            byte tq = (byte)(seg[p] & 0x0F); // table id 0..3
            p++;
            if (tq >= 4) throw new ImageFormatException("Bad DQT table id.");
            var t = new short[64];
            if (pq == 0)
            {
                if (p + 64 > seg.Length) throw new ImageFormatException("Truncated DQT.");
                for (int k = 0; k < 64; k++) t[k] = seg[p + k];
                p += 64;
            }
            else
            {
                if (p + 128 > seg.Length) throw new ImageFormatException("Truncated DQT.");
                for (int k = 0; k < 64; k++)
                {
                    t[k] = (short)((seg[p + 2 * k] << 8) | seg[p + 2 * k + 1]);
                }
                p += 128;
            }
            state.QuantTables[tq] = t;
        }
    }

    private static void ParseDht(ReadOnlySpan<byte> seg, JpegDecoderState state)
    {
        int p = 0;
        while (p < seg.Length)
        {
            byte tc = (byte)(seg[p] >> 4);   // 0 = DC, 1 = AC
            byte th = (byte)(seg[p] & 0x0F); // table id 0..3
            p++;
            if (th >= 4) throw new ImageFormatException("Bad DHT table id.");
            if (p + 16 > seg.Length) throw new ImageFormatException("Truncated DHT.");
            var lengths = seg.Slice(p, 16);
            p += 16;
            int total = 0;
            for (int i = 0; i < 16; i++) total += lengths[i];
            if (p + total > seg.Length) throw new ImageFormatException("Truncated DHT (vals).");
            var values = seg.Slice(p, total);
            p += total;
            var table = HuffmanTable.Build(lengths, values);
            if (tc == 0) state.DcHuffman[th] = table;
            else state.AcHuffman[th] = table;
        }
    }

    private static void ParseSos(ReadOnlySpan<byte> seg, JpegDecoderState state, JpegFrame frame)
    {
        int ns = seg[0];
        if (seg.Length < 1 + ns * 2 + 3) throw new ImageFormatException("Truncated SOS.");
        state.ScanComponentIds = new byte[ns];
        state.ScanDcTables = new byte[ns];
        state.ScanAcTables = new byte[ns];
        int p = 1;
        for (int i = 0; i < ns; i++)
        {
            state.ScanComponentIds[i] = seg[p];
            byte tables = seg[p + 1];
            state.ScanDcTables[i] = (byte)(tables >> 4);
            state.ScanAcTables[i] = (byte)(tables & 0x0F);
            p += 2;
        }
        // Progressive scans use Ss / Se / Ah / Al. For baseline they are 0 / 63 / 0 / 0.
        state.ScanSs = seg[p];
        state.ScanSe = seg[p + 1];
        byte ahAl = seg[p + 2];
        state.ScanAh = ahAl >> 4;
        state.ScanAl = ahAl & 0x0F;
        _ = frame;
    }

    /// <summary>
    /// Reads bytes from <paramref name="s"/> into an entropy-coded segment
    /// until a non-restart, non-stuffed <c>FF xx</c> marker is encountered.
    /// The terminating marker byte is returned in <paramref name="nextMarker"/>;
    /// it is consumed from the stream but not appended to the returned bytes.
    /// FF 00 byte stuffing and RST0..RST7 restart markers are passed through
    /// verbatim (the bit reader strips them as needed).
    /// </summary>
    private static byte[] ReadEntropyUntilNonRestartMarker(Stream s, out byte nextMarker)
    {
        using var ms = new MemoryStream();
        nextMarker = 0;
        int b;
        while ((b = s.ReadByte()) >= 0)
        {
            if (b != 0xFF)
            {
                ms.WriteByte((byte)b);
                continue;
            }
            int n = s.ReadByte();
            if (n < 0)
            {
                ms.WriteByte(0xFF);
                break;
            }
            if (n == 0x00)
            {
                ms.WriteByte(0xFF);
                ms.WriteByte(0x00);
                continue;
            }
            if (n is >= 0xD0 and <= 0xD7)
            {
                // Restart marker — keep it in the stream so the bit reader can resync.
                ms.WriteByte(0xFF);
                ms.WriteByte((byte)n);
                continue;
            }
            // Real marker — stop entropy capture, return it to the caller.
            nextMarker = (byte)n;
            return ms.ToArray();
        }
        return ms.ToArray();
    }

    private static byte[] ReadRestOfStreamUntilEoi(Stream s)
    {
        using var ms = new MemoryStream();
        int b;
        while ((b = s.ReadByte()) >= 0)
        {
            ms.WriteByte((byte)b);
        }
        var arr = ms.ToArray();
        if (arr.Length >= 2 && arr[^2] == 0xFF && arr[^1] == 0xD9)
        {
            return arr[..^2];
        }
        return arr;
    }

    private static double ParseDouble(FrozenDictionary<string, string> tags, string key)
    {
        if (!tags.TryGetValue(key, out var s)) return 0;
        int slash = s.IndexOf('/');
        if (slash > 0 &&
            double.TryParse(s.AsSpan(0, slash), System.Globalization.CultureInfo.InvariantCulture, out var n) &&
            double.TryParse(s.AsSpan(slash + 1), System.Globalization.CultureInfo.InvariantCulture, out var d) &&
            d != 0)
        {
            return n / d;
        }
        return 0;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void ReadExactly(Stream s, Span<byte> dst)
    {
        int read = 0;
        while (read < dst.Length)
        {
            int n = s.Read(dst[read..]);
            if (n <= 0) throw new EndOfStreamException();
            read += n;
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}

/// <summary>JPEG SOFn frame parameters (internal).</summary>
internal sealed class JpegFrame
{
    public bool IsBaseline { get; set; }
    public bool IsProgressive { get; set; }
    public bool IsLossless { get; set; }
    public int Width { get; set; }
    public int Height { get; set; }
    public int BitsPerSample { get; set; } = 8;
    public int NumberOfComponents { get; set; }
    public JpegComponent[] Components { get; set; } = [];
}

/// <summary>Per-component JPEG metadata (internal).</summary>
internal sealed class JpegComponent
{
    public byte Id { get; set; }
    public int HSampling { get; set; } = 1;
    public int VSampling { get; set; } = 1;
    public byte QuantTableId { get; set; }
}

/// <summary>Mutable decoder state populated from marker segments (internal).</summary>
internal sealed class JpegDecoderState
{
    public short[]?[] QuantTables { get; } = new short[4][];
    public HuffmanTable?[] DcHuffman { get; } = new HuffmanTable[4];
    public HuffmanTable?[] AcHuffman { get; } = new HuffmanTable[4];
    public int RestartInterval { get; set; }
    public byte[] ScanComponentIds { get; set; } = [];
    public byte[] ScanDcTables { get; set; } = [];
    public byte[] ScanAcTables { get; set; } = [];
    public int ScanSs { get; set; }
    public int ScanSe { get; set; } = 63;
    public int ScanAh { get; set; }
    public int ScanAl { get; set; }
}

/// <summary>
/// Per-scan parameters captured by the multi-scan JPEG reader so that the
/// progressive decoder can replay each scan in order. Snapshots which
/// Huffman tables were in effect at <c>SOS</c> time, since DHT segments
/// may redefine tables between scans.
/// </summary>
internal sealed class JpegScan
{
    public byte[] ComponentIds { get; init; } = [];
    public byte[] DcTables { get; init; } = [];
    public byte[] AcTables { get; init; } = [];
    public int Ss { get; init; }
    public int Se { get; init; }
    public int Ah { get; init; }
    public int Al { get; init; }
    public int RestartInterval { get; init; }
    public HuffmanTable?[] DcHuffmanSnapshot { get; init; } = new HuffmanTable[4];
    public HuffmanTable?[] AcHuffmanSnapshot { get; init; } = new HuffmanTable[4];
    public byte[] EntropyData { get; set; } = [];
}

