using System.Buffers.Binary;

namespace Mediar.Imaging.Bmp;

/// <summary>
/// Writer for Windows BMP files. Emits the BITMAPINFOHEADER (40-byte V3)
/// variant: <c>BI_RGB</c> for 24/32-bit images and <c>BI_RGB</c>+palette
/// for 8-bit images. Output is bottom-up (BMP convention).
/// </summary>
public sealed class BmpWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Bmp;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    public BmpWriter(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
    }

    /// <summary>Create a BMP writer for <paramref name="path"/>.</summary>
    public static BmpWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("BMP supports a single frame.");
        _written = true;

        var pf = frame.PixelFormat;
        int bpp = pf switch
        {
            PixelFormat.Bgr24 or PixelFormat.Rgb24 => 24,
            PixelFormat.Bgra32 or PixelFormat.Rgba32 or PixelFormat.Argb32 => 32,
            PixelFormat.Indexed8 or PixelFormat.Gray8 => 8,
            _ => throw new NotSupportedException($"BMP writer does not support pixel format {pf}."),
        };
        int width = frame.Width;
        int height = frame.Height;
        int rowStride = ((width * bpp + 31) / 32) * 4;
        int dataSize = rowStride * height;
        int paletteSize = (bpp == 8) ? 256 * 4 : 0;
        int pixelOffset = 14 + 40 + paletteSize;
        int fileSize = pixelOffset + dataSize;

        Span<byte> hdr = stackalloc byte[14 + 40];
        hdr[0] = (byte)'B'; hdr[1] = (byte)'M';
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(2, 4), (uint)fileSize);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(10, 4), (uint)pixelOffset);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(14, 4), 40u);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(18, 4), width);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(22, 4), height); // bottom-up
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(26, 2), 1);
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(28, 2), (ushort)bpp);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(30, 4), 0u);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(34, 4), (uint)dataSize);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(38, 4), 2835);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(42, 4), 2835);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(46, 4), 0u);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(50, 4), 0u);
        await _stream.WriteAsync(hdr.ToArray(), cancellationToken).ConfigureAwait(false);

        if (bpp == 8)
        {
            byte[] pal = new byte[paletteSize];
            var sourcePalette = frame.Palette.Span;
            int count = Math.Min(256, sourcePalette.Length);
            for (int i = 0; i < count; i++)
            {
                uint rgba = sourcePalette[i];
                pal[i * 4 + 0] = (byte)(rgba & 0xFF);          // B
                pal[i * 4 + 1] = (byte)((rgba >> 8) & 0xFF);   // G
                pal[i * 4 + 2] = (byte)((rgba >> 16) & 0xFF);  // R
                pal[i * 4 + 3] = 0;
            }
            if (count == 0)
            {
                // synthesize grayscale palette
                for (int i = 0; i < 256; i++)
                {
                    pal[i * 4 + 0] = (byte)i;
                    pal[i * 4 + 1] = (byte)i;
                    pal[i * 4 + 2] = (byte)i;
                }
            }
            await _stream.WriteAsync(pal, cancellationToken).ConfigureAwait(false);
        }

        ReadOnlyMemory<byte> pixels = frame.Pixels;
        byte[] row = new byte[rowStride];
        for (int y = height - 1; y >= 0; y--)
        {
            ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, frame.Stride);
            int copy = Math.Min(row.Length, src.Length);
            switch (pf)
            {
                case PixelFormat.Bgr24:
                case PixelFormat.Bgra32:
                case PixelFormat.Indexed8:
                case PixelFormat.Gray8:
                    src[..copy].CopyTo(row.AsSpan(0, copy));
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
                case PixelFormat.Argb32:
                    for (int x = 0; x < width; x++)
                    {
                        row[x * 4 + 0] = src[x * 4 + 3];
                        row[x * 4 + 1] = src[x * 4 + 2];
                        row[x * 4 + 2] = src[x * 4 + 1];
                        row[x * 4 + 3] = src[x * 4 + 0];
                    }
                    break;
                default:
                    throw new NotSupportedException();
            }
            await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
        }
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
        if (_ownsStream)
        {
            await _stream.DisposeAsync().ConfigureAwait(false);
        }
    }
}
