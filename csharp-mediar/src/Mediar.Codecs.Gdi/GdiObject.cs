using Mediar.Vector;

namespace Mediar.Codecs.Gdi;

/// <summary>
/// Base type for everything that can be stored in a GDI object table
/// (the per-DC slot array selected through EMR_SELECTOBJECT / META_SELECTOBJECT).
/// </summary>
public abstract record GdiObject;

/// <summary>
/// Standard pen styles per MS-WMF <c>PenStyle</c> / MS-EMF <c>PenStyle</c>.
/// Only the low byte ("style") is interpreted here; cosmetic vs geometric and
/// the join/cap/iconic bits are parsed but stroker only acts on cap / join.
/// </summary>
public enum GdiPenStyle
{
    /// <summary>PS_SOLID - continuous line.</summary>
    Solid = 0,
    /// <summary>PS_DASH - dashed line.</summary>
    Dash = 1,
    /// <summary>PS_DOT - dotted line.</summary>
    Dot = 2,
    /// <summary>PS_DASHDOT - dash dot dash dot ....</summary>
    DashDot = 3,
    /// <summary>PS_DASHDOTDOT - dash dot dot dash dot dot ....</summary>
    DashDotDot = 4,
    /// <summary>PS_NULL - no stroke is emitted at all.</summary>
    Null = 5,
    /// <summary>PS_INSIDEFRAME - draw inside the frame (treated as Solid for now).</summary>
    InsideFrame = 6,
    /// <summary>PS_USERSTYLE - caller-supplied dash array (EMR_EXTCREATEPEN).</summary>
    UserStyle = 7,
    /// <summary>PS_ALTERNATE - every other pixel (treated as Dot).</summary>
    Alternate = 8,
}

/// <summary>
/// Brush style enum (subset of MS-WMF <c>BrushStyle</c>). Hatch patterns
/// are parsed but rendered as a 50%-alpha solid for this Mediar release.
/// </summary>
public enum GdiBrushStyle
{
    /// <summary>BS_SOLID - fill with a solid colour.</summary>
    Solid = 0,
    /// <summary>BS_NULL / BS_HOLLOW - no fill.</summary>
    Null = 1,
    /// <summary>BS_HATCHED - hatch pattern (rendered as half-alpha solid).</summary>
    Hatched = 2,
    /// <summary>BS_PATTERN - bitmap pattern (rendered as half-alpha solid).</summary>
    Pattern = 3,
    /// <summary>BS_DIBPATTERN - DIB pattern (rendered as half-alpha solid).</summary>
    DibPattern = 5,
}

/// <summary>
/// A GDI logical pen. <see cref="Width"/> is in logical units and must be
/// multiplied by the current world-transform scale to get device pixels.
/// </summary>
public sealed record GdiPen(
    GdiPenStyle Style,
    float Width,
    RgbaColor Color,
    LineCap Cap = LineCap.Butt,
    LineJoin Join = LineJoin.Miter,
    IReadOnlyList<float>? UserDashArray = null) : GdiObject
{
    /// <summary>The default "stock pen" inserted into every fresh DC.</summary>
    public static GdiPen Default { get; } = new(GdiPenStyle.Solid, 1f, RgbaColor.Black);

    /// <summary>True if this pen should not draw anything (PS_NULL).</summary>
    public bool IsNullPen => Style == GdiPenStyle.Null;
}

/// <summary>
/// A GDI logical brush.
/// </summary>
public sealed record GdiBrush(
    GdiBrushStyle Style,
    RgbaColor Color) : GdiObject
{
    /// <summary>The default "stock brush" inserted into every fresh DC.</summary>
    public static GdiBrush Default { get; } = new(GdiBrushStyle.Solid, RgbaColor.White);

    /// <summary>True if this brush should not produce any fill (BS_NULL).</summary>
    public bool IsNullBrush => Style == GdiBrushStyle.Null;
}
