namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis window function (sin-window) and overlap-add buffer. Vorbis uses
/// the standard sin² window with optional left/right-edge mode bits that let
/// adjacent short and long blocks meet at a half-block boundary
/// (Vorbis I §1.3.2.4).
/// </summary>
internal static class VorbisWindow
{
    /// <summary>
    /// Apply the Vorbis sin² window to <paramref name="block"/> in place.
    /// The window has independent flags for the left and right halves so a
    /// long block can adjoin a short block. <paramref name="block"/> length
    /// must equal <paramref name="n"/>.
    /// </summary>
    public static void Apply(Span<float> block, int n, int prevN, int nextN)
    {
        if (block.Length != n) throw new ArgumentException("Block length mismatch.", nameof(block));
        int leftWinLen = Math.Min(n, prevN);
        int rightWinLen = Math.Min(n, nextN);
        int leftBegin = n / 2 - leftWinLen / 2;
        int rightBegin = n / 2 + (n / 2 - rightWinLen / 2);

        for (int i = 0; i < leftBegin; i++) block[i] = 0;
        for (int i = 0; i < leftWinLen; i++)
        {
            double inner = Math.PI / 2.0 * (i + 0.5) / leftWinLen;
            float w = (float)Math.Sin(Math.PI / 2.0 * Math.Sin(inner) * Math.Sin(inner));
            block[leftBegin + i] *= w;
        }
        // Center region [leftBegin + leftWinLen, rightBegin) passes through.
        for (int i = 0; i < rightWinLen; i++)
        {
            double inner = Math.PI / 2.0 * (rightWinLen - i - 0.5) / rightWinLen;
            float w = (float)Math.Sin(Math.PI / 2.0 * Math.Sin(inner) * Math.Sin(inner));
            block[rightBegin + i] *= w;
        }
        for (int i = rightBegin + rightWinLen; i < n; i++) block[i] = 0;
    }
}
