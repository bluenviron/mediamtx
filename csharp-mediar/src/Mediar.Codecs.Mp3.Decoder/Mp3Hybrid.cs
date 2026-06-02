namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// Layer III "hybrid" filterbank stages: short-block spectral reorder,
/// alias-reduction butterflies, inverse MDCT (12- or 18-point), and 18-sample
/// overlap-add with the previous granule. Per ISO 11172-3 §2.4.3.4.6–§2.4.3.4.10.
/// </summary>
/// <remarks>
/// Operates on the 576 spectral coefficients of one granule × channel and
/// the 576-sample overlap state carried across granules. Output is 576
/// time-domain samples organized as 32 subbands × 18 samples.
/// </remarks>
internal sealed class Mp3Hybrid
{
    /// <summary>Per-channel 32×18 overlap buffer (last 18 samples per subband).</summary>
    private readonly float[,] _overlap = new float[32, 18];

    private readonly float[] _imdctOut = new float[36];
    private readonly float[] _tmpShort = new float[12];

    public void Reset()
    {
        for (int i = 0; i < 32; i++)
            for (int j = 0; j < 18; j++)
                _overlap[i, j] = 0;
    }

    /// <summary>
    /// Reorder, antialias, IMDCT, frequency-invert, and overlap-add for one
    /// granule × channel. Output is laid out as <c>output[sb, sample]</c> with
    /// sb in 0..31 and sample in 0..17.
    /// </summary>
    public void Process(
        float[] xr576,
        Mp3GranuleInfo gr,
        float[,] output,
        int[] shortBands)
    {
        // Reorder short blocks. The 18 coefficients per subband are interleaved
        // by (window, sample-in-window); after reorder they are laid out as
        // [w0_sample0..w0_sample5, w1_sample0..w1_sample5, w2_sample0..w2_sample5].
        if (gr.BlockType == 2)
        {
            ReorderShortBlock(xr576, gr.MixedBlockFlag, shortBands);
        }

        // Antialias: butterfly across 31 subband boundaries for long blocks,
        // 1 boundary for mixed-short, 0 boundaries for pure-short.
        if (gr.BlockType != 2 || (gr.BlockType == 2 && gr.MixedBlockFlag))
        {
            int bands = (gr.BlockType == 2 && gr.MixedBlockFlag) ? 1 : 31;
            for (int sb = 0; sb < bands; sb++)
            {
                int off = sb * 18;
                for (int i = 0; i < 8; i++)
                {
                    float a = xr576[off + 17 - i];
                    float b = xr576[off + 18 + i];
                    xr576[off + 17 - i] = a * Mp3Tables.Cs[i] - b * Mp3Tables.Ca[i];
                    xr576[off + 18 + i] = b * Mp3Tables.Cs[i] + a * Mp3Tables.Ca[i];
                }
            }
        }

        // IMDCT + windowing + overlap-add for each of 32 subbands.
        for (int sb = 0; sb < 32; sb++)
        {
            int off = sb * 18;
            int blockType = gr.BlockType;
            bool isShortPortion = blockType == 2 && (!gr.MixedBlockFlag || sb >= 2);

            if (isShortPortion)
            {
                // Three 12-point IMDCTs from 6 coefficients each.
                for (int i = 0; i < 36; i++) _imdctOut[i] = 0;
                for (int w = 0; w < 3; w++)
                {
                    for (int i = 0; i < 12; i++)
                    {
                        float sum = 0;
                        for (int k = 0; k < 6; k++)
                            sum += xr576[off + w * 6 + k] * Mp3Tables.ImdctShort[k, i];
                        _tmpShort[i] = sum * Mp3Tables.WindowShort[i];
                    }
                    // Place into output positions 6 + 6w + 0..11.
                    int basePos = 6 + 6 * w;
                    for (int i = 0; i < 12; i++)
                        _imdctOut[basePos + i] += _tmpShort[i];
                }
            }
            else
            {
                // 36-point IMDCT from 18 coefficients with window per block_type.
                float[] window = blockType switch
                {
                    1 => Mp3Tables.WindowStart,
                    3 => Mp3Tables.WindowStop,
                    _ => Mp3Tables.WindowLong,
                };
                for (int i = 0; i < 36; i++)
                {
                    float sum = 0;
                    for (int k = 0; k < 18; k++)
                        sum += xr576[off + k] * Mp3Tables.ImdctLong[k, i];
                    _imdctOut[i] = sum * window[i];
                }
            }

            // Overlap-add: first 18 samples + previous overlap, save last 18.
            for (int i = 0; i < 18; i++)
            {
                float sample = _imdctOut[i] + _overlap[sb, i];
                // Frequency inversion: every other sample of odd subbands.
                if ((sb & 1) == 1 && (i & 1) == 1)
                    sample = -sample;
                output[sb, i] = sample;
            }
            for (int i = 0; i < 18; i++)
                _overlap[sb, i] = _imdctOut[18 + i];
        }
    }

    private static void ReorderShortBlock(float[] xr576, bool mixed, int[] shortBands)
    {
        // For pure-short, reorder all 32 subbands × 18 coefficients (576 total).
        // For mixed-short, the first 2 subbands (= 36 coefs) remain long-laid-out.
        int start = mixed ? 2 : 0;
        Span<float> scratch = stackalloc float[18];
        for (int sb = start; sb < 32; sb++)
        {
            int off = sb * 18;
            // Source layout: 3 windows interleaved sample-by-sample across 18 coefs.
            // Indices [w0_s0, w1_s0, w2_s0, w0_s1, w1_s1, w2_s1, ...] — 6 samples × 3 windows.
            for (int i = 0; i < 6; i++)
            {
                scratch[i] = xr576[off + 3 * i + 0];
                scratch[6 + i] = xr576[off + 3 * i + 1];
                scratch[12 + i] = xr576[off + 3 * i + 2];
            }
            for (int i = 0; i < 18; i++) xr576[off + i] = scratch[i];
            _ = shortBands;
        }
    }
}
