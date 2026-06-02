namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// MPEG-1 Audio Layer III scalefactor band structure for one granule, one channel.
/// Per ISO 11172-3 §2.4.2.7, long blocks have up to 22 scalefactors (one per
/// scalefactor band) and short blocks have up to 12 × 3 (per window).
/// </summary>
internal sealed class Mp3Scalefactors
{
    /// <summary>Long-block scalefactors, indexed by scalefactor band [0..21].</summary>
    public readonly int[] L = new int[23]; // [0..21] used; index 22 zero-padding

    /// <summary>Short-block scalefactors [sfb 0..12, window 0..2].</summary>
    public readonly int[,] S = new int[13, 3];

    public void Clear()
    {
        Array.Clear(L, 0, L.Length);
        for (int i = 0; i < S.GetLength(0); i++)
            for (int j = 0; j < S.GetLength(1); j++)
                S[i, j] = 0;
    }

    public void CopyFrom(Mp3Scalefactors other)
    {
        Buffer.BlockCopy(other.L, 0, L, 0, L.Length * sizeof(int));
        for (int i = 0; i < S.GetLength(0); i++)
            for (int j = 0; j < S.GetLength(1); j++)
                S[i, j] = other.S[i, j];
    }
}

/// <summary>
/// Scalefactor decoders for MPEG-1 and MPEG-2 LSF / MPEG-2.5 Layer III, per
/// ISO 11172-3 §2.4.2.7 and ISO 13818-3 §2.4.3.2.
/// </summary>
internal static class Mp3ScalefactorDecoder
{
    /// <summary>
    /// Decode MPEG-1 scalefactors for one granule × channel. Returns the number
    /// of main-data bits consumed (part2_length).
    /// </summary>
    public static int DecodeMpeg1(
        MainDataReader main,
        Mp3SideInfo si,
        int granule,
        int channel,
        Mp3Scalefactors sf,
        Mp3Scalefactors? prevGranule)
    {
        int startBit = main.BitPosition;
        var gr = si.Granules[granule, channel];
        int sc = gr.ScalefacCompress;
        int slen1 = Mp3Tables.Slen[sc, 0];
        int slen2 = Mp3Tables.Slen[sc, 1];

        if (gr.WindowSwitchingFlag && gr.BlockType == 2)
        {
            // Short or mixed: SCFSI ignored.
            if (gr.MixedBlockFlag)
            {
                // First 8 long sfbs use slen1.
                for (int sfb = 0; sfb < 8; sfb++)
                    sf.L[sfb] = (int)main.ReadBits(slen1);

                // Then short sfbs 3..5 use slen1 (3 windows each).
                for (int sfb = 3; sfb < 6; sfb++)
                    for (int w = 0; w < 3; w++)
                        sf.S[sfb, w] = (int)main.ReadBits(slen1);

                // Then short sfbs 6..11 use slen2.
                for (int sfb = 6; sfb < 12; sfb++)
                    for (int w = 0; w < 3; w++)
                        sf.S[sfb, w] = (int)main.ReadBits(slen2);
            }
            else
            {
                // Pure short: sfbs 0..5 use slen1, 6..11 use slen2 (3 windows each).
                for (int sfb = 0; sfb < 6; sfb++)
                    for (int w = 0; w < 3; w++)
                        sf.S[sfb, w] = (int)main.ReadBits(slen1);

                for (int sfb = 6; sfb < 12; sfb++)
                    for (int w = 0; w < 3; w++)
                        sf.S[sfb, w] = (int)main.ReadBits(slen2);
            }
        }
        else
        {
            // Long block (normal, start, or stop): consult SCFSI on granule 1.
            int[,] groups =
            {
                { 0, 6 }, { 6, 11 }, { 11, 16 }, { 16, 21 }
            };
            for (int g = 0; g < 4; g++)
            {
                int from = groups[g, 0];
                int to = groups[g, 1];
                int slen = g < 2 ? slen1 : slen2;
                bool reuse = granule == 1 && si.Scfsi[channel, g] == 1 && prevGranule != null;
                for (int sfb = from; sfb < to; sfb++)
                {
                    if (reuse)
                        sf.L[sfb] = prevGranule!.L[sfb];
                    else
                        sf.L[sfb] = (int)main.ReadBits(slen);
                }
            }
            sf.L[21] = 0;
            sf.L[22] = 0;
        }

        return main.BitPosition - startBit;
    }

