using System.Buffers;

namespace Mediar.Imaging;

/// <summary>
/// A single decoded image frame. Pixel data is laid out in a tight
/// <see cref="Stride"/>-byte rows, top-down (row 0 = top of image).
/// </summary>
/// <remarks>
/// Frames are pooled - call <see cref="Dispose"/> (or use <c>using</c>) to
/// return the underlying buffer to <see cref="ArrayPool{T}.Shared"/>.
/// </remarks>
public sealed class ImageFrame : IDisposable
{
    private byte[]? _pooled;

    /// <summary>Frame width, pixels.</summary>
    public int Width { get; }

    /// <summary>Frame height, pixels.</summary>
    public int Height { get; }

    /// <summary>Pixel format of <see cref="Pixels"/>.</summary>
    public PixelFormat PixelFormat { get; }

    /// <summary>Bytes per row, including any end-of-row padding.</summary>
    public int Stride { get; }

    /// <summary>Tightly-packed, top-down pixel data of length <c>Height * Stride</c>.</summary>
    public ReadOnlyMemory<byte> Pixels { get; }

    /// <summary>For palette-indexed formats, the active palette as RGBA8888 entries.</summary>
    public ReadOnlyMemory<uint> Palette { get; }

    /// <summary>Frame display duration for animated images. <see cref="TimeSpan.Zero"/> for stills.</summary>
    public TimeSpan Duration { get; init; }

    /// <summary>X offset for compositing (APNG / GIF).</summary>
    public int OffsetX { get; init; }

    /// <summary>Y offset for compositing (APNG / GIF).</summary>
    public int OffsetY { get; init; }

    /// <summary>Constructs a new frame around <paramref name="pixels"/>.</summary>
    public ImageFrame(
        int width, int height, PixelFormat pixelFormat,
        int stride, byte[] pixels, byte[]? pooledOwner = null,
        ReadOnlyMemory<uint> palette = default)
    {
        ArgumentNullException.ThrowIfNull(pixels);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(stride);

        Width = width;
        Height = height;
        PixelFormat = pixelFormat;
        Stride = stride;
        Pixels = new ReadOnlyMemory<byte>(pixels, 0, height * stride);
        Palette = palette;
        _pooled = pooledOwner;
    }

    /// <summary>
    /// Rents and returns a frame backed by <see cref="ArrayPool{T}.Shared"/>.
    /// The returned byte[] is exposed as the frame's pixel buffer.
    /// </summary>
    public static (ImageFrame Frame, byte[] Buffer) Rent(
        int width, int height, PixelFormat pixelFormat, int stride,
        ReadOnlyMemory<uint> palette = default)
    {
        var buf = ArrayPool<byte>.Shared.Rent(height * stride);
        var f = new ImageFrame(width, height, pixelFormat, stride, buf, buf, palette);
        return (f, buf);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_pooled is { } p)
        {
            _pooled = null;
            ArrayPool<byte>.Shared.Return(p);
        }
    }
}
