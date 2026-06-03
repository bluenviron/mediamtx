namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// CELT pre-filter pitch detection — encoder side of
/// <c>Mediar.Codecs.Opus.Decoder.Celt.CeltPostFilter</c>.
/// Mirrors libopus <c>celt/pitch.c:pitch_search</c>:
/// downsample to 4 kHz → normalised autocorrelation → coarse pitch
/// candidate → fine refinement → tap selection for the encoder's
/// <c>comb_filter</c>.
/// </summary>
/// <remarks>
/// Phase B2 v1 ships the encoder with the post-filter disabled
/// (matches CELT's "post-filter off" packet header bit). The full
/// pitch search + comb-filter selection is the follow-up B2.2 task.
/// </remarks>
internal static class CeltPitchSearch
{
    /// <summary>
    /// Placeholder pitch search — always reports "no pitch / post-filter
    /// off". The encoder writes <c>postfilter = 0</c> in its packet
    /// header and skips the comb-filter call, matching what the
    /// decoder sees when libopus sets <c>postfilter_period == 0</c>.
    /// </summary>
    /// <param name="pcm">PCM input window (unused in v1).</param>
    /// <param name="pitchPeriod">Output: pitch period in samples (always 0 in v1).</param>
    /// <param name="taps">Output: 3-tap comb filter coefficients (all 0 in v1).</param>
    public static void PitchSearchPlaceholder(ReadOnlySpan<float> pcm, out int pitchPeriod, out (float t0, float t1, float t2) taps)
    {
        _ = pcm;
        pitchPeriod = 0;
        taps = (0f, 0f, 0f);
    }
}
