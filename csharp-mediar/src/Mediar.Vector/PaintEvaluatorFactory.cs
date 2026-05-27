using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// Factory that turns a logical <see cref="Paint"/> + the geometry it
/// will be applied to (object bounding box) + the current device
/// transform into a flat <see cref="IPaintEvaluator"/> the rasterizer
/// can sample per pixel.
/// </summary>
public static class PaintEvaluatorFactory
{
    /// <summary>
    /// Bake a paint to an evaluator. <paramref name="objectBounds"/> is the
    /// post-transform AABB of the shape being filled and is used to
    /// resolve <see cref="GradientUnits.ObjectBoundingBox"/> coordinates.
    /// </summary>
    public static IPaintEvaluator Create(
        Paint paint,
        (float MinX, float MinY, float MaxX, float MaxY) objectBounds,
        Matrix3x2 deviceTransform,
        float cascadedOpacity = 1f)
    {
        ArgumentNullException.ThrowIfNull(paint);
        return paint switch
        {
            SolidPaint s => new SolidEvaluator(s.Color.WithOpacity(cascadedOpacity)),
            LinearGradientPaint lg => BuildLinear(lg, objectBounds, deviceTransform, cascadedOpacity),
            RadialGradientPaint rg => BuildRadial(rg, objectBounds, deviceTransform, cascadedOpacity),
            _ => new SolidEvaluator(RgbaColor.Transparent),
        };
    }

    private static LinearGradientEvaluator BuildLinear(
        LinearGradientPaint lg,
        (float MinX, float MinY, float MaxX, float MaxY) bounds,
        Matrix3x2 deviceTransform,
        float opacity)
    {
        Vector2 p1 = ResolvePoint(lg.X1, lg.Y1, lg.Units, bounds);
        Vector2 p2 = ResolvePoint(lg.X2, lg.Y2, lg.Units, bounds);

        // gradientTransform is applied in the gradient's own coord space,
        // then we transform to device space.
        if (lg.GradientTransform is { } gt)
        {
            p1 = Vector2.Transform(p1, gt);
            p2 = Vector2.Transform(p2, gt);
        }
        if (lg.Units == GradientUnits.UserSpaceOnUse)
        {
            p1 = Vector2.Transform(p1, deviceTransform);
            p2 = Vector2.Transform(p2, deviceTransform);
        }
        // (For ObjectBoundingBox we already resolved to device-space bounds.)

        var stops = ApplyOpacityToStops(lg.Stops, opacity);
        return new LinearGradientEvaluator(p1, p2, stops, lg.Spread);
    }

    private static RadialGradientEvaluator BuildRadial(
        RadialGradientPaint rg,
        (float MinX, float MinY, float MaxX, float MaxY) bounds,
        Matrix3x2 deviceTransform,
        float opacity)
    {
        Vector2 c = ResolvePoint(rg.Cx, rg.Cy, rg.Units, bounds);
        Vector2 f = ResolvePoint(rg.Fx, rg.Fy, rg.Units, bounds);
        float r = ResolveScalar(rg.R, rg.Units, bounds);

        if (rg.GradientTransform is { } gt)
        {
            c = Vector2.Transform(c, gt);
            f = Vector2.Transform(f, gt);
            r *= AffineMatrix.MaxScale(gt);
        }
        if (rg.Units == GradientUnits.UserSpaceOnUse)
        {
            c = Vector2.Transform(c, deviceTransform);
            f = Vector2.Transform(f, deviceTransform);
            r *= AffineMatrix.MaxScale(deviceTransform);
        }

        var stops = ApplyOpacityToStops(rg.Stops, opacity);
        return new RadialGradientEvaluator(c, r, f, stops, rg.Spread);
    }

    private static Vector2 ResolvePoint(float x, float y, GradientUnits units, (float MinX, float MinY, float MaxX, float MaxY) b)
    {
        if (units == GradientUnits.UserSpaceOnUse) return new Vector2(x, y);
        return new Vector2(b.MinX + x * (b.MaxX - b.MinX), b.MinY + y * (b.MaxY - b.MinY));
    }

    private static float ResolveScalar(float v, GradientUnits units, (float MinX, float MinY, float MaxX, float MaxY) b)
    {
        if (units == GradientUnits.UserSpaceOnUse) return v;
        float dx = b.MaxX - b.MinX, dy = b.MaxY - b.MinY;
        return v * MathF.Sqrt((dx * dx + dy * dy) / 2f);
    }

    private static IReadOnlyList<GradientStop> ApplyOpacityToStops(IReadOnlyList<GradientStop> stops, float opacity)
    {
        if (opacity >= 1f) return stops;
        var result = new GradientStop[stops.Count];
        for (int i = 0; i < stops.Count; i++)
            result[i] = stops[i] with { Color = stops[i].Color.WithOpacity(opacity) };
        return result;
    }
}
