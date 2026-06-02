namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// Layer III requantization per ISO 11172-3 §2.4.3.4.
/// </summary>
internal static class Mp3Requantize
{
    /// <summary>
    /// Requantize one granule × channel's 576 integer spectral lines into floats.
    /// </summary>
    public static void Apply(
        int[] is576,
        float[] xr576,
        Mp3GranuleInfo gr,
        Mp3Scalefactors sf,
        int[] longBands,
        int[] shortBands)
    {
        Array.Clear(xr576, 0, xr576.Length);
        float gainBase = Mp3Tables.GainPow2[gr.GlobalGain + 46]; // 2^((gg-210)/4) ... center the LUT
        // The GainPow2 LUT was filled with 2^((i-256)*0.25). To get 2^((gg-210)/4),
        // we need i such that (i-256)/4 = (gg-210)/4 → i = gg + 46.
        int sfScale = gr.ScalefacScale;

        if (gr.WindowSwitchingFlag && gr.BlockType == 2)
        {
            // Pure short or mixed.
            int idx = 0;
            int sfbStart = 0;
            if (gr.MixedBlockFlag)
            {
                // First 8 long sfbs (or as far as longBands index goes for mixed).
                int mixedLongSfbs = 8;
                int longLimit = Math.Min(longBands[mixedLongSfbs], 576);
                for (int sfb = 0; sfb < mixedLongSfbs && longBands[sfb + 1] <= 576; sfb++)
                {
                    int from = longBands[sfb];
                    int to = longBands[sfb + 1];
                    int sfVal = sf.L[sfb] + (gr.PreFlag ? Mp3Tables.PreTab[sfb] : 0);
                    float scaleAtt = (float)Math.Pow(2.0, -0.5 * (1 + sfScale) * sfVal);
                    float gain = gainBase * scaleAtt;
                    for (int i = from; i < to; i++)
                    {
                        int v = is576[i];
                        if (v == 0) continue;
                        int abs = v < 0 ? -v : v;
                        float mag = Mp3Tables.Pow43Of(abs) * gain;
                        xr576[i] = v < 0 ? -mag : mag;
                    }
                    idx = to;
                }
                sfbStart = 3; // mixed shorts begin at short sfb 3
                _ = longLimit;
            }

            // Short sfbs.
            int win = 0;
            for (int sfb = sfbStart; sfb < 12; sfb++)
            {
                int width = shortBands[sfb + 1] - shortBands[sfb];
                for (int w = 0; w < 3; w++)
                {
                    int subgain = gr.SubblockGain[w];
                    int sfVal = sf.S[sfb, w];
                    float scaleAtt = (float)Math.Pow(2.0, -0.5 * (1 + sfScale) * sfVal);
                    float gain = gainBase * Mp3Tables.SubblockGainPow2[subgain] * scaleAtt;
                    for (int k = 0; k < width; k++)
                    {
                        if (idx >= 576) return;
                        int v = is576[idx];
                        if (v != 0)
                        {
                            int abs = v < 0 ? -v : v;
                            float mag = Mp3Tables.Pow43Of(abs) * gain;
                            xr576[idx] = v < 0 ? -mag : mag;
                        }
                        idx++;
                    }
                    _ = win;
                }
            }
        }
        else
        {
            // Long block (normal, start, stop).
            for (int sfb = 0; sfb < 22; sfb++)
            {
                int from = longBands[sfb];
                int to = longBands[sfb + 1];
                if (to > 576) to = 576;
                int sfVal = sf.L[sfb] + (gr.PreFlag ? Mp3Tables.PreTab[sfb] : 0);
                float scaleAtt = (float)Math.Pow(2.0, -0.5 * (1 + sfScale) * sfVal);
                float gain = gainBase * scaleAtt;
                for (int i = from; i < to; i++)
                {
                    int v = is576[i];
                    if (v == 0) continue;
                    int abs = v < 0 ? -v : v;
                    float mag = Mp3Tables.Pow43Of(abs) * gain;
                    xr576[i] = v < 0 ? -mag : mag;
                }
                if (to >= 576) break;
            }
        }
    }
}
