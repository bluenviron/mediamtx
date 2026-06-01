using System.Collections.Immutable;

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC long-block (2048-sample) window composer per ISO/IEC 14496-3
/// §4.6.11. Composes the full <c>N = 2048</c> sample window envelope
/// for an <see cref="AacWindowSequence.OnlyLong"/>,
/// <see cref="AacWindowSequence.LongStart"/>, or
/// <see cref="AacWindowSequence.LongStop"/> frame, given the previous
/// frame's window shape (which determines the left half) and the
/// current frame's window shape (which determines the right half).
/// </summary>
/// <remarks>
/// <para>
/// The composer hands out the same envelope that gets multiplied
/// elementwise with the IMDCT output before overlap-add. For the
/// transition sequences <c>LongStart</c> and <c>LongStop</c> only
/// part of the 2048-sample window is non-zero / non-flat:
/// </para>
/// <list type="bullet">
/// <item><description>
/// <c>OnlyLong</c>: <c>[w_left(0..1023), w_right(0..1023)]</c> -
/// full long left half (rising, prev shape) then full long right
/// half (falling, current shape).
/// </description></item>
/// <item><description>
/// <c>LongStart</c>: <c>[w_left(0..1023), 1×448, w_short_right(0..127), 0×448]</c>
/// - long left half (prev shape), flat-1 plateau, short right half
/// (current shape), zero tail.
/// </description></item>
/// <item><description>
/// <c>LongStop</c>: <c>[0×448, w_short_left(0..127), 1×448, w_right(0..1023)]</c>
/// - zero head, short left half (prev shape), flat-1 plateau, long
/// right half (current shape).
/// </description></item>
/// </list>
/// <para>
/// EightShort sequences are handled separately because their eight
/// 256-sample windows have an independent overlap-add scheme
/// between siblings.
/// </para>
/// </remarks>
public static class AacBlockWindow
{
    /// <summary>Length of the composed long-block window (2048).</summary>
    public const int LongBlockLength = 2048;

    /// <summary>Half-length of a long window (1024).</summary>
    public const int LongHalfLength = 1024;

    /// <summary>Half-length of a short window (128).</summary>
    public const int ShortHalfLength = 128;

    /// <summary>
    /// Length of each flat / zero plateau in a transition window
    /// (448 = (2048 - 256) / 2 - 384... actually (1024 - 128) / 2
    /// surrounds the short subwindow with 448 samples on each side
    /// within the right or left long half).
    /// </summary>
    public const int TransitionPlateauLength = 448;

    /// <summary>
    /// Compose the full 2048-sample long-block window envelope.
    /// </summary>
    /// <param name="sequence">
    /// Must be <see cref="AacWindowSequence.OnlyLong"/>,
    /// <see cref="AacWindowSequence.LongStart"/>, or
    /// <see cref="AacWindowSequence.LongStop"/>.
    /// </param>
    /// <param name="previousShape">
    /// Window shape of the previous frame; determines the left half.
    /// </param>
    /// <param name="currentShape">
    /// Window shape of the current frame; determines the right half.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="sequence"/> is
    /// <see cref="AacWindowSequence.EightShort"/> or an undefined
    /// value.
    /// </exception>
    public static ImmutableArray<float> ComposeLongBlock(
        AacWindowSequence sequence,
        AacWindowShape previousShape,
        AacWindowShape currentShape)
    {
        var buffer = new float[LongBlockLength];
        WriteLongBlock(buffer, sequence, previousShape, currentShape);
        return System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(buffer);
    }

