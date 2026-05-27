using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Imaging.Png;

namespace Mediar.Imaging.Mng;

/// <summary>
/// Reader for Multiple-image Network Graphics (MNG) files. Parses the
/// MNG-1.0 chunk grammar (MHDR + DEFI/FRAM/LOOP/ENDL/BACK/TERM/SAVE/SEEK/
/// pHYg/eXPI/fPRI/nEED/MAGN), then extracts every embedded PNG datastream
/// (IHDR..IEND) and delegates pixel decoding to <see cref="PngReader"/>.
/// </summary>
/// <remarks>
/// JNG (JPEG Network Graphics, signature <c>\x8B JNG \r\n \x1A \n</c>) sub-streams
/// are recognised and surfaced in <see cref="EmbeddedStreams"/> but their
/// JPEG colour + alpha planes are not yet stitched into frames; only the
/// PNG sub-streams produce <see cref="ImageFrame"/> instances right now.
/// </remarks>
public sealed class MngReader : IImageReader
{
    private static ReadOnlySpan<byte> MngSignature => [0x8A, 0x4D, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];

    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly IReadOnlyList<MngEmbeddedImage> _streams;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Mng;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>Frames-per-second declared in the MHDR (TicksPerSecond / Layer-aware).</summary>
    public int TicksPerSecond { get; }

    /// <summary>Number of layers declared by the MHDR header.</summary>
    public int NominalLayerCount { get; }

    /// <summary>Number of frames declared by the MHDR header.</summary>
    public int NominalFrameCount { get; }

    /// <summary>Total play time (in ticks) declared by the MHDR header. 0 = unbounded.</summary>
    public int NominalPlayTimeTicks { get; }

    /// <summary>MNG profile bitfield (combination of CompressFastVariant / CompressFullVariant / TransparencySimple / TransparencyFull / Validity).</summary>
    public uint Profile { get; }

    /// <summary>All embedded PNG/JNG sub-streams in file order.</summary>
    public IReadOnlyList<MngEmbeddedImage> EmbeddedStreams => _streams;

