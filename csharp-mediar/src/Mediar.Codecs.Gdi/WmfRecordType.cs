namespace Mediar.Codecs.Gdi;

/// <summary>
/// MS-WMF record-function codes (2.1.1.1). Each "function" is a 16-bit
/// value; the high byte encodes the parameter count, the low byte the
/// record class. Only the ones the player currently interprets are named.
/// </summary>
public enum WmfRecordType
{
    /// <summary>META_EOF.</summary>
    Eof = 0x0000,
    /// <summary>META_SETBKCOLOR.</summary>
    SetBkColor = 0x0201,
    /// <summary>META_SETBKMODE.</summary>
    SetBkMode = 0x0102,
    /// <summary>META_SETMAPMODE.</summary>
    SetMapMode = 0x0103,
    /// <summary>META_SETROP2.</summary>
    SetRop2 = 0x0104,
    /// <summary>META_SETPOLYFILLMODE.</summary>
    SetPolyFillMode = 0x0106,
    /// <summary>META_SETSTRETCHBLTMODE.</summary>
    SetStretchBltMode = 0x0107,
    /// <summary>META_SETTEXTCOLOR.</summary>
    SetTextColor = 0x0209,
    /// <summary>META_SETWINDOWORG.</summary>
    SetWindowOrg = 0x020B,
    /// <summary>META_SETWINDOWEXT.</summary>
    SetWindowExt = 0x020C,
    /// <summary>META_SETVIEWPORTORG.</summary>
    SetViewportOrg = 0x020D,
    /// <summary>META_SETVIEWPORTEXT.</summary>
    SetViewportExt = 0x020E,
    /// <summary>META_OFFSETWINDOWORG.</summary>
    OffsetWindowOrg = 0x020F,
    /// <summary>META_SCALEWINDOWEXT.</summary>
    ScaleWindowExt = 0x0410,
    /// <summary>META_OFFSETVIEWPORTORG.</summary>
    OffsetViewportOrg = 0x0211,
    /// <summary>META_SCALEVIEWPORTEXT.</summary>
    ScaleViewportExt = 0x0412,
    /// <summary>META_LINETO.</summary>
    LineTo = 0x0213,
    /// <summary>META_MOVETO.</summary>
    MoveTo = 0x0214,
    /// <summary>META_EXCLUDECLIPRECT.</summary>
    ExcludeClipRect = 0x0415,
    /// <summary>META_INTERSECTCLIPRECT.</summary>
    IntersectClipRect = 0x0416,
    /// <summary>META_SAVEDC.</summary>
    SaveDc = 0x001E,
    /// <summary>META_RESTOREDC.</summary>
    RestoreDc = 0x0127,
    /// <summary>META_SELECTOBJECT.</summary>
    SelectObject = 0x012D,
    /// <summary>META_DELETEOBJECT.</summary>
    DeleteObject = 0x01F0,
    /// <summary>META_ELLIPSE.</summary>
    Ellipse = 0x0418,
    /// <summary>META_RECTANGLE.</summary>
    Rectangle = 0x041B,
    /// <summary>META_ROUNDRECT.</summary>
    RoundRect = 0x061C,
    /// <summary>META_POLYGON.</summary>
    Polygon = 0x0324,
    /// <summary>META_POLYLINE.</summary>
    Polyline = 0x0325,
    /// <summary>META_POLYPOLYGON.</summary>
    PolyPolygon = 0x0538,
    /// <summary>META_SETPIXEL.</summary>
    SetPixel = 0x041F,
    /// <summary>META_ARC.</summary>
    Arc = 0x0817,
    /// <summary>META_CHORD.</summary>
    Chord = 0x0830,
    /// <summary>META_PIE.</summary>
    Pie = 0x081A,
    /// <summary>META_CREATEPENINDIRECT.</summary>
    CreatePenIndirect = 0x02FA,
    /// <summary>META_CREATEBRUSHINDIRECT.</summary>
    CreateBrushIndirect = 0x02FC,
}
