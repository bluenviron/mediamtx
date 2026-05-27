namespace Mediar.Codecs.Apng;

/// <summary>
/// Maintains a running 4-byte-per-pixel canvas onto which animation sub-frames
/// are composited per the APNG specification's <c>dispose_op</c> and
/// <c>blend_op</c> rules.
/// </summary>
/// <remarks>
/// <para>
/// The compositor is reusable across any animation container that follows the
/// same canvas-blend-restore model. APNG callers store RGBA32; WebP callers
/// store BGRA32. The SRC-OVER blend math only references the alpha channel
/// at byte offset 3, so any consistent 4-byte pixel order with alpha last
/// composites correctly.
/// </para>
/// <para>
/// Use <see cref="BackgroundRgba"/> to configure the fill colour for both the
/// initial <see cref="Clear"/> state and the <see cref="ApngDisposeOp.Background"/>
/// dispose path. For APNG the default zero (transparent black) matches the
/// specification; WebP callers should set this to the ANIM chunk background
/// before calling <see cref="Clear"/>.
/// </para>
/// <para>
/// Render order:
/// </para>
/// <list type="number">
///   <item>Apply the *queued* disposal from the previous frame (if any).</item>
///   <item>Save the affected canvas region (if this frame's dispose is <see cref="ApngDisposeOp.Previous"/>).</item>
///   <item>Blit / blend this frame's pixels onto the canvas.</item>
///   <item>Queue this frame's dispose for the next call.</item>
/// </list>
/// <para>
/// Callers should take a snapshot of <see cref="Canvas"/> AFTER each call to
/// <see cref="Render"/> to obtain the visible frame at that point in time;
/// the disposal effect happens at the START of the next call.
/// </para>
/// </remarks>
public sealed class ApngCompositor
{
    private readonly byte[] _canvas;
    private byte[]? _previous;
    private int _queuedDx;
    private int _queuedDy;
    private int _queuedDw;
    private int _queuedDh;
    private ApngDisposeOp _queuedDispose;
    private bool _hasQueued;
    private bool _hasPreviousSave;
    private bool _firstFrame;

    /// <summary>Canvas width in pixels.</summary>
    public int Width { get; }

    /// <summary>Canvas height in pixels.</summary>
    public int Height { get; }

    /// <summary>Bytes per row of the canvas (always <c>Width * 4</c>).</summary>
    public int Stride => Width * 4;

    /// <summary>Live view of the canvas pixel buffer. Length = <c>Width * Height * 4</c>.</summary>
    public ReadOnlySpan<byte> Canvas => _canvas;

    /// <summary>
    /// 4-byte fill pattern used by <see cref="Clear"/> and by the
    /// <see cref="ApngDisposeOp.Background"/> path. Defaults to all zeros
    /// (transparent black) which matches the APNG specification. WebP callers
    /// should assign the ANIM background colour bytes before calling
    /// <see cref="Clear"/>. Pixel byte order is whatever the caller's canvas
    /// uses (RGBA32 for APNG, BGRA32 for WebP); the compositor stores the
    /// four bytes verbatim and does not interpret channel meaning beyond
    /// alpha being the last byte for blending purposes.
    /// </summary>
    public uint BackgroundRgba { get; set; }

