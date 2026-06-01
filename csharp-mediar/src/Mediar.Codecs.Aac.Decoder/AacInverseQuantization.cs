using System.Runtime.CompilerServices;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Inverse quantization of AAC quantised spectral coefficients per
/// ISO/IEC 14496-3 §4.6.2 step 5:
/// <c>x_invquant = sign(x_quant) * |x_quant|^(4/3)</c>.
/// </summary>
/// <remarks>
/// <para>
/// This is the first step on the dequantization path between
/// <see cref="AacSpectralData.Coefficients"/> (integer quantised
/// coefficients emitted by the Huffman-decoded spectral walker) and
/// the floating-point spectrum the scale-factor stage operates on.
/// The output of this step still needs to be multiplied by
/// <c>2^(scalefactor / 4)</c> per scale factor band before it is
/// passed to the joint-stereo / PNS / intensity / TNS / MDCT chain.
/// </para>
/// <para>
/// AAC quantised coefficients are clipped at <c>±8191</c> per the
/// spec (Table 4.121); inputs outside that range are still processed
/// without saturation, but a well-formed bit-stream will never carry
/// such a value.
/// </para>
/// </remarks>
public static class AacInverseQuantization
{
    /// <summary>
    /// Constant exponent (<c>4/3</c>) applied to <c>|x_quant|</c> in
    /// the inverse-quantization formula.
    /// </summary>
    public const float Exponent = 4f / 3f;

    /// <summary>
    /// Inverse-quantise a single integer coefficient per
    /// <c>x_invquant = sign(x) * |x|^(4/3)</c>. The zero input returns
    /// zero (the formula is mathematically continuous at zero). The
    /// power computation runs in double precision and is cast to
    /// <see cref="float"/> on return; this avoids the ~1.5 % rounding
    /// error <see cref="MathF.Pow(float, float)"/> exhibits near the
    /// AAC clip limit (<c>±8191</c>).
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static float Dequantize(int xQuant)
    {
        if (xQuant == 0) return 0f;
        double abs = xQuant < 0 ? -(double)xQuant : (double)xQuant;
        double magnitude = Math.Pow(abs, 4.0 / 3.0);
        return xQuant < 0 ? (float)-magnitude : (float)magnitude;
    }

    /// <summary>
    /// Inverse-quantise <paramref name="source"/> into
    /// <paramref name="destination"/> element-wise. The destination
    /// must be at least as long as the source.
    /// </summary>
    /// <exception cref="ArgumentException">
    /// <paramref name="destination"/> is shorter than <paramref name="source"/>.
    /// </exception>
    public static void Dequantize(ReadOnlySpan<int> source, Span<float> destination)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException(
                $"Destination span (length {destination.Length}) is shorter than source (length {source.Length}).",
                nameof(destination));
        }

        for (int i = 0; i < source.Length; i++)
        {
            destination[i] = Dequantize(source[i]);
        }
    }

    /// <summary>
    /// Allocate and return a new array of inverse-quantised values
    /// the same length as <paramref name="source"/>.
    /// </summary>
    public static float[] Dequantize(ReadOnlySpan<int> source)
    {
        var output = new float[source.Length];
        Dequantize(source, output);
        return output;
    }
}
