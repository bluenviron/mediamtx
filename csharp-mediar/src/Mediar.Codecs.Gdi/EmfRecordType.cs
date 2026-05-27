namespace Mediar.Codecs.Gdi;

#pragma warning disable CA1711 // EMF record names mirror the MS-EMF spec (e.g. EMR_SETWINDOWEXTEX); preserved verbatim.

/// <summary>
/// MS-EMF record-type codes (2.1.1). Only the ones the player currently
/// interprets are named; everything else falls through the dispatcher and
/// gets accumulated in the unsupported-record counter on the result.
/// </summary>
public enum EmfRecordType
{
    /// <summary>EMR_HEADER.</summary>
    Header = 1,
    /// <summary>EMR_POLYBEZIER.</summary>
    PolyBezier = 2,
    /// <summary>EMR_POLYGON.</summary>
    Polygon = 3,
    /// <summary>EMR_POLYLINE.</summary>
    Polyline = 4,
    /// <summary>EMR_POLYBEZIERTO.</summary>
    PolyBezierTo = 5,
    /// <summary>EMR_POLYLINETO.</summary>
    PolylineTo = 6,
    /// <summary>EMR_POLYPOLYLINE.</summary>
    PolyPolyline = 7,
    /// <summary>EMR_POLYPOLYGON.</summary>
    PolyPolygon = 8,
    /// <summary>EMR_SETWINDOWEXTEX.</summary>
    SetWindowExtEx = 9,
    /// <summary>EMR_SETWINDOWORGEX.</summary>
    SetWindowOrgEx = 10,
    /// <summary>EMR_SETVIEWPORTEXTEX.</summary>
    SetViewportExtEx = 11,
    /// <summary>EMR_SETVIEWPORTORGEX.</summary>
    SetViewportOrgEx = 12,
    /// <summary>EMR_EOF.</summary>
    Eof = 14,
    /// <summary>EMR_SETPIXELV.</summary>
    SetPixelV = 15,
    /// <summary>EMR_SETMAPMODE.</summary>
    SetMapMode = 17,
    /// <summary>EMR_SETBKMODE.</summary>
    SetBkMode = 18,
    /// <summary>EMR_SETPOLYFILLMODE.</summary>
    SetPolyFillMode = 19,
    /// <summary>EMR_SETROP2.</summary>
    SetRop2 = 20,
    /// <summary>EMR_SETSTRETCHBLTMODE.</summary>
    SetStretchBltMode = 21,
    /// <summary>EMR_SETTEXTALIGN.</summary>
    SetTextAlign = 22,
    /// <summary>EMR_SETTEXTCOLOR.</summary>
    SetTextColor = 24,
    /// <summary>EMR_SETBKCOLOR.</summary>
    SetBkColor = 25,
    /// <summary>EMR_MOVETOEX.</summary>
    MoveToEx = 27,
    /// <summary>EMR_INTERSECTCLIPRECT.</summary>
    IntersectClipRect = 30,
    /// <summary>EMR_SAVEDC.</summary>
    SaveDc = 33,
    /// <summary>EMR_RESTOREDC.</summary>
    RestoreDc = 34,
    /// <summary>EMR_SETWORLDTRANSFORM.</summary>
    SetWorldTransform = 35,
    /// <summary>EMR_MODIFYWORLDTRANSFORM.</summary>
    ModifyWorldTransform = 36,
    /// <summary>EMR_SELECTOBJECT.</summary>
    SelectObject = 37,
    /// <summary>EMR_CREATEPEN.</summary>
    CreatePen = 38,
    /// <summary>EMR_CREATEBRUSHINDIRECT.</summary>
    CreateBrushIndirect = 39,
    /// <summary>EMR_DELETEOBJECT.</summary>
    DeleteObject = 40,
    /// <summary>EMR_ELLIPSE.</summary>
    Ellipse = 42,
    /// <summary>EMR_RECTANGLE.</summary>
    Rectangle = 43,
    /// <summary>EMR_ROUNDRECT.</summary>
    RoundRect = 44,
    /// <summary>EMR_ARC.</summary>
    Arc = 45,
    /// <summary>EMR_CHORD.</summary>
    Chord = 46,
    /// <summary>EMR_PIE.</summary>
    Pie = 47,
    /// <summary>EMR_LINETO.</summary>
    LineTo = 54,
    /// <summary>EMR_BEGINPATH.</summary>
    BeginPath = 59,
    /// <summary>EMR_ENDPATH.</summary>
    EndPath = 60,
    /// <summary>EMR_CLOSEFIGURE.</summary>
    CloseFigure = 61,
    /// <summary>EMR_FILLPATH.</summary>
    FillPath = 62,
    /// <summary>EMR_STROKEANDFILLPATH.</summary>
    StrokeAndFillPath = 63,
    /// <summary>EMR_STROKEPATH.</summary>
    StrokePath = 64,
    /// <summary>EMR_ABORTPATH.</summary>
    AbortPath = 68,
    /// <summary>EMR_POLYBEZIER16.</summary>
    PolyBezier16 = 85,
    /// <summary>EMR_POLYGON16.</summary>
    Polygon16 = 86,
    /// <summary>EMR_POLYLINE16.</summary>
    Polyline16 = 87,
    /// <summary>EMR_POLYBEZIERTO16.</summary>
    PolyBezierTo16 = 88,
    /// <summary>EMR_POLYLINETO16.</summary>
    PolylineTo16 = 89,
    /// <summary>EMR_POLYPOLYLINE16.</summary>
    PolyPolyline16 = 90,
    /// <summary>EMR_POLYPOLYGON16.</summary>
    PolyPolygon16 = 91,
    /// <summary>EMR_EXTCREATEPEN.</summary>
    ExtCreatePen = 95,
}

/// <summary>
/// MS-EMF map modes (2.1.21). Default = MM_TEXT.
/// </summary>
public enum EmfMapMode
{
    /// <summary>1 logical unit = 1 device pixel; +y down.</summary>
    Text = 1,
    /// <summary>0.1 mm; +y up.</summary>
    LoMetric = 2,
    /// <summary>0.01 mm; +y up.</summary>
    HiMetric = 3,
    /// <summary>0.01 inch; +y up.</summary>
    LoEnglish = 4,
    /// <summary>0.001 inch; +y up.</summary>
    HiEnglish = 5,
    /// <summary>1/1440 inch (twip); +y up.</summary>
    Twips = 6,
    /// <summary>Caller-defined, isotropic scaling enforced.</summary>
    Isotropic = 7,
    /// <summary>Caller-defined window + viewport, no isotropy.</summary>
    Anisotropic = 8,
}

/// <summary>
/// MS-EMF <c>ModifyWorldTransformMode</c> (2.1.24).
/// </summary>
public enum EmfWorldTransformMode
{
    /// <summary>MWT_IDENTITY - reset to identity (the XForm argument is ignored).</summary>
    Identity = 1,
    /// <summary>MWT_LEFTMULTIPLY - newWorld = arg * world.</summary>
    LeftMultiply = 2,
    /// <summary>MWT_RIGHTMULTIPLY - newWorld = world * arg.</summary>
    RightMultiply = 3,
    /// <summary>MWT_SET - newWorld = arg.</summary>
    Set = 4,
}