    /// <summary>Constructs a new transparent-black canvas of the given size.</summary>
    public ApngCompositor(int width, int height)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);
        Width = width;
        Height = height;
        _canvas = new byte[width * height * 4];
        _firstFrame = true;
    }

    /// <summary>Clears the canvas to <see cref="BackgroundRgba"/> and resets all queued state.</summary>
    public void Clear()
    {
        FillRect(0, 0, Width, Height);
        _previous = null;
        _hasPreviousSave = false;
        _hasQueued = false;
        _queuedDispose = ApngDisposeOp.None;
        _firstFrame = true;
    }

    /// <summary>Returns a fresh heap-allocated copy of the current canvas pixels.</summary>
    public byte[] Snapshot()
    {
        var copy = new byte[_canvas.Length];
        Buffer.BlockCopy(_canvas, 0, copy, 0, _canvas.Length);
        return copy;
    }

    /// <summary>
    /// Composites a single sub-frame onto the canvas per the APNG rules.
    /// </summary>
    /// <param name="srcRgba32">
    /// Source pixels in RGBA32 (R G B A, byte order from offset 0) with rows
    /// of <paramref name="srcStride"/> bytes. The first <c>frameWidth * 4</c>
    /// bytes of each row are consumed.
    /// </param>
    /// <param name="srcStride">Bytes per row of <paramref name="srcRgba32"/>.</param>
    /// <param name="frameWidth">Width of the sub-frame in pixels.</param>
    /// <param name="frameHeight">Height of the sub-frame in pixels.</param>
    /// <param name="offsetX">X coordinate on the canvas where the frame is placed.</param>
    /// <param name="offsetY">Y coordinate on the canvas where the frame is placed.</param>
    /// <param name="blend">Blend operation for this frame.</param>
    /// <param name="dispose">Dispose operation queued for after this frame.</param>
    public void Render(
        ReadOnlySpan<byte> srcRgba32, int srcStride,
        int frameWidth, int frameHeight,
        int offsetX, int offsetY,
        ApngBlendOp blend, ApngDisposeOp dispose)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(frameWidth);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(frameHeight);
        ArgumentOutOfRangeException.ThrowIfNegative(offsetX);
        ArgumentOutOfRangeException.ThrowIfNegative(offsetY);
        if (offsetX + frameWidth > Width || offsetY + frameHeight > Height)
            throw new ArgumentException("Frame extends past canvas bounds.");
        if (srcStride < frameWidth * 4)
            throw new ArgumentException("srcStride too small for frameWidth.");
        if (srcRgba32.Length < srcStride * frameHeight)
            throw new ArgumentException("srcRgba32 too short for declared dimensions.");

        if (_hasQueued)
        {
            ApplyDispose(_queuedDispose, _queuedDx, _queuedDy, _queuedDw, _queuedDh);
        }

        ApngDisposeOp effectiveDispose = dispose;
        if (_firstFrame && dispose == ApngDisposeOp.Previous)
        {
            effectiveDispose = ApngDisposeOp.Background;
        }

        if (effectiveDispose == ApngDisposeOp.Previous)
        {
            SavePrevious(offsetX, offsetY, frameWidth, frameHeight);
        }
        else
        {
            _hasPreviousSave = false;
        }

        ApngBlendOp effectiveBlend = blend;
        if (_firstFrame && blend == ApngBlendOp.Over)
        {
            effectiveBlend = ApngBlendOp.Source;
        }

        if (effectiveBlend == ApngBlendOp.Source)
        {
            BlitSource(srcRgba32, srcStride, frameWidth, frameHeight, offsetX, offsetY);
        }
        else
        {
            BlitOver(srcRgba32, srcStride, frameWidth, frameHeight, offsetX, offsetY);
        }

        _queuedDispose = effectiveDispose;
        _queuedDx = offsetX;
        _queuedDy = offsetY;
        _queuedDw = frameWidth;
        _queuedDh = frameHeight;
        _hasQueued = true;
        _firstFrame = false;
    }

    private void ApplyDispose(ApngDisposeOp op, int dx, int dy, int dw, int dh)
    {
        switch (op)
        {
            case ApngDisposeOp.None:
                break;
            case ApngDisposeOp.Background:
                ClearRect(dx, dy, dw, dh);
                break;
            case ApngDisposeOp.Previous:
                if (_hasPreviousSave && _previous is not null)
                {
                    RestoreRect(_previous, dx, dy, dw, dh);
                }
                else
                {
                    ClearRect(dx, dy, dw, dh);
                }
                break;
            default:
                break;
        }
    }

    private void ClearRect(int x, int y, int w, int h) => FillRect(x, y, w, h);

    private void FillRect(int x, int y, int w, int h)
    {
        if (BackgroundRgba == 0)
        {
            for (int row = 0; row < h; row++)
            {
                int dstOffset = ((y + row) * Width + x) * 4;
                Array.Clear(_canvas, dstOffset, w * 4);
            }
            return;
        }

        byte b0 = (byte)(BackgroundRgba & 0xFF);
        byte b1 = (byte)((BackgroundRgba >> 8) & 0xFF);
        byte b2 = (byte)((BackgroundRgba >> 16) & 0xFF);
        byte b3 = (byte)((BackgroundRgba >> 24) & 0xFF);
        for (int row = 0; row < h; row++)
        {
            int dstOffset = ((y + row) * Width + x) * 4;
            for (int col = 0; col < w; col++)
            {
                int o = dstOffset + col * 4;
                _canvas[o + 0] = b0;
                _canvas[o + 1] = b1;
                _canvas[o + 2] = b2;
                _canvas[o + 3] = b3;
            }
        }
    }

    private void SavePrevious(int x, int y, int w, int h)
    {
        int needed = w * h * 4;
        if (_previous is null || _previous.Length < needed)
        {
            _previous = new byte[needed];
        }
        for (int row = 0; row < h; row++)
        {
            int srcOffset = ((y + row) * Width + x) * 4;
            Buffer.BlockCopy(_canvas, srcOffset, _previous, row * w * 4, w * 4);
        }
        _hasPreviousSave = true;
    }

    private void RestoreRect(byte[] previous, int x, int y, int w, int h)
    {
        for (int row = 0; row < h; row++)
        {
            int dstOffset = ((y + row) * Width + x) * 4;
            Buffer.BlockCopy(previous, row * w * 4, _canvas, dstOffset, w * 4);
        }
    }

    private void BlitSource(
        ReadOnlySpan<byte> src, int srcStride,
        int w, int h, int dx, int dy)
    {
        int rowBytes = w * 4;
        for (int row = 0; row < h; row++)
        {
            int dstOffset = ((dy + row) * Width + dx) * 4;
            src.Slice(row * srcStride, rowBytes).CopyTo(_canvas.AsSpan(dstOffset, rowBytes));
        }
    }

    private void BlitOver(
        ReadOnlySpan<byte> src, int srcStride,
        int w, int h, int dx, int dy)
    {
        for (int row = 0; row < h; row++)
        {
            int srcRow = row * srcStride;
            int dstRow = ((dy + row) * Width + dx) * 4;
            for (int col = 0; col < w; col++)
            {
                int s = srcRow + col * 4;
                int d = dstRow + col * 4;
                byte sa = src[s + 3];
                if (sa == 0xFF)
                {
                    _canvas[d + 0] = src[s + 0];
                    _canvas[d + 1] = src[s + 1];
                    _canvas[d + 2] = src[s + 2];
                    _canvas[d + 3] = 0xFF;
                }
                else if (sa == 0x00)
                {
                    // Transparent source: canvas unchanged.
                }
                else
                {
                    byte da = _canvas[d + 3];
                    int outA = sa + da * (255 - sa) / 255;
                    if (outA == 0)
                    {
                        _canvas[d + 0] = 0;
                        _canvas[d + 1] = 0;
                        _canvas[d + 2] = 0;
                        _canvas[d + 3] = 0;
                    }
                    else
                    {
                        int outR = (src[s + 0] * sa + _canvas[d + 0] * da * (255 - sa) / 255) / outA;
                        int outG = (src[s + 1] * sa + _canvas[d + 1] * da * (255 - sa) / 255) / outA;
                        int outB = (src[s + 2] * sa + _canvas[d + 2] * da * (255 - sa) / 255) / outA;
                        _canvas[d + 0] = (byte)outR;
                        _canvas[d + 1] = (byte)outG;
                        _canvas[d + 2] = (byte)outB;
                        _canvas[d + 3] = (byte)outA;
                    }
                }
            }
        }
    }
}
