namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// MS (mid/side) and intensity stereo decoding per ISO 11172-3 §2.4.3.4.5.
/// </summary>
internal static class Mp3Stereo
{
    /// <summary>
    /// Apply MS stereo (mode_extension bit 1 set) and/or intensity stereo
    /// (mode_extension bit 0 set) on the two channels' requantized spectra.
    /// </summary>
    /// <remarks>
    /// For silence the per-coefficient transforms reduce to identities (any
    /// linear combination of zeros is zero), so this is safe to apply
    /// unconditionally on zero-filled input.
    /// </remarks>
    public static void Apply(
        float[] xrLeft,
        float[] xrRight,
        Mp3GranuleInfo grRight,
        Mp3Scalefactors sfRight,
        int modeExtension,
        bool mpeg2Lsf,
        int[] longBands,
        int[] shortBands)
    {
        bool ms = (modeExtension & 0x2) != 0;
        bool intensity = (modeExtension & 0x1) != 0;

        if (!ms && !intensity) return;

        // Find the intensity-stereo boundary in the right channel: last
        // non-zero coefficient, rounded up to an sfb boundary.
        int rzero = 576;
        while (rzero > 0 && xrRight[rzero - 1] == 0) rzero--;

        if (ms)
        {
            // MS for sfbs below the boundary; apply over coefficients 0..msEnd-1.
            // For non-intensity frames, msEnd = 576.
            int msEnd = intensity ? rzero : 576;
            const float Inv = 0.70710677f; // 1/sqrt(2)
            for (int i = 0; i < msEnd; i++)
            {
                float l = xrLeft[i];
                float r = xrRight[i];
                xrLeft[i] = (l + r) * Inv;
                xrRight[i] = (l - r) * Inv;
            }
        }

        if (intensity)
        {
            // Coefficients in [rzero..576) are intensity-coded: distribute the
            // left-channel spectrum into both channels using is_pos ratios.
            // For silent right channel the distribution copies left → both.
            // Detailed bit-exact ISO intensity decode requires per-sfb is_pos
            // lookup against either MPEG-1 tangent table or MPEG-2 LSF table.
            // The implementation here handles the common (and silent) cases.
            if (!mpeg2Lsf)
            {
                ApplyMpeg1Intensity(xrLeft, xrRight, grRight, sfRight, rzero, longBands, shortBands);
            }
            else
            {
                ApplyMpeg2LsfIntensity(xrLeft, xrRight, grRight, sfRight, rzero, longBands, shortBands);
            }
        }
    }

    private static void ApplyMpeg1Intensity(
        float[] xrLeft,
        float[] xrRight,
        Mp3GranuleInfo gr,
        Mp3Scalefactors sf,
        int rzero,
        int[] longBands,
        int[] shortBands)
    {
        if (gr.WindowSwitchingFlag && gr.BlockType == 2)
        {
            int sfbStart = gr.MixedBlockFlag ? 3 : 0;
            for (int sfb = sfbStart; sfb < 12; sfb++)
            {
                for (int w = 0; w < 3; w++)
                {
                    int isPos = sf.S[sfb, w];
                    if (isPos == 7) continue;
                    float ratio = Mp3Tables.IsRatioTan[isPos];
                    float kL = ratio / (1 + ratio);
                    float kR = 1 / (1 + ratio);
                    int from = shortBands[sfb] * 3 + w * (shortBands[sfb + 1] - shortBands[sfb]);
                    int width = shortBands[sfb + 1] - shortBands[sfb];
                    for (int k = 0; k < width; k++)
                    {
                        int idx = from + k;
                        if (idx < rzero || idx >= 576) continue;
                        float v = xrLeft[idx];
                        xrLeft[idx] = v * kL;
                        xrRight[idx] = v * kR;
                    }
                }
            }
        }
        else
        {
            for (int sfb = 0; sfb < 22; sfb++)
            {
                int from = longBands[sfb];
                int to = longBands[sfb + 1];
                if (to <= rzero) continue;
                int isPos = sf.L[sfb];
                if (isPos == 7) continue;
                float ratio = Mp3Tables.IsRatioTan[isPos];
                float kL = ratio / (1 + ratio);
                float kR = 1 / (1 + ratio);
                int start = Math.Max(from, rzero);
                for (int i = start; i < to && i < 576; i++)
                {
                    float v = xrLeft[i];
                    xrLeft[i] = v * kL;
                    xrRight[i] = v * kR;
                }
            }
        }
    }

    private static void ApplyMpeg2LsfIntensity(
        float[] xrLeft,
        float[] xrRight,
        Mp3GranuleInfo gr,
        Mp3Scalefactors sf,
        int rzero,
        int[] longBands,
        int[] shortBands)
    {
        // MPEG-2 LSF intensity factor: from the right channel's scalefactor
        // (which is reused as is_pos), look up LsfIsRatio. For even is_pos,
        // (kL, kR) = (LsfIsRatio[is_pos/2], 1); for odd, (1, LsfIsRatio[is_pos/2]).
        if (gr.WindowSwitchingFlag && gr.BlockType == 2)
        {
            int sfbStart = gr.MixedBlockFlag ? 3 : 0;
            for (int sfb = sfbStart; sfb < 12; sfb++)
            {
                for (int w = 0; w < 3; w++)
                {
                    int isPos = sf.S[sfb, w];
                    float k = Mp3Tables.LsfIsRatio[Math.Min(isPos >> 1, 31)];
                    float kL = (isPos & 1) == 0 ? k : 1f;
                    float kR = (isPos & 1) == 0 ? 1f : k;
                    int width = shortBands[sfb + 1] - shortBands[sfb];
                    int from = shortBands[sfb] * 3 + w * width;
                    for (int i = from; i < from + width && i < 576; i++)
                    {
                        if (i < rzero) continue;
                        float v = xrLeft[i];
                        xrLeft[i] = v * kL;
                        xrRight[i] = v * kR;
                    }
                }
            }
        }
        else
        {
            for (int sfb = 0; sfb < 22; sfb++)
            {
                int from = longBands[sfb];
                int to = longBands[sfb + 1];
                if (to <= rzero) continue;
                int isPos = sf.L[sfb];
                float k = Mp3Tables.LsfIsRatio[Math.Min(isPos >> 1, 31)];
                float kL = (isPos & 1) == 0 ? k : 1f;
                float kR = (isPos & 1) == 0 ? 1f : k;
                int start = Math.Max(from, rzero);
                for (int i = start; i < to && i < 576; i++)
                {
                    float v = xrLeft[i];
                    xrLeft[i] = v * kL;
                    xrRight[i] = v * kR;
                }
            }
        }
    }
}
