namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Two-pass optimised Huffman builder per T.81 Annex K.2. Pass 1
/// gathers a frequency histogram of every symbol the encoder would
/// emit; pass 2 derives a canonical minimum-redundancy code, then
/// enforces the JPEG 16-bit maximum code length using the standard
/// "fix bits" adjustment described in K.2.
/// </summary>
/// <remarks>
/// The Huffman tree construction follows the classic algorithm from
/// Pennebaker &amp; Mitchell (1992), <i>JPEG: Still Image Data
/// Compression Standard</i>, §13.2. The output is a pair of arrays in
/// the format expected by <see cref="JpegEncoderHuffmanTable"/> and by
/// the DHT segment writer (counts of codes per length 1..16, plus the
/// symbol values in ascending code-length order).
/// </remarks>
public static class JpegOptimisedHuffman
{
    /// <summary>
    /// Build a (BITS, HUFFVAL) pair from a frequency histogram.
    /// </summary>
    /// <param name="freq">
    /// Length-257 array of symbol frequencies. Index 256 is a virtual
    /// "reserved" symbol that always has frequency 1 — its purpose is
    /// to prevent any real symbol from being assigned the all-ones
    /// code (which JPEG forbids).
    /// </param>
    public static (byte[] BitsCounts, byte[] Values) Build(int[] freq)
    {
        ArgumentNullException.ThrowIfNull(freq);
        if (freq.Length != 257) throw new ArgumentException("freq must have 257 entries.", nameof(freq));

        // Local working buffers (length 257).
        var counts = new int[33];   // counts[i] = number of codes of length i (lengths up to 32)
        var others = new int[257];  // tree link list
        var codeSizes = new int[257];
        var freqLocal = new int[257];
        Array.Copy(freq, freqLocal, 257);
        freqLocal[256] = 1;
        for (int i = 0; i < 257; i++) others[i] = -1;

        // ---- Build the Huffman tree (K.2 figure K.1). ----
        while (true)
        {
            int c1 = -1, c2 = -1;
            long v1 = long.MaxValue, v2 = long.MaxValue;
            for (int i = 0; i < 257; i++)
            {
                if (freqLocal[i] == 0) continue;
                if (freqLocal[i] <= v1)
                {
                    v2 = v1; c2 = c1;
                    v1 = freqLocal[i]; c1 = i;
                }
                else if (freqLocal[i] <= v2)
                {
                    v2 = freqLocal[i]; c2 = i;
                }
            }
            if (c2 < 0) break;

            freqLocal[c1] += freqLocal[c2];
            freqLocal[c2] = 0;

            codeSizes[c1]++;
            while (others[c1] >= 0)
            {
                c1 = others[c1];
                codeSizes[c1]++;
            }
            others[c1] = c2;

            codeSizes[c2]++;
            while (others[c2] >= 0)
            {
                c2 = others[c2];
                codeSizes[c2]++;
            }
        }

        // ---- Count codes per length. ----
        for (int i = 0; i < 257; i++)
        {
            int len = codeSizes[i];
            if (len > 0)
            {
                if (len >= counts.Length)
                {
                    Array.Resize(ref counts, len + 1);
                }
                counts[len]++;
            }
        }

        // ---- Limit max code length to 16 bits (K.2 §"Procedure to limit code lengths"). ----
        for (int i = counts.Length - 1; i > 16; i--)
        {
            while (counts[i] > 0)
            {
                int j = i - 2;
                while (j > 0 && counts[j] == 0) j--;
                if (j == 0) break;
                counts[i] -= 2;
                counts[i - 1]++;
                counts[j + 1] += 2;
                counts[j]--;
            }
        }
        // Remove the reserved symbol (longest code).
        for (int i = counts.Length - 1; i >= 0; i--)
        {
            if (counts[i] > 0) { counts[i]--; break; }
        }

        var bits = new byte[16];
        for (int i = 1; i <= 16 && i < counts.Length; i++)
        {
            bits[i - 1] = (byte)counts[i];
        }

        int total = 0;
        for (int i = 0; i < 16; i++) total += bits[i];

        // ---- Emit symbol values in ascending code-length order, then ascending value. ----
        var values = new byte[total];
        int k = 0;
        for (int len = 1; len <= 16; len++)
        {
            for (int v = 0; v < 256; v++)
            {
                if (codeSizes[v] == len)
                {
                    values[k++] = (byte)v;
                }
            }
        }
        if (k != total)
        {
            throw new InvalidOperationException("Optimised Huffman: size/count mismatch after length-limiting.");
        }

        return (bits, values);
    }
}
