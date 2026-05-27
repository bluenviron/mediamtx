using System.Buffers.Binary;
using System.Numerics;
using Mediar.Imaging;
using Mediar.Vector;

namespace Mediar.Codecs.Gdi;

/// <summary>
/// Replays an MS-WMF byte stream into a rasterised <see cref="ImageFrame"/>.
/// Accepts either a "standard" WMF (META_HEADER + records) or the 22-byte
/// Aldus Placeable preamble + WMF. Records outside the supported set are
/// silently skipped and counted on
/// <see cref="EmfPlaybackResult.UnsupportedRecordCount"/>.
/// </summary>
public static class WmfPlayer
{
    private const uint AldusPlaceableKey = 0x9AC6CDD7;

    /// <summary>
    /// Render <paramref name="wmfData"/> at the requested output resolution.
    /// When the input is an Aldus Placeable Metafile the embedded bounding
    /// box is used; otherwise the window-extents from META_SETWINDOWEXT are
    /// used (which gives a "best effort" guess for raw WMFs).
    /// </summary>
    public static EmfPlaybackResult Render(
        ReadOnlySpan<byte> wmfData,
        int outputWidth,
        int outputHeight,
        RgbaColor background = default)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(outputWidth);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(outputHeight);
        if (wmfData.Length < 18)
            throw new ArgumentException("WMF data too short to contain a META_HEADER.", nameof(wmfData));

        int offset = 0;
        bool isPlaceable = wmfData.Length >= 22
            && BinaryPrimitives.ReadUInt32LittleEndian(wmfData) == AldusPlaceableKey;

        float boundsW = outputWidth;
        float boundsH = outputHeight;
        Vector2 boundsTopLeft = Vector2.Zero;

        if (isPlaceable)
        {
            short l = BinaryPrimitives.ReadInt16LittleEndian(wmfData[6..]);
            short t = BinaryPrimitives.ReadInt16LittleEndian(wmfData[8..]);
            short r = BinaryPrimitives.ReadInt16LittleEndian(wmfData[10..]);
            short b = BinaryPrimitives.ReadInt16LittleEndian(wmfData[12..]);
            boundsTopLeft = new Vector2(l, t);
            boundsW = Math.Max(1, r - l);
            boundsH = Math.Max(1, b - t);
            offset = 22;
        }

        if (wmfData.Length < offset + 18)
            throw new ArgumentException("WMF body too short for META_HEADER.", nameof(wmfData));

        // Skip META_HEADER (18 bytes).
        int p = offset + 18;
        // For non-placeable WMF we'll detect bounds from the first META_SETWINDOWEXT
        // we encounter and re-fit the output canvas. Defer creating the rasterizer
        // by walking the records twice if needed.
        if (!isPlaceable)
        {
            for (int q = p; q + 6 <= wmfData.Length;)
            {
                uint sizeWords = BinaryPrimitives.ReadUInt32LittleEndian(wmfData[q..]);
                ushort func = BinaryPrimitives.ReadUInt16LittleEndian(wmfData[(q + 4)..]);
                long sizeBytes = (long)sizeWords * 2;
                if (sizeBytes < 6 || q + sizeBytes > wmfData.Length) break;
                if ((WmfRecordType)func == WmfRecordType.SetWindowExt)
                {
                    short eh = BinaryPrimitives.ReadInt16LittleEndian(wmfData[(q + 6)..]);
                    short ew = BinaryPrimitives.ReadInt16LittleEndian(wmfData[(q + 8)..]);
                    boundsW = Math.Max(1, Math.Abs((int)ew));
                    boundsH = Math.Max(1, Math.Abs((int)eh));
                    break;
                }
                q += (int)sizeBytes;
                if (func == 0) break;
            }
        }

        float sx = outputWidth / boundsW;
        float sy = outputHeight / boundsH;
        float scale = MathF.Min(sx, sy);
        float offX = (outputWidth - boundsW * scale) / 2f - boundsTopLeft.X * scale;
        float offY = (outputHeight - boundsH * scale) / 2f - boundsTopLeft.Y * scale;
        var deviceFit = new Matrix3x2(scale, 0, 0, scale, offX, offY);

