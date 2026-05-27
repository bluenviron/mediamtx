using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// A mutable 32-bit BGRA pixel surface. The compositor and rasterizer
/// write through this object so callers can resue a single buffer for
/// many draw operations. Surface storage is owned by the caller -
/// <see cref="RasterTarget"/> does not allocate.
/// </summary>
public sealed class RasterTarget
{
    private readonly byte[] _pixels;
    private readonly int _stride;

    /// <summary>Surface width in pixels.</summary>
    public int Width { get; }
    /// <summary>Surface height in pixels.</summary>
    public int Height { get; }
    /// <summary>Byte stride between successive rows.</summary>
    public int Stride => _stride;
    /// <summary>Raw pixel buffer.</summary>
    public Span<byte> Pixels => _pixels.AsSpan(0, Height * Stride);

    /// <summary>Build a target around an already-allocated <paramref name="pixels"/> buffer.</summary>
    public RasterTarget(int width, int height, byte[] pixels, int? stride = null)
    {
        ArgumentNullException.ThrowIfNull(pixels);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(width);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(height);
        Width = width;
        Height = height;
        _stride = stride ?? (width * 4);
        if (pixels.Length < Height * _stride)
            throw new ArgumentException("Pixel buffer too small for the requested width / height / stride.", nameof(pixels));
        _pixels = pixels;
    }

    /// <summary>Allocate a fresh surface filled with <paramref name="background"/>.</summary>
    public static RasterTarget Create(int width, int height, RgbaColor background = default)
    {
        int stride = width * 4;
        byte[] buf = new byte[height * stride];
        var t = new RasterTarget(width, height, buf, stride);
        t.Clear(background);
        return t;
    }

    /// <summary>Fill every pixel with <paramref name="color"/>.</summary>
    public void Clear(RgbaColor color)
    {
        uint packed = color.ToBgra32();
        var span = Pixels;
        for (int y = 0; y < Height; y++)
        {
            int row = y * Stride;
            for (int x = 0; x < Width; x++)
            {
                int i = row + x * 4;
                span[i + 0] = (byte)(packed & 0xFF);
                span[i + 1] = (byte)((packed >> 8) & 0xFF);
                span[i + 2] = (byte)((packed >> 16) & 0xFF);
                span[i + 3] = (byte)((packed >> 24) & 0xFF);
            }
        }
    }

    /// <summary>
    /// Source-over blend a single span of pixels with per-column 0..255
    /// coverage. <paramref name="evaluator"/> resolves the paint colour at
    /// each pixel (this is what gives us gradients).
    /// </summary>
    public void BlendSpan(int y, int x0, ReadOnlySpan<byte> coverage, IPaintEvaluator evaluator)
    {
        if ((uint)y >= (uint)Height) return;
        int rowOff = y * Stride;
        for (int i = 0; i < coverage.Length; i++)
        {
            int xi = x0 + i;
            if ((uint)xi >= (uint)Width) continue;
            int cov = coverage[i];
            if (cov == 0) continue;
            RgbaColor src = evaluator.Evaluate(xi + 0.5f, y + 0.5f);
            float effAlpha = src.A * (cov / 255f);
            if (effAlpha <= 0) continue;

            int pi = rowOff + xi * 4;
            float invA = 1f - effAlpha;
            byte db = _pixels[pi + 0];
            byte dg = _pixels[pi + 1];
            byte dr = _pixels[pi + 2];
            byte da = _pixels[pi + 3];

            // Convert dst to straight float, blend, convert back. Source is straight-alpha already.
            float dA = da / 255f;
            float oA = effAlpha + dA * invA;
            if (oA <= 0)
            {
                _pixels[pi + 0] = 0;
                _pixels[pi + 1] = 0;
                _pixels[pi + 2] = 0;
                _pixels[pi + 3] = 0;
                continue;
            }
            float oR = (src.R * effAlpha + (dr / 255f) * dA * invA) / oA;
            float oG = (src.G * effAlpha + (dg / 255f) * dA * invA) / oA;
            float oB = (src.B * effAlpha + (db / 255f) * dA * invA) / oA;

            _pixels[pi + 0] = ToByte(oB);
            _pixels[pi + 1] = ToByte(oG);
            _pixels[pi + 2] = ToByte(oR);
            _pixels[pi + 3] = ToByte(oA);
        }
    }

    private static byte ToByte(float v) => (byte)(Math.Clamp(v, 0f, 1f) * 255f + 0.5f);
}

/// <summary>
/// Returns the paint colour at a given device-space pixel centre. Solid
/// paints ignore the inputs; gradients compute parametric distance and
/// look up the stop array.
/// </summary>
public interface IPaintEvaluator
{
    /// <summary>Evaluate the paint at device-space pixel centre <c>(x, y)</c>.</summary>
    RgbaColor Evaluate(float x, float y);
}

/// <summary>Solid-color paint evaluator.</summary>
public sealed class SolidEvaluator(RgbaColor color) : IPaintEvaluator
{
    private readonly RgbaColor _color = color;
    /// <inheritdoc/>
    public RgbaColor Evaluate(float x, float y) => _color;
}