    /// <summary>
    /// Write the full 2048-sample long-block window envelope into
    /// <paramref name="destination"/>.
    /// </summary>
    /// <exception cref="ArgumentException">
    /// <paramref name="destination"/> is not exactly
    /// <see cref="LongBlockLength"/> samples, or
    /// <paramref name="sequence"/> is invalid for a long block.
    /// </exception>
    public static void WriteLongBlock(
        Span<float> destination,
        AacWindowSequence sequence,
        AacWindowShape previousShape,
        AacWindowShape currentShape)
    {
        if (destination.Length != LongBlockLength)
        {
            throw new ArgumentException(
                $"Destination must be {LongBlockLength} samples long, got {destination.Length}.",
                nameof(destination));
        }

        switch (sequence)
        {
            case AacWindowSequence.OnlyLong:
                WriteLongLeftHalf(destination[..LongHalfLength], previousShape);
                WriteLongRightHalf(destination.Slice(LongHalfLength, LongHalfLength), currentShape);
                break;

            case AacWindowSequence.LongStart:
                // [long-left | 1×448 | short-right | 0×448]
                WriteLongLeftHalf(destination[..LongHalfLength], previousShape);
                destination.Slice(LongHalfLength, TransitionPlateauLength).Fill(1.0f);
                WriteShortRightHalf(
                    destination.Slice(LongHalfLength + TransitionPlateauLength, ShortHalfLength),
                    currentShape);
                destination[(LongHalfLength + TransitionPlateauLength + ShortHalfLength)..].Clear();
                break;

            case AacWindowSequence.LongStop:
                // [0×448 | short-left | 1×448 | long-right]
                destination[..TransitionPlateauLength].Clear();
                WriteShortLeftHalf(
                    destination.Slice(TransitionPlateauLength, ShortHalfLength),
                    previousShape);
                destination.Slice(TransitionPlateauLength + ShortHalfLength, TransitionPlateauLength).Fill(1.0f);
                WriteLongRightHalf(destination.Slice(LongHalfLength, LongHalfLength), currentShape);
                break;

            case AacWindowSequence.EightShort:
                throw new ArgumentException(
                    "EightShort sequences require ComposeShortWindow, not the long-block composer.",
                    nameof(sequence));

            default:
                throw new ArgumentException($"Unknown window sequence: {sequence}.", nameof(sequence));
        }
    }

    /// <summary>
    /// Compose a single 256-sample short window with the given left
    /// and right shapes. Useful for the EightShort sequence where
    /// each of the eight short subwindows may have a distinct left
    /// shape (only the first inherits the previous frame's shape;
    /// all subsequent left halves use the current shape).
    /// </summary>
    public static ImmutableArray<float> ComposeShortWindow(
        AacWindowShape leftShape,
        AacWindowShape rightShape)
    {
        var buffer = new float[2 * ShortHalfLength];
        WriteShortLeftHalf(buffer.AsSpan(0, ShortHalfLength), leftShape);
        WriteShortRightHalf(buffer.AsSpan(ShortHalfLength, ShortHalfLength), rightShape);
        return System.Runtime.InteropServices.ImmutableCollectionsMarshal.AsImmutableArray(buffer);
    }

    private static void WriteLongLeftHalf(Span<float> destination, AacWindowShape shape)
    {
        WriteRisingHalf(destination, shape, isLong: true);
    }

    private static void WriteLongRightHalf(Span<float> destination, AacWindowShape shape)
    {
        WriteFallingHalf(destination, shape, isLong: true);
    }

    private static void WriteShortLeftHalf(Span<float> destination, AacWindowShape shape)
    {
        WriteRisingHalf(destination, shape, isLong: false);
    }

    private static void WriteShortRightHalf(Span<float> destination, AacWindowShape shape)
    {
        WriteFallingHalf(destination, shape, isLong: false);
    }

    private static void WriteRisingHalf(Span<float> destination, AacWindowShape shape, bool isLong)
    {
        switch (shape)
        {
            case AacWindowShape.Sine:
                AacSineWindow.WriteRisingHalf(destination);
                break;

            case AacWindowShape.KaiserBesselDerived:
                AacKbdWindow.WriteRisingHalf(
                    destination,
                    isLong ? AacKbdWindow.LongAlpha : AacKbdWindow.ShortAlpha);
                break;

            default:
                throw new ArgumentException($"Unknown window shape: {shape}.", nameof(shape));
        }
    }

    private static void WriteFallingHalf(Span<float> destination, AacWindowShape shape, bool isLong)
    {
        WriteRisingHalf(destination, shape, isLong);
        destination.Reverse();
    }
}
