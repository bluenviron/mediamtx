using System.Buffers;
using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// Coverage-based anti-aliased scanline rasterizer. Edges are bucketed
/// per pixel row, then 16 sub-pixel sub-rows are walked: for each
/// sub-row, edge crossings are sorted by X and the active-edge winding
/// rule resolves which X-intervals are inside the path. Each pixel's
/// fractional coverage within the sub-row is added to a per-row
/// coverage accumulator; after 16 sub-rows the accumulator holds a
/// value in [0, 256] which is fed to the compositor as an alpha
/// modulator. The result is high-quality anti-aliasing equivalent to
/// 16-tap exact-area integration in Y and horizontal-coverage in X.
/// </summary>
public static class ScanlineRasterizer
{
    private const int SubLinesPerRow = 16;

    /// <summary>
    /// Rasterize <paramref name="path"/> filled with <paramref name="paint"/>
    /// through <paramref name="transform"/>, blending into <paramref name="target"/>
    /// using <paramref name="fillRule"/>. The optional <paramref name="clipRect"/>
    /// limits writes to a rectangular region of the target.
    /// </summary>
    public static void Fill(
        RasterTarget target,
        Path2D path,
        Matrix3x2 transform,
        IPaintEvaluator paint,
        FillRule fillRule = FillRule.NonZero,
        (int X, int Y, int W, int H)? clipRect = null)
    {
        ArgumentNullException.ThrowIfNull(target);
        ArgumentNullException.ThrowIfNull(path);
        ArgumentNullException.ThrowIfNull(paint);
        if (path.IsEmpty) return;

        int clipX0 = 0, clipY0 = 0, clipX1 = target.Width, clipY1 = target.Height;
        if (clipRect is { } cr)
        {
            clipX0 = Math.Max(0, cr.X);
            clipY0 = Math.Max(0, cr.Y);
            clipX1 = Math.Min(target.Width, cr.X + cr.W);
            clipY1 = Math.Min(target.Height, cr.Y + cr.H);
            if (clipX0 >= clipX1 || clipY0 >= clipY1) return;
        }

        // Collect flattened edges.
        var edges = new List<Edge>();
        float minY = float.MaxValue, maxY = float.MinValue;
        foreach (var seg in PathFlattener.Flatten(path, transform))
        {
            if (seg.P0 == seg.P1) continue;
            float y0 = seg.P0.Y;
            float y1 = seg.P1.Y;
            int winding;
            Vector2 a, b;
            if (y0 < y1) { a = seg.P0; b = seg.P1; winding = +1; }
            else { a = seg.P1; b = seg.P0; winding = -1; }
            if (a.Y == b.Y) continue;
            edges.Add(new Edge(a.X, a.Y, b.X, b.Y, winding));
            if (a.Y < minY) minY = a.Y;
            if (b.Y > maxY) maxY = b.Y;
        }
        if (edges.Count == 0) return;

        int firstRow = Math.Max(clipY0, (int)MathF.Floor(minY));
        int lastRow = Math.Min(clipY1 - 1, (int)MathF.Ceiling(maxY));
        if (firstRow > lastRow) return;

        int width = clipX1 - clipX0;
        byte[] rowCoverage = ArrayPool<byte>.Shared.Rent(width);
        int[] coverageAcc = ArrayPool<int>.Shared.Rent(width);
        try
        {
            for (int y = firstRow; y <= lastRow; y++)
            {
                Array.Clear(coverageAcc, 0, width);

                for (int s = 0; s < SubLinesPerRow; s++)
                {
                    float yLine = y + (s + 0.5f) / SubLinesPerRow;
                    SubLineSpans(edges, yLine, fillRule, clipX0, clipX1, coverageAcc);
                }

                for (int i = 0; i < width; i++)
                {
                    int cov = coverageAcc[i];
                    rowCoverage[i] = cov >= 255 ? (byte)255 : (byte)cov;
                }

                target.BlendSpan(y, clipX0, rowCoverage.AsSpan(0, width), paint);
            }
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(rowCoverage);
            ArrayPool<int>.Shared.Return(coverageAcc);
        }
    }

