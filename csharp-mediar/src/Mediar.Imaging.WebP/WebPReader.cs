using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.WebP;

/// <summary>
/// Reader for WebP (RFC 9649) files. Parses the RIFF/WEBP outer wrapper
/// plus every container chunk: VP8X, VP8, VP8L, ALPH, ANIM, ANMF, ICCP,
/// EXIF, XMP. Lossless (VP8L) frames are decoded to ARGB through
/// <see cref="Vp8LDecoder"/>; lossy (VP8) frames are recognised but throw
/// <see cref="NotSupportedException"/> from <see cref="ReadFramesAsync"/>
/// until a VP8 codec project lands.
/// </summary>
public sealed class WebPReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly List<WebPChunk> _chunks;
    private readonly List<WebPFrameRecord> _frames;
    private readonly bool _isAnimated;
    private readonly bool _hasAlphaFlag;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.WebP;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels { get; }

    /// <summary>All RIFF chunks in file order.</summary>
    public IReadOnlyList<WebPChunk> Chunks => _chunks;

    /// <summary>True if the container declares VP8X with the alpha flag set.</summary>
    public bool HasAlpha => _hasAlphaFlag;

    /// <summary>Background colour from ANIM (ARGB).</summary>
    public uint BackgroundColor { get; }

    /// <summary>Animation loop count (0 = infinite).</summary>
    public int LoopCount { get; }

    private WebPReader(Stream s, bool owns, byte[] bytes, ImageInfo info, ImageMetadata meta,
                      List<WebPChunk> chunks, List<WebPFrameRecord> frames,
                      bool canDecode, bool isAnimated, bool hasAlpha,
                      uint background, int loopCount)
    {
        _stream = s; _ownsStream = owns; _bytes = bytes;
        Info = info; Metadata = meta;
        _chunks = chunks; _frames = frames;
        CanDecodePixels = canDecode;
        _isAnimated = isAnimated;
        _hasAlphaFlag = hasAlpha;
        BackgroundColor = background;
        LoopCount = loopCount;
    }

    /// <summary>Open a WebP file by path.</summary>
    public static WebPReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a WebP from a stream.</summary>
    public static WebPReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 12
            || bytes[0] != (byte)'R' || bytes[1] != (byte)'I' || bytes[2] != (byte)'F' || bytes[3] != (byte)'F'
            || bytes[8] != (byte)'W' || bytes[9] != (byte)'E' || bytes[10] != (byte)'B' || bytes[11] != (byte)'P')
        {
            throw new ImageFormatException("Not a RIFF/WEBP file.");
        }

        var chunks = new List<WebPChunk>();
        var frames = new List<WebPFrameRecord>();
        var tags = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        string? title = null, copyright = null;
        ReadOnlyMemory<byte> icc = default;
        int canvasWidth = 0, canvasHeight = 0;
        bool hasAlphaFlag = false;
        bool isAnimated = false;
        uint background = 0;
        int loopCount = 1;

        int cursor = 12;
        while (cursor + 8 <= bytes.Length)
        {
            uint fourcc = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(cursor));
            uint size = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(cursor + 4));
            int dataStart = cursor + 8;
            if (dataStart + (int)size > bytes.Length) break;
            string id = FourCcToString(fourcc);
            var chunk = new WebPChunk { FourCC = id, Offset = cursor, DataOffset = dataStart, DataLength = (int)size };
            chunks.Add(chunk);

            switch (id)
            {
                case "VP8X":
                    if (size >= 10)
                    {
                        byte flags = bytes[dataStart];
                        hasAlphaFlag = (flags & 0x10) != 0;
                        isAnimated = (flags & 0x02) != 0;
                        canvasWidth = ReadU24(bytes, dataStart + 4) + 1;
                        canvasHeight = ReadU24(bytes, dataStart + 7) + 1;
                    }
                    break;
                case "VP8L":
                    if (size >= 5)
                    {
                        // bytes[dataStart] should be 0x2F, then 14+14 bits w/h
                        int w = (bytes[dataStart + 1] | (bytes[dataStart + 2] << 8)) & 0x3FFF;
                        int h = ((bytes[dataStart + 2] >> 6) | (bytes[dataStart + 3] << 2) | (bytes[dataStart + 4] << 10)) & 0x3FFF;
                        if (canvasWidth == 0) canvasWidth = w + 1;
                        if (canvasHeight == 0) canvasHeight = h + 1;
                        if ((bytes[dataStart + 4] >> 4 & 1) == 1) hasAlphaFlag = true;
                        frames.Add(new WebPFrameRecord
                        {
                            Kind = WebPFrameKind.Vp8L,
                            Offset = dataStart,
                            Length = (int)size,
                            Width = w + 1,
                            Height = h + 1,
                        });
                    }
                    break;
                case "VP8 ":
                    if (size >= 10)
                    {
                        // VP8 lossy frame header — skip 3-byte frame tag, then read keyframe magic + 14+14 width/height
                        int p = dataStart + 3;
                        if (bytes[p] == 0x9D && bytes[p + 1] == 0x01 && bytes[p + 2] == 0x2A && p + 6 < bytes.Length)
                        {
                            int w = (bytes[p + 3] | (bytes[p + 4] << 8)) & 0x3FFF;
                            int h = (bytes[p + 5] | (bytes[p + 6] << 8)) & 0x3FFF;
                            if (canvasWidth == 0) canvasWidth = w;
                            if (canvasHeight == 0) canvasHeight = h;
                            frames.Add(new WebPFrameRecord
                            {
                                Kind = WebPFrameKind.Vp8,
                                Offset = dataStart,
                                Length = (int)size,
                                Width = w,
                                Height = h,
                            });
                        }
                    }
                    break;
                case "ALPH":
                    // Alpha sub-stream for the matching VP8 frame; recognised, not decoded yet.
                    break;
                case "ANIM":
                    if (size >= 6)
                    {
                        background = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(dataStart));
                        loopCount = BinaryPrimitives.ReadUInt16LittleEndian(bytes.AsSpan(dataStart + 4));
                    }
                    break;
                case "ANMF":
                    if (size >= 16)
                    {
                        int x = ReadU24(bytes, dataStart) * 2;
                        int y = ReadU24(bytes, dataStart + 3) * 2;
                        int w = ReadU24(bytes, dataStart + 6) + 1;
                        int h = ReadU24(bytes, dataStart + 9) + 1;
                        int duration = ReadU24(bytes, dataStart + 12);
                        byte flags = bytes[dataStart + 15];
                        // Walk sub-chunks for VP8L/VP8/ALPH
                        int innerCursor = dataStart + 16;
                        int innerEnd = dataStart + (int)size;
                        WebPFrameKind kind = WebPFrameKind.Unknown;
                        int payloadOffset = innerCursor;
                        int payloadLen = 0;
                        while (innerCursor + 8 <= innerEnd)
                        {
                            uint subFourcc = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(innerCursor));
                            uint subSize = BinaryPrimitives.ReadUInt32LittleEndian(bytes.AsSpan(innerCursor + 4));
                            string subId = FourCcToString(subFourcc);
                            int subStart = innerCursor + 8;
                            if (subId == "VP8L")
                            {
                                kind = WebPFrameKind.Vp8L;
                                payloadOffset = subStart;
                                payloadLen = (int)subSize;
                                break;
                            }
                            if (subId == "VP8 ")
                            {
                                kind = WebPFrameKind.Vp8;
                                payloadOffset = subStart;
                                payloadLen = (int)subSize;
                                break;
                            }
                            innerCursor = subStart + (int)subSize;
                            if ((subSize & 1) == 1) innerCursor++;
                        }
                        frames.Add(new WebPFrameRecord
                        {
                            Kind = kind,
                            Offset = payloadOffset,
                            Length = payloadLen,
                            Width = w,
                            Height = h,
                            X = x,
                            Y = y,
                            DurationMs = duration,
                            BlendOverPrev = (flags & 0x02) == 0,
                            DisposeToBackground = (flags & 0x01) == 1,
                        });
                    }
                    break;
                case "ICCP":
                    icc = bytes.AsMemory(dataStart, (int)size);
                    break;
                case "EXIF":
                    tags["EXIF"] = "(present, " + size + " bytes)";
                    break;
                case "XMP ":
                    tags["XMP"] = Encoding.UTF8.GetString(bytes, dataStart, (int)size);
                    break;
            }
            cursor = dataStart + (int)size;
            if ((size & 1) == 1) cursor++;
        }

        var canDecodeFrames = frames.Any(f => f.Kind == WebPFrameKind.Vp8L);
        var pf = hasAlphaFlag ? PixelFormat.Bgra32 : PixelFormat.Bgra32;  // We emit BGRA from ARGB swap

        var info = new ImageInfo
        {
            Width = canvasWidth,
            Height = canvasHeight,
            BitsPerPixel = 32,
            ChannelCount = hasAlphaFlag ? 4 : 3,
            PixelFormat = canDecodeFrames ? pf : PixelFormat.Unknown,
            Format = ImageFormat.WebP,
            HasAlpha = hasAlphaFlag,
            IsAnimated = isAnimated,
            FrameCount = isAnimated ? frames.Count(f => f.X != 0 || f.Y != 0 || f.DurationMs != 0 || frames.Count > 1) : 1,
            IccProfile = icc,
        };

        var meta = new ImageMetadata
        {
            Title = title,
            Copyright = copyright,
            Tags = tags.ToFrozenDictionary(StringComparer.OrdinalIgnoreCase),
        };

        return new WebPReader(stream, ownsStream, bytes, info, meta, chunks, frames,
                              canDecodeFrames, isAnimated, hasAlphaFlag,
                              background, loopCount);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
        await Task.CompletedTask.ConfigureAwait(false);
        foreach (var f in _frames)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (f.Kind == WebPFrameKind.Vp8L)
            {
                var src = new ReadOnlySpan<byte>(_bytes, f.Offset, f.Length);
                var decoded = Vp8LDecoder.Decode(src);
                var (frame, buf) = ImageFrame.Rent(decoded.Width, decoded.Height, PixelFormat.Bgra32, decoded.Width * 4);
                CopyArgbToBgra(decoded.PixelsArgb, buf);
                yield return new ImageFrame(decoded.Width, decoded.Height, PixelFormat.Bgra32,
                    decoded.Width * 4, buf, buf)
                {
                    Duration = TimeSpan.FromMilliseconds(f.DurationMs),
                    OffsetX = f.X,
                    OffsetY = f.Y,
                };
                _ = frame;  // Rent allocated frame is unused; we constructed our own to set duration/offsets.
            }
            else
            {
                throw new NotSupportedException(
                    $"WebP lossy (VP8) decoding is not yet implemented in Mediar. " +
                    $"Use Info / Chunks / Metadata for now.");
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

    private static void CopyArgbToBgra(ReadOnlySpan<uint> src, byte[] dst)
    {
        for (int i = 0; i < src.Length; i++)
        {
            uint c = src[i];
            int p = i * 4;
            dst[p + 0] = (byte)(c & 0xFF);          // B
            dst[p + 1] = (byte)((c >> 8) & 0xFF);   // G
            dst[p + 2] = (byte)((c >> 16) & 0xFF);  // R
            dst[p + 3] = (byte)((c >> 24) & 0xFF);  // A
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static int ReadU24(byte[] b, int o) => b[o] | (b[o + 1] << 8) | (b[o + 2] << 16);

    private static string FourCcToString(uint cc)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(b, cc);
        return Encoding.ASCII.GetString(b);
    }
}

/// <summary>Identifies the codec carried by a WebP frame.</summary>
public enum WebPFrameKind
{
    /// <summary>Unknown / unrecognised payload.</summary>
    Unknown,
    /// <summary>VP8 lossy frame (not yet pixel-decoded by Mediar).</summary>
    Vp8,
    /// <summary>VP8L lossless frame.</summary>
    Vp8L,
}

/// <summary>Lightweight description of a chunk encountered in the RIFF stream.</summary>
public sealed record WebPChunk
{
    /// <summary>FourCC (4-character) chunk identifier.</summary>
    public string FourCC { get; init; } = string.Empty;
    /// <summary>Absolute byte offset of the chunk header in the file.</summary>
    public int Offset { get; init; }
    /// <summary>Absolute byte offset of the chunk payload (after the 8-byte header).</summary>
    public int DataOffset { get; init; }
    /// <summary>Payload length, bytes.</summary>
    public int DataLength { get; init; }
}

/// <summary>Description of a still or animated WebP frame.</summary>
public sealed record WebPFrameRecord
{
    /// <summary>Codec carried by the frame.</summary>
    public WebPFrameKind Kind { get; init; }
    /// <summary>Frame width in canvas pixels.</summary>
    public int Width { get; init; }
    /// <summary>Frame height in canvas pixels.</summary>
    public int Height { get; init; }
    /// <summary>X offset on the canvas (animated frames only).</summary>
    public int X { get; init; }
    /// <summary>Y offset on the canvas (animated frames only).</summary>
    public int Y { get; init; }
    /// <summary>Display duration in milliseconds (animated frames only).</summary>
    public int DurationMs { get; init; }
    /// <summary>Blend mode: true = blend over the previous frame.</summary>
    public bool BlendOverPrev { get; init; } = true;
    /// <summary>Dispose mode: true = clear to background colour after display.</summary>
    public bool DisposeToBackground { get; init; }
    /// <summary>Absolute byte offset of the codec payload.</summary>
    public int Offset { get; init; }
    /// <summary>Length of the codec payload, bytes.</summary>
    public int Length { get; init; }
}
