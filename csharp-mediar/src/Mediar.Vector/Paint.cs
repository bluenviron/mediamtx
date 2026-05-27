namespace Mediar.Vector;

/// <summary>
/// One color stop in a gradient. <see cref="Offset"/> is in [0, 1].
/// </summary>
public readonly record struct GradientStop(float Offset, RgbaColor Color);

/// <summary>
/// How a gradient samples values outside the [0, 1] parametric range.
/// </summary>
public enum GradientSpread
{
    /// <summary>Repeat the end colors (SVG default).</summary>
    Pad,
    /// <summary>Mirror at each end.</summary>
    Reflect,
    /// <summary>Repeat the gradient pattern.</summary>
    Repeat,
}

/// <summary>
/// Coordinate space a gradient's positional parameters are expressed in.
/// </summary>
public enum GradientUnits
{
    /// <summary>Use the painted shape's bounding box (the SVG default).</summary>
    ObjectBoundingBox,
    /// <summary>Use the canvas / user coordinate system directly.</summary>
    UserSpaceOnUse,
}

/// <summary>
/// Base type for fill / stroke paints.
/// </summary>
public abstract record Paint
{
    /// <summary>Singleton "no paint" sentinel ("fill=none" / "stroke=none").</summary>
    public static Paint None { get; } = new NonePaint();

    private sealed record NonePaint : Paint;
}

/// <summary>A flat solid color paint.</summary>
public sealed record SolidPaint(RgbaColor Color) : Paint;

/// <summary>
/// SVG-style linear gradient. Goes from <c>(X1, Y1)</c> to <c>(X2, Y2)</c>
/// in coordinates determined by <see cref="Units"/>; any
/// <see cref="GradientTransform"/> is applied on top.
/// </summary>
public sealed record LinearGradientPaint(
    float X1, float Y1, float X2, float Y2,
    IReadOnlyList<GradientStop> Stops,
    GradientUnits Units = GradientUnits.ObjectBoundingBox,
    GradientSpread Spread = GradientSpread.Pad,
    System.Numerics.Matrix3x2? GradientTransform = null) : Paint;

/// <summary>
/// SVG-style radial gradient. Outer circle is centred at <c>(Cx, Cy)</c>
/// with radius <see cref="R"/>; the focal point is <c>(Fx, Fy)</c>.
/// </summary>
public sealed record RadialGradientPaint(
    float Cx, float Cy, float R, float Fx, float Fy,
    IReadOnlyList<GradientStop> Stops,
    GradientUnits Units = GradientUnits.ObjectBoundingBox,
    GradientSpread Spread = GradientSpread.Pad,
    System.Numerics.Matrix3x2? GradientTransform = null) : Paint;
