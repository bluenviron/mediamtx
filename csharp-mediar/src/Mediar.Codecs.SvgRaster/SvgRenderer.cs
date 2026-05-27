using System.Globalization;
using System.Numerics;
using System.Xml.Linq;
using Mediar.Imaging;
using Mediar.Vector;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// Renders an SVG document into a Bgra32 <see cref="ImageFrame"/>.
/// The renderer parses the source XML, resolves all <c>&lt;defs&gt;</c>
/// gradients and <c>&lt;use&gt;</c> references, walks the element tree
/// while cascading the current transform and style, builds
/// <see cref="Path2D"/> geometry for every primitive shape, and hands it
/// off to <see cref="ScanlineRasterizer"/> for high-quality anti-aliased
/// composition.
/// </summary>
public static class SvgRenderer
{
    /// <summary>
    /// Render <paramref name="svgXml"/> at the canvas size given on the
    /// root <c>&lt;svg&gt;</c> element. The output frame is exactly
    /// <c>(viewportWidth, viewportHeight)</c> in size.
    /// </summary>
    public static ImageFrame Render(string svgXml, RgbaColor background = default)
    {
        ArgumentException.ThrowIfNullOrEmpty(svgXml);
        var doc = XDocument.Parse(svgXml, LoadOptions.PreserveWhitespace);
        if (doc.Root is null) throw new ImageFormatException("SVG document has no root element.");
        return Render(doc.Root, null, null, background);
    }

    /// <summary>
    /// Render at an explicit output resolution, scaling content via the
    /// <c>viewBox</c> + <c>preserveAspectRatio</c> rules.
    /// </summary>
    public static ImageFrame Render(string svgXml, int outputWidth, int outputHeight, RgbaColor background = default)
    {
        ArgumentException.ThrowIfNullOrEmpty(svgXml);
        var doc = XDocument.Parse(svgXml, LoadOptions.PreserveWhitespace);
        if (doc.Root is null) throw new ImageFormatException("SVG document has no root element.");
        return Render(doc.Root, outputWidth, outputHeight, background);
    }

    private static ImageFrame Render(XElement root, int? outW, int? outH, RgbaColor background)
    {
        // Resolve canvas size.
        var (intrinsicW, intrinsicH, viewBox) = ResolveCanvas(root);
        int width = outW ?? Math.Max(1, (int)MathF.Round(intrinsicW));
        int height = outH ?? Math.Max(1, (int)MathF.Round(intrinsicH));

        // Build viewBox-to-viewport transform.
        Matrix3x2 fit = BuildViewBoxTransform(viewBox, width, height,
            (string?)root.Attribute("preserveAspectRatio") ?? "xMidYMid meet");

        var target = RasterTarget.Create(width, height, background);
        var resolver = new SvgGradientResolver(root);
        var initial = new SvgStyle();

        RenderElement(root, target, fit, initial, resolver, viewportW: width, viewportH: height, isInsideDefs: false);

        // Copy to ImageFrame.
        int stride = width * 4;
        byte[] buffer = new byte[height * stride];
        target.Pixels.CopyTo(buffer);
        return new ImageFrame(width, height, PixelFormat.Bgra32, stride, buffer);
    }

    private static (float W, float H, (float X, float Y, float W, float H)? ViewBox) ResolveCanvas(XElement root)
    {
        string? wAttr = (string?)root.Attribute("width");
        string? hAttr = (string?)root.Attribute("height");
        string? vbAttr = (string?)root.Attribute("viewBox");

        (float, float, float, float)? vb = null;
        if (!string.IsNullOrEmpty(vbAttr))
        {
            var parts = vbAttr.Split([',', ' ', '\t', '\n', '\r'], StringSplitOptions.RemoveEmptyEntries);
            if (parts.Length == 4
                && float.TryParse(parts[0], NumberStyles.Float, CultureInfo.InvariantCulture, out float vx)
                && float.TryParse(parts[1], NumberStyles.Float, CultureInfo.InvariantCulture, out float vy)
                && float.TryParse(parts[2], NumberStyles.Float, CultureInfo.InvariantCulture, out float vw)
                && float.TryParse(parts[3], NumberStyles.Float, CultureInfo.InvariantCulture, out float vh))
            {
                vb = (vx, vy, vw, vh);
            }
        }

        float w = SvgLength.Parse(wAttr, defaultIfMissing: vb?.Item3 ?? 300f);
        float h = SvgLength.Parse(hAttr, defaultIfMissing: vb?.Item4 ?? 150f);
        return (w, h, vb);
    }

