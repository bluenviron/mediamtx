using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// Converts a stroked path into a filled outline path. Implements all
/// SVG stroke parameters: width, line cap (butt / round / square), line
/// join (miter / round / bevel) with miterlimit fallback, and
/// stroke-dasharray with stroke-dashoffset.
/// </summary>
public static class StrokeToFill
{
    private const float Epsilon = 1e-6f;

    /// <summary>Convert a stroked path into a filled outline path.</summary>
    public static Path2D Stroke(Path2D source, in StrokeStyle style)
    {
        ArgumentNullException.ThrowIfNull(source);
        if (style.Width <= 0 || source.IsEmpty) return new Path2D();

        // First flatten each sub-path into a polyline so we can offset segment-by-segment.
        var subPaths = ExtractSubPaths(source);
        var output = new Path2D();

        foreach (var sp in subPaths)
        {
            var polylines = style.DashArray is { Count: > 0 }
                ? ApplyDash(sp, style.DashArray, style.DashOffset)
                : new List<Sub> { sp };

            foreach (var poly in polylines)
            {
                if (poly.Points.Count < 2) continue;
                EmitOffsetOutline(output, poly, style);
            }
        }

        return output;
    }

    private static void EmitOffsetOutline(Path2D output, Sub poly, StrokeStyle style)
    {
        float half = style.Width / 2f;
        var pts = poly.Points;
        bool closed = poly.Closed;

        // Build the left and right offset strips.
        var leftSide = new List<Vector2>(pts.Count * 2);
        var rightSide = new List<Vector2>(pts.Count * 2);

        for (int i = 0; i < pts.Count - 1; i++)
        {
            Vector2 a = pts[i];
            Vector2 b = pts[i + 1];
            Vector2 dir = b - a;
            float len = dir.Length();
            if (len < Epsilon) continue;
            dir /= len;
            Vector2 nrm = new(-dir.Y, dir.X);

            Vector2 aL = a + nrm * half;
            Vector2 aR = a - nrm * half;
            Vector2 bL = b + nrm * half;
            Vector2 bR = b - nrm * half;

            if (leftSide.Count == 0)
            {
                leftSide.Add(aL);
                rightSide.Add(aR);
            }
            else
            {
                // Join with the previous segment.
                int prev = i - 1;
                while (prev >= 0 && (pts[prev + 1] - pts[prev]).LengthSquared() < Epsilon) prev--;
                if (prev >= 0)
                {
                    Vector2 prevDir = (pts[prev + 1] - pts[prev]);
                    prevDir = Vector2.Normalize(prevDir);
                    Vector2 prevNrm = new(-prevDir.Y, prevDir.X);

                    Vector2 inL = pts[i] + prevNrm * half;
                    Vector2 inR = pts[i] - prevNrm * half;

                    EmitJoin(leftSide, rightSide, pts[i], inL, inR, aL, aR, prevDir, dir, half, style);
                }
            }

            leftSide.Add(bL);
            rightSide.Add(bR);
        }

        if (leftSide.Count < 2) return;

        if (closed)
        {
            // For a closed loop, also stitch the end-to-start join.
            Vector2 a = pts[^2];
            Vector2 b = pts[^1];
            Vector2 c = pts[1];
            Vector2 d1 = Vector2.Normalize(b - a);
            Vector2 d2 = Vector2.Normalize(c - b);
            Vector2 n1 = new(-d1.Y, d1.X);
            Vector2 n2 = new(-d2.Y, d2.X);

            Vector2 inL = b + n1 * half;
            Vector2 inR = b - n1 * half;
            Vector2 outL = b + n2 * half;
            Vector2 outR = b - n2 * half;

            var tmpL = new List<Vector2>();
            var tmpR = new List<Vector2>();
            EmitJoin(tmpL, tmpR, b, inL, inR, outL, outR, d1, d2, half, style);

            // Build a single closed path by concatenating leftSide (forward) + reversed rightSide.
            output.MoveTo(leftSide[0]);
            for (int i = 1; i < leftSide.Count; i++) output.LineTo(leftSide[i]);
            foreach (var p in tmpL) output.LineTo(p);
            output.Close();

            output.MoveTo(rightSide[0]);
            for (int i = 1; i < rightSide.Count; i++) output.LineTo(rightSide[i]);
            foreach (var p in tmpR) output.LineTo(p);
            output.Close();
        }
        else
        {
            // Open polyline: emit one filled ring = leftSide + endCap + reverse(rightSide) + startCap.
            Vector2 start = pts[0];
            Vector2 startNext = pts[1];
            Vector2 startDir = Vector2.Normalize(startNext - start);
            Vector2 end = pts[^1];
            Vector2 endPrev = pts[^2];
            Vector2 endDir = Vector2.Normalize(end - endPrev);

            output.MoveTo(leftSide[0]);
            for (int i = 1; i < leftSide.Count; i++) output.LineTo(leftSide[i]);

            EmitCap(output, end, endDir, half, style.Cap, fromLeftToRight: true);

            for (int i = rightSide.Count - 1; i >= 0; i--) output.LineTo(rightSide[i]);

            EmitCap(output, start, -startDir, half, style.Cap, fromLeftToRight: true);

            output.Close();
        }
    }

