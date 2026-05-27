using System.Buffers.Binary;
using System.IO.Compression;

namespace Mediar.Imaging.Png;

/// <summary>
/// Writer for plain (non-animated) PNG files. Produces a single IDAT chunk
/// per frame call. Supports <see cref="PixelFormat.Gray8"/>,
/// <see cref="PixelFormat.Rgb24"/>, <see cref="PixelFormat.Rgba32"/>,
/// <see cref="PixelFormat.Indexed8"/>, and the 16-bit variants
/// <see cref="PixelFormat.Gray16"/>, <see cref="PixelFormat.Rgb48"/>,
/// <see cref="PixelFormat.Rgba64"/>.
/// </summary>
public sealed class PngWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Png;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    public PngWriter(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
    }

    /// <summary>Create a PNG writer for <paramref name="path"/>.</summary>
    public static PngWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("PngWriter writes a single frame.");
        _written = true;

        // PNG signature
        await _stream.WriteAsync(new byte[] { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A },
                                 cancellationToken).ConfigureAwait(false);

        (byte bitDepth, PngColorType colorType, int channels, int sampleBytes) = frame.PixelFormat switch
        {
            PixelFormat.Gray8 => ((byte)8, PngColorType.Grayscale, 1, 1),
            PixelFormat.Gray16 => ((byte)16, PngColorType.Grayscale, 1, 2),
            PixelFormat.GrayAlpha16 => ((byte)8, PngColorType.GrayscaleAlpha, 2, 1),
            PixelFormat.Rgb24 => ((byte)8, PngColorType.Rgb, 3, 1),
            PixelFormat.Rgb48 => ((byte)16, PngColorType.Rgb, 3, 2),
            PixelFormat.Rgba32 => ((byte)8, PngColorType.Rgba, 4, 1),
            PixelFormat.Rgba64 => ((byte)16, PngColorType.Rgba, 4, 2),
            PixelFormat.Indexed8 => ((byte)8, PngColorType.Indexed, 1, 1),
            _ => throw new NotSupportedException($"PngWriter cannot encode {frame.PixelFormat}.")
        };

        // IHDR
        byte[] ihdr = new byte[13];
        BinaryPrimitives.WriteUInt32BigEndian(ihdr.AsSpan(0, 4), (uint)frame.Width);
        BinaryPrimitives.WriteUInt32BigEndian(ihdr.AsSpan(4, 4), (uint)frame.Height);
        ihdr[8] = bitDepth;
        ihdr[9] = (byte)colorType;
        ihdr[10] = 0; // compression
        ihdr[11] = 0; // filter
        ihdr[12] = 0; // interlace
        await WriteChunkAsync("IHDR"u8.ToArray(), ihdr, cancellationToken).ConfigureAwait(false);

        // PLTE for indexed images
        if (colorType == PngColorType.Indexed)
        {
            var paletteMem = frame.Palette;
            int paletteEntries = paletteMem.Length == 0 ? 256 : paletteMem.Length;
            byte[] plte = new byte[paletteEntries * 3];
            byte[]? trns = null;
            bool needTrns = false;
            {
                ReadOnlySpan<uint> palette = paletteMem.Span;
                for (int i = 0; i < paletteEntries; i++)
                {
                    uint rgba = i < palette.Length ? palette[i] : ((uint)i << 16 | (uint)i << 8 | (uint)i | 0xFF000000u);
                    plte[i * 3 + 0] = (byte)((rgba >> 16) & 0xFF);
                    plte[i * 3 + 1] = (byte)((rgba >> 8) & 0xFF);
                    plte[i * 3 + 2] = (byte)(rgba & 0xFF);
                    byte a = (byte)((rgba >> 24) & 0xFF);
                    if (a != 0xFF) needTrns = true;
                }
                if (needTrns)
                {
                    trns = new byte[paletteEntries];
                    for (int i = 0; i < paletteEntries; i++)
                    {
                        uint rgba = i < palette.Length ? palette[i] : 0xFF000000u;
                        trns[i] = (byte)((rgba >> 24) & 0xFF);
                    }
                }
            }
            await WriteChunkAsync("PLTE"u8.ToArray(), plte, cancellationToken).ConfigureAwait(false);
            if (trns is not null)
            {
                await WriteChunkAsync("tRNS"u8.ToArray(), trns, cancellationToken).ConfigureAwait(false);
            }
        }

        // IDAT: filter-byte-prefixed rows, zlib-compressed.
        int srcRowBytes = frame.Stride;
        int outRowBytes = frame.Width * channels * sampleBytes;
        byte[] compressed;
        {
            using var raw = new MemoryStream(checked((outRowBytes + 1) * frame.Height));
            ReadOnlySpan<byte> pixSpan = frame.Pixels.Span;
            for (int y = 0; y < frame.Height; y++)
            {
                raw.WriteByte(0); // filter = None
                ReadOnlySpan<byte> row = pixSpan.Slice(y * srcRowBytes, outRowBytes);
                // For 16-bit formats, PNG expects big-endian samples.
                if (sampleBytes == 2)
                {
                    for (int i = 0; i < outRowBytes; i += 2)
                    {
                        raw.WriteByte(row[i + 1]);
                        raw.WriteByte(row[i + 0]);
                    }
                }
                else
                {
                    raw.Write(row);
                }
            }
            compressed = ZlibCompress(raw.ToArray());
        }
        await WriteChunkAsync("IDAT"u8.ToArray(), compressed, cancellationToken).ConfigureAwait(false);

        // IEND
        await WriteChunkAsync("IEND"u8.ToArray(), [], cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public ValueTask FinishAsync(CancellationToken cancellationToken = default)
        => ValueTask.CompletedTask;

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        await _stream.FlushAsync().ConfigureAwait(false);
        if (_ownsStream) await _stream.DisposeAsync().ConfigureAwait(false);
    }

    private async ValueTask WriteChunkAsync(byte[] type, byte[] payload, CancellationToken ct)
    {
        byte[] lengthBuf = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(lengthBuf, (uint)payload.Length);
        await _stream.WriteAsync(lengthBuf, ct).ConfigureAwait(false);
        await _stream.WriteAsync(type, ct).ConfigureAwait(false);
        await _stream.WriteAsync(payload, ct).ConfigureAwait(false);
        uint crc = Crc32(0xFFFFFFFFu, type);
        crc = Crc32(crc, payload);
        crc ^= 0xFFFFFFFFu;
        byte[] crcOut = new byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(crcOut, crc);
        await _stream.WriteAsync(crcOut, ct).ConfigureAwait(false);
    }

    private static readonly uint[] s_crcTable = BuildCrcTable();

    private static uint[] BuildCrcTable()
    {
        var t = new uint[256];
        for (uint n = 0; n < 256; n++)
        {
            uint c = n;
            for (int k = 0; k < 8; k++)
            {
                c = (c & 1) != 0 ? 0xEDB88320u ^ (c >> 1) : c >> 1;
            }
            t[n] = c;
        }
        return t;
    }

    private static uint Crc32(uint seed, ReadOnlySpan<byte> data)
    {
        uint c = seed;
        for (int i = 0; i < data.Length; i++)
        {
            c = s_crcTable[(c ^ data[i]) & 0xFF] ^ (c >> 8);
        }
        return c;
    }

    private static byte[] ZlibCompress(byte[] raw)
    {
        using var ms = new MemoryStream();
        using (var zs = new ZLibStream(ms, CompressionLevel.Optimal, leaveOpen: true))
        {
            zs.Write(raw, 0, raw.Length);
        }
        return ms.ToArray();
    }
}
