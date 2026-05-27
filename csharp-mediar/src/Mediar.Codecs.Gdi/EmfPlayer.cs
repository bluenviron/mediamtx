using System.Buffers.Binary;
using System.Numerics;
using Mediar.Imaging;
using Mediar.Vector;

namespace Mediar.Codecs.Gdi;

/// <summary>
/// Replays an MS-EMF byte stream into a rasterised <see cref="ImageFrame"/>.
/// The player implements the most common 50+ EMR_* records (header, EOF,
/// world transform stack, save / restore DC, window / viewport mapping,
/// pen + brush object table, all major shape primitives and their 16-bit
/// variants, path bracketing + fill / stroke / both, set pixel). Records
/// outside the supported set are silently skipped and counted on
/// <see cref="EmfPlaybackResult.UnsupportedRecordCount"/>.
/// </summary>
public static class EmfPlayer
{
    /// <summary>
    /// Render <paramref name="emfData"/> at the requested output resolution.
    /// The EMF header bounds rectangle is fitted into the output canvas
    /// preserving aspect ratio.
    /// </summary>
    public static EmfPlaybackResult Render(
        ReadOnlySpan<byte> emfData,
        int outputWidth,
        int outputHeight,
        RgbaColor background = default)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(outputWidth);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(outputHeight);
        if (emfData.Length < 88)
            throw new ArgumentException("EMF data too short to contain a valid header.", nameof(emfData));
        if (BinaryPrimitives.ReadUInt32LittleEndian(emfData) != 1u)
            throw new ArgumentException("EMF data must start with an EMR_HEADER record.", nameof(emfData));

        // Header bounds + frame.
        int boundsL = BinaryPrimitives.ReadInt32LittleEndian(emfData[8..]);
        int boundsT = BinaryPrimitives.ReadInt32LittleEndian(emfData[12..]);
        int boundsR = BinaryPrimitives.ReadInt32LittleEndian(emfData[16..]);
        int boundsB = BinaryPrimitives.ReadInt32LittleEndian(emfData[20..]);

        int boundsW = Math.Max(1, boundsR - boundsL);
        int boundsH = Math.Max(1, boundsB - boundsT);

        // Map bounds rect -> output rect (fit, preserve aspect ratio).
        float sx = outputWidth / (float)boundsW;
        float sy = outputHeight / (float)boundsH;
        float scale = MathF.Min(sx, sy);
        float offX = (outputWidth - boundsW * scale) / 2f - boundsL * scale;
        float offY = (outputHeight - boundsH * scale) / 2f - boundsT * scale;
        var deviceFit = new Matrix3x2(scale, 0, 0, scale, offX, offY);

        var target = RasterTarget.Create(outputWidth, outputHeight, background);
        var state = new GdiState();

        int p = 0;
        int recordsRead = 0;
        int unsupported = 0;

        while (p + 8 <= emfData.Length)
        {
            uint type = BinaryPrimitives.ReadUInt32LittleEndian(emfData[p..]);
            uint size = BinaryPrimitives.ReadUInt32LittleEndian(emfData[(p + 4)..]);
            if (size < 8 || p + size > emfData.Length) break;

            ReadOnlySpan<byte> payload = emfData.Slice(p + 8, (int)size - 8);
            if (!Dispatch((EmfRecordType)type, payload, state, target, deviceFit))
                unsupported++;

            recordsRead++;
            p += (int)size;
            if (type == (uint)EmfRecordType.Eof) break;
        }

