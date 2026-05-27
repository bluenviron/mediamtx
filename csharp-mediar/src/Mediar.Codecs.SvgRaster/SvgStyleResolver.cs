using System.Globalization;
using System.Xml.Linq;
using Mediar.Vector;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// Resolved SVG presentation state after walking down the element tree.
/// Inherits from parent and replaces values found on the current element.
/// </summary>
public sealed record SvgStyle
{
    /// <summary>Fill paint - <see cref="Paint.None"/> means "no fill".</summary>
    public Paint Fill { get; init; } = new SolidPaint(RgbaColor.Black);
    /// <summary>Stroke paint - defaults to no stroke per SVG.</summary>
    public Paint Stroke { get; init; } = Paint.None;
    /// <summary>Stroke width in user units.</summary>
    public float StrokeWidth { get; init; } = 1f;
    /// <summary>Line cap.</summary>
    public LineCap StrokeLineCap { get; init; } = LineCap.Butt;
    /// <summary>Line join.</summary>
    public LineJoin StrokeLineJoin { get; init; } = LineJoin.Miter;
    /// <summary>Miter limit.</summary>
    public float StrokeMiterLimit { get; init; } = 4f;
    /// <summary>Dash array.</summary>
    public IReadOnlyList<float>? StrokeDashArray { get; init; }
    /// <summary>Dash offset.</summary>
    public float StrokeDashOffset { get; init; }
    /// <summary>Cascaded opacity multiplier.</summary>
    public float Opacity { get; init; } = 1f;
    /// <summary>Fill-opacity multiplier (applied on top of <see cref="Opacity"/>).</summary>
    public float FillOpacity { get; init; } = 1f;
    /// <summary>Stroke-opacity multiplier.</summary>
    public float StrokeOpacity { get; init; } = 1f;
    /// <summary>Fill rule.</summary>
    public FillRule FillRule { get; init; } = FillRule.NonZero;
    /// <summary>Resolved <c>color</c> property (the value of <c>currentColor</c>).</summary>
    public RgbaColor CurrentColor { get; init; } = RgbaColor.Black;
    /// <summary>True if the element should not be rendered.</summary>
    public bool Display { get; init; } = true;
    /// <summary>True if the element is visible.</summary>
    public bool Visibility { get; init; } = true;
}

/// <summary>
/// Resolves an element's cascaded <see cref="SvgStyle"/> by walking
/// presentation attributes plus the inline <c>style="..."</c> attribute,
/// inheriting unspecified properties from the parent.
/// </summary>
public static class SvgStyleResolver
{
    /// <summary>
    /// Produce an effective style for <paramref name="element"/> given the
    /// inherited <paramref name="parent"/> style.
    /// </summary>
    public static SvgStyle Resolve(XElement element, SvgStyle parent, Func<string, Paint?>? paintRef = null)
    {
        ArgumentNullException.ThrowIfNull(element);
        ArgumentNullException.ThrowIfNull(parent);

        var values = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);

        // Inline style first - per CSS specificity, inline beats presentation
        // attributes which beats inherited values.
        string? inline = (string?)element.Attribute("style");
        if (inline is { Length: > 0 })
        {
            foreach (var decl in inline.Split(';', StringSplitOptions.RemoveEmptyEntries))
            {
                int colon = decl.IndexOf(':');
                if (colon <= 0) continue;
                string k = decl[..colon].Trim();
                string v = decl[(colon + 1)..].Trim();
                values[k] = v;
            }
        }

        // Presentation attributes (only set if not already specified inline).
        foreach (var attr in element.Attributes())
        {
            string n = attr.Name.LocalName;
            if (!values.ContainsKey(n)) values[n] = attr.Value;
        }

        // Resolve currentColor first - it's the basis everything else falls back to.
        var color = parent.CurrentColor;
        if (values.TryGetValue("color", out string? colorStr) &&
            SvgColorParser.TryParse(colorStr, out var c) && c != SvgColorParser.CurrentColorSentinel)
        {
            color = c;
        }

        Paint fill = parent.Fill;
        if (values.TryGetValue("fill", out string? fillStr))
            fill = ResolvePaint(fillStr, color, parent.Fill, paintRef);

        Paint stroke = parent.Stroke;
        if (values.TryGetValue("stroke", out string? strokeStr))
            stroke = ResolvePaint(strokeStr, color, parent.Stroke, paintRef);