    private static void EmitJoin(
        List<Vector2> leftSide, List<Vector2> rightSide,
        Vector2 corner,
        Vector2 inL, Vector2 inR,
        Vector2 outL, Vector2 outR,
        Vector2 d1, Vector2 d2,
        float half, in StrokeStyle style)
    {
        // Cross product tells us which side is the inner corner.
        float cross = d1.X * d2.Y - d1.Y * d2.X;
        if (MathF.Abs(cross) < Epsilon)
        {
            // Collinear: nothing to do, the offset edges already meet at the boundary.
            return;
        }

        bool leftIsOuter = cross > 0;

        switch (style.Join)
        {
            case LineJoin.Bevel:
                // Bevel = straight cut between offset ends. Already in place;
                // we just need to emit the missing edge on the outer side.
                if (leftIsOuter) leftSide.Add(outL);
                else rightSide.Add(outR);
                break;

            case LineJoin.Round:
                EmitRoundJoin(leftIsOuter ? leftSide : rightSide,
                              corner, leftIsOuter ? inL : inR,
                              leftIsOuter ? outL : outR, half, leftIsOuter);
                break;

            case LineJoin.Miter:
            default:
                Vector2 miter = ComputeMiter(corner, d1, d2, half, leftIsOuter, out float miterLen);
                float miterLimit = style.MiterLimit * half;
                if (miterLen <= miterLimit && !float.IsNaN(miter.X) && !float.IsNaN(miter.Y))
                {
                    if (leftIsOuter) leftSide.Add(miter);
                    else rightSide.Add(miter);
                }
                else
                {
                    if (leftIsOuter) leftSide.Add(outL);
                    else rightSide.Add(outR);
                }
                break;
        }
    }

    private static Vector2 ComputeMiter(Vector2 corner, Vector2 d1, Vector2 d2, float half, bool leftIsOuter, out float miterLen)
    {
        Vector2 n1 = new(-d1.Y, d1.X);
        Vector2 n2 = new(-d2.Y, d2.X);
        Vector2 bisector = Vector2.Normalize(n1 + n2);
        if (!leftIsOuter) bisector = -bisector;
        // miter length = half / sin(theta/2), where theta is the corner angle.
        float c = MathF.Abs(Vector2.Dot(n1, bisector));
        if (c < Epsilon) { miterLen = float.MaxValue; return new Vector2(float.NaN, float.NaN); }
        miterLen = half / c;
        return corner + bisector * miterLen;
    }

    private static void EmitRoundJoin(List<Vector2> sink, Vector2 c, Vector2 from, Vector2 to, float radius, bool leftIsOuter)
    {
        Vector2 a = from - c;
        Vector2 b = to - c;
        float ang1 = MathF.Atan2(a.Y, a.X);
        float ang2 = MathF.Atan2(b.Y, b.X);
        float sweep = ang2 - ang1;
        if (leftIsOuter)
        {
            while (sweep < 0) sweep += MathF.Tau;
        }
        else
        {
            while (sweep > 0) sweep -= MathF.Tau;
        }

        int steps = Math.Max(2, (int)MathF.Ceiling(MathF.Abs(sweep) * radius / 0.5f));
        for (int i = 1; i <= steps; i++)
        {
            float t = (float)i / steps;
            float ang = ang1 + sweep * t;
            sink.Add(c + new Vector2(MathF.Cos(ang), MathF.Sin(ang)) * radius);
        }
    }

    private static void EmitCap(Path2D output, Vector2 p, Vector2 dir, float half, LineCap cap, bool fromLeftToRight)
    {
        Vector2 nrm = new(-dir.Y, dir.X);
        switch (cap)
        {
            case LineCap.Butt:
                // No extension - both ends already lie on perpendicular offsets.
                break;
            case LineCap.Square:
                output.LineTo(p + nrm * half + dir * half);
                output.LineTo(p - nrm * half + dir * half);
                break;
            case LineCap.Round:
                int steps = 8;
                for (int i = 1; i < steps; i++)
                {
                    float t = MathF.PI * i / steps;
                    Vector2 v = nrm * MathF.Cos(t) + dir * MathF.Sin(t);
                    output.LineTo(p + v * half);
                }
                break;
        }
    }

