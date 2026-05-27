namespace Mediar.Acceleration;

/// <summary>
/// Saturates signed 32-bit integers into unsigned 8-bit bytes (clamping to
/// the inclusive range <c>[0, 255]</c>). Used by IDCT output stages,
/// colour-conversion pipelines, and dithering passes throughout the
/// imaging stack.
/// </summary>
/// <remarks>
/// Implementations are registered with <see cref="AccelerationDispatcher"/>
/// and selected at runtime based on the host CPU's instruction set.
/// The scalar fallback (<see cref="ScalarByteSaturator"/>) is always
/// available and AOT-safe.
/// </remarks>
public interface IByteSaturator : IAcceleratedKernel
{
    /// <summary>
    /// Clamps each element of <paramref name="source"/> to <c>[0, 255]</c>
    /// and writes the result as <see cref="byte"/> values into
    /// <paramref name="destination"/>.
    /// </summary>
    /// <exception cref="ArgumentException">
    /// <paramref name="destination"/> is shorter than <paramref name="source"/>.
    /// </exception>
    void Saturate(ReadOnlySpan<int> source, Span<byte> destination);
}
