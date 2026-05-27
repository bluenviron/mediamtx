namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Helpers for finding the b1 / b2 reference-line positions used by the
/// two-dimensional T.4 / T.6 coding modes. Definitions follow T.6 §2:
/// b1 is the first changing element on the reference line strictly to
/// the right of a0 and of opposite colour to a0; b2 is the first
/// changing element to the right of b1.
/// </summary>
internal static class CcittReferenceLine
{
    /// <summary>Find <c>b1</c> and <c>b2</c> for the current <c>a0</c> position.</summary>
    public static (int b1, int b2) FindB1B2(ReadOnlySpan<byte> refLine, int width, int a0, int colourOfA0)
    {
        int b1 = width;
        int b2 = width;
        int prev = 0; // imaginary white before column 0
        int p = 0;

        for (; p < width; p++)
        {
            int curr = CcittBitmap.GetPixel(refLine, p);
            if (p > a0 && curr != prev && curr == (1 - colourOfA0))
            {
                b1 = p;
                prev = curr;
                p++;
                break;
            }
            prev = curr;
        }

        for (; p < width; p++)
        {
            int curr = CcittBitmap.GetPixel(refLine, p);
            if (curr != prev && curr == colourOfA0)
            {
                b2 = p;
                break;
            }
            prev = curr;
        }

        return (b1, b2);
    }
}
