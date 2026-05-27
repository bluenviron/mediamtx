using System.Globalization;
using System.Numerics;

namespace Mediar.Codecs.SvgRaster;

/// <summary>
/// Parses an SVG <c>transform="..."</c> attribute value into a
/// <see cref="Matrix3x2"/>. Supports the six transform functions
/// defined in SVG 1.1: <c>matrix</c>, <c>translate</c>, <c>scale</c>,
/// <c>rotate</c>, <c>skewX</c>, <c>skewY</c>.
/// </summary>
public static class SvgTransformParser
{
    /// <summary>
    /// Parse <paramref name="text"/> into a combined transform. Returns
    /// <see cref="Matrix3x2.Identity"/> for empty / null input.
    /// Transforms in SVG are applied right-to-left in the list, which is
    /// equivalent to left-multiplying each new factor.
    /// </summary>
    public static Matrix3x2 Parse(string? text)
    {
        if (string.IsNullOrWhiteSpace(text)) return Matrix3x2.Identity;
        var m = Matrix3x2.Identity;
        var i = 0;
        var s = text.AsSpan();
        while (i < s.Length)
        {
            SkipWs(s, ref i);
            if (i >= s.Length) break;
            int nameStart = i;
            while (i < s.Length && (char.IsLetter(s[i]) || s[i] == '_')) i++;
            if (i == nameStart) { i++; continue; }
            var name = s[nameStart..i];

            SkipWs(s, ref i);
            if (i >= s.Length || s[i] != '(') break;
            i++;
            int argStart = i;
            while (i < s.Length && s[i] != ')') i++;
            if (i >= s.Length) break;
            var args = s[argStart..i];
            i++; // skip ')'
            SkipWs(s, ref i);
            if (i < s.Length && (s[i] == ',' || s[i] == ' ')) i++;

            var nums = ParseNumbers(args);
            Matrix3x2 t = name switch
            {
                _ when name.Equals("matrix", StringComparison.OrdinalIgnoreCase) && nums.Count >= 6
                    => new Matrix3x2(nums[0], nums[1], nums[2], nums[3], nums[4], nums[5]),
                _ when name.Equals("translate", StringComparison.OrdinalIgnoreCase)
                    => Matrix3x2.CreateTranslation(nums.Count >= 1 ? nums[0] : 0, nums.Count >= 2 ? nums[1] : 0),
                _ when name.Equals("scale", StringComparison.OrdinalIgnoreCase)
                    => Matrix3x2.CreateScale(nums.Count >= 1 ? nums[0] : 1, nums.Count >= 2 ? nums[1] : (nums.Count >= 1 ? nums[0] : 1)),
                _ when name.Equals("rotate", StringComparison.OrdinalIgnoreCase) => nums.Count switch
                {
                    >= 3 => Matrix3x2.CreateRotation(nums[0] * MathF.PI / 180f, new Vector2(nums[1], nums[2])),
                    >= 1 => Matrix3x2.CreateRotation(nums[0] * MathF.PI / 180f),
                    _ => Matrix3x2.Identity,
                },
                _ when name.Equals("skewX", StringComparison.OrdinalIgnoreCase) && nums.Count >= 1
                    => CreateSkewX(nums[0]),
                _ when name.Equals("skewY", StringComparison.OrdinalIgnoreCase) && nums.Count >= 1
                    => CreateSkewY(nums[0]),
                _ => Matrix3x2.Identity,
            };
            m = t * m;
        }
        return m;
    }

    private static Matrix3x2 CreateSkewX(float angleDeg)
    {
        float t = MathF.Tan(angleDeg * MathF.PI / 180f);
        return new Matrix3x2(1, 0, t, 1, 0, 0);
    }

    private static Matrix3x2 CreateSkewY(float angleDeg)
    {
        float t = MathF.Tan(angleDeg * MathF.PI / 180f);
        return new Matrix3x2(1, t, 0, 1, 0, 0);
    }

    private static List<float> ParseNumbers(ReadOnlySpan<char> args)
    {
        var result = new List<float>(8);
        int i = 0;
        while (i < args.Length)
        {
            SkipWsOrComma(args, ref i);
            if (i >= args.Length) break;
            int start = i;
            // Read sign
            if (i < args.Length && (args[i] == '+' || args[i] == '-')) i++;
            bool sawDigit = false;
            while (i < args.Length && (char.IsDigit(args[i]) || args[i] == '.')) { i++; sawDigit = true; }
            // Exponent
            if (i < args.Length && (args[i] == 'e' || args[i] == 'E'))
            {
                i++;
                if (i < args.Length && (args[i] == '+' || args[i] == '-')) i++;
                while (i < args.Length && char.IsDigit(args[i])) i++;
            }
            if (!sawDigit) { i++; continue; }
            if (float.TryParse(args[start..i], NumberStyles.Float, CultureInfo.InvariantCulture, out float v))
                result.Add(v);
        }
        return result;
    }

    private static void SkipWs(ReadOnlySpan<char> s, ref int i)
    {
        while (i < s.Length && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n')) i++;
    }

    private static void SkipWsOrComma(ReadOnlySpan<char> s, ref int i)
    {
        while (i < s.Length && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n' || s[i] == ',')) i++;
    }
}
