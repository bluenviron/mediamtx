using System.Globalization;
using System.Xml.Linq;
using Mediar.Vector;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// Builds <see cref="LinearGradientPaint"/> / <see cref="RadialGradientPaint"/>
/// instances from <c>linearGradient</c> / <c>radialGradient</c> elements
/// resolved by id from the <c>&lt;defs&gt;</c> section. Follows the
/// SVG <c>href</c> / <c>xlink:href</c> chain so a child gradient can
/// inherit stops or geometry from a parent.
/// </summary>
public sealed class SvgGradientResolver
{
    private readonly Dictionary<string, XElement> _byId;

    /// <summary>Index every element in <paramref name="root"/> that has an id.</summary>
    public SvgGradientResolver(XElement root)
    {
        ArgumentNullException.ThrowIfNull(root);
        _byId = new Dictionary<string, XElement>(StringComparer.Ordinal);
        foreach (var el in root.DescendantsAndSelf())
        {
            string? id = (string?)el.Attribute("id");
            if (!string.IsNullOrEmpty(id)) _byId[id] = el;
        }
    }

    /// <summary>Resolve a paint reference by id.</summary>
    public Paint? Resolve(string id)
    {
        if (!_byId.TryGetValue(id, out var el)) return null;
        return el.Name.LocalName switch
        {
            "linearGradient" => BuildLinear(el),
            "radialGradient" => BuildRadial(el),
            _ => null,
        };
    }

    private LinearGradientPaint? BuildLinear(XElement el)
    {
        var (stops, attrs) = ResolveChain(el);
        if (stops.Count == 0) return null;

        var units = ParseUnits(attrs.GetValueOrDefault("gradientUnits"));
        var spread = ParseSpread(attrs.GetValueOrDefault("spreadMethod"));
        var gt = SvgTransformParser.Parse(attrs.GetValueOrDefault("gradientTransform"));

        float vw = units == GradientUnits.ObjectBoundingBox ? 1f : 1f;
        float x1 = ParseGradFloat(attrs, "x1", 0f, vw, units);
        float y1 = ParseGradFloat(attrs, "y1", 0f, vw, units);
        float x2 = ParseGradFloat(attrs, "x2", 1f, vw, units);
        float y2 = ParseGradFloat(attrs, "y2", 0f, vw, units);

        return new LinearGradientPaint(x1, y1, x2, y2, stops, units, spread,
            gt == System.Numerics.Matrix3x2.Identity ? null : gt);
    }

    private RadialGradientPaint? BuildRadial(XElement el)
    {
        var (stops, attrs) = ResolveChain(el);
        if (stops.Count == 0) return null;

        var units = ParseUnits(attrs.GetValueOrDefault("gradientUnits"));
        var spread = ParseSpread(attrs.GetValueOrDefault("spreadMethod"));
        var gt = SvgTransformParser.Parse(attrs.GetValueOrDefault("gradientTransform"));

        float cx = ParseGradFloat(attrs, "cx", 0.5f, 1f, units);
        float cy = ParseGradFloat(attrs, "cy", 0.5f, 1f, units);
        float r = ParseGradFloat(attrs, "r", 0.5f, 1f, units);
        float fx = ParseGradFloat(attrs, "fx", cx, 1f, units);
        float fy = ParseGradFloat(attrs, "fy", cy, 1f, units);

        return new RadialGradientPaint(cx, cy, r, fx, fy, stops, units, spread,
            gt == System.Numerics.Matrix3x2.Identity ? null : gt);
    }

    private (List<GradientStop> Stops, Dictionary<string, string> Attrs) ResolveChain(XElement start)
    {
        var attrs = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        var stops = new List<GradientStop>();
        var visited = new HashSet<XElement>();
        XElement? cur = start;
        while (cur is not null && visited.Add(cur))
        {
            foreach (var a in cur.Attributes())
            {
                string n = a.Name.LocalName;
                if (!attrs.ContainsKey(n)) attrs[n] = a.Value;
            }
            if (stops.Count == 0)
            {
                foreach (var sEl in cur.Elements())
                {
                    if (!sEl.Name.LocalName.Equals("stop", StringComparison.OrdinalIgnoreCase)) continue;
                    var st = ParseStop(sEl);
                    if (st is { } v) stops.Add(v);
                }
            }
            string? href = (string?)cur.Attribute("href") ?? (string?)cur.Attribute(XName.Get("href", "http://www.w3.org/1999/xlink"));
            if (string.IsNullOrEmpty(href) || !href.StartsWith('#')) break;
            string refId = href[1..];
            cur = _byId.TryGetValue(refId, out var next) ? next : null;
        }
        return (stops, attrs);
    }

    private static GradientStop? ParseStop(XElement el)
    {
        string? offsetStr = (string?)el.Attribute("offset");
        float offset = 0f;
        if (!string.IsNullOrEmpty(offsetStr))
        {
            if (offsetStr.EndsWith('%') && float.TryParse(offsetStr.AsSpan(0, offsetStr.Length - 1), NumberStyles.Float, CultureInfo.InvariantCulture, out float pct))
                offset = pct / 100f;
            else if (float.TryParse(offsetStr, NumberStyles.Float, CultureInfo.InvariantCulture, out float v))
                offset = v;
        }
        offset = Math.Clamp(offset, 0f, 1f);

        var values = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        string? inline = (string?)el.Attribute("style");
        if (!string.IsNullOrEmpty(inline))
        {
            foreach (var decl in inline.Split(';', StringSplitOptions.RemoveEmptyEntries))
            {
                int colon = decl.IndexOf(':');
                if (colon <= 0) continue;
                values[decl[..colon].Trim()] = decl[(colon + 1)..].Trim();
            }
        }
        foreach (var a in el.Attributes())
            if (!values.ContainsKey(a.Name.LocalName)) values[a.Name.LocalName] = a.Value;

        RgbaColor color = RgbaColor.Black;
        if (values.TryGetValue("stop-color", out string? colStr) && SvgColorParser.TryParse(colStr, out var c))
            color = c;
        if (values.TryGetValue("stop-opacity", out string? alphaStr) && float.TryParse(alphaStr, NumberStyles.Float, CultureInfo.InvariantCulture, out float alpha))
            color = color.WithOpacity(Math.Clamp(alpha, 0f, 1f));
        return new GradientStop(offset, color);
    }

    private static GradientUnits ParseUnits(string? s) =>
        s != null && s.Equals("userSpaceOnUse", StringComparison.OrdinalIgnoreCase)
            ? GradientUnits.UserSpaceOnUse
            : GradientUnits.ObjectBoundingBox;

    private static GradientSpread ParseSpread(string? s) => s?.ToLowerInvariant() switch
    {
        "reflect" => GradientSpread.Reflect,
        "repeat" => GradientSpread.Repeat,
        _ => GradientSpread.Pad,
    };

    private static float ParseGradFloat(Dictionary<string, string> attrs, string key, float defValue, float viewport, GradientUnits units)
    {
        if (!attrs.TryGetValue(key, out string? s) || string.IsNullOrEmpty(s)) return defValue;
        s = s.Trim();
        if (s.EndsWith('%'))
        {
            if (float.TryParse(s.AsSpan(0, s.Length - 1), NumberStyles.Float, CultureInfo.InvariantCulture, out float pct))
                return units == GradientUnits.ObjectBoundingBox ? pct / 100f : pct * viewport / 100f;
            return defValue;
        }
        if (float.TryParse(s, NumberStyles.Float, CultureInfo.InvariantCulture, out float v)) return v;
        return defValue;
    }
}
