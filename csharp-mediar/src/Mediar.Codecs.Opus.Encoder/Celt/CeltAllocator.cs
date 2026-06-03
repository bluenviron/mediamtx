namespace Mediar.Codecs.Opus.Encoder.Celt;

/// <summary>
/// Encoder-side bit-allocation entry point — RFC 6716 §4.3.3 / libopus
/// <c>celt/rate.c:compute_allocation</c>. The allocation algorithm itself
/// is identical between encoder and decoder (it's a deterministic
/// function of the inputs); the encoder's job is to choose the inputs
/// (TF flags, dyn-alloc boosts, intensity / dual-stereo decisions).
/// </summary>
/// <remarks>
/// Phase B2 v1 ships a placeholder that defers to a fixed per-band
/// pulse budget so the rest of the pipeline can be exercised end-to-end.
/// The full <c>compute_allocation</c> + TF/spread analysis port is the
/// follow-up B2.1 task — it will reuse
/// <c>Mediar.Codecs.Opus.Decoder.Celt.CeltAllocation</c> directly via
/// the <c>InternalsVisibleTo</c> bridge once that module is exposed.
/// </remarks>
internal static class CeltAllocator
{
    /// <summary>
    /// Compute a flat pulse allocation: <paramref name="K"/> pulses per
    /// CELT band. Useful for the Phase B2 round-trip sanity tests
    /// before the full bit-allocator port lands.
    /// </summary>
    public static void FlatAllocation(Span<int> pulsesPerBand, int K)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(K, 0);
        for (int i = 0; i < pulsesPerBand.Length; i++) pulsesPerBand[i] = K;
    }
}