        var target = RasterTarget.Create(outputWidth, outputHeight, background);
        var state = new GdiState();
        int recordsRead = 0;
        int unsupported = 0;

        // For WMF the object table is a positional slot list; SELECTOBJECT
        // uses an index that addresses the next free slot from CREATEPEN/BRUSH.
        var slots = new List<GdiObject?>();

        while (p + 6 <= wmfData.Length)
        {
            uint sizeWords = BinaryPrimitives.ReadUInt32LittleEndian(wmfData[p..]);
            ushort func = BinaryPrimitives.ReadUInt16LittleEndian(wmfData[(p + 4)..]);
            long sizeBytes = (long)sizeWords * 2;
            if (sizeBytes < 6 || p + sizeBytes > wmfData.Length) break;

            ReadOnlySpan<byte> args = wmfData.Slice(p + 6, (int)sizeBytes - 6);
            if (!Dispatch((WmfRecordType)func, args, state, target, deviceFit, slots))
                unsupported++;

            recordsRead++;
            p += (int)sizeBytes;
            if (func == 0) break;
        }

        int stride = outputWidth * 4;
        byte[] buf = new byte[outputHeight * stride];
        target.Pixels.CopyTo(buf);
        var frame = new ImageFrame(outputWidth, outputHeight, PixelFormat.Bgra32, stride, buf);
        return new EmfPlaybackResult(frame, recordsRead, unsupported);
    }

    private static bool Dispatch(
        WmfRecordType type, ReadOnlySpan<byte> a, GdiState s, RasterTarget target,
        in Matrix3x2 fit, List<GdiObject?> slots)
    {
        switch (type)
        {
            case WmfRecordType.Eof: return true;
            case WmfRecordType.SetWindowOrg: s.WindowOrigin = Read16Point(a, reverseYX: true); return true;
            case WmfRecordType.SetWindowExt: s.WindowExtent = Read16Point(a, reverseYX: true); return true;
            case WmfRecordType.SetViewportOrg: s.ViewportOrigin = Read16Point(a, reverseYX: true); return true;
            case WmfRecordType.SetViewportExt: s.ViewportExtent = Read16Point(a, reverseYX: true); return true;
            case WmfRecordType.OffsetWindowOrg:
                s.WindowOrigin = new Vector2(
                    s.WindowOrigin.X + BinaryPrimitives.ReadInt16LittleEndian(a[2..]),
                    s.WindowOrigin.Y + BinaryPrimitives.ReadInt16LittleEndian(a));
                return true;
            case WmfRecordType.OffsetViewportOrg:
                s.ViewportOrigin = new Vector2(
                    s.ViewportOrigin.X + BinaryPrimitives.ReadInt16LittleEndian(a[2..]),
                    s.ViewportOrigin.Y + BinaryPrimitives.ReadInt16LittleEndian(a));
                return true;
            case WmfRecordType.SetMapMode:
                if (a.Length >= 2) s.MapMode = (EmfMapMode)BinaryPrimitives.ReadUInt16LittleEndian(a);
                return true;
            case WmfRecordType.SetPolyFillMode:
                if (a.Length >= 2)
                {
                    ushort mode = BinaryPrimitives.ReadUInt16LittleEndian(a);
                    s.PolyFillRule = mode == 2 ? FillRule.NonZero : FillRule.EvenOdd;
                }
                return true;
            case WmfRecordType.MoveTo:
                if (a.Length >= 4)
                {
                    short y = BinaryPrimitives.ReadInt16LittleEndian(a);
                    short x = BinaryPrimitives.ReadInt16LittleEndian(a[2..]);
                    s.CurrentPoint = new Vector2(x, y);
                }
                return true;
            case WmfRecordType.LineTo:
                if (a.Length >= 4)
                {
                    short y = BinaryPrimitives.ReadInt16LittleEndian(a);
                    short x = BinaryPrimitives.ReadInt16LittleEndian(a[2..]);
                    var end = new Vector2(x, y);
                    var path = new Path2D();
                    path.MoveTo(s.CurrentPoint.X, s.CurrentPoint.Y).LineTo(end.X, end.Y);
                    StrokeImmediate(path, s, target, fit);
                    s.CurrentPoint = end;
                }
                return true;
            case WmfRecordType.SetPixel:
                if (a.Length >= 8)
                {
                    uint c = BinaryPrimitives.ReadUInt32LittleEndian(a);
                    short y = BinaryPrimitives.ReadInt16LittleEndian(a[4..]);
                    short x = BinaryPrimitives.ReadInt16LittleEndian(a[6..]);
                    var path = new Path2D();
                    path.MoveTo(x, y).LineTo(x + 1, y).LineTo(x + 1, y + 1).LineTo(x, y + 1).Close();
                    FillSolid(path, GdiCoords.DecodeColorRef(c), s, target, fit);
                }
                return true;
            case WmfRecordType.Rectangle:
                if (a.Length >= 8)
                {
                    short bot = BinaryPrimitives.ReadInt16LittleEndian(a);
                    short rht = BinaryPrimitives.ReadInt16LittleEndian(a[2..]);
                    short top = BinaryPrimitives.ReadInt16LittleEndian(a[4..]);
                    short lft = BinaryPrimitives.ReadInt16LittleEndian(a[6..]);
                    var path = new Path2D();
                    path.MoveTo(lft, top).LineTo(rht, top).LineTo(rht, bot).LineTo(lft, bot).Close();
                    ExecuteDraw(path, s, target, fit);
                }
                return true;
            case WmfRecordType.Ellipse:
                if (a.Length >= 8)
                {
                    short bot = BinaryPrimitives.ReadInt16LittleEndian(a);
                    short rht = BinaryPrimitives.ReadInt16LittleEndian(a[2..]);
                    short top = BinaryPrimitives.ReadInt16LittleEndian(a[4..]);
                    short lft = BinaryPrimitives.ReadInt16LittleEndian(a[6..]);
                    var path = EllipsePath(lft, top, rht, bot);
                    ExecuteDraw(path, s, target, fit);
                }
                return true;
            case WmfRecordType.RoundRect:
                if (a.Length >= 12)
                {
                    short ch = BinaryPrimitives.ReadInt16LittleEndian(a);
                    short cw = BinaryPrimitives.ReadInt16LittleEndian(a[2..]);
                    short bot = BinaryPrimitives.ReadInt16LittleEndian(a[4..]);
                    short rht = BinaryPrimitives.ReadInt16LittleEndian(a[6..]);
                    short top = BinaryPrimitives.ReadInt16LittleEndian(a[8..]);
                    short lft = BinaryPrimitives.ReadInt16LittleEndian(a[10..]);
                    var path = RoundRectPath(lft, top, rht, bot, cw, ch);
                    ExecuteDraw(path, s, target, fit);
                }
                return true;
            case WmfRecordType.Polygon: HandleWmfPoly(a, s, target, fit, close: true, fill: true); return true;
            case WmfRecordType.Polyline: HandleWmfPoly(a, s, target, fit, close: false, fill: false); return true;
            case WmfRecordType.PolyPolygon: HandleWmfPolyPoly(a, s, target, fit); return true;
            case WmfRecordType.CreatePenIndirect:
                if (a.Length >= 10)
                {
                    ushort style = BinaryPrimitives.ReadUInt16LittleEndian(a);
                    short widthY = BinaryPrimitives.ReadInt16LittleEndian(a[2..]);
                    short widthX = BinaryPrimitives.ReadInt16LittleEndian(a[4..]);
                    uint colorRef = BinaryPrimitives.ReadUInt32LittleEndian(a[6..]);
                    float w = Math.Max(MathF.Abs(widthX), MathF.Abs(widthY));
                    slots.Add(new GdiPen(NormalisePenStyle(style), w, GdiCoords.DecodeColorRef(colorRef)));
                }
                return true;
            case WmfRecordType.CreateBrushIndirect:
                if (a.Length >= 8)
                {
                    ushort style = BinaryPrimitives.ReadUInt16LittleEndian(a);
                    uint colorRef = BinaryPrimitives.ReadUInt32LittleEndian(a[2..]);
                    slots.Add(new GdiBrush((GdiBrushStyle)style, GdiCoords.DecodeColorRef(colorRef)));
                }
                return true;
            case WmfRecordType.DeleteObject:
                if (a.Length >= 2)
                {
                    ushort idx = BinaryPrimitives.ReadUInt16LittleEndian(a);
                    if (idx < slots.Count) slots[idx] = null;
                }
                return true;
            case WmfRecordType.SelectObject:
                if (a.Length >= 2)
                {
                    ushort idx = BinaryPrimitives.ReadUInt16LittleEndian(a);
                    if (idx < slots.Count)
                    {
                        var obj = slots[idx];
                        if (obj is GdiPen pen) s.CurrentPen = pen;
                        else if (obj is GdiBrush brush) s.CurrentBrush = brush;
                    }
                }
                return true;
            case WmfRecordType.SaveDc: s.SaveCounter++; s.Stack.Push(s.Snapshot()); return true;
            case WmfRecordType.RestoreDc:
                if (s.Stack.Count > 0) s.Restore(s.Stack.Pop());
                return true;
            default: return false;
        }
    }

    private static void HandleWmfPoly(
        ReadOnlySpan<byte> a, GdiState s, RasterTarget target, in Matrix3x2 fit, bool close, bool fill)
    {
        if (a.Length < 2) return;
        ushort n = BinaryPrimitives.ReadUInt16LittleEndian(a);
        if (n == 0 || a.Length < 2 + n * 4) return;
        var path = new Path2D();
        for (int i = 0; i < n; i++)
        {
            short x = BinaryPrimitives.ReadInt16LittleEndian(a[(2 + i * 4)..]);
            short y = BinaryPrimitives.ReadInt16LittleEndian(a[(4 + i * 4)..]);
            if (i == 0) path.MoveTo(x, y); else path.LineTo(x, y);
        }
        if (close) path.Close();
        if (fill) ExecuteDraw(path, s, target, fit);
        else StrokeImmediate(path, s, target, fit);
    }

    private static void HandleWmfPolyPoly(ReadOnlySpan<byte> a, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (a.Length < 2) return;
        ushort nPolys = BinaryPrimitives.ReadUInt16LittleEndian(a);
        if (nPolys == 0 || a.Length < 2 + nPolys * 2) return;
        Span<ushort> counts = stackalloc ushort[nPolys];
        int total = 0;
        for (int i = 0; i < nPolys; i++)
        {
            counts[i] = BinaryPrimitives.ReadUInt16LittleEndian(a[(2 + i * 2)..]);
            total += counts[i];
        }
        int pOff = 2 + nPolys * 2;
        if (a.Length < pOff + total * 4) return;
        var path = new Path2D();
        int idx = 0;
        for (int p = 0; p < nPolys; p++)
        {
            int n = counts[p];
            for (int i = 0; i < n; i++, idx++)
            {
                short x = BinaryPrimitives.ReadInt16LittleEndian(a[(pOff + idx * 4)..]);
                short y = BinaryPrimitives.ReadInt16LittleEndian(a[(pOff + idx * 4 + 2)..]);
                if (i == 0) path.MoveTo(x, y); else path.LineTo(x, y);
            }
            path.Close();
        }
        ExecuteDraw(path, s, target, fit);
    }

    private static Path2D EllipsePath(int l, int t, int r, int b)
    {
        float cx = (l + r) / 2f, cy = (t + b) / 2f;
        float rx = (r - l) / 2f, ry = (b - t) / 2f;
        var path = new Path2D();
        path.MoveTo(cx + rx, cy);
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx, cy + ry));
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx - rx, cy));
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx, cy - ry));
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx + rx, cy));
        path.Close();
        return path;
    }

    private static Path2D RoundRectPath(int l, int t, int r, int b, int cw, int ch)
    {
        float rx = Math.Min(cw / 2f, (r - l) / 2f);
        float ry = Math.Min(ch / 2f, (b - t) / 2f);
        var path = new Path2D();
        path.MoveTo(l + rx, t);
        path.LineTo(r - rx, t);
        path.ArcTo(rx, ry, 0, false, true, new Vector2(r, t + ry));
        path.LineTo(r, b - ry);
        path.ArcTo(rx, ry, 0, false, true, new Vector2(r - rx, b));
        path.LineTo(l + rx, b);
        path.ArcTo(rx, ry, 0, false, true, new Vector2(l, b - ry));
        path.LineTo(l, t + ry);
        path.ArcTo(rx, ry, 0, false, true, new Vector2(l + rx, t));
        path.Close();
        return path;
    }

    private static void ExecuteDraw(Path2D path, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (!s.CurrentBrush.IsNullBrush)
            FillSolid(path, s.CurrentBrush.Color, s, target, fit);
        if (!s.CurrentPen.IsNullPen)
            StrokeImmediate(path, s, target, fit);
    }

    private static void FillSolid(Path2D path, RgbaColor color, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        Matrix3x2 m = ComposeTransform(s, fit);
        var bounds = path.GetBounds();
        var eval = PaintEvaluatorFactory.Create(new SolidPaint(color), bounds, m);
        ScanlineRasterizer.Fill(target, path, m, eval, s.PolyFillRule);
    }

    private static void StrokeImmediate(Path2D path, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (s.CurrentPen.IsNullPen) return;
        Matrix3x2 m = ComposeTransform(s, fit);
        float width = MathF.Max(s.CurrentPen.Width, 1f);
        var stroke = new StrokeStyle(width, s.CurrentPen.Cap, s.CurrentPen.Join);
        var filled = StrokeToFill.Stroke(path, stroke);
        if (filled.IsEmpty) return;
        var bounds = filled.GetBounds();
        var eval = PaintEvaluatorFactory.Create(new SolidPaint(s.CurrentPen.Color), bounds, m);
        ScanlineRasterizer.Fill(target, filled, m, eval, FillRule.EvenOdd);
    }

    private static Matrix3x2 ComposeTransform(GdiState s, in Matrix3x2 deviceFit)
    {
        float kx = s.WindowExtent.X == 0 ? 1f : s.ViewportExtent.X / s.WindowExtent.X;
        float ky = s.WindowExtent.Y == 0 ? 1f : s.ViewportExtent.Y / s.WindowExtent.Y;
        var win2vp = new Matrix3x2(
            kx, 0,
            0, ky,
            s.ViewportOrigin.X - s.WindowOrigin.X * kx,
            s.ViewportOrigin.Y - s.WindowOrigin.Y * ky);
        return s.WorldTransform * win2vp * deviceFit;
    }

    /// <summary>
    /// Read a WMF Point16. In WMF parameter lists points are packed in
    /// reverse YX order (Y first, X second) when <paramref name="reverseYX"/>
    /// is true (which is the case for SetWindowOrg / SetWindowExt and friends).
    /// </summary>
    private static Vector2 Read16Point(ReadOnlySpan<byte> p, bool reverseYX)
    {
        short y = BinaryPrimitives.ReadInt16LittleEndian(p);
        short x = BinaryPrimitives.ReadInt16LittleEndian(p[2..]);
        return reverseYX ? new Vector2(x, y) : new Vector2(y, x);
    }

    private static GdiPenStyle NormalisePenStyle(uint style) => (style & 0xFF) switch
    {
        0 => GdiPenStyle.Solid,
        1 => GdiPenStyle.Dash,
        2 => GdiPenStyle.Dot,
        3 => GdiPenStyle.DashDot,
        4 => GdiPenStyle.DashDotDot,
        5 => GdiPenStyle.Null,
        6 => GdiPenStyle.InsideFrame,
        _ => GdiPenStyle.Solid,
    };
}
