using System.Numerics;
using Mediar.Vector;

namespace Mediar.Codecs.Gdi;

/// <summary>
/// Mutable per-DC state evolved by the GDI playback engine. A fresh
/// instance carries the stock pen + brush, identity world transform,
/// 1:1 window / viewport, and an empty object table.
/// </summary>
public sealed class GdiState
{
    /// <summary>The object table (handle index -> object).</summary>
    public Dictionary<uint, GdiObject> Objects { get; } = [];

    /// <summary>Currently selected pen.</summary>
    public GdiPen CurrentPen { get; set; } = GdiPen.Default;

    /// <summary>Currently selected brush.</summary>
    public GdiBrush CurrentBrush { get; set; } = GdiBrush.Default;

    /// <summary>The MS-EMF world transform (applied to logical coordinates).</summary>
    public Matrix3x2 WorldTransform { get; set; } = Matrix3x2.Identity;

    /// <summary>Current logical point (updated by MoveTo / LineTo / PolylineTo / ...).</summary>
    public Vector2 CurrentPoint { get; set; }

    /// <summary>Window origin.</summary>
    public Vector2 WindowOrigin { get; set; } = Vector2.Zero;

    /// <summary>Window extents (1 means "logical unit = device unit" in that axis).</summary>
    public Vector2 WindowExtent { get; set; } = Vector2.One;

    /// <summary>Viewport origin.</summary>
    public Vector2 ViewportOrigin { get; set; } = Vector2.Zero;

    /// <summary>Viewport extents.</summary>
    public Vector2 ViewportExtent { get; set; } = Vector2.One;

    /// <summary>Map mode (default MM_TEXT).</summary>
    public EmfMapMode MapMode { get; set; } = EmfMapMode.Text;

    /// <summary>Polygon fill rule (ALTERNATE = EvenOdd, WINDING = NonZero).</summary>
    public FillRule PolyFillRule { get; set; } = FillRule.EvenOdd;

    /// <summary>The path currently being built between BeginPath / EndPath, or null.</summary>
    public Path2D? PathBuilder { get; set; }

    /// <summary>True once EndPath has been seen and the path is ready for FillPath / StrokePath.</summary>
    public bool PathClosed { get; set; }

    /// <summary>Snapshot stack for SAVEDC / RESTOREDC.</summary>
    public Stack<GdiStateSnapshot> Stack { get; } = new();

    /// <summary>Saved-DC counter (positive monotonic id assigned to each SAVEDC).</summary>
    public int SaveCounter { get; set; }

    /// <summary>Take a snapshot suitable for stacking onto <see cref="Stack"/>.</summary>
    public GdiStateSnapshot Snapshot() => new(
        CurrentPen, CurrentBrush, WorldTransform, CurrentPoint,
        WindowOrigin, WindowExtent, ViewportOrigin, ViewportExtent,
        MapMode, PolyFillRule);

    /// <summary>Restore from a snapshot (PathBuilder is left untouched).</summary>
    public void Restore(GdiStateSnapshot snap)
    {
        CurrentPen = snap.Pen;
        CurrentBrush = snap.Brush;
        WorldTransform = snap.WorldTransform;
        CurrentPoint = snap.CurrentPoint;
        WindowOrigin = snap.WindowOrigin;
        WindowExtent = snap.WindowExtent;
        ViewportOrigin = snap.ViewportOrigin;
        ViewportExtent = snap.ViewportExtent;
        MapMode = snap.MapMode;
        PolyFillRule = snap.PolyFillRule;
    }
}

/// <summary>
/// Snapshot of a <see cref="GdiState"/> usable from SAVEDC / RESTOREDC.
/// </summary>
public readonly record struct GdiStateSnapshot(
    GdiPen Pen,
    GdiBrush Brush,
    Matrix3x2 WorldTransform,
    Vector2 CurrentPoint,
    Vector2 WindowOrigin,
    Vector2 WindowExtent,
    Vector2 ViewportOrigin,
    Vector2 ViewportExtent,
    EmfMapMode MapMode,
    FillRule PolyFillRule);

/// <summary>
/// Helpers to translate GDI colour references / coordinates into
/// Mediar.Vector primitives.
/// </summary>
public static class GdiCoords
{
    /// <summary>Decode a 32-bit ColorRef (0x00BBGGRR) into an opaque RgbaColor.</summary>
    public static RgbaColor DecodeColorRef(uint colorRef)
    {
        byte r = (byte)(colorRef & 0xFF);
        byte g = (byte)((colorRef >> 8) & 0xFF);
        byte b = (byte)((colorRef >> 16) & 0xFF);
        return RgbaColor.FromBytes(r, g, b);
    }
}
