using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class AffineMatrixTests
{
    [Fact]
    public void TransformPoint_Identity_Is_Noop()
    {
        var p = AffineMatrix.TransformPoint(new Vector2(3, 4), Matrix3x2.Identity);
        Assert.Equal(new Vector2(3, 4), p);
    }

    [Fact]
    public void TransformPoint_Applies_Translation_And_Scale()
    {
        var m = Matrix3x2.CreateScale(2f) * Matrix3x2.CreateTranslation(10f, 20f);
        var p = AffineMatrix.TransformPoint(new Vector2(1, 1), m);
        Assert.Equal(new Vector2(12, 22), p);
    }

    [Fact]
    public void TransformVector_Ignores_Translation()
    {
        var m = Matrix3x2.CreateScale(2f) * Matrix3x2.CreateTranslation(10f, 20f);
        var v = AffineMatrix.TransformVector(new Vector2(1, 1), m);
        Assert.Equal(new Vector2(2, 2), v);
    }

    [Fact]
    public void Compose_Applies_Child_First()
    {
        // Scale-then-translate vs translate-then-scale ordering matters.
        var parent = Matrix3x2.CreateTranslation(10f, 0f);
        var child = Matrix3x2.CreateScale(2f);
        var composed = AffineMatrix.Compose(parent, child);

        // Point (1,0): child scales -> (2,0); parent translates -> (12,0).
        var p = AffineMatrix.TransformPoint(new Vector2(1, 0), composed);
        Assert.Equal(12f, p.X, 4);
        Assert.Equal(0f, p.Y, 4);
    }

    [Fact]
    public void MaxScale_Identity_Is_One()
    {
        Assert.Equal(1f, AffineMatrix.MaxScale(Matrix3x2.Identity), 4);
    }

    [Fact]
    public void MaxScale_Uniform_Returns_Scale_Factor()
    {
        Assert.Equal(3f, AffineMatrix.MaxScale(Matrix3x2.CreateScale(3f)), 4);
    }

    [Fact]
    public void MaxScale_NonUniform_Returns_Larger_Axis()
    {
        Assert.Equal(4f, AffineMatrix.MaxScale(Matrix3x2.CreateScale(2f, 4f)), 4);
    }

    [Fact]
    public void MaxScale_With_Rotation_Preserves_Length()
    {
        var m = Matrix3x2.CreateRotation(MathF.PI / 3f);
        Assert.Equal(1f, AffineMatrix.MaxScale(m), 4);
    }

    [Fact]
    public void InvertOrIdentity_Returns_True_Inverse_For_NonSingular()
    {
        var m = Matrix3x2.CreateScale(2f) * Matrix3x2.CreateTranslation(3f, 4f);
        var inv = AffineMatrix.InvertOrIdentity(m);
        var p = AffineMatrix.TransformPoint(AffineMatrix.TransformPoint(new Vector2(5, 5), m), inv);
        Assert.Equal(5f, p.X, 3);
        Assert.Equal(5f, p.Y, 3);
    }

    [Fact]
    public void InvertOrIdentity_Falls_Back_For_Singular()
    {
        // Zero matrix is singular.
        var singular = new Matrix3x2(0, 0, 0, 0, 0, 0);
        Assert.Equal(Matrix3x2.Identity, AffineMatrix.InvertOrIdentity(singular));
    }

    [Fact]
    public void Compose_Is_Not_Commutative()
    {
        var a = Matrix3x2.CreateTranslation(10f, 0f);
        var b = Matrix3x2.CreateScale(2f);
        var ab = AffineMatrix.Compose(a, b);
        var ba = AffineMatrix.Compose(b, a);
        var pAb = AffineMatrix.TransformPoint(new Vector2(1, 0), ab);
        var pBa = AffineMatrix.TransformPoint(new Vector2(1, 0), ba);
        Assert.NotEqual(pAb, pBa);
    }

    [Fact]
    public void TransformVector_Applies_Rotation_Only()
    {
        // 90 degrees CCW; translation should be ignored.
        var m = Matrix3x2.CreateRotation(MathF.PI / 2f) * Matrix3x2.CreateTranslation(100f, 200f);
        var v = AffineMatrix.TransformVector(new Vector2(1, 0), m);
        Assert.Equal(0f, v.X, 4);
        Assert.Equal(1f, v.Y, 4);
    }

    [Fact]
    public void TransformVector_Identity_Returns_Same_Vector()
    {
        var v = AffineMatrix.TransformVector(new Vector2(3, -5), Matrix3x2.Identity);
        Assert.Equal(new Vector2(3, -5), v);
    }

    [Fact]
    public void MaxScale_Translation_Only_Returns_One()
    {
        var m = Matrix3x2.CreateTranslation(100f, -200f);
        Assert.Equal(1f, AffineMatrix.MaxScale(m), 4);
    }

    [Fact]
    public void MaxScale_Negative_Scale_Returns_Absolute_Value()
    {
        // Reflection along X: M11=-1, M22=1; operator norm = 1.
        var m = Matrix3x2.CreateScale(-1f, 1f);
        Assert.Equal(1f, AffineMatrix.MaxScale(m), 4);
    }

    [Fact]
    public void InvertOrIdentity_Inverts_Pure_Translation()
    {
        var m = Matrix3x2.CreateTranslation(5f, -7f);
        var inv = AffineMatrix.InvertOrIdentity(m);
        var p = AffineMatrix.TransformPoint(AffineMatrix.TransformPoint(new Vector2(10, 20), m), inv);
        Assert.Equal(10f, p.X, 3);
        Assert.Equal(20f, p.Y, 3);
    }

    [Fact]
    public void InvertOrIdentity_Inverts_Pure_Rotation()
    {
        var m = Matrix3x2.CreateRotation(MathF.PI / 4f);
        var inv = AffineMatrix.InvertOrIdentity(m);
        var p = AffineMatrix.TransformPoint(AffineMatrix.TransformPoint(new Vector2(7, 11), m), inv);
        Assert.Equal(7f, p.X, 3);
        Assert.Equal(11f, p.Y, 3);
    }

    [Fact]
    public void Compose_With_Identity_Parent_Returns_Child()
    {
        var child = Matrix3x2.CreateRotation(0.7f) * Matrix3x2.CreateTranslation(5f, 6f);
        var composed = AffineMatrix.Compose(Matrix3x2.Identity, child);
        var p1 = AffineMatrix.TransformPoint(new Vector2(2, 3), composed);
        var p2 = AffineMatrix.TransformPoint(new Vector2(2, 3), child);
        Assert.Equal(p2.X, p1.X, 4);
        Assert.Equal(p2.Y, p1.Y, 4);
    }

    [Fact]
    public void Compose_With_Identity_Child_Returns_Parent()
    {
        var parent = Matrix3x2.CreateScale(2f, 3f) * Matrix3x2.CreateTranslation(1f, -1f);
        var composed = AffineMatrix.Compose(parent, Matrix3x2.Identity);
        var p1 = AffineMatrix.TransformPoint(new Vector2(4, 5), composed);
        var p2 = AffineMatrix.TransformPoint(new Vector2(4, 5), parent);
        Assert.Equal(p2.X, p1.X, 4);
        Assert.Equal(p2.Y, p1.Y, 4);
    }

    [Fact]
    public void TransformPoint_90Deg_CCW_Rotation_Maps_X_To_Y()
    {
        var m = Matrix3x2.CreateRotation(MathF.PI / 2f);
        var p = AffineMatrix.TransformPoint(new Vector2(1, 0), m);
        Assert.Equal(0f, p.X, 4);
        Assert.Equal(1f, p.Y, 4);
    }

    [Fact]
    public void MaxScale_Of_Shear_Is_Greater_Than_One()
    {
        // Horizontal shear: x' = x + k*y, y' = y. Operator norm > 1 for k>0.
        var shear = new Matrix3x2(1f, 0f, 2f, 1f, 0f, 0f);
        Assert.True(AffineMatrix.MaxScale(shear) > 1f);
    }

    [Fact]
    public void MaxScale_Large_Uniform_Scale_Returns_Magnitude()
    {
        Assert.Equal(100f, AffineMatrix.MaxScale(Matrix3x2.CreateScale(100f)), 3);
    }

    [Fact]
    public void InvertOrIdentity_RoundTrips_Shear()
    {
        var shear = new Matrix3x2(1f, 0f, 0.5f, 1f, 0f, 0f);
        var inv = AffineMatrix.InvertOrIdentity(shear);
        var p = AffineMatrix.TransformPoint(
            AffineMatrix.TransformPoint(new Vector2(3, 7), shear), inv);
        Assert.Equal(3f, p.X, 3);
        Assert.Equal(7f, p.Y, 3);
    }

    [Fact]
    public void Compose_Three_Transforms_Applies_Right_To_Left()
    {
        // Apply T then S then R to point (1,0). Compose builds R*(S*T) under
        // SVG cascade order (parent first, then child); a point at (1,0)
        // should first be translated, then scaled, then rotated.
        var t = Matrix3x2.CreateTranslation(1f, 0f);  // (1,0) -> (2,0)
        var s = Matrix3x2.CreateScale(3f);            // (2,0) -> (6,0)
        var r = Matrix3x2.CreateRotation(MathF.PI / 2f); // (6,0) -> (0,6)

        var combined = AffineMatrix.Compose(AffineMatrix.Compose(r, s), t);
        var p = AffineMatrix.TransformPoint(new Vector2(1, 0), combined);
        Assert.Equal(0f, p.X, 3);
        Assert.Equal(6f, p.Y, 3);
    }
}