    private static List<Sub> ExtractSubPaths(Path2D path)
    {
        var result = new List<Sub>();
        Sub current = new();
        bool closed = false;

        foreach (var s in path.Segments)
        {
            switch (s.Verb)
            {
                case PathVerb.MoveTo:
                    if (current.Points.Count > 1)
                    {
                        current.Closed = closed;
                        result.Add(current);
                    }
                    current = new Sub();
                    current.Points.Add(s.P0);
                    closed = false;
                    break;
                case PathVerb.LineTo:
                    current.Points.Add(s.P0);
                    break;
                case PathVerb.QuadTo:
                    AppendFlattenedQuad(current.Points, current.Points[^1], s.P0, s.P1);
                    break;
                case PathVerb.CubicTo:
                    AppendFlattenedCubic(current.Points, current.Points[^1], s.P0, s.P1, s.P2);
                    break;
                case PathVerb.Close:
                    if (current.Points.Count > 0 && current.Points[^1] != current.Points[0])
                        current.Points.Add(current.Points[0]);
                    closed = true;
                    break;
            }
        }
        if (current.Points.Count > 1)
        {
            current.Closed = closed;
            result.Add(current);
        }
        return result;
    }

    private static void AppendFlattenedQuad(List<Vector2> out_, Vector2 p0, Vector2 c, Vector2 p1)
    {
        foreach (var seg in PathFlattener.Flatten(MakePath(PathVerb.QuadTo, p0, c, p1), Matrix3x2.Identity))
            out_.Add(seg.P1);
    }

    private static void AppendFlattenedCubic(List<Vector2> out_, Vector2 p0, Vector2 c1, Vector2 c2, Vector2 p1)
    {
        foreach (var seg in PathFlattener.Flatten(MakePath(PathVerb.CubicTo, p0, c1, c2, p1), Matrix3x2.Identity))
            out_.Add(seg.P1);
    }

    private static Path2D MakePath(PathVerb v, Vector2 p0, Vector2 a, Vector2 b = default, Vector2 c = default)
    {
        var p = new Path2D();
        p.MoveTo(p0);
        if (v == PathVerb.QuadTo) p.QuadTo(a, b);
        else p.CubicTo(a, b, c);
        return p;
    }

    private static List<Sub> ApplyDash(Sub source, IReadOnlyList<float> dashes, float dashOffset)
    {
        // Build the cumulative dash pattern.
        var dashList = new List<float>(dashes);
        // Odd-count arrays are duplicated per the CSS / SVG spec.
        if (dashList.Count % 2 == 1)
        {
            int orig = dashList.Count;
            for (int i = 0; i < orig; i++) dashList.Add(dashList[i]);
        }
        float patternLen = 0;
        foreach (var d in dashList) patternLen += d;
        if (patternLen <= 0) return new List<Sub> { source };

        // Normalise dashOffset into [0, patternLen).
        float offset = dashOffset % patternLen;
        if (offset < 0) offset += patternLen;

        // Walk pattern to find starting dash index + leftover.
        int dashIdx = 0;
        float remaining = dashList[0] - offset;
        while (remaining <= 0)
        {
            dashIdx++;
            if (dashIdx >= dashList.Count) dashIdx = 0;
            remaining += dashList[dashIdx];
        }
        bool penDown = (dashIdx % 2) == 0;

        var result = new List<Sub>();
        Sub current = new();

        var pts = source.Points;
        for (int i = 0; i < pts.Count - 1; i++)
        {
            Vector2 a = pts[i];
            Vector2 b = pts[i + 1];
            float segLen = (b - a).Length();
            if (segLen < Epsilon) continue;

            Vector2 dir = (b - a) / segLen;
            float consumed = 0;
            Vector2 cursor = a;

            while (consumed < segLen)
            {
                float take = MathF.Min(remaining, segLen - consumed);
                Vector2 nextCursor = cursor + dir * take;

                if (penDown)
                {
                    if (current.Points.Count == 0) current.Points.Add(cursor);
                    current.Points.Add(nextCursor);
                }
                else if (current.Points.Count > 1)
                {
                    result.Add(current);
                    current = new Sub();
                }

                cursor = nextCursor;
                consumed += take;
                remaining -= take;

                if (remaining <= 0)
                {
                    dashIdx = (dashIdx + 1) % dashList.Count;
                    remaining = dashList[dashIdx];
                    penDown = !penDown;
                    if (!penDown && current.Points.Count > 1)
                    {
                        result.Add(current);
                        current = new Sub();
                    }
                }
            }
        }
        if (current.Points.Count > 1) result.Add(current);
        return result;
    }

    private sealed class Sub
    {
        public List<Vector2> Points { get; } = [];
        public bool Closed { get; set; }
    }
}
