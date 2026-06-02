using System.Collections.Frozen;
using System.Runtime.CompilerServices;
using System.Text;
using Mediar.Codecs.Lzw;

namespace Mediar.Imaging.Gif;

/// <summary>
/// Reader for GIF87a and GIF89a, including animated multi-frame GIFs.
/// Each <see cref="ImageFrame"/> is emitted as <see cref="PixelFormat.Rgba32"/>
/// with the active local-or-global palette resolved per frame.
/// </summary>
public sealed class GifReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly int _pos;
    private readonly uint[] _globalPalette;
    private readonly int _backgroundIndex;
    private readonly int _frameCount;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format { get; }

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }

    /// <inheritdoc/>
    public bool CanDecodePixels => true;

    private GifReader(
        Stream stream, bool ownsStream, byte[] bytes, int afterHeaderPos,
        uint[] globalPalette, int backgroundIndex, int frameCount,
        ImageFormat format, ImageInfo info, ImageMetadata metadata)
    {
        _stream = stream;
        _ownsStream = ownsStream;
        _bytes = bytes;
        _pos = afterHeaderPos;
        _globalPalette = globalPalette;
        _backgroundIndex = backgroundIndex;
        _frameCount = frameCount;
        Format = format;
        Info = info;
        Metadata = metadata;
    }

    /// <summary>Open a GIF from a file path.</summary>
    public static GifReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try
        {
            return Open(fs, ownsStream: true);
        }
        catch
        {
            fs.Dispose();
            throw;
        }
    }

    /// <summary>Open a GIF from a stream (fully buffered).</summary>
    public static GifReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();

        if (bytes.Length < 13 ||
            bytes[0] != (byte)'G' || bytes[1] != (byte)'I' || bytes[2] != (byte)'F' ||
            bytes[3] != (byte)'8' || (bytes[4] != (byte)'7' && bytes[4] != (byte)'9') ||
            bytes[5] != (byte)'a')
        {
            throw new ImageFormatException("Not a GIF file.");
        }

        int width = bytes[6] | (bytes[7] << 8);
        int height = bytes[8] | (bytes[9] << 8);
        byte packed = bytes[10];
        bool hasGct = (packed & 0x80) != 0;
        int gctSize = 1 << ((packed & 0x07) + 1);
        int backgroundIndex = bytes[11];
        int pos = 13;
        uint[] palette = [];
        if (hasGct)
        {
            palette = new uint[gctSize];
            for (int i = 0; i < gctSize; i++)
            {
                palette[i] = 0xFF000000u
                    | ((uint)bytes[pos] << 0)
                    | ((uint)bytes[pos + 1] << 8)
                    | ((uint)bytes[pos + 2] << 16);
                pos += 3;
            }
        }

        // Count animation frames + extract comment/netscape blocks for metadata.
        int frames = CountFrames(bytes, pos);
        var meta = ExtractMetadata(bytes, pos);

        var info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = 8,
            ChannelCount = 4,
            PixelFormat = PixelFormat.Rgba32,
            Format = ImageFormat.Gif,
            HasAlpha = true,
            IsAnimated = frames > 1,
            FrameCount = frames,
        };

        return new GifReader(stream, ownsStream, bytes, pos, palette,
                              backgroundIndex, frames, ImageFormat.Gif, info, meta);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        int canvasW = Info.Width;
        int canvasH = Info.Height;
        var canvas = new byte[canvasW * canvasH * 4];
        if (_globalPalette.Length > 0 && _backgroundIndex < _globalPalette.Length)
        {
            uint bg = _globalPalette[_backgroundIndex];
            for (int i = 0; i < canvasW * canvasH; i++)
            {
                WritePixel(canvas, i * 4, bg);
            }
        }

        int p = _pos;
        int delayCs = 0;
        int transparentIndex = -1;
        int disposalMethod = 0;
        byte[] previousCanvas = [];

        while (p < _bytes.Length)
        {
            cancellationToken.ThrowIfCancellationRequested();
            byte tag = _bytes[p++];
            if (tag == 0x3B) yield break;
            if (tag == 0x21)
            {
                byte label = _bytes[p++];
                if (label == 0xF9 && p + 5 < _bytes.Length)
                {
                    byte size = _bytes[p++];
                    byte gpacked = _bytes[p++];
                    delayCs = _bytes[p++] | (_bytes[p++] << 8);
                    byte tIdx = _bytes[p++];
                    transparentIndex = (gpacked & 0x01) != 0 ? tIdx : -1;
                    disposalMethod = (gpacked >> 2) & 0x07;
                    p++;
                    _ = size;
                }
                else
                {
                    while (p < _bytes.Length && _bytes[p] != 0) p += 1 + _bytes[p];
                    if (p < _bytes.Length) p++;
                }
                continue;
            }
            if (tag != 0x2C) continue;

            int ix = _bytes[p++] | (_bytes[p++] << 8);
            int iy = _bytes[p++] | (_bytes[p++] << 8);
            int iw = _bytes[p++] | (_bytes[p++] << 8);
            int ih = _bytes[p++] | (_bytes[p++] << 8);
            byte ipacked = _bytes[p++];
            bool hasLct = (ipacked & 0x80) != 0;
            bool interlaced = (ipacked & 0x40) != 0;
            int lctSize = 1 << ((ipacked & 0x07) + 1);
            uint[] palette = _globalPalette;
            if (hasLct)
            {
                palette = new uint[lctSize];
                for (int i = 0; i < lctSize; i++)
                {
                    palette[i] = 0xFF000000u
                        | ((uint)_bytes[p] << 0)
                        | ((uint)_bytes[p + 1] << 8)
                        | ((uint)_bytes[p + 2] << 16);
                    p += 3;
                }
            }

            byte lzwMinCode = _bytes[p++];
            byte[] lzwBytes = ReadSubBlocks(_bytes, ref p);
            byte[] indices = LzwDecoder.DecodeGif(lzwBytes, lzwMinCode, iw * ih);

            if (disposalMethod == 3)
            {
                previousCanvas = (byte[])canvas.Clone();
            }

            int[]? interlaceMap = interlaced ? BuildInterlaceMap(ih) : null;
            int srcIdx = 0;
            for (int row = 0; row < ih; row++)
            {
                int dstRow = interlaceMap is null ? row : interlaceMap[row];
                int dstY = iy + dstRow;
                if ((uint)dstY >= (uint)canvasH) { srcIdx += iw; continue; }
                for (int col = 0; col < iw; col++)
                {
                    int dx = ix + col;
                    if ((uint)dx >= (uint)canvasW) continue;
                    byte ci = indices[srcIdx + col];
                    if (ci == transparentIndex) continue;
                    if (ci < palette.Length)
                    {
                        int o = (dstY * canvasW + dx) * 4;
                        WritePixel(canvas, o, palette[ci]);
                    }
                }
                srcIdx += iw;
            }

            byte[] copy = new byte[canvas.Length];
            Buffer.BlockCopy(canvas, 0, copy, 0, canvas.Length);
            // copy is not rented from ArrayPool — pass pooledOwner: null so
            // ImageFrame.Dispose doesn't try to return it.
            yield return new ImageFrame(canvasW, canvasH, PixelFormat.Rgba32,
                canvasW * 4, copy, pooledOwner: null)
            {
                Duration = TimeSpan.FromMilliseconds(delayCs * 10),
            };

            switch (disposalMethod)
            {
                case 2:
                    for (int row = 0; row < ih; row++)
                    {
                        int dstY = iy + row;
                        if ((uint)dstY >= (uint)canvasH) continue;
                        for (int col = 0; col < iw; col++)
                        {
                            int dx = ix + col;
                            if ((uint)dx >= (uint)canvasW) continue;
                            int o = (dstY * canvasW + dx) * 4;
                            canvas[o] = canvas[o + 1] = canvas[o + 2] = canvas[o + 3] = 0;
                        }
                    }
                    break;
                case 3:
                    if (previousCanvas.Length == canvas.Length)
                    {
                        Buffer.BlockCopy(previousCanvas, 0, canvas, 0, canvas.Length);
                    }
                    break;
                default:
                    break;
            }
            transparentIndex = -1;
            delayCs = 0;
        }
    }

    private static int[] BuildInterlaceMap(int height)
    {
        var map = new int[height];
        int idx = 0;
        for (int y = 0; y < height; y += 8) map[idx++] = y;
        for (int y = 4; y < height; y += 8) map[idx++] = y;
        for (int y = 2; y < height; y += 4) map[idx++] = y;
        for (int y = 1; y < height; y += 2) map[idx++] = y;
        return map;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static void WritePixel(byte[] dst, int offset, uint argb)
    {
        dst[offset + 0] = (byte)(argb & 0xFF);
        dst[offset + 1] = (byte)((argb >> 8) & 0xFF);
        dst[offset + 2] = (byte)((argb >> 16) & 0xFF);
        dst[offset + 3] = (byte)((argb >> 24) & 0xFF);
    }

    private static byte[] ReadSubBlocks(byte[] src, ref int p)
    {
        using var ms = new MemoryStream();
        while (p < src.Length)
        {
            byte n = src[p++];
            if (n == 0) break;
            ms.Write(src, p, n);
            p += n;
        }
        return ms.ToArray();
    }

    private static int CountFrames(byte[] bytes, int pos)
    {
        int p = pos, n = 0;
        while (p < bytes.Length)
        {
            byte tag = bytes[p++];
            if (tag == 0x3B) break;
            if (tag == 0x21)
            {
                p++;
                while (p < bytes.Length && bytes[p] != 0) p += 1 + bytes[p];
                if (p < bytes.Length) p++;
                continue;
            }
            if (tag != 0x2C) continue;
            n++;
            // Skip image descriptor (9 bytes already past tag).
            p += 8;
            byte ipacked = bytes[p++];
            if ((ipacked & 0x80) != 0) p += 3 * (1 << ((ipacked & 0x07) + 1));
            p++; // LZW min code
            while (p < bytes.Length && bytes[p] != 0) p += 1 + bytes[p];
            if (p < bytes.Length) p++;
        }
        return n;
    }

    private static ImageMetadata ExtractMetadata(byte[] bytes, int pos)
    {
        var tags = new Dictionary<string, string>(StringComparer.Ordinal);
        int p = pos;
        var sb = new StringBuilder();
        int commentIndex = 0;
        while (p < bytes.Length)
        {
            byte tag = bytes[p++];
            if (tag == 0x3B) break;
            if (tag == 0x21)
            {
                byte label = bytes[p++];
                if (label == 0xFE)
                {
                    sb.Clear();
                    while (p < bytes.Length && bytes[p] != 0)
                    {
                        int n = bytes[p++];
                        sb.Append(Encoding.Latin1.GetString(bytes, p, n));
                        p += n;
                    }
                    if (p < bytes.Length) p++;
                    tags[$"GIF:Comment{commentIndex++}"] = sb.ToString();
                    continue;
                }
                else if (label == 0xFF && p + 11 < bytes.Length && bytes[p] == 11)
                {
                    string app = Encoding.Latin1.GetString(bytes, p + 1, 11);
                    tags["GIF:Application"] = app;
                    p += 12;
                    while (p < bytes.Length && bytes[p] != 0) p += 1 + bytes[p];
                    if (p < bytes.Length) p++;
                    continue;
                }
                else
                {
                    while (p < bytes.Length && bytes[p] != 0) p += 1 + bytes[p];
                    if (p < bytes.Length) p++;
                }
                continue;
            }
            if (tag != 0x2C) continue;
            p += 8;
            byte ipacked = bytes[p++];
            if ((ipacked & 0x80) != 0) p += 3 * (1 << ((ipacked & 0x07) + 1));
            p++;
            while (p < bytes.Length && bytes[p] != 0) p += 1 + bytes[p];
            if (p < bytes.Length) p++;
        }
        tags.TryGetValue("GIF:Comment0", out var c0);
        return new ImageMetadata
        {
            Description = c0,
            Tags = tags.ToFrozenDictionary(StringComparer.Ordinal),
        };
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}
