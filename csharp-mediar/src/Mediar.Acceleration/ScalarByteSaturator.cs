using System.Runtime.CompilerServices;

namespace Mediar.Acceleration;

/// <summary>
/// Portable scalar fallback implementation of <see cref="IByteSaturator"/>.
/// Always-available and AOT-safe.
/// </summary>
public sealed class ScalarByteSaturator : IByteSaturator
{
    /// <summary>Singleton instance — the type is stateless.</summary>
    public static ScalarByteSaturator Instance { get; } = new();

    /// <inheritdoc/>
    public AccelerationTier IsaTier => AccelerationTier.Scalar;

    /// <inheritdoc/>
    public void Saturate(ReadOnlySpan<int> source, Span<byte> destination)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException("destination span is shorter than source.", nameof(destination));
        }

        for (int i = 0; i < source.Length; i++)
        {
            destination[i] = ClampByte(source[i]);
        }
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    private static byte ClampByte(int v) => v < 0 ? (byte)0 : v > 255 ? (byte)255 : (byte)v;
}