        // Copy pixels into an ImageFrame.
        int stride = outputWidth * 4;
        byte[] buf = new byte[outputHeight * stride];
        target.Pixels.CopyTo(buf);
        var frame = new ImageFrame(outputWidth, outputHeight, PixelFormat.Bgra32, stride, buf);
        return new EmfPlaybackResult(frame, recordsRead, unsupported);
    }

    private static bool Dispatch(
        EmfRecordType type, ReadOnlySpan<byte> payload,
        GdiState state, RasterTarget target, in Matrix3x2 deviceFit)
    {
        switch (type)
        {
            case EmfRecordType.Header: return true;
            case EmfRecordType.Eof: return true;
            case EmfRecordType.SetWindowOrgEx: HandleSetWindowOrg(payload, state); return true;
            case EmfRecordType.SetWindowExtEx: HandleSetWindowExt(payload, state); return true;
            case EmfRecordType.SetViewportOrgEx: HandleSetViewportOrg(payload, state); return true;
            case EmfRecordType.SetViewportExtEx: HandleSetViewportExt(payload, state); return true;
            case EmfRecordType.SetMapMode: HandleSetMapMode(payload, state); return true;
            case EmfRecordType.SetPolyFillMode: HandleSetPolyFillMode(payload, state); return true;
            case EmfRecordType.SetWorldTransform: HandleSetWorldTransform(payload, state); return true;
            case EmfRecordType.ModifyWorldTransform: HandleModifyWorldTransform(payload, state); return true;
            case EmfRecordType.SaveDc: HandleSaveDc(state); return true;
            case EmfRecordType.RestoreDc: HandleRestoreDc(payload, state); return true;
            case EmfRecordType.CreatePen: HandleCreatePen(payload, state); return true;
            case EmfRecordType.ExtCreatePen: HandleExtCreatePen(payload, state); return true;
            case EmfRecordType.CreateBrushIndirect: HandleCreateBrush(payload, state); return true;
            case EmfRecordType.DeleteObject: HandleDeleteObject(payload, state); return true;
            case EmfRecordType.SelectObject: HandleSelectObject(payload, state); return true;
            case EmfRecordType.MoveToEx: HandleMoveTo(payload, state); return true;
            case EmfRecordType.LineTo: HandleLineTo(payload, state, target, deviceFit); return true;
            case EmfRecordType.SetPixelV: HandleSetPixelV(payload, state, target, deviceFit); return true;
            case EmfRecordType.Rectangle: HandleRectangle(payload, state, target, deviceFit); return true;
            case EmfRecordType.Ellipse: HandleEllipse(payload, state, target, deviceFit); return true;
            case EmfRecordType.RoundRect: HandleRoundRect(payload, state, target, deviceFit); return true;
            case EmfRecordType.Polygon: HandlePolyShape(payload, state, target, deviceFit, points32: true, fill: true, close: true); return true;
            case EmfRecordType.Polygon16: HandlePolyShape(payload, state, target, deviceFit, points32: false, fill: true, close: true); return true;
            case EmfRecordType.Polyline: HandlePolyShape(payload, state, target, deviceFit, points32: true, fill: false, close: false); return true;
            case EmfRecordType.Polyline16: HandlePolyShape(payload, state, target, deviceFit, points32: false, fill: false, close: false); return true;
            case EmfRecordType.PolyBezier: HandlePolyBezier(payload, state, target, deviceFit, points32: true, useCurrent: false); return true;
            case EmfRecordType.PolyBezier16: HandlePolyBezier(payload, state, target, deviceFit, points32: false, useCurrent: false); return true;
            case EmfRecordType.PolyBezierTo: HandlePolyBezier(payload, state, target, deviceFit, points32: true, useCurrent: true); return true;
            case EmfRecordType.PolyBezierTo16: HandlePolyBezier(payload, state, target, deviceFit, points32: false, useCurrent: true); return true;
            case EmfRecordType.PolylineTo: HandlePolylineTo(payload, state, target, deviceFit, points32: true); return true;
            case EmfRecordType.PolylineTo16: HandlePolylineTo(payload, state, target, deviceFit, points32: false); return true;
            case EmfRecordType.PolyPolygon: HandlePolyPolygon(payload, state, target, deviceFit, points32: true); return true;
            case EmfRecordType.PolyPolygon16: HandlePolyPolygon(payload, state, target, deviceFit, points32: false); return true;
            case EmfRecordType.PolyPolyline: HandlePolyPolyline(payload, state, target, deviceFit, points32: true); return true;
            case EmfRecordType.PolyPolyline16: HandlePolyPolyline(payload, state, target, deviceFit, points32: false); return true;
            case EmfRecordType.BeginPath: state.PathBuilder = new Path2D(); state.PathClosed = false; return true;
            case EmfRecordType.EndPath: state.PathClosed = true; return true;
            case EmfRecordType.AbortPath: state.PathBuilder = null; state.PathClosed = false; return true;
            case EmfRecordType.CloseFigure: state.PathBuilder?.Close(); return true;
            case EmfRecordType.FillPath: FlushPath(state, target, deviceFit, fill: true, stroke: false); return true;
            case EmfRecordType.StrokePath: FlushPath(state, target, deviceFit, fill: false, stroke: true); return true;
            case EmfRecordType.StrokeAndFillPath: FlushPath(state, target, deviceFit, fill: true, stroke: true); return true;
            default: return false;
        }
    }

    // -- handlers -----------------------------------------------------------

    private static void HandleSetWindowOrg(ReadOnlySpan<byte> p, GdiState s) =>
        s.WindowOrigin = ReadPointL(p);

    private static void HandleSetWindowExt(ReadOnlySpan<byte> p, GdiState s) =>
        s.WindowExtent = ReadPointL(p);

    private static void HandleSetViewportOrg(ReadOnlySpan<byte> p, GdiState s) =>
        s.ViewportOrigin = ReadPointL(p);

    private static void HandleSetViewportExt(ReadOnlySpan<byte> p, GdiState s) =>
        s.ViewportExtent = ReadPointL(p);

    private static void HandleSetMapMode(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 4) return;
        s.MapMode = (EmfMapMode)BinaryPrimitives.ReadUInt32LittleEndian(p);
    }

    private static void HandleSetPolyFillMode(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 4) return;
        uint mode = BinaryPrimitives.ReadUInt32LittleEndian(p);
        s.PolyFillRule = mode == 2 ? FillRule.NonZero : FillRule.EvenOdd;
    }

    private static void HandleSetWorldTransform(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 24) return;
        s.WorldTransform = ReadXForm(p);
    }

    private static void HandleModifyWorldTransform(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 28) return;
        var arg = ReadXForm(p);
        var mode = (EmfWorldTransformMode)BinaryPrimitives.ReadUInt32LittleEndian(p[24..]);
        s.WorldTransform = mode switch
        {
            EmfWorldTransformMode.Identity => Matrix3x2.Identity,
            EmfWorldTransformMode.LeftMultiply => arg * s.WorldTransform,
            EmfWorldTransformMode.RightMultiply => s.WorldTransform * arg,
            EmfWorldTransformMode.Set => arg,
            _ => s.WorldTransform,
        };
    }

    private static void HandleSaveDc(GdiState s)
    {
        s.SaveCounter++;
        s.Stack.Push(s.Snapshot());
    }

    private static void HandleRestoreDc(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 4) return;
        int nSavedDc = BinaryPrimitives.ReadInt32LittleEndian(p);
        // GDI semantics: negative = relative (pop -nSavedDc snapshots).
        int popCount = nSavedDc < 0 ? -nSavedDc : Math.Max(0, s.Stack.Count - (nSavedDc - 1));
        for (int i = 0; i < popCount && s.Stack.Count > 0; i++)
        {
            var snap = s.Stack.Pop();
            if (i == popCount - 1) s.Restore(snap);
        }
    }

    private static void HandleCreatePen(ReadOnlySpan<byte> p, GdiState s)
    {
        // LogPen32 = uint handle + uint penStyle + (int32 width, int32 widthY) + ColorRef
        if (p.Length < 24) return;
        uint handle = BinaryPrimitives.ReadUInt32LittleEndian(p);
        uint style = BinaryPrimitives.ReadUInt32LittleEndian(p[4..]);
        int width = BinaryPrimitives.ReadInt32LittleEndian(p[8..]);
        uint colorRef = BinaryPrimitives.ReadUInt32LittleEndian(p[16..]);
        var pen = new GdiPen(NormalisePenStyle(style), Math.Max(0, width), GdiCoords.DecodeColorRef(colorRef));
        s.Objects[handle] = pen;
    }

    private static void HandleExtCreatePen(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 28) return;
        uint handle = BinaryPrimitives.ReadUInt32LittleEndian(p);
        // Skip BMI + Brush offsets/lengths to reach LogPenEx at offset 20.
        uint penStyle = BinaryPrimitives.ReadUInt32LittleEndian(p[20..]);
        uint penWidth = BinaryPrimitives.ReadUInt32LittleEndian(p[24..]);
        // Brush style + ColorRef live at +28 / +32 if payload is long enough.
        RgbaColor color = RgbaColor.Black;
        if (p.Length >= 36)
        {
            uint colorRef = BinaryPrimitives.ReadUInt32LittleEndian(p[32..]);
            color = GdiCoords.DecodeColorRef(colorRef);
        }
        // Parse the joinable cap/join bits if present.
        uint endCap = (penStyle >> 8) & 0xF;
        uint join = (penStyle >> 12) & 0xF;
        LineCap cap = endCap switch { 1 => LineCap.Square, 2 => LineCap.Round, _ => LineCap.Butt };
        LineJoin lj = join switch { 1 => LineJoin.Bevel, 2 => LineJoin.Miter, _ => LineJoin.Round };
        var pen = new GdiPen(NormalisePenStyle(penStyle & 0xFF), penWidth, color, cap, lj);
        s.Objects[handle] = pen;
    }

    private static void HandleCreateBrush(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 16) return;
        uint handle = BinaryPrimitives.ReadUInt32LittleEndian(p);
        uint style = BinaryPrimitives.ReadUInt32LittleEndian(p[4..]);
        uint colorRef = BinaryPrimitives.ReadUInt32LittleEndian(p[8..]);
        var brush = new GdiBrush((GdiBrushStyle)style, GdiCoords.DecodeColorRef(colorRef));
        s.Objects[handle] = brush;
    }

    private static void HandleDeleteObject(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 4) return;
        uint h = BinaryPrimitives.ReadUInt32LittleEndian(p);
        s.Objects.Remove(h);
    }

    private static void HandleSelectObject(ReadOnlySpan<byte> p, GdiState s)
    {
        if (p.Length < 4) return;
        uint h = BinaryPrimitives.ReadUInt32LittleEndian(p);
        // Stock objects have the high bit set (0x80000000); we mostly ignore them.
        if ((h & 0x80000000u) != 0)
        {
            uint stock = h & 0x7FFFFFFFu;
            switch (stock)
            {
                case 0: s.CurrentBrush = new GdiBrush(GdiBrushStyle.Solid, RgbaColor.White); break;     // WHITE_BRUSH
                case 1: s.CurrentBrush = new GdiBrush(GdiBrushStyle.Solid, RgbaColor.FromBytes(192, 192, 192)); break; // LTGRAY_BRUSH
                case 2: s.CurrentBrush = new GdiBrush(GdiBrushStyle.Solid, RgbaColor.FromBytes(128, 128, 128)); break; // GRAY_BRUSH
                case 3: s.CurrentBrush = new GdiBrush(GdiBrushStyle.Solid, RgbaColor.FromBytes(64, 64, 64)); break;    // DKGRAY_BRUSH
                case 4: s.CurrentBrush = new GdiBrush(GdiBrushStyle.Solid, RgbaColor.Black); break;     // BLACK_BRUSH
                case 5: s.CurrentBrush = new GdiBrush(GdiBrushStyle.Null, RgbaColor.Transparent); break;// NULL_BRUSH / HOLLOW_BRUSH
                case 6: s.CurrentPen = new GdiPen(GdiPenStyle.Solid, 1f, RgbaColor.White); break;       // WHITE_PEN
                case 7: s.CurrentPen = new GdiPen(GdiPenStyle.Solid, 1f, RgbaColor.Black); break;       // BLACK_PEN
                case 8: s.CurrentPen = new GdiPen(GdiPenStyle.Null, 0, RgbaColor.Transparent); break;   // NULL_PEN
            }
            return;
        }
        if (s.Objects.TryGetValue(h, out var obj))
        {
            if (obj is GdiPen pen) s.CurrentPen = pen;
            else if (obj is GdiBrush brush) s.CurrentBrush = brush;
        }
    }

    private static void HandleMoveTo(ReadOnlySpan<byte> p, GdiState s)
    {
        var pt = ReadPointL(p);
        s.CurrentPoint = pt;
        s.PathBuilder?.MoveTo(pt.X, pt.Y);
    }

    private static void HandleLineTo(ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        var pt = ReadPointL(p);
        if (s.PathBuilder is { } pb)
        {
            pb.LineTo(pt.X, pt.Y);
        }
        else
        {
            var path = new Path2D();
            path.MoveTo(s.CurrentPoint.X, s.CurrentPoint.Y);
            path.LineTo(pt.X, pt.Y);
            StrokeImmediate(path, s, target, fit);
        }
        s.CurrentPoint = pt;
    }

    private static void HandleSetPixelV(ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (p.Length < 12) return;
        var pt = ReadPointL(p);
        uint colorRef = BinaryPrimitives.ReadUInt32LittleEndian(p[8..]);
        var color = GdiCoords.DecodeColorRef(colorRef);
        // Emit a 1x1 rectangle in logical coordinates.
        var path = new Path2D();
        path.MoveTo(pt.X, pt.Y);
        path.LineTo(pt.X + 1, pt.Y);
        path.LineTo(pt.X + 1, pt.Y + 1);
        path.LineTo(pt.X, pt.Y + 1);
        path.Close();
        FillPath(path, color, s, target, fit);
    }

    private static void HandleRectangle(ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (p.Length < 16) return;
        var (l, t, r, b) = ReadRectL(p);
        var path = new Path2D();
        path.MoveTo(l, t).LineTo(r, t).LineTo(r, b).LineTo(l, b).Close();
        ExecuteDraw(path, s, target, fit);
    }

    private static void HandleEllipse(ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (p.Length < 16) return;
        var (l, t, r, b) = ReadRectL(p);
        float cx = (l + r) / 2f, cy = (t + b) / 2f;
        float rx = (r - l) / 2f, ry = (b - t) / 2f;
        var path = new Path2D();
        path.MoveTo(cx + rx, cy);
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx, cy + ry));
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx - rx, cy));
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx, cy - ry));
        path.ArcTo(rx, ry, 0, false, true, new Vector2(cx + rx, cy));
        path.Close();
        ExecuteDraw(path, s, target, fit);
    }

    private static void HandleRoundRect(ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (p.Length < 24) return;
        var (l, t, r, b) = ReadRectL(p);
        int cw = BinaryPrimitives.ReadInt32LittleEndian(p[16..]);
        int ch = BinaryPrimitives.ReadInt32LittleEndian(p[20..]);
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
        ExecuteDraw(path, s, target, fit);
    }

    private static void HandlePolyShape(
        ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit,
        bool points32, bool fill, bool close)
    {
        // Layout: RectL bounds (16) + uint count + points.
        if (p.Length < 20) return;
        int n = (int)BinaryPrimitives.ReadUInt32LittleEndian(p[16..]);
        if (n <= 0) return;
        int ptSize = points32 ? 8 : 4;
        if (p.Length < 20 + n * ptSize) return;

        var path = new Path2D();
        for (int i = 0; i < n; i++)
        {
            var pt = points32
                ? ReadPointL(p[(20 + i * 8)..])
                : ReadPoint16(p[(20 + i * 4)..]);
            if (i == 0) path.MoveTo(pt.X, pt.Y);
            else path.LineTo(pt.X, pt.Y);
        }
        if (close) path.Close();
        if (fill)
            ExecuteDraw(path, s, target, fit);
        else
            StrokeImmediate(path, s, target, fit);
    }

    private static void HandlePolyBezier(
        ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit,
        bool points32, bool useCurrent)
    {
        // PolyBezier: 1 + 3k points (start + groups of 3). PolyBezierTo: 3k (uses current).
        if (p.Length < 20) return;
        int n = (int)BinaryPrimitives.ReadUInt32LittleEndian(p[16..]);
        if (n <= 0) return;
        int ptSize = points32 ? 8 : 4;
        if (p.Length < 20 + n * ptSize) return;

        Vector2 ReadIdx(ReadOnlySpan<byte> data, int idx)
        {
            return points32 ? ReadPointL(data[(20 + idx * 8)..]) : ReadPoint16(data[(20 + idx * 4)..]);
        }

        var path = new Path2D();
        Vector2 cur;
        int startIdx;
        if (useCurrent)
        {
            cur = s.CurrentPoint;
            path.MoveTo(cur.X, cur.Y);
            startIdx = 0;
        }
        else
        {
            cur = ReadIdx(p, 0);
            path.MoveTo(cur.X, cur.Y);
            startIdx = 1;
        }

        // We need (n - startIdx) to be a positive multiple of 3 for a well-formed record.
        int remaining = n - startIdx;
        int groups = remaining / 3;
        for (int g = 0; g < groups; g++)
        {
            int baseIdx = startIdx + g * 3;
            Vector2 c1 = ReadIdx(p, baseIdx);
            Vector2 c2 = ReadIdx(p, baseIdx + 1);
            Vector2 ep = ReadIdx(p, baseIdx + 2);
            path.CubicTo(c1, c2, ep);
            cur = ep;
        }
        s.CurrentPoint = cur;

        if (s.PathBuilder is { } pb) pb.Append(path);
        else StrokeImmediate(path, s, target, fit);
    }

    private static void HandlePolylineTo(
        ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit, bool points32)
    {
        if (p.Length < 20) return;
        int n = (int)BinaryPrimitives.ReadUInt32LittleEndian(p[16..]);
        if (n <= 0) return;
        int ptSize = points32 ? 8 : 4;
        if (p.Length < 20 + n * ptSize) return;

        var path = new Path2D();
        path.MoveTo(s.CurrentPoint.X, s.CurrentPoint.Y);
        Vector2 cur = s.CurrentPoint;
        for (int i = 0; i < n; i++)
        {
            cur = points32 ? ReadPointL(p[(20 + i * 8)..]) : ReadPoint16(p[(20 + i * 4)..]);
            path.LineTo(cur.X, cur.Y);
        }
        s.CurrentPoint = cur;
        if (s.PathBuilder is { } pb) pb.Append(path);
        else StrokeImmediate(path, s, target, fit);
    }

    private static void HandlePolyPolygon(
        ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit, bool points32) =>
        HandlePolyPolyImpl(p, s, target, fit, points32, fillEach: true, closeEach: true);

    private static void HandlePolyPolyline(
        ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit, bool points32) =>
        HandlePolyPolyImpl(p, s, target, fit, points32, fillEach: false, closeEach: false);

    private static void HandlePolyPolyImpl(
        ReadOnlySpan<byte> p, GdiState s, RasterTarget target, in Matrix3x2 fit,
        bool points32, bool fillEach, bool closeEach)
    {
        // Layout: RectL bounds (16) + uint nPolys + uint totalCount + nPolys * uint counts + points.
        if (p.Length < 24) return;
        int nPolys = (int)BinaryPrimitives.ReadUInt32LittleEndian(p[16..]);
        int totalPts = (int)BinaryPrimitives.ReadUInt32LittleEndian(p[20..]);
        if (nPolys <= 0 || totalPts <= 0) return;
        int countsOff = 24;
        int pointsOff = countsOff + nPolys * 4;
        int ptSize = points32 ? 8 : 4;
        if (p.Length < pointsOff + totalPts * ptSize) return;

        var path = new Path2D();
        int ptIdx = 0;
        for (int poly = 0; poly < nPolys; poly++)
        {
            int n = (int)BinaryPrimitives.ReadUInt32LittleEndian(p[(countsOff + poly * 4)..]);
            for (int i = 0; i < n; i++, ptIdx++)
            {
                var pt = points32
                    ? ReadPointL(p[(pointsOff + ptIdx * 8)..])
                    : ReadPoint16(p[(pointsOff + ptIdx * 4)..]);
                if (i == 0) path.MoveTo(pt.X, pt.Y);
                else path.LineTo(pt.X, pt.Y);
            }
            if (closeEach) path.Close();
        }

        if (fillEach) ExecuteDraw(path, s, target, fit);
        else StrokeImmediate(path, s, target, fit);
    }

    // -- drawing helpers ----------------------------------------------------

    /// <summary>Draw a shape with the current pen + brush (no path bracketing).</summary>
    private static void ExecuteDraw(Path2D path, GdiState s, RasterTarget target, in Matrix3x2 fit)
    {
        if (s.PathBuilder is { } pb)
        {
            pb.Append(path);
            return;
        }
        if (!s.CurrentBrush.IsNullBrush)
            FillPath(path, s.CurrentBrush.Color, s, target, fit);
        if (!s.CurrentPen.IsNullPen)
            StrokeImmediate(path, s, target, fit);
    }

    private static void FlushPath(GdiState s, RasterTarget target, in Matrix3x2 fit, bool fill, bool stroke)
    {
        if (s.PathBuilder is not { } path) return;
        if (fill && !s.CurrentBrush.IsNullBrush)
            FillPath(path, s.CurrentBrush.Color, s, target, fit);
        if (stroke && !s.CurrentPen.IsNullPen)
            StrokeImmediate(path, s, target, fit);
        s.PathBuilder = null;
        s.PathClosed = false;
    }

    private static void FillPath(Path2D path, RgbaColor color, GdiState s, RasterTarget target, in Matrix3x2 fit)
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
        // logical -> world: WorldTransform
        // world -> device: (pt - WindowOrg) * (ViewportExt / WindowExt) + ViewportOrg
        float kx = s.WindowExtent.X == 0 ? 1f : s.ViewportExtent.X / s.WindowExtent.X;
        float ky = s.WindowExtent.Y == 0 ? 1f : s.ViewportExtent.Y / s.WindowExtent.Y;
        var win2vp = new Matrix3x2(
            kx, 0,
            0, ky,
            s.ViewportOrigin.X - s.WindowOrigin.X * kx,
            s.ViewportOrigin.Y - s.WindowOrigin.Y * ky);
        // Apply world transform first, then window-to-viewport, then fit-to-output.
        return s.WorldTransform * win2vp * deviceFit;
    }

    // -- low-level decoders -------------------------------------------------

    private static Vector2 ReadPointL(ReadOnlySpan<byte> p) =>
        new(BinaryPrimitives.ReadInt32LittleEndian(p),
            BinaryPrimitives.ReadInt32LittleEndian(p[4..]));

    private static Vector2 ReadPoint16(ReadOnlySpan<byte> p) =>
        new(BinaryPrimitives.ReadInt16LittleEndian(p),
            BinaryPrimitives.ReadInt16LittleEndian(p[2..]));

    private static (int L, int T, int R, int B) ReadRectL(ReadOnlySpan<byte> p) => (
        BinaryPrimitives.ReadInt32LittleEndian(p),
        BinaryPrimitives.ReadInt32LittleEndian(p[4..]),
        BinaryPrimitives.ReadInt32LittleEndian(p[8..]),
        BinaryPrimitives.ReadInt32LittleEndian(p[12..]));

    private static Matrix3x2 ReadXForm(ReadOnlySpan<byte> p) => new(
        BitConverter.Int32BitsToSingle(BinaryPrimitives.ReadInt32LittleEndian(p)),
        BitConverter.Int32BitsToSingle(BinaryPrimitives.ReadInt32LittleEndian(p[4..])),
        BitConverter.Int32BitsToSingle(BinaryPrimitives.ReadInt32LittleEndian(p[8..])),
        BitConverter.Int32BitsToSingle(BinaryPrimitives.ReadInt32LittleEndian(p[12..])),
        BitConverter.Int32BitsToSingle(BinaryPrimitives.ReadInt32LittleEndian(p[16..])),
        BitConverter.Int32BitsToSingle(BinaryPrimitives.ReadInt32LittleEndian(p[20..])));

    private static GdiPenStyle NormalisePenStyle(uint style) => style switch
    {
        0 => GdiPenStyle.Solid,
        1 => GdiPenStyle.Dash,
        2 => GdiPenStyle.Dot,
        3 => GdiPenStyle.DashDot,
        4 => GdiPenStyle.DashDotDot,
        5 => GdiPenStyle.Null,
        6 => GdiPenStyle.InsideFrame,
        7 => GdiPenStyle.UserStyle,
        8 => GdiPenStyle.Alternate,
        _ => GdiPenStyle.Solid,
    };
}

/// <summary>
/// Outcome of an <see cref="EmfPlayer.Render"/> call.
/// </summary>
public sealed record EmfPlaybackResult(
    ImageFrame Frame,
    int RecordsRead,
    int UnsupportedRecordCount);
