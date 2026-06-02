namespace Mediar.Codecs.Alac.Decoder;

/// <summary>
/// ALAC stereo inter-channel decorrelation ("unmix"). The encoder may
/// rotate a stereo pair (L, R) into a mid/side-like (u, v) basis to
/// improve compression; the decoder undoes the rotation here.
/// </summary>
/// <remarks>
/// The formulas mirror Apple's ALAC Apache-2.0 reference (codec/matrix_dec.c).
/// When <c>mixRes == 0</c> the encoder did not decorrelate and we emit the
/// channel pair straight through; otherwise the inverse of the encoder's
/// fractional mid/side rotation is applied.
/// </remarks>
internal static class AlacMatrix
{
    /// <summary>
    /// Un-mix a stereo pair from <paramref name="u"/> / <paramref name="v"/>
    /// into output buffers <paramref name="leftOut"/> / <paramref name="rightOut"/>.
    /// </summary>
    public static void Unmix(
        ReadOnlySpan<int> u,
        ReadOnlySpan<int> v,
        Span<int> leftOut,
        Span<int> rightOut,
        int numSamples,
        int mixBits,
        int mixRes)
    {
        if (mixRes != 0)
        {
            for (int i = 0; i < numSamples; i++)
            {
                int uVal = u[i];
                int vVal = v[i];
                int l = uVal + vVal - ((vVal * mixRes) >> mixBits);
                int r = l - vVal;
                leftOut[i] = l;
                rightOut[i] = r;
            }
        }
        else
        {
            // No decorrelation — straight L/R passthrough.
            for (int i = 0; i < numSamples; i++)
            {
                leftOut[i] = u[i];
                rightOut[i] = v[i];
            }
        }
    }
}
