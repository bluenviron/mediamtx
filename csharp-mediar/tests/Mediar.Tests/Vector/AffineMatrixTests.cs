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
}
