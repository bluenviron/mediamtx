using System.Globalization;
using System.Numerics;
using Mediar.Vector;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// Parses an SVG <c>d</c> path-data attribute into a <see cref="Path2D"/>.
/// Implements every command in SVG 1.1 § 9 path data, with both absolute
/// (uppercase) and relative (lowercase) variants, implicit-command repeats
/// and trailing-coordinate continuation.
/// </summary>
public static class SvgPathDataParser
{
    /// <summary>Parse <paramref name="d"/> into a fresh path.</summary>
    public static Path2D Parse(string? d)
    {
        var path = new Path2D();
        if (string.IsNullOrWhiteSpace(d)) return path;

        var t = new PathTokenizer(d);
        Vector2 current = Vector2.Zero;
        Vector2 subStart = Vector2.Zero;
        char prevCmd = '\0';

        while (t.HasMore)
        {
            char cmd = t.PeekCommand();
            if (cmd == '\0')
            {
                // Implicit repeat of previous command. Per SVG: M -> L, m -> l.
                if (prevCmd == '\0') break;
                cmd = prevCmd switch
                {
                    'M' => 'L',
                    'm' => 'l',
                    _ => prevCmd,
                };
            }
            else
            {
                t.ConsumeCommand();
            }

            bool rel = char.IsLower(cmd);
            switch (char.ToUpperInvariant(cmd))
            {
                case 'M':
                    current = ReadPoint(t, current, rel);
                    path.MoveTo(current);
                    subStart = current;
                    // Subsequent pairs after M/m are implicit L/l.
                    while (t.HasNumber)
                    {
                        current = ReadPoint(t, current, rel);
                        path.LineTo(current);
                    }
                    break;
                case 'L':
                    while (t.HasNumber)
                    {
                        current = ReadPoint(t, current, rel);
                        path.LineTo(current);
                    }
                    break;
                case 'H':
                    while (t.HasNumber)
                    {
                        float x = t.ReadNumber();
                        if (rel) x += current.X;
                        current = new Vector2(x, current.Y);
                        path.LineTo(current);
                    }
                    break;
                case 'V':
                    while (t.HasNumber)
                    {
                        float y = t.ReadNumber();
                        if (rel) y += current.Y;
                        current = new Vector2(current.X, y);
                        path.LineTo(current);
                    }
                    break;
                case 'C':
                    while (t.HasNumber)
                    {
                        Vector2 c1 = ReadPoint(t, current, rel);
                        Vector2 c2 = ReadPoint(t, current, rel);
                        Vector2 p = ReadPoint(t, current, rel);
                        path.CubicTo(c1, c2, p);
                        current = p;
                    }
                    break;
                case 'S':
                    while (t.HasNumber)
                    {
                        Vector2 c2 = ReadPoint(t, current, rel);
                        Vector2 p = ReadPoint(t, current, rel);
                        path.SmoothCubicTo(c2, p);
                        current = p;
                    }
                    break;
                case 'Q':
                    while (t.HasNumber)
                    {
                        Vector2 c = ReadPoint(t, current, rel);
                        Vector2 p = ReadPoint(t, current, rel);
                        path.QuadTo(c, p);
                        current = p;
                    }
                    break;
                case 'T':
                    while (t.HasNumber)
                    {
                        Vector2 p = ReadPoint(t, current, rel);
                        path.SmoothQuadTo(p);
                        current = p;
                    }
                    break;
                case 'A':
                    while (t.HasNumber)
                    {
                        float rx = t.ReadNumber();
                        float ry = t.ReadNumber();
                        float xRot = t.ReadNumber();
                        bool large = t.ReadFlag();
                        bool sweep = t.ReadFlag();
                        Vector2 p = ReadPoint(t, current, rel);
                        path.ArcTo(rx, ry, xRot, large, sweep, p);
                        current = p;
                    }
                    break;
                case 'Z':
                    path.Close();
                    current = subStart;
                    break;
            }
            prevCmd = cmd;
        }

        return path;
    }

    private static Vector2 ReadPoint(PathTokenizer t, Vector2 current, bool rel)
    {
        float x = t.ReadNumber();
        float y = t.ReadNumber();
        return rel ? new Vector2(current.X + x, current.Y + y) : new Vector2(x, y);
    }

    private sealed class PathTokenizer(string input)
    {
        private readonly string _s = input;
        private int _i;

        public bool HasMore => SkipWs() < _s.Length;

        public bool HasNumber
        {
            get
            {
                int i = SkipWs();
                if (i >= _s.Length) return false;
                char c = _s[i];
                return char.IsDigit(c) || c == '-' || c == '+' || c == '.';
            }
        }

        public char PeekCommand()
        {
            int i = SkipWs();
            if (i >= _s.Length) return '\0';
            char c = _s[i];
            return IsCommand(c) ? c : '\0';
        }

        public void ConsumeCommand()
        {
            _i = SkipWs() + 1;
        }

        public float ReadNumber()
        {
            int i = SkipWsOrComma();
            if (i >= _s.Length) throw new FormatException("SVG path data ended mid-number.");
            int start = i;
            if (_s[i] == '+' || _s[i] == '-') i++;
            bool sawDigit = false;
            while (i < _s.Length && char.IsDigit(_s[i])) { i++; sawDigit = true; }
            if (i < _s.Length && _s[i] == '.')
            {
                i++;
                while (i < _s.Length && char.IsDigit(_s[i])) { i++; sawDigit = true; }
            }
            if (i < _s.Length && (_s[i] == 'e' || _s[i] == 'E'))
            {
                i++;
                if (i < _s.Length && (_s[i] == '+' || _s[i] == '-')) i++;
                while (i < _s.Length && char.IsDigit(_s[i])) i++;
            }
            if (!sawDigit) throw new FormatException($"SVG path data: expected number near offset {start}.");
            float v = float.Parse(_s.AsSpan(start, i - start), NumberStyles.Float, CultureInfo.InvariantCulture);
            _i = i;
            return v;
        }

        public bool ReadFlag()
        {
            int i = SkipWsOrComma();
            if (i >= _s.Length || (_s[i] != '0' && _s[i] != '1'))
                throw new FormatException($"SVG path data: expected 0 or 1 flag near offset {i}.");
            bool v = _s[i] == '1';
            _i = i + 1;
            return v;
        }

        private int SkipWs()
        {
            int i = _i;
            while (i < _s.Length && IsWs(_s[i])) i++;
            _i = i;
            return i;
        }

        private int SkipWsOrComma()
        {
            int i = _i;
            while (i < _s.Length && (IsWs(_s[i]) || _s[i] == ',')) i++;
            _i = i;
            return i;
        }

        private static bool IsWs(char c) => c is ' ' or '\t' or '\r' or '\n';
        private static bool IsCommand(char c) => "MmLlHhVvCcSsQqTtAaZz".Contains(c);
    }
}
