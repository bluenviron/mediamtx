using System.Runtime.CompilerServices;
using System.Text;

namespace Mediar.Imaging.Hdr;

/// <summary>
/// Writer for Greg Ward's Radiance RGBE (.hdr / .pic) high-dynamic-range
/// image format. Encodes floating-point RGB input
/// (<see cref="PixelFormat.Rgb96Float"/>, <see cref="PixelFormat.Rgba128Float"/>,
/// or <see cref="PixelFormat.Rgbe32"/>) to the canonical 32-bit packed
/// RGBE byte layout with Radiance "new" scanline run-length compression
/// (RLE only legal for widths in <c>[8, 32767]</c> — wider/narrower
/// scanlines are emitted uncompressed).
/// </summary>
public sealed class HdrWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Hdr;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    public HdrWriter(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
    }

    /// <summary>Create an HDR writer for <paramref name="path"/>.</summary>
    public static HdrWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("HDR supports a single frame.");
        _written = true;

        int width = frame.Width;
        int height = frame.Height;
        var pf = frame.PixelFormat;
        if (pf is not (PixelFormat.Rgb96Float or PixelFormat.Rgba128Float or PixelFormat.Rgbe32))
        {
            throw new NotSupportedException($"HDR writer does not support pixel format {pf}.");
        }

        // Standard Radiance header.
        var header = new StringBuilder(64);
        header.Append("#?RADIANCE\n")
              .Append("FORMAT=32-bit_rle_rgbe\n")
              .Append("SOFTWARE=Mediar.Imaging.Hdr\n")
              .Append('\n')
              .Append("-Y ").Append(height).Append(" +X ").Append(width).Append('\n');
        byte[] headerBytes = Encoding.ASCII.GetBytes(header.ToString());
        await _stream.WriteAsync(headerBytes, cancellationToken).ConfigureAwait(false);

        byte[] scanline = new byte[width * 4];
        bool canRle = width >= 8 && width <= 0x7FFF;
        for (int y = 0; y < height; y++)
        {
            ConvertRowToRgbe(frame, y, scanline);
            if (canRle)
            {
                await WriteRleScanlineAsync(scanline, width, cancellationToken).ConfigureAwait(false);
            }
            else
            {
                await _stream.WriteAsync(scanline, cancellationToken).ConfigureAwait(false);
            }
        }
    }

    private static void ConvertRowToRgbe(ImageFrame frame, int y, byte[] rgbe)
    {
        int width = frame.Width;
        ReadOnlySpan<byte> src = frame.Pixels.Span.Slice(y * frame.Stride);
        switch (frame.PixelFormat)
        {
            case PixelFormat.Rgb96Float:
                for (int x = 0; x < width; x++)
                {
                    float r = Unsafe.ReadUnaligned<float>(ref Unsafe.AsRef(in src[x * 12 + 0]));
                    float g = Unsafe.ReadUnaligned<float>(ref Unsafe.AsRef(in src[x * 12 + 4]));
                    float b = Unsafe.ReadUnaligned<float>(ref Unsafe.AsRef(in src[x * 12 + 8]));
                    PackRgbe(r, g, b, rgbe, x * 4);
                }
                break;
            case PixelFormat.Rgba128Float:
                for (int x = 0; x < width; x++)
                {
                    float r = Unsafe.ReadUnaligned<float>(ref Unsafe.AsRef(in src[x * 16 + 0]));
                    float g = Unsafe.ReadUnaligned<float>(ref Unsafe.AsRef(in src[x * 16 + 4]));
                    float b = Unsafe.ReadUnaligned<float>(ref Unsafe.AsRef(in src[x * 16 + 8]));
                    PackRgbe(r, g, b, rgbe, x * 4);
                }
                break;
            case PixelFormat.Rgbe32:
                for (int x = 0; x < width; x++)
                {
                    rgbe[x * 4 + 0] = src[x * 4 + 0];
                    rgbe[x * 4 + 1] = src[x * 4 + 1];
                    rgbe[x * 4 + 2] = src[x * 4 + 2];
                    rgbe[x * 4 + 3] = src[x * 4 + 3];
                }
                break;
        }
    }

    private static void PackRgbe(float r, float g, float b, byte[] dst, int o)
    {
        // Clamp negatives to zero; HDR is non-negative radiance.
        if (r < 0) r = 0;
        if (g < 0) g = 0;
        if (b < 0) b = 0;
        float v = MathF.Max(MathF.Max(r, g), b);
        if (v < 1e-32f)
        {
            dst[o + 0] = dst[o + 1] = dst[o + 2] = dst[o + 3] = 0;
            return;
        }
        int e = 0;
        float m = MathF.Truncate(MathF.Log2(v)) + 1;
        e = (int)m;
        float scale = MathF.Pow(2f, -e) * 256f;
        dst[o + 0] = (byte)Math.Clamp((int)(r * scale), 0, 255);
        dst[o + 1] = (byte)Math.Clamp((int)(g * scale), 0, 255);
        dst[o + 2] = (byte)Math.Clamp((int)(b * scale), 0, 255);
        dst[o + 3] = (byte)Math.Clamp(e + 128, 0, 255);
    }

    private async ValueTask WriteRleScanlineAsync(byte[] rgbe, int width, CancellationToken ct)
    {
        // Radiance "new" scanline preamble: 0x02 0x02 hi(width) lo(width).
        byte[] preamble = [0x02, 0x02, (byte)(width >> 8), (byte)(width & 0xFF)];
        await _stream.WriteAsync(preamble, ct).ConfigureAwait(false);

        byte[] channel = new byte[width];
        for (int ch = 0; ch < 4; ch++)
        {
            for (int x = 0; x < width; x++) channel[x] = rgbe[x * 4 + ch];
            byte[] encoded = EncodeChannel(channel, width);
            await _stream.WriteAsync(encoded, ct).ConfigureAwait(false);
        }
    }

    /// <summary>
    /// Encode one channel using Radiance's run-length packet format:
    /// a header byte &lt;= 128 introduces a raw packet of that many bytes,
    /// a header byte &gt; 128 introduces a run of the next byte repeated
    /// <c>(header - 128)</c> times. Runs are emitted when length &gt;= 4.
    /// </summary>
    private static byte[] EncodeChannel(byte[] src, int len)
    {
        using var ms = new MemoryStream(len + 16);
        int i = 0;
        while (i < len)
        {
            // Locate next run of 4 or more identical bytes.
            int runStart = i;
            while (runStart < len)
            {
                int runEnd = runStart + 1;
                while (runEnd < len && src[runEnd] == src[runStart]) runEnd++;
                int runLen = runEnd - runStart;
                if (runLen >= 4) break;
                runStart = runEnd;
            }
            // Emit raw packet for [i, runStart) in chunks of 128.
            while (i < runStart)
            {
                int chunk = Math.Min(128, runStart - i);
                ms.WriteByte((byte)chunk);
                ms.Write(src, i, chunk);
                i += chunk;
            }
            if (i >= len) break;
            // Emit run packet.
            int rs = runStart;
            int re = rs + 1;
            while (re < len && src[re] == src[rs] && re - rs < 127) re++;
            int rl = re - rs;
            ms.WriteByte((byte)(128 + rl));
            ms.WriteByte(src[rs]);
            i = re;
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
