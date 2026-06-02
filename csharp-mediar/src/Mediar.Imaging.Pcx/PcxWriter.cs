using System.Buffers.Binary;

namespace Mediar.Imaging.Pcx;

/// <summary>
/// Writer for ZSoft PCX (version 5) files. Supports
/// <see cref="PixelFormat.Indexed8"/> (256-color palette + 0x0C marker
/// trailer), <see cref="PixelFormat.Gray8"/> (written as Indexed8 with a
/// synthesized grayscale palette), and <see cref="PixelFormat.Rgb24"/> /
/// <see cref="PixelFormat.Bgr24"/> (3-plane, 8 bpp per plane). Scanlines
/// are always RLE-compressed per the PCX byte-level convention; planes
/// are stored R, G, B and each plane is padded to an even byte count.
/// </summary>
public sealed class PcxWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Pcx;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    public PcxWriter(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
    }

    /// <summary>Create a PCX writer for <paramref name="path"/>.</summary>
    public static PcxWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("PCX supports a single frame.");
        _written = true;

        int width = frame.Width;
        int height = frame.Height;
        var pf = frame.PixelFormat;
        int nPlanes;
        int bitsPerPlane = 8;
        bool needsPalette = false;
        switch (pf)
        {
            case PixelFormat.Indexed8:
            case PixelFormat.Gray8:
                nPlanes = 1;
                needsPalette = true;
                break;
            case PixelFormat.Rgb24:
            case PixelFormat.Bgr24:
                nPlanes = 3;
                break;
            default:
                throw new NotSupportedException($"PCX writer does not support pixel format {pf}.");
        }

        // Bytes per line per plane, padded to even.
        int bytesPerLine = (width + 1) & ~1;

        Span<byte> header = stackalloc byte[128];
        header[0] = 0x0A;                   // Magic
        header[1] = 5;                      // Version (5 = PC Paintbrush 3.0+)
        header[2] = 1;                      // Encoding = RLE
        header[3] = (byte)bitsPerPlane;
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(4, 2), 0);
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(6, 2), 0);
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(8, 2), (ushort)(width - 1));
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(10, 2), (ushort)(height - 1));
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(12, 2), 72); // HRes
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(14, 2), 72); // VRes
        // EGA palette (16 bytes * 3) at offset 16; zeroed.
        header[65] = (byte)nPlanes;
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(66, 2), (ushort)bytesPerLine);
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(68, 2), 1); // Palette interp: color/BW
        await _stream.WriteAsync(header.ToArray(), cancellationToken).ConfigureAwait(false);

        byte[] line = new byte[bytesPerLine * nPlanes];
        for (int y = 0; y < height; y++)
        {
            Array.Clear(line);
            ReadOnlySpan<byte> src = frame.Pixels.Span.Slice(y * frame.Stride);
            switch (pf)
            {
                case PixelFormat.Indexed8:
                case PixelFormat.Gray8:
                    src[..Math.Min(width, src.Length)].CopyTo(line);
                    break;
                case PixelFormat.Rgb24:
                    for (int x = 0; x < width; x++)
                    {
                        line[x] = src[x * 3 + 0];
                        line[bytesPerLine + x] = src[x * 3 + 1];
                        line[2 * bytesPerLine + x] = src[x * 3 + 2];
                    }
                    break;
                case PixelFormat.Bgr24:
                    for (int x = 0; x < width; x++)
                    {
                        line[x] = src[x * 3 + 2];
                        line[bytesPerLine + x] = src[x * 3 + 1];
                        line[2 * bytesPerLine + x] = src[x * 3 + 0];
                    }
                    break;
            }
            byte[] encoded = EncodeRle(line);
            await _stream.WriteAsync(encoded, cancellationToken).ConfigureAwait(false);
        }

        if (needsPalette)
        {
            // 0x0C marker + 768 bytes RGB palette.
            byte[] tail = new byte[769];
            tail[0] = 0x0C;
            ReadOnlySpan<uint> pal = frame.Palette.Span;
            int count = Math.Min(256, pal.Length);
            for (int i = 0; i < count; i++)
            {
                // PcxReader stores palette entries as (a<<24)|(b<<16)|(g<<8)|r —
                // R in the low byte. Mirror that so round-trip stays bit-exact.
                uint rgba = pal[i];
                tail[1 + i * 3 + 0] = (byte)(rgba & 0xFF);          // R
                tail[1 + i * 3 + 1] = (byte)((rgba >> 8) & 0xFF);   // G
                tail[1 + i * 3 + 2] = (byte)((rgba >> 16) & 0xFF);  // B
            }
            if (count == 0)
            {
                // Synthesise grayscale palette.
                for (int i = 0; i < 256; i++)
                {
                    tail[1 + i * 3 + 0] = (byte)i;
                    tail[1 + i * 3 + 1] = (byte)i;
                    tail[1 + i * 3 + 2] = (byte)i;
                }
            }
            await _stream.WriteAsync(tail, cancellationToken).ConfigureAwait(false);
        }
    }

    /// <summary>
    /// PCX byte-level RLE: bytes whose top two bits are <c>11</c> introduce
    /// a run (low 6 bits = count, next byte = value); other bytes are
    /// literal but any literal &gt;= 0xC0 must be escaped as a count-1 run.
    /// </summary>
    private static byte[] EncodeRle(byte[] line)
    {
        using var ms = new MemoryStream(line.Length + 16);
        int i = 0;
        while (i < line.Length)
        {
            byte v = line[i];
            int run = 1;
            while (run < 63 && i + run < line.Length && line[i + run] == v) run++;
            if (run > 1 || (v & 0xC0) == 0xC0)
            {
                ms.WriteByte((byte)(0xC0 | run));
                ms.WriteByte(v);
            }
            else
            {
                ms.WriteByte(v);
            }
            i += run;
        }
        return ms.ToArray();
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
}
