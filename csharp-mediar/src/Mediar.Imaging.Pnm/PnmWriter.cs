using System.Globalization;
using System.Text;

namespace Mediar.Imaging.Pnm;

/// <summary>
/// Writer for Netpbm raw bitmap / graymap / pixmap files (P4 / P5 / P6).
/// The chosen sub-format is derived from the input
/// <see cref="ImageFrame.PixelFormat"/>; ASCII variants (P1 / P2 / P3) are
/// not emitted because they are strictly larger.
/// </summary>
public sealed class PnmWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly ImageFormat _format;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => _format;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    /// <param name="stream">Destination stream. Must be writable.</param>
    /// <param name="ownsStream">When <c>true</c>, the stream is disposed alongside the writer.</param>
    /// <param name="format">
    /// One of <see cref="ImageFormat.Pbm"/>, <see cref="ImageFormat.Pgm"/>,
    /// <see cref="ImageFormat.Ppm"/>, <see cref="ImageFormat.Pnm"/>. Defaults
    /// to <see cref="ImageFormat.Pnm"/>, in which case the magic is inferred
    /// from the frame pixel format.
    /// </param>
    public PnmWriter(Stream stream, bool ownsStream = false, ImageFormat format = ImageFormat.Pnm)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
        _format = format == ImageFormat.Pnm ? ImageFormat.Pnm : format;
    }

    /// <summary>Create a PNM writer for <paramref name="path"/>.</summary>
    public static PnmWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("PNM supports a single frame.");
        _written = true;

        int width = frame.Width;
        int height = frame.Height;
        var pf = frame.PixelFormat;
        int magic;
        int maxVal;
        switch (pf)
        {
            case PixelFormat.Indexed1:
                magic = 4; maxVal = 1; break;
            case PixelFormat.Gray8:
                magic = 5; maxVal = 255; break;
            case PixelFormat.Gray16:
                magic = 5; maxVal = 65535; break;
            case PixelFormat.Rgb24:
            case PixelFormat.Bgr24:
                magic = 6; maxVal = 255; break;
            case PixelFormat.Rgb48:
                magic = 6; maxVal = 65535; break;
            default:
                throw new NotSupportedException($"PNM writer does not support pixel format {pf}.");
        }

        var header = new StringBuilder(32);
        header.Append('P').Append(magic).Append('\n')
              .Append(width.ToString(CultureInfo.InvariantCulture)).Append(' ')
              .Append(height.ToString(CultureInfo.InvariantCulture)).Append('\n');
        if (magic is 5 or 6)
        {
            header.Append(maxVal.ToString(CultureInfo.InvariantCulture)).Append('\n');
        }
        byte[] headerBytes = Encoding.ASCII.GetBytes(header.ToString());
        await _stream.WriteAsync(headerBytes, cancellationToken).ConfigureAwait(false);

        ReadOnlyMemory<byte> pixels = frame.Pixels;
        switch (pf)
        {
            case PixelFormat.Indexed1:
            {
                int rowBytes = (width + 7) / 8;
                byte[] row = new byte[rowBytes];
                for (int y = 0; y < height; y++)
                {
                    ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, Math.Min(rowBytes, frame.Stride));
                    src[..Math.Min(row.Length, src.Length)].CopyTo(row);
                    await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
                }
                break;
            }
            case PixelFormat.Gray8:
            {
                byte[] row = new byte[width];
                for (int y = 0; y < height; y++)
                {
                    ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, Math.Min(width, frame.Stride));
                    src[..Math.Min(row.Length, src.Length)].CopyTo(row);
                    await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
                }
                break;
            }
            case PixelFormat.Gray16:
            {
                byte[] row = new byte[width * 2];
                for (int y = 0; y < height; y++)
                {
                    ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, Math.Min(row.Length, frame.Stride));
                    // PNM raw 16-bit is big-endian.
                    for (int i = 0; i < width; i++)
                    {
                        ushort u = (ushort)(src[i * 2] | (src[i * 2 + 1] << 8));
                        row[i * 2 + 0] = (byte)(u >> 8);
                        row[i * 2 + 1] = (byte)u;
                    }
                    await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
                }
                break;
            }
            case PixelFormat.Rgb24:
            {
                byte[] row = new byte[width * 3];
                for (int y = 0; y < height; y++)
                {
                    ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, Math.Min(row.Length, frame.Stride));
                    src[..Math.Min(row.Length, src.Length)].CopyTo(row);
                    await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
                }
                break;
            }
            case PixelFormat.Bgr24:
            {
                byte[] row = new byte[width * 3];
                for (int y = 0; y < height; y++)
                {
                    ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, Math.Min(row.Length, frame.Stride));
                    for (int x = 0; x < width; x++)
                    {
                        row[x * 3 + 0] = src[x * 3 + 2];
                        row[x * 3 + 1] = src[x * 3 + 1];
                        row[x * 3 + 2] = src[x * 3 + 0];
                    }
                    await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
                }
                break;
            }
            case PixelFormat.Rgb48:
            {
                byte[] row = new byte[width * 6];
                for (int y = 0; y < height; y++)
                {
                    ReadOnlySpan<byte> src = pixels.Span.Slice(y * frame.Stride, Math.Min(row.Length, frame.Stride));
                    for (int i = 0; i < width * 3; i++)
                    {
                        ushort u = (ushort)(src[i * 2] | (src[i * 2 + 1] << 8));
                        row[i * 2 + 0] = (byte)(u >> 8);
                        row[i * 2 + 1] = (byte)u;
                    }
                    await _stream.WriteAsync(row, cancellationToken).ConfigureAwait(false);
                }
                break;
            }
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
        if (_ownsStream) await _stream.DisposeAsync().ConfigureAwait(false);
    }
}