    private MngReader(Stream s, bool owns, byte[] bytes, IReadOnlyList<MngEmbeddedImage> streams,
                     ImageInfo info, ImageMetadata meta, bool canDecode,
                     int ticks, int layers, int frames, int playTime, uint profile)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes; _streams = streams;
        Info = info; Metadata = meta; CanDecodePixels = canDecode;
        TicksPerSecond = ticks;
        NominalLayerCount = layers;
        NominalFrameCount = frames;
        NominalPlayTimeTicks = playTime;
        Profile = profile;
    }

    /// <summary>Open an MNG file by path.</summary>
    public static MngReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an MNG from a stream.</summary>
    public static MngReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < MngSignature.Length || !bytes.AsSpan(0, MngSignature.Length).SequenceEqual(MngSignature))
            throw new ImageFormatException("Not an MNG file (signature mismatch).");

        int pos = MngSignature.Length;
        int width = 0, height = 0, ticks = 0, layers = 0, frames = 0, playTime = 0;
        uint profile = 0;
        var streams = new List<MngEmbeddedImage>();
        var meta = new MetadataBuilder();
        IReadOnlyList<MngEmbeddedImage> emit;

        bool seenMhdr = false;
        bool seenEnd = false;
        byte[]? pendingPng = null;
        int pendingPngLen = 0;
        bool insidePng = false;

        while (pos + 8 <= bytes.Length && !seenEnd)
        {
            uint len = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(pos));
            uint type = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(pos + 4));
            int dataStart = pos + 8;
            if (dataStart + (int)len + 4 > bytes.Length) break;

            string ty = ChunkTypeName(type);

            if (ty == "MHDR" && len >= 28)
            {
                width = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart));
                height = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 4));
                ticks = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 8));
                layers = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 12));
                frames = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 16));
                playTime = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 20));
                profile = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 24));
                seenMhdr = true;
            }
            else if (ty == "IHDR")
            {
                // Begin embedded PNG: write the PNG signature + this IHDR chunk into a buffer.
                pendingPng = new byte[bytes.Length - pos + 8];
                pendingPng[0] = 0x89; pendingPng[1] = 0x50; pendingPng[2] = 0x4E; pendingPng[3] = 0x47;
                pendingPng[4] = 0x0D; pendingPng[5] = 0x0A; pendingPng[6] = 0x1A; pendingPng[7] = 0x0A;
                pendingPngLen = 8;
                AppendChunk(ref pendingPng, ref pendingPngLen, bytes, pos, (int)len);
                insidePng = true;
            }
            else if (ty == "JHDR")
            {
                streams.Add(new MngEmbeddedImage
                {
                    Kind = MngEmbeddedStreamKind.Jng,
                    Width = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart)),
                    Height = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(dataStart + 4)),
                    Offset = pos,
                });
            }
            else if (insidePng)
            {
                AppendChunk(ref pendingPng!, ref pendingPngLen, bytes, pos, (int)len);
                if (ty == "IEND")
                {
                    var pngBytes = new byte[pendingPngLen];
                    Buffer.BlockCopy(pendingPng!, 0, pngBytes, 0, pendingPngLen);
                    streams.Add(new MngEmbeddedImage
                    {
                        Kind = MngEmbeddedStreamKind.Png,
                        Width = ReadIhdrWidth(pngBytes),
                        Height = ReadIhdrHeight(pngBytes),
                        Offset = pos,
                        PngBytes = pngBytes,
                    });
                    insidePng = false;
                    pendingPng = null;
                    pendingPngLen = 0;
                }
            }
            else if (ty == "MEND")
            {
                seenEnd = true;
            }
            else if (ty == "tEXt" || ty == "iTXt" || ty == "zTXt")
            {
                meta.IngestText(bytes, dataStart, (int)len, ty);
            }

            pos = dataStart + (int)len + 4;
        }

        if (!seenMhdr) throw new ImageFormatException("MNG missing MHDR chunk.");

        emit = streams;
        int frameCount = streams.Count(s => s.Kind == MngEmbeddedStreamKind.Png);
        bool canDecode = frameCount > 0;
        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = 32,
            ChannelCount = 4,
            PixelFormat = canDecode ? PixelFormat.Rgba32 : PixelFormat.Unknown,
            Format = ImageFormat.Mng,
            IsAnimated = frameCount > 1,
            FrameCount = frameCount,
        };

        return new MngReader(stream, ownsStream, bytes, emit, info, meta.Build(), canDecode,
                             ticks, layers, frames, playTime, profile);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
        foreach (var sub in _streams)
        {
            if (sub.Kind != MngEmbeddedStreamKind.Png || sub.PngBytes is null) continue;
            cancellationToken.ThrowIfCancellationRequested();
            using var ms = new MemoryStream(sub.PngBytes, writable: false);
            using var png = PngReader.Open(ms, ownsStream: false);
            await foreach (var f in png.ReadFramesAsync(cancellationToken).ConfigureAwait(false))
                yield return f;
        }
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
        _ = _bytes;
    }

    private static int ReadIhdrWidth(byte[] png)
    {
        if (png.Length < 24) return 0;
        return (int)BinaryPrimitives.ReadUInt32BigEndian(png.AsSpan(16));
    }
    private static int ReadIhdrHeight(byte[] png)
    {
        if (png.Length < 24) return 0;
        return (int)BinaryPrimitives.ReadUInt32BigEndian(png.AsSpan(20));
    }

    private static string ChunkTypeName(uint type)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(b, type);
        return Encoding.ASCII.GetString(b);
    }

    private static void AppendChunk(ref byte[] buf, ref int len, byte[] src, int srcPos, int dataLen)
    {
        int total = 8 + dataLen + 4;
        EnsureCapacity(ref buf, len + total);
        Buffer.BlockCopy(src, srcPos, buf, len, total);
        len += total;
    }

    private static void EnsureCapacity(ref byte[] buf, int needed)
    {
        if (buf.Length >= needed) return;
        int cap = Math.Max(buf.Length * 2, needed);
        var bigger = new byte[cap];
        Buffer.BlockCopy(buf, 0, bigger, 0, buf.Length);
        buf = bigger;
    }

    private sealed class MetadataBuilder
    {
        private readonly Dictionary<string, string> _tags = new(StringComparer.OrdinalIgnoreCase);
        private string? _title, _author, _description, _copyright, _software;

        public void IngestText(byte[] bytes, int start, int len, string chunkType)
        {
            int nul = Array.IndexOf(bytes, (byte)0, start, len);
            if (nul < 0) return;
            string key = Encoding.ASCII.GetString(bytes, start, nul - start);
            string val;
            if (chunkType == "tEXt")
            {
                val = Encoding.GetEncoding("ISO-8859-1").GetString(bytes, nul + 1, start + len - nul - 1);
            }
            else
            {
                // iTXt / zTXt — best-effort: skip language tag, decode as UTF-8.
                int p = nul + 3;
                int langEnd = Array.IndexOf(bytes, (byte)0, p, start + len - p);
                if (langEnd < 0) langEnd = p;
                int trEnd = Array.IndexOf(bytes, (byte)0, langEnd + 1, start + len - langEnd - 1);
                if (trEnd < 0) trEnd = langEnd + 1;
                val = Encoding.UTF8.GetString(bytes, trEnd + 1, Math.Max(0, start + len - trEnd - 1));
            }
            _tags[key] = val;
            switch (key)
            {
                case "Title": _title = val; break;
                case "Author": _author = val; break;
                case "Description": _description = val; break;
                case "Copyright": _copyright = val; break;
                case "Software": _software = val; break;
            }
        }

        public ImageMetadata Build() => new()
        {
            Title = _title,
            Author = _author,
            Description = _description,
            Copyright = _copyright,
            Software = _software,
            Tags = _tags.ToFrozenDictionary(StringComparer.OrdinalIgnoreCase),
        };
    }
}

/// <summary>
/// Kind of sub-stream embedded inside an MNG container.
/// </summary>
public enum MngEmbeddedStreamKind
{
    /// <summary>Embedded PNG datastream (IHDR..IEND).</summary>
    Png,

    /// <summary>Embedded JNG (JPEG Network Graphics) datastream.</summary>
    Jng,
}

/// <summary>
/// Information about one sub-stream encountered while parsing an MNG.
/// </summary>
public sealed record MngEmbeddedImage
{
    /// <summary>Type of sub-stream.</summary>
    public MngEmbeddedStreamKind Kind { get; init; }

    /// <summary>Width of the sub-stream's image.</summary>
    public int Width { get; init; }

    /// <summary>Height of the sub-stream's image.</summary>
    public int Height { get; init; }

    /// <summary>Byte offset of the sub-stream's header chunk inside the MNG container.</summary>
    public int Offset { get; init; }

    /// <summary>Re-assembled PNG bytes (for PNG sub-streams), ready to feed to PngReader. Null for JNG.</summary>
    public byte[]? PngBytes { get; init; }
}
