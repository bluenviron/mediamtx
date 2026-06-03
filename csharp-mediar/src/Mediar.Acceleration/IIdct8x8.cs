namespace Mediar.Acceleration;

/// <summary>
/// 8×8 inverse-DCT kernel used by JPEG-family decoders. Implementations are
/// bit-exact across scalar / SSE2 / AVX2 / AdvSimd because they share the
/// same integer Loeffler-Lightenberg-Moschytz fixed-point algorithm
/// (Loeffler, Lightenberg, Moschytz 1989; libjpeg-turbo <c>jidctint.c</c>).
/// </summary>
/// <remarks>
/// <para>
/// The decoder dequantises 64 short coefficients into <c>input</c> in
/// natural (row-major) order. The kernel computes the inverse 8×8 DCT,
/// level-shifts by +128, clamps to <c>[0, 255]</c>, and writes an 8×8
/// block of bytes to <c>output</c> with rows separated by
/// <paramref name="outputStride"/> bytes.
/// </para>
/// </remarks>
public interface IIdct8x8 : IAcceleratedKernel
{
    /// <summary>
    /// Compute the 8×8 inverse DCT of <paramref name="input"/> into
    /// <paramref name="output"/>. <paramref name="input"/> must have
    /// exactly 64 entries.
    /// </summary>
    void Idct8x8(ReadOnlySpan<short> input, Span<byte> output, int outputStride);
}
