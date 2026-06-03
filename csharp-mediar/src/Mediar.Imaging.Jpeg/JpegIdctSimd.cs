using System.Runtime.CompilerServices;
using Mediar.Acceleration;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Thin adapter that exposes <see cref="IIdct8x8"/> from
/// <see cref="AccelerationDispatcher"/> to JPEG decode paths. New decoders
/// (the 12-bit SOF1 path and the arithmetic-coded SOF9/10/11 paths) route
/// through this so they automatically pick up any future hardware kernel
/// without code changes.
/// </summary>
/// <remarks>
/// <para>
/// All registered backends share the same integer Loeffler-Lightenberg-
/// Moschytz fixed-point algorithm (LLM 1989, "Practical fast 1-D DCT
/// algorithms with 11 multiplications"). The scalar reference in
/// <see cref="ScalarIdct8x8"/> defines the exact arithmetic each
/// hardware backend must reproduce so that <see cref="IIdct8x8.Idct8x8"/>
/// is bit-exact across implementations — verified by the
/// <c>JpegIdctSimdTests</c> suite.
/// </para>
/// </remarks>
internal static class JpegIdctSimd
{
    /// <summary>The current best-available IDCT kernel.</summary>
    public static IIdct8x8 Current => Kernels.Idct8x8;

    /// <summary>
    /// Dequantise a 64-coefficient zig-zag block in place into natural
    /// order using a 16-bit Q-table, then run the 8×8 IDCT into
    /// <paramref name="output"/>. Used by the high-bit-depth and
    /// arithmetic decoders.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static void DequantizeAndIdct(
        ReadOnlySpan<short> zigZagCoefs,
        ReadOnlySpan<short> qTable,
        Span<short> naturalScratch,
        Span<byte> output,
        int outputStride)
    {
        for (int i = 0; i < 64; i++)
        {
            naturalScratch[JpegDecoderShared.Zigzag[i]] = (short)(zigZagCoefs[i] * qTable[i]);
        }
        Current.Idct8x8(naturalScratch, output, outputStride);
    }
}