    private static void SubLineSpans(
        List<Edge> edges, float yLine, FillRule fillRule,
        int clipX0, int clipX1, int[] coverageAcc)
    {
        // Collect crossings (X, winding) for this sub-line.
        Span<float> xs = stackalloc float[64];
        Span<int> ws = stackalloc int[64];

        int n = 0;
        for (int i = 0; i < edges.Count; i++)
        {
            var e = edges[i];
            if (yLine < e.Y0 || yLine >= e.Y1) continue;
            float dx = e.X1 - e.X0;
            float dy = e.Y1 - e.Y0;
            float x = e.X0 + dx * (yLine - e.Y0) / dy;
            if (n >= xs.Length)
            {
                // Rare: fall back to a List path - 64 was the fast path.
                CollectAllCrossings(edges, yLine, fillRule, clipX0, clipX1, coverageAcc);
                return;
            }
            xs[n] = x;
            ws[n] = e.Winding;
            n++;
        }
        if (n == 0) return;

        SortByX(xs[..n], ws[..n]);
        AccumulateSpans(xs[..n], ws[..n], fillRule, clipX0, clipX1, coverageAcc);
    }

    private static void CollectAllCrossings(
        List<Edge> edges, float yLine, FillRule fillRule,
        int clipX0, int clipX1, int[] coverageAcc)
    {
        var xs = new List<float>();
        var ws = new List<int>();
        foreach (var e in edges)
        {
            if (yLine < e.Y0 || yLine >= e.Y1) continue;
            float dx = e.X1 - e.X0;
            float dy = e.Y1 - e.Y0;
            float x = e.X0 + dx * (yLine - e.Y0) / dy;
            xs.Add(x);
            ws.Add(e.Winding);
        }
        if (xs.Count == 0) return;
        Span<float> xsSpan = [.. xs];
        Span<int> wsSpan = [.. ws];
        SortByX(xsSpan, wsSpan);
        AccumulateSpans(xsSpan, wsSpan, fillRule, clipX0, clipX1, coverageAcc);
    }

    private static void SortByX(Span<float> xs, Span<int> ws)
    {
        for (int i = 1; i < xs.Length; i++)
        {
            float kx = xs[i]; int kw = ws[i]; int j = i - 1;
            while (j >= 0 && xs[j] > kx)
            {
                xs[j + 1] = xs[j];
                ws[j + 1] = ws[j];
                j--;
            }
            xs[j + 1] = kx;
            ws[j + 1] = kw;
        }
    }

    private static void AccumulateSpans(
        ReadOnlySpan<float> xs, ReadOnlySpan<int> ws, FillRule fillRule,
        int clipX0, int clipX1, int[] coverageAcc)
    {
        // Each sub-line contributes coverage in [0, 256/SubLinesPerRow] per pixel.
        // We store 0..256 in coverageAcc by adding (256 / SubLinesPerRow) per
        // fully-covered sub-pixel-row at each pixel.
        const float SubWeight = 256f / SubLinesPerRow;

        if (fillRule == FillRule.NonZero)
        {
            int wind = 0;
            float spanStart = 0;
            bool inside = false;
            for (int i = 0; i < xs.Length; i++)
            {
                int newWind = wind + ws[i];
                if (!inside && newWind != 0)
                {
                    spanStart = xs[i];
                    inside = true;
                }
                else if (inside && newWind == 0)
                {
                    AddSpan(spanStart, xs[i], SubWeight, clipX0, clipX1, coverageAcc);
                    inside = false;
                }
                wind = newWind;
            }
        }
        else
        {
            for (int i = 0; i + 1 < xs.Length; i += 2)
            {
                AddSpan(xs[i], xs[i + 1], SubWeight, clipX0, clipX1, coverageAcc);
            }
        }
    }

    private static void AddSpan(float x0, float x1, float weight, int clipX0, int clipX1, int[] coverageAcc)
    {
        if (x1 <= x0) return;
        if (x1 <= clipX0 || x0 >= clipX1) return;

        int width = clipX1 - clipX0;
        float cx0 = Math.Max(x0 - clipX0, 0);
        float cx1 = Math.Min(x1 - clipX0, width);
        int i0 = (int)MathF.Floor(cx0);
        int i1 = (int)MathF.Floor(cx1);
        if (i1 >= width) i1 = width - 1;

        if (i0 == i1)
        {
            coverageAcc[i0] += (int)((cx1 - cx0) * weight + 0.5f);
            return;
        }

        coverageAcc[i0] += (int)(((i0 + 1) - cx0) * weight + 0.5f);
        for (int i = i0 + 1; i < i1; i++) coverageAcc[i] += (int)(weight + 0.5f);
        if (i1 < width) coverageAcc[i1] += (int)((cx1 - i1) * weight + 0.5f);
    }

    private readonly record struct Edge(float X0, float Y0, float X1, float Y1, int Winding);
}
