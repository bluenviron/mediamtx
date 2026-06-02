namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// Huffman decoding of one granule's big_values + count1 regions into the
/// 576-coefficient spectral buffer, per ISO 11172-3 §2.4.2.7.
/// </summary>
internal static class Mp3Huffman
{
    /// <summary>
    /// Decode the spectral coefficients (is[576]) for one granule × channel.
    /// </summary>
    public static void Decode(
        MainDataReader main,
        Mp3SideInfo si,
        int granule,
        int channel,
        int part2Bits,
        int[] is576,
        int[] sfbBoundaries,
        out int rzeroStart)
    {
        Array.Clear(is576, 0, is576.Length);
        var gr = si.Granules[granule, channel];

        int totalBits = gr.Part2_3_Length;
        int part3Bits = totalBits - part2Bits;
        if (part3Bits <= 0)
        {
            rzeroStart = 0;
            return;
        }

        int startBit = main.BitPosition;
        int endBit = startBit + part3Bits;
        int bigValuesCoeffs = gr.BigValues * 2;
        if (bigValuesCoeffs > 576) bigValuesCoeffs = 576;

        // Determine region split boundaries.
        int region1Start, region2Start;
        if (gr.WindowSwitchingFlag && gr.BlockType == 2)
        {
            // For short blocks, region boundaries are fixed.
            region1Start = 36;
            region2Start = 576;
        }
        else
        {
            int r0 = Math.Min(gr.Region0Count + 1, sfbBoundaries.Length - 1);
            int r1 = Math.Min(gr.Region0Count + gr.Region1Count + 2, sfbBoundaries.Length - 1);
            region1Start = sfbBoundaries[r0];
            region2Start = sfbBoundaries[r1];
        }

        int idx = 0;
        while (idx < bigValuesCoeffs && main.BitPosition < endBit)
        {
            int tableSel;
            if (idx < region1Start) tableSel = gr.TableSelect[0];
            else if (idx < region2Start) tableSel = gr.TableSelect[1];
            else tableSel = gr.TableSelect[2];

            int linBits = Mp3HuffmanTables.LinBits[tableSel];

            if (!Mp3HuffmanTables.DecodePair(main, tableSel, out int x, out int y))
            {
                // Unsupported / corrupt table — emit zeros and advance pair count
                // without consuming bits beyond what DecodePair already failed on.
                // To stay within part3 bit budget we conservatively skip ahead.
                main.SetBitPosition(endBit);
                break;
            }

            if (x == 15 && linBits > 0)
                x += (int)main.ReadBits(linBits);
            if (x != 0 && main.ReadBit()) x = -x;
            if (y == 15 && linBits > 0)
                y += (int)main.ReadBits(linBits);
            if (y != 0 && main.ReadBit()) y = -y;

            is576[idx++] = x;
            is576[idx++] = y;
        }

        // count1 region — read until end-of-part3 or 576 coefficients reached.
        while (main.BitPosition < endBit && idx <= 572)
        {
            if (!Mp3HuffmanTables.DecodeQuad(main, gr.Count1TableSelect, out int v, out int w, out int x, out int y))
            {
                main.SetBitPosition(endBit);
                break;
            }
            if (v != 0 && main.ReadBit()) v = -v;
            if (w != 0 && main.ReadBit()) w = -w;
            if (x != 0 && main.ReadBit()) x = -x;
            if (y != 0 && main.ReadBit()) y = -y;
            is576[idx++] = v;
            is576[idx++] = w;
            is576[idx++] = x;
            is576[idx++] = y;
        }

        rzeroStart = idx;

        // Skip stuffing bits (or rewind if we over-read).
        if (main.BitPosition > endBit)
        {
            // Over-read — back up to end boundary. Coefficients beyond rzeroStart
            // remain zero from the initial clear.
            main.SetBitPosition(endBit);
        }
        else if (main.BitPosition < endBit)
        {
            main.SetBitPosition(endBit);
        }
    }
}
