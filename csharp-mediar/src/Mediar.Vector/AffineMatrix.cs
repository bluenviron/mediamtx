using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// Static helpers for building common 2D affine transforms in the same
/// row-major <see cref="Matrix3x2"/> layout used by System.Numerics.
/// SVG transforms left-multiply (the first listed transform is applied
/// last to coordinates), so the helpers preserve that convention by
/// returning <c>Matrix3x2.Multiply(child, parent)</c> from
/// <see cref="Compose"/>.
/// </summary>
public static class AffineMatrix
{
    /// <summary>Compose two transforms in SVG cascade order (parent then child).</summary>
    public static Matrix3x2 Compose(Matrix3x2 parent, Matrix3x2 child) =>
        child * parent;

    /// <summary>Apply a transform to a point.</summary>
    public static Vector2 TransformPoint(Vector2 p, in Matrix3x2 m) =>
        Vector2.Transform(p, m);

    /// <summary>
    /// Apply only the rotation/scale part of a transform to a free vector
    /// (i.e. ignore the translation column).
    /// </summary>
    public static Vector2 TransformVector(Vector2 v, in Matrix3x2 m) =>
        new(v.X * m.M11 + v.Y * m.M21, v.X * m.M12 + v.Y * m.M22);

    /// <summary>
    /// Maximum linear scale factor implied by <paramref name="m"/>. Used by
    /// <see cref="PathFlattener"/> to tighten its flatness tolerance when
    /// the path is drawn through a zooming transform.
    /// </summary>
    public static float MaxScale(in Matrix3x2 m)
    {
        // Operator 2-norm = sqrt(largest eigenvalue of M^T M). Closed form
        // for a 2x2 sub-block lives in any "matrix decomposition" reference.
        float a = m.M11, b = m.M21, c = m.M12, d = m.M22;
        float e = (a * a + b * b + c * c + d * d) / 2f;
        float f = MathF.Sqrt(MathF.Max(0f, (a * a + b * b - c * c - d * d) * (a * a + b * b - c * c - d * d) / 4f
                                          + (a * c + b * d) * (a * c + b * d)));
        return MathF.Sqrt(e + f);
    }

    /// <summary>
    /// Try to invert a transform. Returns the identity when the matrix is
    /// singular (this is what gradient fallbacks expect).
    /// </summary>
    public static Matrix3x2 InvertOrIdentity(in Matrix3x2 m) =>
        Matrix3x2.Invert(m, out var inv) ? inv : Matrix3x2.Identity;
}