    /// <summary>
    /// Decode MPEG-2 LSF / MPEG-2.5 scalefactors for one granule × channel.
    /// Returns the number of main-data bits consumed.
    /// </summary>
    /// <remarks>
    /// Per ISO 13818-3 §2.4.3.2. The <paramref name="isIntensityChannel"/>
    /// flag is true when this is the right channel in an intensity-stereo
    /// frame (channel_mode joint with mode_extension bit 0 set).
    /// </remarks>
    public static int DecodeMpeg2(
        MainDataReader main,
        Mp3GranuleInfo gr,
        bool isIntensityChannel,
        Mp3Scalefactors sf,
        out bool preFlag)
    {
        int startBit = main.BitPosition;
        int sc = gr.ScalefacCompress;
        preFlag = false;

        int blockClass;
        if (!isIntensityChannel)
        {
            if (sc < 400) { blockClass = 0; }
            else if (sc < 500) { blockClass = 1; sc -= 400; }
            else { blockClass = 2; sc -= 500; preFlag = true; }
        }
        else
        {
            int isScale = sc & 1;
            sc >>= 1;
            if (sc < 180) { blockClass = 3; }
            else if (sc < 244) { blockClass = 4; sc -= 180; }
            else { blockClass = 5; sc -= 244; }
            _ = isScale;
        }

        int slen1, slen2, slen3, slen4;
        switch (blockClass)
        {
            case 0:
            case 3:
                slen1 = (sc >> 4) / 5;
                slen2 = (sc >> 4) % 5;
                slen3 = (sc & 15) >> 2;
                slen4 = sc & 3;
                break;
            case 1:
            case 4:
                slen1 = (sc >> 2) / 5;
                slen2 = (sc >> 2) % 5;
                slen3 = (sc & 3) >> 1;
                slen4 = sc & 1;
                break;
            default: // 2 or 5
                slen1 = sc / 3;
                slen2 = sc % 3;
                slen3 = 0;
                slen4 = 0;
                break;
        }

        // Map (blockClass, block_type, mixed) to nr_sfb widths per ISO 13818-3 Table B.1.
        int classIdx = blockClass;
        int btIdx = gr.BlockType == 2 ? 1 : 0;
        int mixIdx = gr.MixedBlockFlag ? 1 : 0;
        int[] nrSfb = LsfNrSfb[classIdx, btIdx, mixIdx];

        int[] slens = { slen1, slen2, slen3, slen4 };

        if (gr.BlockType == 2)
        {
            // Short or mixed.
            if (gr.MixedBlockFlag)
            {
                // First nrSfb[0] are long-block (single value per band).
                int sfbLong = 0;
                int count = nrSfb[0];
                for (int i = 0; i < count; i++, sfbLong++)
                    sf.L[sfbLong] = (int)main.ReadBits(slens[0]);

                // Remaining groups are short-block (3 windows per band).
                int sfbShort = 3; // mixed starts shorts at sfb 3
                for (int group = 1; group < 4; group++)
                {
                    int n = nrSfb[group];
                    int sl = slens[group];
                    for (int i = 0; i < n; i++, sfbShort++)
                    {
                        if (sfbShort >= 12) break;
                        for (int w = 0; w < 3; w++)
                            sf.S[sfbShort, w] = sl > 0 ? (int)main.ReadBits(sl) : 0;
                    }
                }
            }
            else
            {
                int sfbShort = 0;
                for (int group = 0; group < 4; group++)
                {
                    int n = nrSfb[group];
                    int sl = slens[group];
                    for (int i = 0; i < n; i++, sfbShort++)
                    {
                        if (sfbShort >= 12) break;
                        for (int w = 0; w < 3; w++)
                            sf.S[sfbShort, w] = sl > 0 ? (int)main.ReadBits(sl) : 0;
                    }
                }
            }
        }
        else
        {
            int sfb = 0;
            for (int group = 0; group < 4; group++)
            {
                int n = nrSfb[group];
                int sl = slens[group];
                for (int i = 0; i < n; i++, sfb++)
                {
                    if (sfb >= 22) break;
                    sf.L[sfb] = sl > 0 ? (int)main.ReadBits(sl) : 0;
                }
            }
            sf.L[21] = 0;
            sf.L[22] = 0;
        }

        return main.BitPosition - startBit;
    }

    /// <summary>
    /// LSF nr_sfb widths per ISO 13818-3 Table B.1.
    /// Indexed by [block_class 0..5, block_type 0=long 1=short, mixed_flag 0..1].
    /// </summary>
    private static readonly int[,,][] LsfNrSfb =
    {
        // class 0 (normal, sc<400)
        { { new[] { 6, 5, 5, 5 }, new[] { 6, 5, 5, 5 } }, { new[] { 9, 9, 9, 9 }, new[] { 6, 9, 9, 9 } } },
        // class 1 (normal, sc<500)
        { { new[] { 6, 5, 7, 3 }, new[] { 6, 5, 7, 3 } }, { new[] { 9, 9, 12, 6 }, new[] { 6, 9, 12, 6 } } },
        // class 2 (normal, sc<512, preflag=1)
        { { new[] { 11, 10, 0, 0 }, new[] { 11, 10, 0, 0 } }, { new[] { 18, 18, 0, 0 }, new[] { 15, 18, 0, 0 } } },
        // class 3 (IS right, sc<180)
        { { new[] { 7, 7, 7, 0 }, new[] { 7, 7, 7, 0 } }, { new[] { 12, 12, 12, 0 }, new[] { 6, 15, 12, 0 } } },
        // class 4 (IS right, sc<244)
        { { new[] { 6, 6, 6, 3 }, new[] { 6, 6, 6, 3 } }, { new[] { 12, 9, 9, 6 }, new[] { 6, 12, 9, 6 } } },
        // class 5 (IS right, sc rest)
        { { new[] { 8, 8, 5, 0 }, new[] { 8, 8, 5, 0 } }, { new[] { 15, 12, 9, 0 }, new[] { 6, 18, 9, 0 } } },
    };
}
