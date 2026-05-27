using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// SVG-correct elliptical-arc-to-cubic-Bezier conversion. The decomposition
/// follows the W3C SVG 1.1 implementation note "F.6 Elliptical arc
/// implementation notes" verbatim.
/// </summary>
internal static class Arc
{
    public static void AppendEllipticalArc(
        Path2D path, Vector2 start, float rx, float ry,
        float xAxisRotationDeg, bool largeArc, bool sweep, Vector2 end)
    {
        // Trivial: if endpoints are the same the arc is a no-op (per F.6.2).
        if (start == end) return;

        rx = MathF.Abs(rx); ry = MathF.Abs(ry);

        // Trivial: zero-radius arc collapses to a line (F.6.2 step 1).
        if (rx == 0f || ry == 0f) { path.LineTo(end); return; }

        float angle = xAxisRotationDeg * MathF.PI / 180f;
        float cosA = MathF.Cos(angle), sinA = MathF.Sin(angle);

        // Step 1 (F.6.5): transform to the un-rotated, centre-relative frame.
        float dx = (start.X - end.X) / 2f;
        float dy = (start.Y - end.Y) / 2f;
        float x1p = cosA * dx + sinA * dy;
        float y1p = -sinA * dx + cosA * dy;

        // Correct out-of-range radii (F.6.6.2).
        float rxSq = rx * rx, rySq = ry * ry;
        float x1pSq = x1p * x1p, y1pSq = y1p * y1p;
        float lambda = x1pSq / rxSq + y1pSq / rySq;
        if (lambda > 1f)
        {
            float s = MathF.Sqrt(lambda);
            rx *= s; ry *= s;
            rxSq = rx * rx; rySq = ry * ry;
        }

        // Step 2 (F.6.5.2): compute centre in transformed frame.
        float sign = largeArc == sweep ? -1f : 1f;
        float numer = rxSq * rySq - rxSq * y1pSq - rySq * x1pSq;
        float denom = rxSq * y1pSq + rySq * x1pSq;
        float coef = sign * MathF.Sqrt(MathF.Max(0f, numer / denom));
        float cxp = coef * rx * y1p / ry;
        float cyp = -coef * ry * x1p / rx;

        // Step 3 (F.6.5.3): un-rotate centre back.
        float cx = cosA * cxp - sinA * cyp + (start.X + end.X) / 2f;
        float cy = sinA * cxp + cosA * cyp + (start.Y + end.Y) / 2f;

        // Step 4 (F.6.5.5/6): start angle + sweep.
        float ux = (x1p - cxp) / rx, uy = (y1p - cyp) / ry;
        float vx = (-x1p - cxp) / rx, vy = (-y1p - cyp) / ry;

        float theta1 = AngleBetween(1, 0, ux, uy);
        float dTheta = AngleBetween(ux, uy, vx, vy);

        if (!sweep && dTheta > 0) dTheta -= MathF.Tau;
        else if (sweep && dTheta < 0) dTheta += MathF.Tau;

        // Step 5: split into ≤ 90° segments and emit cubic Beziers.
        int n = MathF.Max(1, MathF.Ceiling(MathF.Abs(dTheta) / (MathF.PI / 2f))) is var nf ? (int)nf : 1;
        float seg = dTheta / n;
        float t = (8f / 3f) * MathF.Sin(seg / 4f) * MathF.Sin(seg / 4f) / MathF.Sin(seg / 2f);

        float theta = theta1;
        float curCos = MathF.Cos(theta), curSin = MathF.Sin(theta);
        for (int i = 0; i < n; i++)
        {
            float nextTheta = theta + seg;
            float nxtCos = MathF.Cos(nextTheta), nxtSin = MathF.Sin(nextTheta);

            float c1xp = rx * (curCos - t * curSin);
            float c1yp = ry * (curSin + t * curCos);
            float c2xp = rx * (nxtCos + t * nxtSin);
            float c2yp = ry * (nxtSin - t * nxtCos);
            float ep_x = rx * nxtCos;
            float ep_y = ry * nxtSin;

            Vector2 c1 = new(cosA * c1xp - sinA * c1yp + cx, sinA * c1xp + cosA * c1yp + cy);
            Vector2 c2 = new(cosA * c2xp - sinA * c2yp + cx, sinA * c2xp + cosA * c2yp + cy);
            Vector2 ep = i == n - 1
                ? end
                : new Vector2(cosA * ep_x - sinA * ep_y + cx, sinA * ep_x + cosA * ep_y + cy);

            path.CubicTo(c1, c2, ep);

            theta = nextTheta;
            curCos = nxtCos;
            curSin = nxtSin;
        }
    }

    private static float AngleBetween(float ux, float uy, float vx, float vy)
    {
        float dot = ux * vx + uy * vy;
        float len = MathF.Sqrt((ux * ux + uy * uy) * (vx * vx + vy * vy));
        float c = Math.Clamp(dot / len, -1f, 1f);
        float sign = (ux * vy - uy * vx) < 0 ? -1f : 1f;
        return sign * MathF.Acos(c);
    }
}
