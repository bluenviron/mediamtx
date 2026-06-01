using System.Collections.Immutable;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC sine analysis/synthesis window per ISO/IEC 14496-3
/// §4.6.11.3.1: <c>w_sin(n) = sin((π / (2N)) · (n + 0.5))</c> for
/// <c>n = 0 .. 2N-1</c>, where <c>2N</c> is the MDCT transform
/// length (<c>2N = 2048</c> for long blocks, <c>2N = 256</c> for
/// short blocks).
/// </summary>
/// <remarks>
/// <para>
/// The full sine window is symmetric: <c>w(2N - 1 - n) == w(n)</c>.
/// AAC's window-shape select per <c>ics_info()</c> chooses between
/// this sine window and the Kaiser-Bessel-derived (KBD) window
/// (shipped separately).
/// </para>
/// <para>
/// Helpers below compute either the rising left half
/// (<see cref="ComputeRisingHalf"/>) or the full <c>2N</c>-length
/// window (<see cref="ComputeFull"/>). The rising half is the form
/// the MDCT inverse uses directly during overlap-add since the
/// falling right half is its mirror.
/// </para>
/// </remarks>
public static class AacSineWindow
{
    /// <summary>
    /// Number of samples in a long-window half (<c>N = 1024</c>).
    /// </summary>
    public const int LongHalfLength = 1024;

    /// <summary>
    /// Number of samples in a short-window half (<c>N = 128</c>).
    /// </summary>
    public const int ShortHalfLength = 128;

    /// <summary>
    /// Compute the rising left half of the sine window of length
    /// <paramref name="halfLength"/>. The corresponding falling
    /// right half is the reflection
    /// (<c>w[2·halfLength - 1 - n] = w[n]</c>).
    /// </summary>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="halfLength"/> is &lt;= 0.
    /// </exception>
    public static ImmutableArray<float> ComputeRisingHalf(int halfLength)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(halfLength, 0);

        var buffer = new float[halfLength];
        double scale = Math.PI / (2.0 * halfLength);
        for (int n = 0; n < halfLength; n++)
        {
            buffer[n] = (float)Math.Sin(scale * (n + 0.5));
        }
        return System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(buffer);
    }

    /// <summary>
    /// Compute the full symmetric sine window of length
    /// <c>2 · <paramref name="halfLength"/></c>.
    /// </summary>
    /// <exception cref="ArgumentOutOfRangeException">
    /// <paramref name="halfLength"/> is &lt;= 0.
    /// </exception>
    public static ImmutableArray<float> ComputeFull(int halfLength)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(halfLength, 0);

        int fullLength = 2 * halfLength;
        var buffer = new float[fullLength];
        double scale = Math.PI / (2.0 * halfLength);
        for (int n = 0; n < fullLength; n++)
        {
            buffer[n] = (float)Math.Sin(scale * (n + 0.5));
        }
        return System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(buffer);
    }

    /// <summary>
    /// Write the rising left half of the sine window of length
    /// <paramref name="destination"/>.Length into
    /// <paramref name="destination"/>.
    /// </summary>
    public static void WriteRisingHalf(Span<float> destination)
    {
        if (destination.Length == 0) return;
        double scale = Math.PI / (2.0 * destination.Length);
        for (int n = 0; n < destination.Length; n++)
        {
            destination[n] = (float)Math.Sin(scale * (n + 0.5));
        }
    }
}