        return new SvgStyle
        {
            Fill = fill,
            Stroke = stroke,
            StrokeWidth = values.TryGetValue("stroke-width", out string? sw)
                ? SvgLength.Parse(sw, defaultIfMissing: parent.StrokeWidth) : parent.StrokeWidth,
            StrokeLineCap = values.TryGetValue("stroke-linecap", out string? slc) ? ParseLineCap(slc, parent.StrokeLineCap) : parent.StrokeLineCap,
            StrokeLineJoin = values.TryGetValue("stroke-linejoin", out string? slj) ? ParseLineJoin(slj, parent.StrokeLineJoin) : parent.StrokeLineJoin,
            StrokeMiterLimit = values.TryGetValue("stroke-miterlimit", out string? sml) && float.TryParse(sml, NumberStyles.Float, CultureInfo.InvariantCulture, out float mlV) ? mlV : parent.StrokeMiterLimit,
            StrokeDashArray = values.TryGetValue("stroke-dasharray", out string? sda) ? ParseDashArray(sda) ?? parent.StrokeDashArray : parent.StrokeDashArray,
            StrokeDashOffset = values.TryGetValue("stroke-dashoffset", out string? sdo) ? SvgLength.Parse(sdo, defaultIfMissing: parent.StrokeDashOffset) : parent.StrokeDashOffset,
            Opacity = values.TryGetValue("opacity", out string? op) && float.TryParse(op, NumberStyles.Float, CultureInfo.InvariantCulture, out float opV) ? Math.Clamp(opV, 0f, 1f) * parent.Opacity : parent.Opacity,
            FillOpacity = values.TryGetValue("fill-opacity", out string? fop) && float.TryParse(fop, NumberStyles.Float, CultureInfo.InvariantCulture, out float fopV) ? Math.Clamp(fopV, 0f, 1f) : parent.FillOpacity,
            StrokeOpacity = values.TryGetValue("stroke-opacity", out string? sop) && float.TryParse(sop, NumberStyles.Float, CultureInfo.InvariantCulture, out float sopV) ? Math.Clamp(sopV, 0f, 1f) : parent.StrokeOpacity,
            FillRule = values.TryGetValue("fill-rule", out string? fr) && fr.Equals("evenodd", StringComparison.OrdinalIgnoreCase) ? FillRule.EvenOdd : parent.FillRule,
            CurrentColor = color,
            Display = !values.TryGetValue("display", out string? dis) || !dis.Equals("none", StringComparison.OrdinalIgnoreCase),
            Visibility = !values.TryGetValue("visibility", out string? vis) || !vis.Equals("hidden", StringComparison.OrdinalIgnoreCase),
        };
    }

    private static Paint ResolvePaint(string text, RgbaColor currentColor, Paint fallback, Func<string, Paint?>? paintRef)
    {
        text = text.Trim();
        if (text.Length == 0) return fallback;
        if (text.StartsWith("url(", StringComparison.OrdinalIgnoreCase))
        {
            int end = text.IndexOf(')');
            if (end < 0) return fallback;
            string inner = text[4..end].Trim();
            if (inner.StartsWith('#')) inner = inner[1..];
            if (inner.StartsWith('"') || inner.StartsWith('\'')) inner = inner[1..^1];
            if (inner.StartsWith('#')) inner = inner[1..];
            var resolved = paintRef?.Invoke(inner);
            return resolved ?? fallback;
        }
        if (!SvgColorParser.TryParse(text, out var c)) return Paint.None;
        if (c == SvgColorParser.CurrentColorSentinel) c = currentColor;
        return new SolidPaint(c);
    }

    private static LineCap ParseLineCap(string s, LineCap fallback) => s.ToLowerInvariant() switch
    {
        "butt" => LineCap.Butt,
        "round" => LineCap.Round,
        "square" => LineCap.Square,
        _ => fallback,
    };

    private static LineJoin ParseLineJoin(string s, LineJoin fallback) => s.ToLowerInvariant() switch
    {
        "miter" => LineJoin.Miter,
        "round" => LineJoin.Round,
        "bevel" => LineJoin.Bevel,
        _ => fallback,
    };

    private static float[]? ParseDashArray(string s)
    {
        if (s.Equals("none", StringComparison.OrdinalIgnoreCase)) return null;
        var parts = s.Split([',', ' ', '\t', '\n', '\r'], StringSplitOptions.RemoveEmptyEntries);
        if (parts.Length == 0) return null;
        var result = new float[parts.Length];
        for (int i = 0; i < parts.Length; i++)
        {
            if (!float.TryParse(parts[i].Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out result[i]))
                return null;
        }
        return result;
    }
}
