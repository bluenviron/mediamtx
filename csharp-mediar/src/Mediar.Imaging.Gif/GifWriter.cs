using Mediar.Codecs.Lzw;

namespace Mediar.Imaging.Gif;

/// <summary>
/// Writer for a single-image GIF89a file from an <see cref="PixelFormat.Indexed8"/>
/// frame. The palette is taken from <see cref="ImageFrame.Palette"/>, padded up
/// to the next power of two (2..256 entries) and emitted as the Global Color
/// Table. LZW compression uses <see cref="LzwEncoder.EncodeGif"/> with a
/// minimum-code-size derived from the GCT size.
/// </summary>
/// <remarks>
/// Multi-frame / animated GIF output is intentionally out of scope; for that
/// you would add Graphic Control Extensions and a NETSCAPE2.0 loop block.
/// Non-indexed input is not quantized — pass <see cref="PixelFormat.Indexed8"/>
/// (typically from a quantizer such as the planned Mediar palette quantizer).
/// </remarks>
public sealed class GifWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _wrote;
    private bool _finished;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Gif;

    /// <summary>Construct a writer over <paramref name="stream"/>.</summary>
    public GifWriter(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
    }

    /// <summary>Create a writer that emits to <paramref name="path"/>.</summary>
    public static GifWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_wrote)
            throw new InvalidOperationException(
                "GIF writer currently emits a single image; multi-frame animation is not yet supported.");
        if (frame.PixelFormat != PixelFormat.Indexed8)
            throw new NotSupportedException(
                "GIF writer requires Indexed8 input. Quantize Rgb24 / Rgba32 to Indexed8 first.");
        if (frame.Palette.IsEmpty)
            throw new ArgumentException("Indexed8 frame must carry a palette.", nameof(frame));

        _wrote = true;
        var paletteMem = frame.Palette;
        int paletteCount = paletteMem.Length;
        int gctEntries = NextPow2(Math.Clamp(paletteCount, 2, 256));
        int gctSizeField = Log2(gctEntries) - 1; // packed-field GCT size: 2^(N+1) entries.
        int minCodeSize = Math.Max(2, Log2(gctEntries));

        await _stream.WriteAsync(GifBytes.Header, cancellationToken).ConfigureAwait(false);
        await _stream.WriteAsync(BuildScreenDescriptor(frame.Width, frame.Height, gctSizeField),
                                  cancellationToken).ConfigureAwait(false);
        await _stream.WriteAsync(BuildGlobalColorTable(paletteMem, gctEntries),
                                  cancellationToken).ConfigureAwait(false);
        await _stream.WriteAsync(BuildImageDescriptor(frame.Width, frame.Height),
                                  cancellationToken).ConfigureAwait(false);

        // Tightly pack indices: GIF doesn't carry stride.
        byte[] indices = TightlyPack(frame);
        byte[] lzw = LzwEncoder.EncodeGif(indices, minCodeSize);

        await _stream.WriteAsync(new byte[] { (byte)minCodeSize }, cancellationToken).ConfigureAwait(false);
        await WriteAsSubBlocksAsync(lzw, cancellationToken).ConfigureAwait(false);
        await _stream.WriteAsync(GifBytes.Trailer, cancellationToken).ConfigureAwait(false);
    }

    /// <inheritdoc/>
    public ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        _finished = true;
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished) await FinishAsync().ConfigureAwait(false);
        await _stream.FlushAsync().ConfigureAwait(false);
        if (_ownsStream) await _stream.DisposeAsync().ConfigureAwait(false);
    }

    private static byte[] BuildScreenDescriptor(int width, int height, int gctSizeField)
    {
        byte packed = (byte)(0x80 | (0x07 << 4) | (gctSizeField & 0x07)); // GCT on, max colour resolution.
        return
        [
            (byte)(width & 0xFF), (byte)((width >> 8) & 0xFF),
            (byte)(height & 0xFF), (byte)((height >> 8) & 0xFF),
            packed,
            0x00, // background colour index
            0x00, // pixel aspect ratio (default)
        ];
    }

    private static byte[] BuildGlobalColorTable(ReadOnlyMemory<uint> paletteMem, int entries)
    {
        // NOTE: GifReader builds palette entries as 0xFF000000 | r | (g<<8) | (b<<16)
        // (i.e. R in the low byte), which is the inverse of the convention used
        // by BmpReader / PngReader. We mirror GifReader so files round-trip.
        ReadOnlySpan<uint> palette = paletteMem.Span;
        byte[] gct = new byte[entries * 3];
        for (int i = 0; i < entries; i++)
        {
            uint rgba = i < palette.Length ? palette[i] : 0u;
            gct[i * 3 + 0] = (byte)(rgba & 0xFF);          // R
            gct[i * 3 + 1] = (byte)((rgba >> 8) & 0xFF);   // G
            gct[i * 3 + 2] = (byte)((rgba >> 16) & 0xFF);  // B
        }
        return gct;
    }

    private static byte[] BuildImageDescriptor(int width, int height)
    {
        return
        [
            0x2C,
            0x00, 0x00, // left
            0x00, 0x00, // top
            (byte)(width & 0xFF), (byte)((width >> 8) & 0xFF),
            (byte)(height & 0xFF), (byte)((height >> 8) & 0xFF),
            0x00, // no LCT, not interlaced, not sorted
        ];
    }

    private static byte[] TightlyPack(ImageFrame frame)
    {
        if (frame.Stride == frame.Width) return frame.Pixels.ToArray();
        byte[] dst = new byte[frame.Width * frame.Height];
        ReadOnlySpan<byte> src = frame.Pixels.Span;
        for (int y = 0; y < frame.Height; y++)
            src.Slice(y * frame.Stride, frame.Width).CopyTo(dst.AsSpan(y * frame.Width));
        return dst;
    }

    private async ValueTask WriteAsSubBlocksAsync(byte[] data, CancellationToken cancellationToken)
    {
        // GIF wraps the raw LZW stream in length-prefixed chunks of ≤255 bytes
        // each, terminated by an empty (length=0) chunk.
        int offset = 0;
        byte[] header = new byte[1];
        while (offset < data.Length)
        {
            int chunk = Math.Min(255, data.Length - offset);
            header[0] = (byte)chunk;
            await _stream.WriteAsync(header, cancellationToken).ConfigureAwait(false);
            await _stream.WriteAsync(data.AsMemory(offset, chunk), cancellationToken).ConfigureAwait(false);
            offset += chunk;
        }
        header[0] = 0;
        await _stream.WriteAsync(header, cancellationToken).ConfigureAwait(false);
    }

    private static int NextPow2(int n)
    {
        int p = 1;
        while (p < n) p <<= 1;
        return p;
    }

    private static int Log2(int n)
    {
        int b = 0;
        while ((1 << b) < n) b++;
        return b;
    }

    private static class GifBytes
    {
        public static readonly byte[] Header =
        [
            (byte)'G', (byte)'I', (byte)'F', (byte)'8', (byte)'9', (byte)'a',
        ];
        public static readonly byte[] Trailer = [0x3B];
    }
}
