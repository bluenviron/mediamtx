namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis sin² window (Vorbis I §1.3.2.4). Each block is windowed in place by
/// a ramp that rises through sin²(π/2·sin²(·)) on its left edge, holds at 1
/// across the plateau, falls on its right edge, and is zero outside the
/// active region. The left and right ramps can independently use either the
/// short or long block half-length so that long/short block transitions
/// produce overlapping ramps of matching length for TDAC.
/// </summary>
internal static class VorbisWindow
{
    /// <summary>
    /// Apply the Vorbis sin² window to <paramref name="block"/> in place.
    /// <paramref name="leftWindowLength"/> and <paramref name="rightWindowLength"/>
    /// are the <em>full</em> ramp lengths (typically <c>blocksize0/2</c> for a
    /// "short side" ramp and <c>n/2</c> for a "long side" ramp on a block of
    /// size <c>n</c>). The ramps sit at:
    /// <list type="bullet">
    ///   <item>left: <c>[n/4 - L/2, n/4 + L/2)</c></item>
    ///   <item>right: <c>[3n/4 - R/2, 3n/4 + R/2)</c></item>
    /// </list>
    /// Samples outside the union of ramps + plateau region are zeroed.
    /// </summary>
    public static void Apply(Span<float> block, int leftWindowLength, int rightWindowLength)
    {
        int n = block.Length;
        if (n == 0) return;
        if (leftWindowLength <= 0 || leftWindowLength > n)
            throw new ArgumentOutOfRangeException(nameof(leftWindowLength));
        if (rightWindowLength <= 0 || rightWindowLength > n)
            throw new ArgumentOutOfRangeException(nameof(rightWindowLength));

        int leftStart = n / 4 - leftWindowLength / 2;
        int leftEnd = leftStart + leftWindowLength; // exclusive
        int rightStart = 3 * n / 4 - rightWindowLength / 2;
        int rightEnd = rightStart + rightWindowLength; // exclusive

        // Zero region before left ramp.
        for (int i = 0; i < leftStart; i++) block[i] = 0;
        // Left ramp: sin(π/2 · sin²(((k+0.5)/L) · π/2))
        double leftScale = Math.PI / 2.0 / leftWindowLength;
        for (int k = 0; k < leftWindowLength; k++)
        {
            double inner = Math.Sin((k + 0.5) * leftScale);
            float w = (float)Math.Sin(Math.PI / 2.0 * inner * inner);
            block[leftStart + k] *= w;
        }
        // Plateau region [leftEnd, rightStart) passes through unchanged.
        _ = leftEnd;
        // Right ramp: mirrored — sin(π/2 · sin²(((L-1-k+0.5)/L) · π/2))
        double rightScale = Math.PI / 2.0 / rightWindowLength;
        for (int k = 0; k < rightWindowLength; k++)
        {
            double inner = Math.Sin((rightWindowLength - 1 - k + 0.5) * rightScale);
            float w = (float)Math.Sin(Math.PI / 2.0 * inner * inner);
            block[rightStart + k] *= w;
        }
        // Zero region after right ramp.
        for (int i = rightEnd; i < n; i++) block[i] = 0;
    }
}

