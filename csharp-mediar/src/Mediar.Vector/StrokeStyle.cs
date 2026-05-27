namespace Mediar.Vector;

/// <summary>SVG/PDF/PostScript path fill rules.</summary>
public enum FillRule
{
    /// <summary>Non-zero winding (the SVG default).</summary>
    NonZero,
    /// <summary>Even-odd winding ("fill-rule: evenodd").</summary>
    EvenOdd,
}

/// <summary>SVG stroke line-cap.</summary>
public enum LineCap
{
    /// <summary>No cap; stroke ends exactly at endpoint.</summary>
    Butt,
    /// <summary>Round cap with radius = half stroke width.</summary>
    Round,
    /// <summary>Square cap that extends half a stroke width past the endpoint.</summary>
    Square,
}

/// <summary>SVG stroke line-join.</summary>
public enum LineJoin
{
    /// <summary>Sharp miter, falls back to <see cref="Bevel"/> at <c>miterlimit</c>.</summary>
    Miter,
    /// <summary>Round join (arc with radius = half stroke width).</summary>
    Round,
    /// <summary>Bevel join (straight cut between the offset edges).</summary>
    Bevel,
}

/// <summary>
/// Stroke parameters - all of these are simple value-types so they cascade
/// cheaply.
/// </summary>
public readonly record struct StrokeStyle(
    float Width,
    LineCap Cap = LineCap.Butt,
    LineJoin Join = LineJoin.Miter,
    float MiterLimit = 4f,
    IReadOnlyList<float>? DashArray = null,
    float DashOffset = 0f)
{
    /// <summary>Default 1-pixel-wide butt-capped miter-joined stroke (SVG default).</summary>
    public static StrokeStyle Default => new(1f);
}
