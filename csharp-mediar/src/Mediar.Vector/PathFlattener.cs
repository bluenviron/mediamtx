using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// Flattens a <see cref="Path2D"/> into a stream of <see cref="LineSegment"/>
/// records suitable for the scanline rasterizer. Quadratic and cubic
/// Beziers are adaptively subdivided until the maximum control-point
/// deflection is within the requested device-space tolerance.
/// </summary>
public static class PathFlattener
{
    private const int MaxSubdivisionDepth = 24;

    /// <summary>
    /// Iterate flattened line segments from <paramref name="path"/> after
    /// applying <paramref name="transform"/>. Closing segments are emitted
    /// as explicit lines from the current point back to the sub-path
    /// start so the rasterizer doesn't need to track sub-path state.
    /// </summary>
    public static IEnumerable<LineSegment> Flatten(Path2D path, Matrix3x2 transform, float toleranceDevicePixels = 0.25f)
    {
        ArgumentNullException.ThrowIfNull(path);
        float tol = toleranceDevicePixels / MathF.Max(1e-6f, AffineMatrix.MaxScale(transform));
        // Squared form so the inner deflection test is allocation-free and branch-light.
        float tolSq = tol * tol;

        Vector2 cur = Vector2.Zero;
        Vector2 sub = Vector2.Zero;
        bool hasCur = false;

        foreach (var seg in path.Segments)
        {
            Vector2 p0 = AffineMatrix.TransformPoint(seg.P0, transform);
            Vector2 p1 = AffineMatrix.TransformPoint(seg.P1, transform);
            Vector2 p2 = AffineMatrix.TransformPoint(seg.P2, transform);

            switch (seg.Verb)
            {
                case PathVerb.MoveTo:
                    cur = p0; sub = p0; hasCur = true;
                    break;

                case PathVerb.LineTo:
                    if (!hasCur) { cur = sub = Vector2.Zero; hasCur = true; }
                    yield return new LineSegment(cur, p0);
                    cur = p0;
                    break;

                case PathVerb.QuadTo:
                    foreach (var l in FlattenQuad(cur, p0, p1, tolSq)) yield return l;
                    cur = p1;
                    break;

                case PathVerb.CubicTo:
                    foreach (var l in FlattenCubic(cur, p0, p1, p2, tolSq)) yield return l;
                    cur = p2;
                    break;

                case PathVerb.Close:
                    if (cur != sub) yield return new LineSegment(cur, sub);
                    cur = sub;
                    break;
            }
        }
    }

    private static IEnumerable<LineSegment> FlattenQuad(Vector2 p0, Vector2 c, Vector2 p1, float tolSq)
    {
        var stack = new Stack<(Vector2 a, Vector2 b, Vector2 c, int depth)>();
        stack.Push((p0, c, p1, 0));

        while (stack.Count > 0)
        {
            var (a, b, e, depth) = stack.Pop();

            // Deflection = perpendicular distance from control point to chord.
            Vector2 d = e - a;
            Vector2 v = b - a;
            float cross = d.X * v.Y - d.Y * v.X;
            float lenSq = d.LengthSquared();
            float devSq = lenSq > 1e-12f ? (cross * cross) / lenSq : v.LengthSquared();

            if (devSq <= tolSq || depth >= MaxSubdivisionDepth)
            {
                yield return new LineSegment(a, e);
            }
            else
            {
                Vector2 m01 = (a + b) * 0.5f;
                Vector2 m12 = (b + e) * 0.5f;
                Vector2 m = (m01 + m12) * 0.5f;
                stack.Push((m, m12, e, depth + 1));
                stack.Push((a, m01, m, depth + 1));
            }
        }
    }

    private static IEnumerable<LineSegment> FlattenCubic(Vector2 p0, Vector2 c1, Vector2 c2, Vector2 p1, float tolSq)
    {
        var stack = new Stack<(Vector2 a, Vector2 b, Vector2 c, Vector2 d, int depth)>();
        stack.Push((p0, c1, c2, p1, 0));

        while (stack.Count > 0)
        {
            var (a, b, c, e, depth) = stack.Pop();

            // de Casteljau "flatness" test: max perpendicular distance of
            // the two control points to the chord (Hain et al. 2005).
            Vector2 d = e - a;
            float lenSq = d.LengthSquared();
            float devSq;
            if (lenSq > 1e-12f)
            {
                float cross1 = (b.X - a.X) * d.Y - (b.Y - a.Y) * d.X;
                float cross2 = (c.X - a.X) * d.Y - (c.Y - a.Y) * d.X;
                float dev1Sq = (cross1 * cross1) / lenSq;
                float dev2Sq = (cross2 * cross2) / lenSq;
                devSq = MathF.Max(dev1Sq, dev2Sq);
            }
            else
            {
                devSq = MathF.Max((b - a).LengthSquared(), (c - a).LengthSquared());
            }

            if (devSq <= tolSq || depth >= MaxSubdivisionDepth)
            {
                yield return new LineSegment(a, e);
            }
            else
            {
                Vector2 m01 = (a + b) * 0.5f;
                Vector2 m12 = (b + c) * 0.5f;
                Vector2 m23 = (c + e) * 0.5f;
                Vector2 m012 = (m01 + m12) * 0.5f;
                Vector2 m123 = (m12 + m23) * 0.5f;
                Vector2 m = (m012 + m123) * 0.5f;
                stack.Push((m, m123, m23, e, depth + 1));
                stack.Push((a, m01, m012, m, depth + 1));
            }
        }
    }
}

/// <summary>One straight line segment in device coordinates.</summary>
public readonly record struct LineSegment(Vector2 P0, Vector2 P1);
