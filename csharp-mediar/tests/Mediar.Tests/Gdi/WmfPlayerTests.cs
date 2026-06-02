using Mediar.Codecs.Gdi;
using Mediar.Imaging;
using Mediar.Vector;
using Xunit;

namespace Mediar.Tests.Gdi;

/// <summary>
/// Tests for <see cref="WmfPlayer"/>: synthesise minimal WMF byte streams
/// (with or without the Aldus Placeable preamble) from <see cref="WmfWriter"/>
/// and verify the rasteriser produces the expected pixel output.
/// </summary>
public class WmfPlayerTests
{
    [Fact]
    public void RejectsTooShort()
    {
        Assert.Throws<ArgumentException>(() => WmfPlayer.Render(new byte[10], 32, 32));
    }

    [Fact]
    public void HeaderOnly_NonPlaceable_RendersBlank()
    {
        using var w = new WmfWriter();
        w.SetWindowExt(100, 100);
        var bytes = w.Build();
        var result = WmfPlayer.Render(bytes, 64, 64, RgbaColor.FromBytes(255, 255, 255));
        Assert.Equal(64, result.Frame.Width);
        Assert.Equal(64, result.Frame.Height);
        AssertPixelEquals(result.Frame, 32, 32, 255, 255, 255);
    }

    [Fact]
    public void Placeable_HeaderBoundsRecognized()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100, inch: 96);
        var bytes = w.Build();
        var result = WmfPlayer.Render(bytes, 64, 64);
        Assert.Equal(64, result.Frame.Width);
    }

    [Fact]
    public void Rectangle_WithRedBrush_FillsInterior()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100);
        ushort nextSlot = 0;
        ushort brush = w.CreateSolidBrush(255, 0, 0, ref nextSlot);
        // Need an explicit NULL pen to suppress the default black stroke;
        // simplest: create a pen and select it. PS_NULL = 5.
        ushort pen = w.CreatePen(5, 0, 0, 0, 0, ref nextSlot);
        w.SelectObject(brush);
        w.SelectObject(pen);
        w.Rectangle(20, 20, 80, 80);
        var result = WmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        AssertRedGreater(result.Frame, 50, 50, threshold: 200);
        AssertPixelEquals(result.Frame, 5, 5, 255, 255, 255);
    }

    [Fact]
    public void Polygon_GreenTriangle()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100);
        ushort nextSlot = 0;
        ushort brush = w.CreateSolidBrush(0, 200, 0, ref nextSlot);
        ushort pen = w.CreatePen(5, 0, 0, 0, 0, ref nextSlot);
        w.SelectObject(brush);
        w.SelectObject(pen);
        w.Polygon([(50, 10), (90, 90), (10, 90)]);
        var result = WmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(0, 0, 0));
        AssertGreenGreater(result.Frame, 50, 60, threshold: 150);
        AssertPixelEquals(result.Frame, 5, 5, 0, 0, 0);
    }

    [Fact]
    public void LineTo_DrawsBlueStroke()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100);
        ushort nextSlot = 0;
        ushort pen = w.CreatePen(0, 3, 0, 0, 255, ref nextSlot);
        w.SelectObject(pen);
        w.MoveTo(10, 50);
        w.LineTo(90, 50);
        var result = WmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        AssertBlueGreater(result.Frame, 50, 50, threshold: 150);
    }

    [Fact]
    public void Unsupported_RecordIsCounted()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        // META_TEXTOUT (0x0521) is not implemented — should be counted as unsupported.
        w.RawRecord(0x0521, [0x00, 0x00]);
        var result = WmfPlayer.Render(w.Build(), 50, 50);
        Assert.True(result.UnsupportedRecordCount >= 1, $"unsupported={result.UnsupportedRecordCount}");
    }

    [Fact]
    public void Rejects_Zero_Width()
    {
        using var w = new WmfWriter();
        w.SetWindowExt(100, 100);
        Assert.Throws<ArgumentOutOfRangeException>(() => WmfPlayer.Render(w.Build(), 0, 32));
    }

    [Fact]
    public void Rejects_Negative_Height()
    {
        using var w = new WmfWriter();
        w.SetWindowExt(100, 100);
        Assert.Throws<ArgumentOutOfRangeException>(() => WmfPlayer.Render(w.Build(), 32, -10));
    }

    [Fact]
    public void RecordsRead_Counts_Supported_Records()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        w.SetWindowExt(100, 100);
        var result = WmfPlayer.Render(w.Build(), 32, 32, RgbaColor.FromBytes(255, 255, 255));
        // SetWindowExt + Eof should both count.
        Assert.True(result.RecordsRead >= 2, $"records={result.RecordsRead}");
        Assert.Equal(0, result.UnsupportedRecordCount);
    }

    [Fact]
    public void Background_Color_Visible_At_Corners()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100);
        ushort nextSlot = 0;
        ushort brush = w.CreateSolidBrush(0, 0, 255, ref nextSlot);
        ushort pen = w.CreatePen(5, 0, 0, 0, 0, ref nextSlot);
        w.SelectObject(brush);
        w.SelectObject(pen);
        w.Rectangle(40, 40, 60, 60);
        var result = WmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(200, 100, 50));
        // Corner pixel should be background, not brush color.
        AssertPixelEquals(result.Frame, 1, 1, 200, 100, 50);
        AssertPixelEquals(result.Frame, 98, 98, 200, 100, 50);
    }

    [Fact]
    public void Ellipse_Fills_Interior_With_Brush()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100);
        ushort nextSlot = 0;
        ushort brush = w.CreateSolidBrush(0, 255, 0, ref nextSlot);
        ushort pen = w.CreatePen(5, 0, 0, 0, 0, ref nextSlot);
        w.SelectObject(brush);
        w.SelectObject(pen);
        // META_ELLIPSE = 0x0418, args = bottom, right, top, left (Int16 each).
        Span<byte> args = stackalloc byte[8];
        System.Buffers.Binary.BinaryPrimitives.WriteInt16LittleEndian(args[..2], 90);
        System.Buffers.Binary.BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2, 2), 90);
        System.Buffers.Binary.BinaryPrimitives.WriteInt16LittleEndian(args.Slice(4, 2), 10);
        System.Buffers.Binary.BinaryPrimitives.WriteInt16LittleEndian(args.Slice(6, 2), 10);
        w.RawRecord(0x0418, args);
        var result = WmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(0, 0, 0));
        AssertGreenGreater(result.Frame, 50, 50, threshold: 150);
    }

    [Fact]
    public void SaveDc_RestoreDc_Counted_As_Supported()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        w.RawRecord(0x001E, []);  // META_SAVEDC
        w.RawRecord(0x0127, [0xFF, 0xFF]);  // META_RESTOREDC with nSave=-1
        var result = WmfPlayer.Render(w.Build(), 32, 32);
        Assert.Equal(0, result.UnsupportedRecordCount);
    }

    [Fact]
    public void SetMapMode_Counted_As_Supported()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        Span<byte> mode = stackalloc byte[2];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(mode, 8); // MM_ANISOTROPIC
        w.RawRecord(0x0103, mode);  // META_SETMAPMODE
        var result = WmfPlayer.Render(w.Build(), 32, 32);
        Assert.Equal(0, result.UnsupportedRecordCount);
    }

    [Fact]
    public void SetPolyFillMode_Counted_As_Supported()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        Span<byte> mode = stackalloc byte[2];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(mode, 1); // WINDING
        w.RawRecord(0x0106, mode);
        var result = WmfPlayer.Render(w.Build(), 32, 32);
        Assert.Equal(0, result.UnsupportedRecordCount);
    }

    [Fact]
    public void SetPixel_Counted_As_Supported()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        Span<byte> args = stackalloc byte[8];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(args[..4], 0x00FF0000u); // BGR colorref
        System.Buffers.Binary.BinaryPrimitives.WriteInt16LittleEndian(args.Slice(4, 2), 25);
        System.Buffers.Binary.BinaryPrimitives.WriteInt16LittleEndian(args.Slice(6, 2), 25);
        w.RawRecord(0x041F, args);
        var result = WmfPlayer.Render(w.Build(), 64, 64);
        Assert.Equal(0, result.UnsupportedRecordCount);
    }

    [Fact]
    public void DeleteObject_Counted_As_Supported()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 50, b: 50);
        ushort nextSlot = 0;
        ushort brush = w.CreateSolidBrush(255, 0, 0, ref nextSlot);
        Span<byte> args = stackalloc byte[2];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(args, brush);
        w.RawRecord(0x01F0, args); // META_DELETEOBJECT
        var result = WmfPlayer.Render(w.Build(), 32, 32);
        Assert.Equal(0, result.UnsupportedRecordCount);
    }

    [Fact]
    public void NullPen_Suppresses_Stroke_Around_Brushed_Rectangle()
    {
        using var w = new WmfWriter(placeable: true, l: 0, t: 0, r: 100, b: 100);
        ushort nextSlot = 0;
        ushort brush = w.CreateSolidBrush(0, 0, 255, ref nextSlot);
        ushort pen = w.CreatePen(5, 0, 0, 0, 0, ref nextSlot); // PS_NULL
        w.SelectObject(brush);
        w.SelectObject(pen);
        w.Rectangle(20, 20, 80, 80);
        var result = WmfPlayer.Render(w.Build(), 100, 100, RgbaColor.FromBytes(255, 255, 255));
        // Just outside rectangle should still be background, no stroke leakage.
        AssertPixelEquals(result.Frame, 10, 10, 255, 255, 255);
    }

    // ---------- pixel-assertion helpers ----------------------------------

    private static void AssertPixelEquals(ImageFrame frame, int x, int y, byte r, byte g, byte b)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.InRange(span[idx + 0], b - 2, b + 2);
        Assert.InRange(span[idx + 1], g - 2, g + 2);
        Assert.InRange(span[idx + 2], r - 2, r + 2);
    }

    private static void AssertRedGreater(ImageFrame frame, int x, int y, int threshold)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.True(span[idx + 2] > threshold, $"R={span[idx + 2]} G={span[idx + 1]} B={span[idx + 0]}");
    }

    private static void AssertGreenGreater(ImageFrame frame, int x, int y, int threshold)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.True(span[idx + 1] > threshold, $"R={span[idx + 2]} G={span[idx + 1]} B={span[idx + 0]}");
    }

    private static void AssertBlueGreater(ImageFrame frame, int x, int y, int threshold)
    {
        var span = frame.Pixels.Span;
        int idx = y * frame.Stride + x * 4;
        Assert.True(span[idx + 0] > threshold, $"R={span[idx + 2]} G={span[idx + 1]} B={span[idx + 0]}");
    }
}
