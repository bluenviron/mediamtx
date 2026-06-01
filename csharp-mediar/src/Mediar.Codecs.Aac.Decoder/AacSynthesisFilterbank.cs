namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// AAC long-block synthesis filterbank per ISO/IEC 14496-3 §4.6.11:
/// drives <see cref="AacImdctNaive"/>, applies the
/// <see cref="AacBlockWindow"/> envelope, and overlap-adds with the
/// previous frame's right-half tail to produce 1024 PCM samples per
/// frame.
/// </summary>
/// <remarks>
/// <para>
/// One instance owns the 1024-sample overlap buffer for a single
/// channel. Subsequent frames must be fed in stream order via
/// <see cref="ProcessLongBlock"/>; the overlap state is mutated on
/// every call.
/// </para>
/// <para>
/// EightShort sequences are not handled here; their eight 256-sample
/// short transforms have their own internal overlap-add scheme.
/// LongStart and LongStop transition windows on either side of an
/// EightShort frame are supported by this class for the long-block
/// side of the transition - the EightShort frame in between is
/// the caller's responsibility.
/// </para>
/// </remarks>
public sealed class AacSynthesisFilterbank
{
    /// <summary>
    /// Number of PCM samples produced per long-block frame.
    /// </summary>
    public const int LongFrameLength = 1024;

    private readonly float[] _overlap = new float[LongFrameLength];
    private readonly float[] _imdctOutput = new float[2 * LongFrameLength];
    private readonly float[] _window = new float[2 * LongFrameLength];

    /// <summary>
    /// Window shape of the previous frame; used as the left-half
    /// shape of the next frame's composed window. Defaults to
    /// <see cref="AacWindowShape.Sine"/> until the first frame is
    /// processed.
    /// </summary>
    public AacWindowShape PreviousWindowShape { get; private set; } = AacWindowShape.Sine;

    /// <summary>
    /// Snapshot of the overlap buffer (for diagnostics / tests).
    /// </summary>
    public ReadOnlySpan<float> Overlap => _overlap;

    /// <summary>
    /// Reset the overlap buffer (zero-filled) and the previous
    /// window shape to sine. Call this at stream start or after a
    /// seek.
    /// </summary>
    public void Reset()
    {
        Array.Clear(_overlap);
        PreviousWindowShape = AacWindowShape.Sine;
    }

    /// <summary>
    /// Process a single long-block frame.
    /// </summary>
    /// <param name="coefs">
    /// <see cref="LongFrameLength"/> spectral coefficients.
    /// </param>
    /// <param name="sequence">
    /// Must be <see cref="AacWindowSequence.OnlyLong"/>,
    /// <see cref="AacWindowSequence.LongStart"/>, or
    /// <see cref="AacWindowSequence.LongStop"/>.
    /// </param>
    /// <param name="currentWindowShape">
    /// Window shape carried by this frame's <c>ics_info</c>.
    /// </param>
    /// <param name="output">
    /// Receives <see cref="LongFrameLength"/> PCM samples.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="coefs"/> or <paramref name="output"/> is not
    /// <see cref="LongFrameLength"/> samples long, or
    /// <paramref name="sequence"/> is invalid for a long block.
    /// </exception>
    public void ProcessLongBlock(
        ReadOnlySpan<float> coefs,
        AacWindowSequence sequence,
        AacWindowShape currentWindowShape,
        Span<float> output)
    {
        if (coefs.Length != LongFrameLength)
        {
            throw new ArgumentException(
                $"Spectral input must be {LongFrameLength} samples long, got {coefs.Length}.",
                nameof(coefs));
        }
        if (output.Length != LongFrameLength)
        {
            throw new ArgumentException(
                $"Output must be {LongFrameLength} samples long, got {output.Length}.",
                nameof(output));
        }

        AacImdctNaive.Inverse(coefs, _imdctOutput.AsSpan());

        AacBlockWindow.WriteLongBlock(
            _window.AsSpan(), sequence, PreviousWindowShape, currentWindowShape);

        for (int i = 0; i < 2 * LongFrameLength; i++)
        {
            _imdctOutput[i] *= _window[i];
        }

        for (int i = 0; i < LongFrameLength; i++)
        {
            output[i] = _imdctOutput[i] + _overlap[i];
        }

        for (int i = 0; i < LongFrameLength; i++)
        {
            _overlap[i] = _imdctOutput[LongFrameLength + i];
        }

        PreviousWindowShape = currentWindowShape;
    }
}
