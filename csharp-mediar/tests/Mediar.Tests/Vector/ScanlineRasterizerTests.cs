using System.Numerics;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Vector;

public class ScanlineRasterizerTests
{
    private static (RasterTarget Target, byte[] Buffer) NewTarget(int w, int h)
    {
        var buf = new byte[w * h * 4];
        return (new RasterTarget(w, h, buf), buf);
    }

    private static (byte B, byte G, byte R, byte A) PixelAt(byte[] buf, int w, int x, int y)
    {
        int i = y * w * 4 + x * 4;
        return (buf[i], buf[i + 1], buf[i + 2], buf[i + 3]);
    }

    [Fact]
    public void Filled_Rectangle_Has_Opaque_Interior_Pixels()
    {
        var (target, buf) = NewTarget(20, 20);
        var path = new Path2D().MoveTo(5, 5).LineTo(15, 5).LineTo(15, 15).LineTo(5, 15).Close();
        var red = new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0));
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity, red);

        // Center pixel must be fully opaque red.
        var (b, g, r, a) = PixelAt(buf, 20, 10, 10);
        Assert.Equal(255, a);
        Assert.Equal(255, r);
        Assert.Equal(0, g);
        Assert.Equal(0, b);
    }

    [Fact]
    public void Filled_Rectangle_Leaves_Outside_Transparent()
    {
        var (target, buf) = NewTarget(20, 20);
        var path = new Path2D().MoveTo(5, 5).LineTo(15, 5).LineTo(15, 15).LineTo(5, 15).Close();
        var red = new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0));
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity, red);

        // Pixel at (0,0) was never touched - still default zero alpha.
        var (b, g, r, a) = PixelAt(buf, 20, 0, 0);
        Assert.Equal(0, a);
        Assert.Equal(0, r);
        Assert.Equal(0, g);
        Assert.Equal(0, b);
    }

    [Fact]
    public void Coverage_AA_Renders_Partial_Edges()
    {
        // 0.5 px offset rectangle: edges fall mid-pixel, so border pixels must be partial.
        var (target, buf) = NewTarget(20, 20);
        var path = new Path2D().MoveTo(5.5f, 5.5f).LineTo(15.5f, 5.5f).LineTo(15.5f, 15.5f).LineTo(5.5f, 15.5f).Close();
        var red = new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0));
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity, red);

        // Pixel (5,10) covers the left edge - should be partial alpha (~50%).
        var (_, _, _, a) = PixelAt(buf, 20, 5, 10);
        Assert.InRange(a, 80, 200);
    }

    [Fact]
    public void EvenOdd_Donut_Has_Transparent_Hole()
    {
        // Outer 20x20 square at offset (5,5), inner 10x10 square at offset (10,10).
        var (target, buf) = NewTarget(30, 30);
        var path = new Path2D()
            // outer CW
            .MoveTo(5, 5).LineTo(25, 5).LineTo(25, 25).LineTo(5, 25).Close()
            // inner CW (same direction) - evenodd cancels out
            .MoveTo(10, 10).LineTo(20, 10).LineTo(20, 20).LineTo(10, 20).Close();
        var red = new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0));
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity, red, FillRule.EvenOdd);

        // Outside outer: still transparent.
        Assert.Equal(0, PixelAt(buf, 30, 0, 0).A);
        // Inside outer but outside inner (e.g. (7,15)): opaque.
        Assert.Equal(255, PixelAt(buf, 30, 7, 15).A);
        // Inside inner ring (15,15): transparent because evenodd cancels it out.
        Assert.Equal(0, PixelAt(buf, 30, 15, 15).A);
    }

    [Fact]
    public void NonZero_Same_Direction_Fills_Both_Rings()
    {
        var (target, buf) = NewTarget(30, 30);
        var path = new Path2D()
            .MoveTo(5, 5).LineTo(25, 5).LineTo(25, 25).LineTo(5, 25).Close()
            .MoveTo(10, 10).LineTo(20, 10).LineTo(20, 20).LineTo(10, 20).Close();
        var red = new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0));
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity, red);

        // With nonzero + same direction both rings have winding 2: still filled.
        Assert.Equal(255, PixelAt(buf, 30, 15, 15).A);
    }

    [Fact]
    public void Transform_Scales_Path_Into_Surface()
    {
        var (target, buf) = NewTarget(40, 40);
        // 1×1 unit square scaled by 20 = 20×20 box at origin.
        var path = new Path2D().MoveTo(0, 0).LineTo(1, 0).LineTo(1, 1).LineTo(0, 1).Close();
        ScanlineRasterizer.Fill(target, path, Matrix3x2.CreateScale(20f),
            new SolidEvaluator(RgbaColor.FromBytes(0, 255, 0)));
        // Pixel (10,10) inside the scaled box must be green.
        Assert.Equal(255, PixelAt(buf, 40, 10, 10).A);
        Assert.Equal(255, PixelAt(buf, 40, 10, 10).G);
        // Pixel (30,30) outside: transparent.
        Assert.Equal(0, PixelAt(buf, 40, 30, 30).A);
    }

    [Fact]
    public void Clip_Limits_Output_Area()
    {
        var (target, buf) = NewTarget(20, 20);
        var path = new Path2D().MoveTo(0, 0).LineTo(20, 0).LineTo(20, 20).LineTo(0, 20).Close();
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity,
            new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0)),
            FillRule.NonZero,
            clipRect: (5, 5, 10, 10));
        // Inside clip rect.
        Assert.Equal(255, PixelAt(buf, 20, 7, 7).A);
        // Outside clip rect (still inside the path, but clip excluded).
        Assert.Equal(0, PixelAt(buf, 20, 1, 1).A);
    }

    [Fact]
    public void Empty_Path_Is_NoOp()
    {
        var (target, buf) = NewTarget(10, 10);
        var path = new Path2D();
        ScanlineRasterizer.Fill(target, path, Matrix3x2.Identity,
            new SolidEvaluator(RgbaColor.FromBytes(255, 0, 0)));
        // Buffer unchanged - all zero.
        Assert.All(buf, b => Assert.Equal(0, b));
    }
}
