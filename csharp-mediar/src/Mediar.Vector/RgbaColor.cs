namespace Mediar.Vector;

/// <summary>
/// A non-premultiplied straight-alpha 32-bit RGBA color in linearised
/// channel order. Components are normalised floats in the closed range
/// [0, 1] so gradient interpolation and coverage compositing can be done
/// without per-pixel integer rounding.
/// </summary>
public readonly record struct RgbaColor(float R, float G, float B, float A)
{
    /// <summary>Fully transparent black (the SVG default "fill" when nothing is specified).</summary>
    public static RgbaColor Transparent => new(0, 0, 0, 0);

    /// <summary>Opaque black.</summary>
    public static RgbaColor Black => new(0, 0, 0, 1);

    /// <summary>Opaque white.</summary>
    public static RgbaColor White => new(1, 1, 1, 1);

    /// <summary>
    /// Build from 8-bit sRGB components (the natural SVG / CSS encoding).
    /// </summary>
    public static RgbaColor FromBytes(byte r, byte g, byte b, byte a = 255) =>
        new(r / 255f, g / 255f, b / 255f, a / 255f);

    /// <summary>
    /// Pack as a 32-bit BGRA value with the byte at offset 0 = B (the
    /// memory order expected by <c>PixelFormat.Bgra32</c>).
    /// </summary>
    public uint ToBgra32()
    {
        byte r = ToByte(R);
        byte g = ToByte(G);
        byte b = ToByte(B);
        byte a = ToByte(A);
        return ((uint)a << 24) | ((uint)r << 16) | ((uint)g << 8) | b;
    }

    /// <summary>
    /// Pack as a 32-bit RGBA value with the byte at offset 0 = R.
    /// </summary>
    public uint ToRgba32()
    {
        byte r = ToByte(R);
        byte g = ToByte(G);
        byte b = ToByte(B);
        byte a = ToByte(A);
        return ((uint)a << 24) | ((uint)b << 16) | ((uint)g << 8) | r;
    }

    /// <summary>
    /// Multiply alpha by an external opacity (cascading SVG <c>opacity</c>).
    /// </summary>
    public RgbaColor WithOpacity(float opacity) => this with { A = A * Math.Clamp(opacity, 0f, 1f) };

    /// <summary>
    /// Linear-RGB lerp suitable for gradient stop interpolation.
    /// </summary>
    public static RgbaColor Lerp(RgbaColor a, RgbaColor b, float t)
    {
        t = Math.Clamp(t, 0f, 1f);
        return new RgbaColor(
            a.R + (b.R - a.R) * t,
            a.G + (b.G - a.G) * t,
            a.B + (b.B - a.B) * t,
            a.A + (b.A - a.A) * t);
    }

    private static byte ToByte(float v)
    {
        if (float.IsNaN(v)) return 0;
        v = Math.Clamp(v, 0f, 1f);
        return (byte)(v * 255f + 0.5f);
    }
}
