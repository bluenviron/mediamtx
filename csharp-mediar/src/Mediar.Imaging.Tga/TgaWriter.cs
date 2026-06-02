using System.Buffers.Binary;
using System.Text;

namespace Mediar.Imaging.Tga;

/// <summary>
/// Writer for Truevision TGA files. Emits image type 2 (uncompressed
/// truecolor), 3 (uncompressed grayscale), or 10 (RLE truecolor) depending
/// on the requested <see cref="TgaCompression"/>. Output is top-down (the
/// image-descriptor "origin in upper-left" bit is set), 24 or 32 bits per
/// pixel for color and 8 bits per pixel for grayscale.
/// </summary>
public sealed class TgaWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly TgaCompression _compression;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Tga;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    public TgaWriter(Stream stream, bool ownsStream = false,
                     TgaCompression compression = TgaCompression.None)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
        _compression = compression;
    }

    /// <summary>Create a TGA writer for <paramref name="path"/>.</summary>
    public static TgaWriter Create(string path, TgaCompression compression = TgaCompression.None)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true, compression);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("TGA supports a single frame.");
        _written = true;

        int width = frame.Width;
        int height = frame.Height;
        var pf = frame.PixelFormat;
        bool rle = _compression == TgaCompression.Rle;

        byte imageType;
        byte pixelDepth;
        int bytesPerPixel;
        switch (pf)
        {
            case PixelFormat.Gray8:
                imageType = (byte)(rle ? 11 : 3);
                pixelDepth = 8;
                bytesPerPixel = 1;
                break;
            case PixelFormat.Bgr24:
            case PixelFormat.Rgb24:
                imageType = (byte)(rle ? 10 : 2);
                pixelDepth = 24;
                bytesPerPixel = 3;
                break;
            case PixelFormat.Bgra32:
            case PixelFormat.Rgba32:
                imageType = (byte)(rle ? 10 : 2);
                pixelDepth = 32;
                bytesPerPixel = 4;
                break;
            default:
                throw new NotSupportedException($"TGA writer does not support pixel format {pf}.");
        }

        Span<byte> header = stackalloc byte[18];
        header[2] = imageType;
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(12, 2), (ushort)width);
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(14, 2), (ushort)height);
        header[16] = pixelDepth;
        // Descriptor: bit 5 = origin at top-left; low nibble = alpha bits.
        byte alphaBits = (byte)(pixelDepth == 32 ? 8 : 0);
        header[17] = (byte)(0x20 | alphaBits);
        await _stream.WriteAsync(header.ToArray(), cancellationToken).ConfigureAwait(false);

        byte[] row = new byte[width * bytesPerPixel];
        for (int y = 0; y < height; y++)
        {
            ReadOnlySpan<byte> src = frame.Pixels.Span.Slice(y * frame.Stride, Math.Min(row.Length, frame.Stride));
            switch (pf)
            {
                case PixelFormat.Gray8:
                case PixelFormat.Bgr24:
                case PixelFormat.Bgra32:
                    src[..Math.Min(row.Length, src.Length)].CopyTo(row);
                    break;
                case PixelFormat.Rgb24:
                    for (int x = 0; x < width; x++)
                    {
                        row[x * 3 + 0] = src[x * 3 + 2];
                        row[x * 3 + 1] = src[x * 3 + 1];
                        row[x * 3 + 2] = src[x * 3 + 0];
                    }
                    break;
                case PixelFormat.Rgba32:
                    for (int x = 0; x < width; x++)
                    {
                        row[x * 4 + 0] = src[x * 4 + 2];
                        row[x * 4 + 1] = src[x * 4 + 1];
                        row[x * 4 + 2] = src[x * 4 + 0];
                        row[x * 4 + 3] = src[x * 4 + 3];
                    }
                    break;
            }

            if (!rle)
            {
                await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
            }
            else
            {
                byte[] encoded = RleEncode(row, width, bytesPerPixel);
                await _stream.WriteAsync(encoded, cancellationToken).ConfigureAwait(false);
            }
        }

        // TGA 2.0 footer (optional but harmless): zero offsets + signature.
        var footer = new byte[26];
        Encoding.ASCII.GetBytes("TRUEVISION-XFILE.", footer.AsSpan(8, 17));
        footer[25] = 0x00;
        await _stream.WriteAsync(footer, cancellationToken).ConfigureAwait(false);
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

    /// <summary>
    /// Truevision TGA RLE encoder: packetizes a single scanline into run
    /// packets (count|0x80 followed by one pixel repeated <c>count+1</c>
    /// times) and raw packets (count followed by <c>count+1</c> raw pixels).
    /// Packets never span scanlines (per the spec).
    /// </summary>
    private static byte[] RleEncode(ReadOnlySpan<byte> row, int width, int bpp)
    {
        // Worst case: every pixel is a raw packet of 1 → 1 + bpp bytes per pixel.
        using var ms = new MemoryStream(width * (bpp + 1));
        int x = 0;
        while (x < width)
        {
            int runLen = 1;
            while (runLen < 128 && x + runLen < width &&
                   row.Slice((x + runLen) * bpp, bpp).SequenceEqual(row.Slice(x * bpp, bpp)))
            {
                runLen++;
            }
            if (runLen >= 2)
            {
                ms.WriteByte((byte)(0x80 | (runLen - 1)));
                ms.Write(row.Slice(x * bpp, bpp));
                x += runLen;
            }
            else
            {
                // Raw packet: gather pixels until next run of >= 3 is detected.
                int rawLen = 1;
                while (rawLen < 128 && x + rawLen < width)
                {
                    // Stop if a run of 3 starts here, to let the next iteration capture it.
                    if (x + rawLen + 2 < width &&
                        row.Slice((x + rawLen) * bpp, bpp).SequenceEqual(row.Slice((x + rawLen + 1) * bpp, bpp)) &&
                        row.Slice((x + rawLen) * bpp, bpp).SequenceEqual(row.Slice((x + rawLen + 2) * bpp, bpp)))
                    {
                        break;
                    }
                    rawLen++;
                }
                ms.WriteByte((byte)(rawLen - 1));
                ms.Write(row.Slice(x * bpp, rawLen * bpp));
                x += rawLen;
            }
        }
        return ms.ToArray();
    }
}

/// <summary>Compression mode for <see cref="TgaWriter"/>.</summary>
public enum TgaCompression
{
    /// <summary>Uncompressed image (image type 2 / 3).</summary>
    None,
    /// <summary>Run-length encoded image (image type 10 / 11).</summary>
    Rle,
}