    internal static Matrix3x2 BuildViewBoxTransform((float X, float Y, float W, float H)? viewBox, float vpW, float vpH, string preserveAspectRatio)
    {
        if (viewBox is not { } vb) return Matrix3x2.Identity;
        if (vb.W <= 0 || vb.H <= 0) return Matrix3x2.Identity;

        float sx = vpW / vb.W;
        float sy = vpH / vb.H;

        var par = preserveAspectRatio.Trim();
        bool meet;
        string align;
        int sp = par.IndexOf(' ');
        if (sp < 0) { align = par; meet = true; }
        else
        {
            align = par[..sp].Trim();
            meet = !par[(sp + 1)..].Trim().Equals("slice", StringComparison.OrdinalIgnoreCase);
        }

        if (!align.Equals("none", StringComparison.OrdinalIgnoreCase))
        {
            float s = meet ? MathF.Min(sx, sy) : MathF.Max(sx, sy);
            sx = sy = s;
        }

        float tx, ty;
        float scaledW = vb.W * sx, scaledH = vb.H * sy;
        tx = align switch
        {
            var a when a.StartsWith("xMin", StringComparison.OrdinalIgnoreCase) => 0,
            var a when a.StartsWith("xMax", StringComparison.OrdinalIgnoreCase) => vpW - scaledW,
            _ => (vpW - scaledW) / 2f,
        };
        ty = align.ToUpperInvariant() switch
        {
            var a when a.EndsWith("YMIN", StringComparison.Ordinal) => 0,
            var a when a.EndsWith("YMAX", StringComparison.Ordinal) => vpH - scaledH,
            _ => (vpH - scaledH) / 2f,
        };

        var m = Matrix3x2.CreateTranslation(-vb.X, -vb.Y);
        m = Matrix3x2.CreateScale(sx, sy) * m;
        m = Matrix3x2.CreateTranslation(tx, ty) * m;
        return m;
    }

