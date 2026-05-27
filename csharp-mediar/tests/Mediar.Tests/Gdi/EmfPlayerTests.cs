using System.Buffers.Binary;
using Mediar.Codecs.Gdi;
using Mediar.Imaging;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Gdi;

/// <summary>
/// Tests for <see cref="EmfPlayer"/>: synthesise minimal EMF byte streams
/// from <see cref="EmfWriter"/> and verify the rasteriser produces the
/// expected pixel output for each supported record type.
/// </summary>
public class EmfPlayerTests
{
    [Fact]
    public void RejectsTooShort()
    {
        Assert.Throws<ArgumentException>(() => EmfPlayer.Render(new byte[16], 32, 32));
    }

    [Fact]
    public void RejectsMissingHeader()
    {
        var buf = new byte[88];
        BinaryPrimitives.WriteUInt32LittleEndian(buf, 99); // not EMR_HEADER
        BinaryPrimitives.WriteUInt32LittleEndian(buf.AsSpan(4), 88);
        Assert.Throws<ArgumentException>(() => EmfPlayer.Render(buf, 32, 32));
    }

    [Fact]
    public void HeaderOnly_ProducesBlankFrame()
    {
        var emf = new EmfWriter(0, 0, 64, 64).Eof().Build();
        var result = EmfPlayer.Render(emf, 64, 64, RgbaColor.FromBytes(255, 255, 255));
        Assert.Equal(64, result.Frame.Width);
        Assert.Equal(64, result.Frame.Height);
        Assert.Equal(PixelFormat.Bgra32, result.Frame.PixelFormat);
        AssertPixelEquals(result.Frame, 32, 32, 255, 255, 255);
    }

    [Fact]
    public void Rectangle_WithRedBrush_FillsInterior()
    {
        var w = new EmfWriter(0, 0, 100, 100);
        uint brush = w.CreateSolidBrush(255, 0, 0);
        w.SelectObject(brush);
        w.SelectStockObject(8); // NULL_PEN
        w.Rectangle(20, 20, 80, 80);
        w.Eof();

        var result = EmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        AssertRedGreater(result.Frame, 50, 50, threshold: 200);
        AssertPixelEquals(result.Frame, 5, 5, 255, 255, 255);
    }

    [Fact]
    public void Polygon_TriangleFill()
    {
        var w = new EmfWriter(0, 0, 100, 100);
        uint brush = w.CreateSolidBrush(0, 200, 0);
        w.SelectObject(brush);
        w.SelectStockObject(8); // NULL_PEN
        w.Polygon16([(50, 10), (90, 90), (10, 90)]);
        w.Eof();

        var result = EmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(0, 0, 0));
        AssertGreenGreater(result.Frame, 50, 60, threshold: 150);
        AssertPixelEquals(result.Frame, 5, 5, 0, 0, 0);
    }

    [Fact]
    public void MoveTo_LineTo_DrawsStroke()
    {
        var w = new EmfWriter(0, 0, 100, 100);
        uint pen = w.CreatePen(0 /* PS_SOLID */, 3, 0, 0, 255);
        w.SelectObject(pen);
        w.MoveToEx(10, 50);
        w.LineTo(90, 50);
        w.Eof();

        var result = EmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        AssertBlueGreater(result.Frame, 50, 50, threshold: 150);
    }

    [Fact]
    public void Path_BeginEndFill_FillsCustomPath()
    {
        var w = new EmfWriter(0, 0, 100, 100);
        uint brush = w.CreateSolidBrush(0, 0, 255);
        w.SelectObject(brush);
        w.SelectStockObject(8); // NULL_PEN
        w.BeginPath();
        w.MoveToEx(10, 10);
        w.LineTo(90, 10);
        w.LineTo(90, 90);
        w.LineTo(10, 90);
        w.CloseFigure();
        w.EndPath();
        w.FillPath();
        w.Eof();

        var result = EmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        AssertBlueGreater(result.Frame, 50, 50, threshold: 150);
    }

    [Fact]
    public void Unsupported_RecordIsCounted()
    {
        var w = new EmfWriter(0, 0, 50, 50);
        // Record-type 250 is reserved/unused — should be counted as unsupported.
        w.RawRecord(recordType: 250, payload: new byte[8]);
        w.Eof();
        var result = EmfPlayer.Render(w.Build(), 50, 50);
        Assert.True(result.UnsupportedRecordCount >= 1, $"unsupported={result.UnsupportedRecordCount}");
        Assert.True(result.RecordsRead >= 3, $"records={result.RecordsRead}");
    }

    [Fact]
    public void SaveDc_RestoreDc_RoundTripsBrush()
    {
        var w = new EmfWriter(0, 0, 100, 100);
        uint redBrush = w.CreateSolidBrush(255, 0, 0);
        uint blueBrush = w.CreateSolidBrush(0, 0, 255);
        w.SelectObject(redBrush);
        w.SelectStockObject(8); // NULL_PEN
        w.SaveDc();
        w.SelectObject(blueBrush);
        w.Rectangle(10, 10, 40, 40);
        w.RestoreDc(-1);
        w.Rectangle(60, 60, 90, 90);
        w.Eof();

        var result = EmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        AssertBlueGreater(result.Frame, 25, 25, threshold: 150);
        AssertRedGreater(result.Frame, 75, 75, threshold: 150);
    }

    // ---------- pixel-assertion helpers ----------------------------------

    private static void AssertPixelEquals(ImageFrame frame, int x, int y, byte r, byte g, byte b)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        // Bgra32: B, G, R, A
        Assert.InRange(span[idx + 0], b - 2, b + 2);
        Assert.InRange(span[idx + 1], g - 2, g + 2);
        Assert.InRange(span[idx + 2], r - 2, r + 2);
    }

    private static void AssertRedGreater(ImageFrame frame, int x, int y, int threshold)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.True(span[idx + 2] > threshold, $"R={span[idx + 2]} G={span[idx + 1]} B={span[idx + 0]} expected R>{threshold}");
    }

    private static void AssertGreenGreater(ImageFrame frame, int x, int y, int threshold)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.True(span[idx + 1] > threshold, $"R={span[idx + 2]} G={span[idx + 1]} B={span[idx + 0]} expected G>{threshold}");
    }

    private static void AssertBlueGreater(ImageFrame frame, int x, int y, int threshold)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.True(span[idx + 0] > threshold, $"R={span[idx + 2]} G={span[idx + 1]} B={span[idx + 0]} expected B>{threshold}");
    }
}
