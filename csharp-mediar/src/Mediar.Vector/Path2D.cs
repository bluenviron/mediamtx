using System.Numerics;

namespace Mediar.Vector;

/// <summary>
/// Individual path segment recorded by <see cref="Path2D"/>. Endpoints
/// are post-transformed into device space lazily by the renderer.
/// </summary>
public enum PathVerb
{
    /// <summary>Begin a new sub-path at the given point.</summary>
    MoveTo,
    /// <summary>Line from current point to target.</summary>
    LineTo,
    /// <summary>Quadratic Bezier (one control point + endpoint).</summary>
    QuadTo,
    /// <summary>Cubic Bezier (two control points + endpoint).</summary>
    CubicTo,
    /// <summary>Close current sub-path back to its starting point.</summary>
    Close,
}

/// <summary>
/// One operation in a <see cref="Path2D"/>. Up to three control points
/// are stored inline (LineTo uses one, QuadTo two, CubicTo three).
/// </summary>
public readonly record struct PathSegment(
    PathVerb Verb, Vector2 P0, Vector2 P1 = default, Vector2 P2 = default);

/// <summary>
/// Mutable 2D path builder. Holds a flat list of <see cref="PathSegment"/>
/// records so the rasterizer can stream over them once without
/// allocation. Arc operations are converted on the fly to cubic Beziers.
/// </summary>
public sealed class Path2D
{
    private readonly List<PathSegment> _segments = [];
    private Vector2 _current;
    private Vector2 _subPathStart;
    private Vector2 _lastQuadControl;
    private Vector2 _lastCubicControl;
    private PathVerb _lastVerb;
    private bool _hasCurrent;

    /// <summary>The recorded segments, in order.</summary>
    public IReadOnlyList<PathSegment> Segments => _segments;

    /// <summary>True if no segments have been added.</summary>
    public bool IsEmpty => _segments.Count == 0;

    /// <summary>Begin a new sub-path at (<paramref name="x"/>, <paramref name="y"/>).</summary>
    public Path2D MoveTo(float x, float y) => MoveTo(new Vector2(x, y));

    /// <summary>Begin a new sub-path at <paramref name="p"/>.</summary>
    public Path2D MoveTo(Vector2 p)
    {
        _segments.Add(new PathSegment(PathVerb.MoveTo, p));
        _current = p;
        _subPathStart = p;
        _hasCurrent = true;
        _lastVerb = PathVerb.MoveTo;
        return this;
    }

    /// <summary>Straight line from the current point to (<paramref name="x"/>, <paramref name="y"/>).</summary>
    public Path2D LineTo(float x, float y) => LineTo(new Vector2(x, y));

    /// <summary>Straight line from the current point to <paramref name="p"/>.</summary>
    public Path2D LineTo(Vector2 p)
    {
        EnsureCurrent();
        _segments.Add(new PathSegment(PathVerb.LineTo, p));
        _current = p;
        _lastVerb = PathVerb.LineTo;
        return this;
    }

    /// <summary>Quadratic Bezier with control <paramref name="c"/> and endpoint <paramref name="p"/>.</summary>
    public Path2D QuadTo(Vector2 c, Vector2 p)
    {
        EnsureCurrent();
        _segments.Add(new PathSegment(PathVerb.QuadTo, c, p));
        _current = p;
        _lastQuadControl = c;
        _lastVerb = PathVerb.QuadTo;
        return this;
    }

    /// <summary>Cubic Bezier with controls <paramref name="c1"/> + <paramref name="c2"/> and endpoint <paramref name="p"/>.</summary>
    public Path2D CubicTo(Vector2 c1, Vector2 c2, Vector2 p)
    {
        EnsureCurrent();
        _segments.Add(new PathSegment(PathVerb.CubicTo, c1, c2, p));
        _current = p;
        _lastCubicControl = c2;
        _lastVerb = PathVerb.CubicTo;
        return this;
    }

    /// <summary>SVG "S" command — cubic with first control reflected from previous cubic.</summary>
    public Path2D SmoothCubicTo(Vector2 c2, Vector2 p)
    {
        EnsureCurrent();
        Vector2 c1 = _lastVerb == PathVerb.CubicTo
            ? 2 * _current - _lastCubicControl
            : _current;
        return CubicTo(c1, c2, p);
    }

    /// <summary>SVG "T" command — quadratic with control reflected from previous quad.</summary>
    public Path2D SmoothQuadTo(Vector2 p)
    {
        EnsureCurrent();
        Vector2 c = _lastVerb == PathVerb.QuadTo
            ? 2 * _current - _lastQuadControl
            : _current;
        return QuadTo(c, p);
    }

    /// <summary>
    /// SVG "A" elliptical arc command. Converts the (potentially rotated)
    /// elliptical arc into a sequence of cubic Beziers using the standard
    /// centre-parametric decomposition (max 90° per segment).
    /// </summary>
    public Path2D ArcTo(
        float rx, float ry, float xAxisRotationDeg,
        bool largeArc, bool sweep, Vector2 endpoint)
    {
        EnsureCurrent();
        Arc.AppendEllipticalArc(this, _current, rx, ry, xAxisRotationDeg, largeArc, sweep, endpoint);
        _current = endpoint;
        return this;
    }

    /// <summary>Close the current sub-path (back to last MoveTo).</summary>
    public Path2D Close()
    {
        if (_segments.Count == 0) return this;
        _segments.Add(new PathSegment(PathVerb.Close, _subPathStart));
        _current = _subPathStart;
        _lastVerb = PathVerb.Close;
        return this;
    }

    /// <summary>Append every segment from <paramref name="other"/>.</summary>
    public Path2D Append(Path2D other)
    {
        ArgumentNullException.ThrowIfNull(other);
        foreach (var s in other._segments) _segments.Add(s);
        if (other._hasCurrent) { _current = other._current; _subPathStart = other._subPathStart; _hasCurrent = true; }
        _lastVerb = other._lastVerb;
        _lastCubicControl = other._lastCubicControl;
        _lastQuadControl = other._lastQuadControl;
        return this;
    }

    /// <summary>
    /// Axis-aligned bounding box of the recorded segment endpoints + control
    /// points. This over-estimates the true curve bounds (control points may
    /// lie outside the curve hull) but is fast and adequate for clipping
    /// and gradient-unit resolution.
    /// </summary>
    public (float MinX, float MinY, float MaxX, float MaxY) GetBounds()
    {
        if (_segments.Count == 0) return (0, 0, 0, 0);
        float minX = float.MaxValue, minY = float.MaxValue;
        float maxX = float.MinValue, maxY = float.MinValue;
        foreach (var s in _segments)
        {
            Include(s.P0);
            if (s.Verb is PathVerb.QuadTo or PathVerb.CubicTo) Include(s.P1);
            if (s.Verb == PathVerb.CubicTo) Include(s.P2);
        }
        return (minX, minY, maxX, maxY);

        void Include(Vector2 v)
        {
            if (v.X < minX) minX = v.X;
            if (v.Y < minY) minY = v.Y;
            if (v.X > maxX) maxX = v.X;
            if (v.Y > maxY) maxY = v.Y;
        }
    }

    private void EnsureCurrent()
    {
        if (!_hasCurrent)
        {
            _current = Vector2.Zero;
            _subPathStart = Vector2.Zero;
            _segments.Add(new PathSegment(PathVerb.MoveTo, Vector2.Zero));
            _hasCurrent = true;
        }
    }
}