    private static void RenderElement(
        XElement el, RasterTarget target, Matrix3x2 transform,
        SvgStyle parentStyle, SvgGradientResolver resolver,
        int viewportW, int viewportH, bool isInsideDefs)
    {
        string ln = el.Name.LocalName;

        if (ln.Equals("defs", StringComparison.OrdinalIgnoreCase))
            return; // gradients and patterns are resolved on demand.
        if (ln.Equals("title", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("desc", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("metadata", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("clipPath", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("linearGradient", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("radialGradient", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("symbol", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("mask", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("filter", StringComparison.OrdinalIgnoreCase) ||
            ln.Equals("style", StringComparison.OrdinalIgnoreCase))
            return;

        var style = SvgStyleResolver.Resolve(el, parentStyle, resolver.Resolve);
        if (!style.Display || !style.Visibility) return;

        var localT = SvgTransformParser.Parse((string?)el.Attribute("transform"));
        var current = localT * transform;

        Path2D? geometry = null;
        switch (ln.ToLowerInvariant())
        {
            case "svg":
            case "g":
                foreach (var child in el.Elements())
                    RenderElement(child, target, current, style, resolver, viewportW, viewportH, isInsideDefs);
                return;
            case "use":
                RenderUse(el, target, current, style, resolver, viewportW, viewportH);
                return;
            case "rect":
                geometry = BuildRect(el, viewportW, viewportH);
                break;
            case "circle":
                geometry = BuildCircle(el, viewportW, viewportH);
                break;
            case "ellipse":
                geometry = BuildEllipse(el, viewportW, viewportH);
                break;
            case "line":
                geometry = BuildLine(el);
                break;
            case "polyline":
                geometry = BuildPoly(el, closed: false);
                break;
            case "polygon":
                geometry = BuildPoly(el, closed: true);
                break;
            case "path":
                geometry = SvgPathDataParser.Parse((string?)el.Attribute("d"));
                break;
            default:
                // Unknown: walk into children (some SVGs nest unknown wrappers).
                foreach (var child in el.Elements())
                    RenderElement(child, target, current, style, resolver, viewportW, viewportH, isInsideDefs);
                return;
        }

        if (geometry is null || geometry.IsEmpty) return;
        DrawGeometry(geometry, target, current, style);

        // Children of shape elements (rare, but valid for path/text/etc.).
        foreach (var child in el.Elements())
            RenderElement(child, target, current, style, resolver, viewportW, viewportH, isInsideDefs);
    }

    private static void RenderUse(
        XElement use, RasterTarget target, Matrix3x2 transform,
        SvgStyle style, SvgGradientResolver resolver, int vpW, int vpH)
    {
        string? href = (string?)use.Attribute("href") ?? (string?)use.Attribute(XName.Get("href", "http://www.w3.org/1999/xlink"));
        if (string.IsNullOrEmpty(href) || !href.StartsWith('#')) return;
        string id = href[1..];
        var doc = use.Document!;
        var target_ = doc.Descendants().FirstOrDefault(e => (string?)e.Attribute("id") == id);
        if (target_ is null) return;

        float x = SvgLength.Parse((string?)use.Attribute("x"), vpW);
        float y = SvgLength.Parse((string?)use.Attribute("y"), vpH);
        var t = Matrix3x2.CreateTranslation(x, y) * transform;
        RenderElement(target_, target, t, style, resolver, vpW, vpH, isInsideDefs: false);
    }

    private static Path2D BuildRect(XElement el, int vpW, int vpH)
    {
        float x = SvgLength.Parse((string?)el.Attribute("x"), vpW);
        float y = SvgLength.Parse((string?)el.Attribute("y"), vpH);
        float w = SvgLength.Parse((string?)el.Attribute("width"), vpW);
        float h = SvgLength.Parse((string?)el.Attribute("height"), vpH);
        float rx = SvgLength.Parse((string?)el.Attribute("rx"), vpW);
        float ry = SvgLength.Parse((string?)el.Attribute("ry"), vpH);
        var p = new Path2D();
        if (w <= 0 || h <= 0) return p;
        rx = Math.Min(rx, w / 2f);
        ry = Math.Min(ry, h / 2f);
        if (rx > 0 && ry == 0) ry = rx;
        if (ry > 0 && rx == 0) rx = ry;

        if (rx == 0 && ry == 0)
        {
            p.MoveTo(x, y);
            p.LineTo(x + w, y);
            p.LineTo(x + w, y + h);
            p.LineTo(x, y + h);
            p.Close();
        }
        else
        {
            p.MoveTo(x + rx, y);
            p.LineTo(x + w - rx, y);
            p.ArcTo(rx, ry, 0, false, true, new Vector2(x + w, y + ry));
            p.LineTo(x + w, y + h - ry);
            p.ArcTo(rx, ry, 0, false, true, new Vector2(x + w - rx, y + h));
            p.LineTo(x + rx, y + h);
            p.ArcTo(rx, ry, 0, false, true, new Vector2(x, y + h - ry));
            p.LineTo(x, y + ry);
            p.ArcTo(rx, ry, 0, false, true, new Vector2(x + rx, y));
            p.Close();
        }
        return p;
    }

    private static Path2D BuildCircle(XElement el, int vpW, int vpH)
    {
        float cx = SvgLength.Parse((string?)el.Attribute("cx"), vpW);
        float cy = SvgLength.Parse((string?)el.Attribute("cy"), vpH);
        float r = SvgLength.Parse((string?)el.Attribute("r"), vpW);
        var p = new Path2D();
        if (r <= 0) return p;
        p.MoveTo(cx + r, cy);
        p.ArcTo(r, r, 0, false, true, new Vector2(cx - r, cy));
        p.ArcTo(r, r, 0, false, true, new Vector2(cx + r, cy));
        p.Close();
        return p;
    }

    private static Path2D BuildEllipse(XElement el, int vpW, int vpH)
    {
        float cx = SvgLength.Parse((string?)el.Attribute("cx"), vpW);
        float cy = SvgLength.Parse((string?)el.Attribute("cy"), vpH);
        float rx = SvgLength.Parse((string?)el.Attribute("rx"), vpW);
        float ry = SvgLength.Parse((string?)el.Attribute("ry"), vpH);
        var p = new Path2D();
        if (rx <= 0 || ry <= 0) return p;
        p.MoveTo(cx + rx, cy);
        p.ArcTo(rx, ry, 0, false, true, new Vector2(cx - rx, cy));
        p.ArcTo(rx, ry, 0, false, true, new Vector2(cx + rx, cy));
        p.Close();
        return p;
    }

    private static Path2D BuildLine(XElement el)
    {
        float x1 = SvgLength.Parse((string?)el.Attribute("x1"));
        float y1 = SvgLength.Parse((string?)el.Attribute("y1"));
        float x2 = SvgLength.Parse((string?)el.Attribute("x2"));
        float y2 = SvgLength.Parse((string?)el.Attribute("y2"));
        var p = new Path2D();
        p.MoveTo(x1, y1);
        p.LineTo(x2, y2);
        return p;
    }

    private static Path2D BuildPoly(XElement el, bool closed)
    {
        string? raw = (string?)el.Attribute("points");
        var p = new Path2D();
        if (string.IsNullOrWhiteSpace(raw)) return p;
        var nums = new List<float>();
        var tokens = raw.Split([',', ' ', '\t', '\n', '\r'], StringSplitOptions.RemoveEmptyEntries);
        foreach (var token in tokens)
        {
            if (float.TryParse(token, NumberStyles.Float, CultureInfo.InvariantCulture, out float v))
                nums.Add(v);
        }
        for (int i = 0; i + 1 < nums.Count; i += 2)
        {
            if (i == 0) p.MoveTo(nums[0], nums[1]);
            else p.LineTo(nums[i], nums[i + 1]);
        }
        if (closed) p.Close();
        return p;
    }

    private static void DrawGeometry(Path2D path, RasterTarget target, Matrix3x2 transform, SvgStyle style)
    {
        // Compute object bounds (in user space, post element-transform) for
        // ObjectBoundingBox gradients.
        var (minX, minY, maxX, maxY) = path.GetBounds();
        var p0 = Vector2.Transform(new Vector2(minX, minY), transform);
        var p1 = Vector2.Transform(new Vector2(maxX, minY), transform);
        var p2 = Vector2.Transform(new Vector2(minX, maxY), transform);
        var p3 = Vector2.Transform(new Vector2(maxX, maxY), transform);
        float bxMin = MathF.Min(MathF.Min(p0.X, p1.X), MathF.Min(p2.X, p3.X));
        float byMin = MathF.Min(MathF.Min(p0.Y, p1.Y), MathF.Min(p2.Y, p3.Y));
        float bxMax = MathF.Max(MathF.Max(p0.X, p1.X), MathF.Max(p2.X, p3.X));
        float byMax = MathF.Max(MathF.Max(p0.Y, p1.Y), MathF.Max(p2.Y, p3.Y));
        var bounds = (bxMin, byMin, bxMax, byMax);

        if (style.Fill is not { } fill || fill is not (SolidPaint or LinearGradientPaint or RadialGradientPaint))
        {
            // skip fill
        }
        else
        {
            float fillOpacity = style.Opacity * style.FillOpacity;
            var eval = PaintEvaluatorFactory.Create(fill, bounds, transform, fillOpacity);
            ScanlineRasterizer.Fill(target, path, transform, eval, style.FillRule);
        }

        if (style.Stroke is SolidPaint or LinearGradientPaint or RadialGradientPaint && style.StrokeWidth > 0)
        {
            var strokeStyle = new StrokeStyle(
                style.StrokeWidth, style.StrokeLineCap, style.StrokeLineJoin,
                style.StrokeMiterLimit, style.StrokeDashArray, style.StrokeDashOffset);
            var stroked = StrokeToFill.Stroke(path, strokeStyle);
            if (!stroked.IsEmpty)
            {
                float strokeOpacity = style.Opacity * style.StrokeOpacity;
                var seval = PaintEvaluatorFactory.Create(style.Stroke, bounds, transform, strokeOpacity);
                // EvenOdd makes the inner + outer rings of a closed stroke
                // form a proper "donut" - the interior of the original path
                // is left empty, only the offset band is painted.
                ScanlineRasterizer.Fill(target, stroked, transform, seval, FillRule.EvenOdd);
            }
        }
    }
}