/// <summary>Linear-gradient paint evaluator (pre-baked into device space).</summary>
public sealed class LinearGradientEvaluator : IPaintEvaluator
{
    private readonly GradientStop[] _stops;
    private readonly GradientSpread _spread;
    private readonly float _x1, _y1, _dx, _dy, _lenSq;

    /// <summary>
    /// Create an evaluator. The gradient endpoints are in device space.
    /// </summary>
    public LinearGradientEvaluator(Vector2 p1, Vector2 p2, IReadOnlyList<GradientStop> stops, GradientSpread spread)
    {
        ArgumentNullException.ThrowIfNull(stops);
        _stops = SortAndFillStops(stops);
        _spread = spread;
        _x1 = p1.X; _y1 = p1.Y;
        _dx = p2.X - p1.X; _dy = p2.Y - p1.Y;
        _lenSq = _dx * _dx + _dy * _dy;
    }

    /// <inheritdoc/>
    public RgbaColor Evaluate(float x, float y)
    {
        if (_lenSq < 1e-9f) return _stops[0].Color;
        float t = ((x - _x1) * _dx + (y - _y1) * _dy) / _lenSq;
        t = ApplySpread(t, _spread);
        return SampleStops(_stops, t);
    }

    internal static GradientStop[] SortAndFillStops(IReadOnlyList<GradientStop> stops)
    {
        if (stops.Count == 0) return [new GradientStop(0, RgbaColor.Transparent)];
        var sorted = stops.OrderBy(s => s.Offset).ToList();
        if (sorted[0].Offset > 0) sorted.Insert(0, new GradientStop(0, sorted[0].Color));
        if (sorted[^1].Offset < 1) sorted.Add(new GradientStop(1, sorted[^1].Color));
        // Per SVG, equal offsets are honoured in order; clamp monotonic by tiny epsilon.
        for (int i = 1; i < sorted.Count; i++)
            if (sorted[i].Offset < sorted[i - 1].Offset)
                sorted[i] = sorted[i] with { Offset = sorted[i - 1].Offset };
        return [.. sorted];
    }

    internal static float ApplySpread(float t, GradientSpread spread) => spread switch
    {
        GradientSpread.Pad => Math.Clamp(t, 0f, 1f),
        GradientSpread.Repeat => t - MathF.Floor(t),
        GradientSpread.Reflect => Reflect(t),
        _ => Math.Clamp(t, 0f, 1f),
    };

    internal static float Reflect(float t)
    {
        // Period 2, with mirror at [0,1] -> identity, (1,2] -> 2 - t.
        float m = t - 2f * MathF.Floor(t / 2f);
        return m > 1f ? 2f - m : m;
    }

    internal static RgbaColor SampleStops(GradientStop[] stops, float t)
    {
        if (t <= stops[0].Offset) return stops[0].Color;
        if (t >= stops[^1].Offset) return stops[^1].Color;
        for (int i = 1; i < stops.Length; i++)
        {
            if (t <= stops[i].Offset)
            {
                var a = stops[i - 1];
                var b = stops[i];
                float span = b.Offset - a.Offset;
                float u = span <= 0 ? 0 : (t - a.Offset) / span;
                return RgbaColor.Lerp(a.Color, b.Color, u);
            }
        }
        return stops[^1].Color;
    }
}

/// <summary>Radial-gradient paint evaluator (pre-baked into device space).</summary>
public sealed class RadialGradientEvaluator : IPaintEvaluator
{
    private readonly GradientStop[] _stops;
    private readonly GradientSpread _spread;
    private readonly float _cx, _cy, _r, _fx, _fy;

    /// <summary>
    /// Build an evaluator. Centre, focal point and radius are in device space.
    /// </summary>
    public RadialGradientEvaluator(Vector2 c, float r, Vector2 f, IReadOnlyList<GradientStop> stops, GradientSpread spread)
    {
        ArgumentNullException.ThrowIfNull(stops);
        _stops = LinearGradientEvaluator.SortAndFillStops(stops);
        _spread = spread;
        _cx = c.X; _cy = c.Y; _r = r; _fx = f.X; _fy = f.Y;
    }

    /// <inheritdoc/>
    public RgbaColor Evaluate(float x, float y)
    {
        // Classic two-point radial: solve for t such that the focal-to-pixel
        // ray exits the unit circle (per SVG 1.1 "Radial Gradient Math").
        if (_r <= 0) return _stops[^1].Color;

        float dx = x - _fx, dy = y - _fy;
        float ox = _fx - _cx, oy = _fy - _cy;

        float a = dx * dx + dy * dy;
        // Query point coincides with the focal point: this is t = 0.
        if (a == 0) return _stops[0].Color;
        float b = 2 * (ox * dx + oy * dy);
        float c = ox * ox + oy * oy - _r * _r;
        float disc = b * b - 4 * a * c;
        if (disc < 0) return _stops[^1].Color;
        float t = (-b + MathF.Sqrt(disc)) / (2 * a);
        if (t <= 0) return _stops[0].Color;
        float u = 1f / t;
        u = LinearGradientEvaluator.ApplySpread(u, _spread);
        return LinearGradientEvaluator.SampleStops(_stops, u);
    }
}
