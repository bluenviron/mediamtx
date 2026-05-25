namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis I residue decoder (spec §8.6). Residue carries the per-frequency-bin
/// residual after the floor curve is subtracted, encoded as a partitioned VQ
/// stream with up to 8 cascade passes per partition.
///
/// Three residue types share the same partition/classification framing but
/// differ in how the decoded vectors are placed into the per-channel output:
/// <list type="bullet">
///   <item><c>type 0</c>: interleaved scalar placement — element <c>j</c> of a
///     decoded length-<c>D</c> vector lands at offset <c>i + j * step</c>.</item>
///   <item><c>type 1</c>: sequential placement — the vector overwrites
///     consecutive samples at <c>offset..offset+D</c>.</item>
///   <item><c>type 2</c>: all participating channels are first round-robin
///     interleaved into a single virtual stream that is decoded as type 1,
///     then de-interleaved back into the channels.</item>
/// </list>
/// </summary>
internal static class VorbisResidue
{
    /// <summary>
    /// Decode one residue group and accumulate into <paramref name="vectors"/>.
    /// Each channel's vector has length <paramref name="n"/> (= blocksize/2).
    /// </summary>
    /// <param name="r">Bit reader positioned at the start of the residue stream.</param>
    /// <param name="res">Residue configuration from the setup header.</param>
    /// <param name="books">Codebooks defined in the setup header.</param>
    /// <param name="n">Half-block size (frequency bin count per channel).</param>
    /// <param name="doNotDecode">Per-channel mask — channel skipped when true.</param>
    /// <param name="vectors">Per-channel output, length <paramref name="n"/>.
    ///   On entry the entries should be zero (or pre-existing partial residue
    ///   from a previous group); this method <em>accumulates</em>.</param>
    public static void Decode(
        ref VorbisBitReader r,
        VorbisSetup.Residue res,
        VorbisCodebook[] books,
        int n,
        scoped ReadOnlySpan<bool> doNotDecode,
        float[][] vectors)
    {
        if (res.Type == 2)
        {
            DecodeType2(ref r, res, books, n, doNotDecode, vectors);
            return;
        }
        DecodeType01(ref r, res, books, n, doNotDecode, vectors);
    }

    private static void DecodeType01(
        ref VorbisBitReader r,
        VorbisSetup.Residue res,
        VorbisCodebook[] books,
        int n,
        scoped ReadOnlySpan<bool> doNotDecode,
        float[][] vectors)
    {
        int channels = doNotDecode.Length;
        int actualEnd = Math.Min(res.End, n);
        int actualBegin = Math.Min(res.Begin, actualEnd);
        int partitionSize = res.PartitionSize;
        int partitionsToRead = (actualEnd - actualBegin) / partitionSize;
        if (partitionsToRead <= 0) return;

        var classBook = books[res.ClassBook];
        int classwordsPerCodeword = classBook.Dimensions;
        int classifications = res.Classifications;

        // classifications[ch][i] — selected codebook class for partition i, channel ch.
        var classes = new int[channels][];
        for (int ch = 0; ch < channels; ch++)
        {
            if (doNotDecode[ch]) continue;
            classes[ch] = new int[partitionsToRead + classwordsPerCodeword];
        }

        for (int pass = 0; pass < 8; pass++)
        {
            int partitionCount = 0;
            while (partitionCount < partitionsToRead)
            {
                if (pass == 0)
                {
                    for (int ch = 0; ch < channels; ch++)
                    {
                        if (doNotDecode[ch]) continue;
                        int temp = classBook.DecodeScalar(ref r);
                        if (temp < 0) return;
                        for (int i = classwordsPerCodeword - 1; i >= 0; i--)
                        {
                            classes[ch][partitionCount + i] = temp % classifications;
                            temp /= classifications;
                        }
                    }
                }

                for (int i = 0; i < classwordsPerCodeword && partitionCount < partitionsToRead; i++)
                {
                    for (int ch = 0; ch < channels; ch++)
                    {
                        if (doNotDecode[ch]) continue;
                        int vqclass = classes[ch][partitionCount];
                        int vqbook = res.Books[vqclass, pass];
                        if (vqbook >= 0)
                        {
                            int offset = actualBegin + partitionCount * partitionSize;
                            if (res.Type == 0)
                            {
                                ResidueType0Decode(ref r, books[vqbook], vectors[ch], offset, partitionSize);
                            }
                            else
                            {
                                ResidueType1Decode(ref r, books[vqbook], vectors[ch], offset, partitionSize);
                            }
                        }
                    }
                    partitionCount++;
                }
            }
        }
    }

    private static void DecodeType2(
        ref VorbisBitReader r,
        VorbisSetup.Residue res,
        VorbisCodebook[] books,
        int n,
        scoped ReadOnlySpan<bool> doNotDecode,
        float[][] vectors)
    {
        int channels = doNotDecode.Length;
        bool anyDecoded = false;
        for (int ch = 0; ch < channels; ch++)
        {
            if (!doNotDecode[ch]) { anyDecoded = true; break; }
        }
        if (!anyDecoded) return;

        // Build a single virtual channel of length channels * n by round-robin
        // interleaving. Spec §8.6.2 paragraph "type 2":
        //   "all channels are first interleaved sample-by-sample into one
        //    vector before residue is decoded type-1 style on the result".
        int virtN = channels * n;
        var virt = new float[virtN];

        // Build a synthetic "single channel" residue with virtual length virtN.
        // The doNotDecode mask collapses to a single false (we decode the whole
        // virtual channel) unless ALL channels are skipped (handled above).
        Span<bool> virtMask = stackalloc bool[1] { false };
        var virtVectors = new float[1][];
        virtVectors[0] = virt;

        DecodeType01Virtual(ref r, res, books, virtN, virtMask, virtVectors);

        // De-interleave back into per-channel vectors. Channels with
        // doNotDecode=true would have non-zero virt samples (decoded as part
        // of the virtual stream) — they are silently discarded.
        for (int sample = 0; sample < n; sample++)
        {
            for (int ch = 0; ch < channels; ch++)
            {
                if (!doNotDecode[ch])
                {
                    vectors[ch][sample] += virt[sample * channels + ch];
                }
            }
        }
    }

    private static void DecodeType01Virtual(
        ref VorbisBitReader r,
        VorbisSetup.Residue res,
        VorbisCodebook[] books,
        int n,
        scoped ReadOnlySpan<bool> doNotDecode,
        float[][] vectors)
    {
        // Functionally identical to DecodeType01 but type is forced to 1
        // (sequential placement) per the type-2 spec.
        int channels = doNotDecode.Length;
        int actualEnd = Math.Min(res.End, n);
        int actualBegin = Math.Min(res.Begin, actualEnd);
        int partitionSize = res.PartitionSize;
        int partitionsToRead = (actualEnd - actualBegin) / partitionSize;
        if (partitionsToRead <= 0) return;

        var classBook = books[res.ClassBook];
        int classwordsPerCodeword = classBook.Dimensions;
        int classifications = res.Classifications;

        var classes = new int[channels][];
        for (int ch = 0; ch < channels; ch++)
        {
            if (doNotDecode[ch]) continue;
            classes[ch] = new int[partitionsToRead + classwordsPerCodeword];
        }

        for (int pass = 0; pass < 8; pass++)
        {
            int partitionCount = 0;
            while (partitionCount < partitionsToRead)
            {
                if (pass == 0)
                {
                    for (int ch = 0; ch < channels; ch++)
                    {
                        if (doNotDecode[ch]) continue;
                        int temp = classBook.DecodeScalar(ref r);
                        if (temp < 0) return;
                        for (int i = classwordsPerCodeword - 1; i >= 0; i--)
                        {
                            classes[ch][partitionCount + i] = temp % classifications;
                            temp /= classifications;
                        }
                    }
                }

                for (int i = 0; i < classwordsPerCodeword && partitionCount < partitionsToRead; i++)
                {
                    for (int ch = 0; ch < channels; ch++)
                    {
                        if (doNotDecode[ch]) continue;
                        int vqclass = classes[ch][partitionCount];
                        int vqbook = res.Books[vqclass, pass];
                        if (vqbook >= 0)
                        {
                            int offset = actualBegin + partitionCount * partitionSize;
                            ResidueType1Decode(ref r, books[vqbook], vectors[ch], offset, partitionSize);
                        }
                    }
                    partitionCount++;
                }
            }
        }
    }

    private static void ResidueType0Decode(
        ref VorbisBitReader r,
        VorbisCodebook book,
        float[] vector,
        int offset,
        int partitionSize)
    {
        int dims = book.Dimensions;
        if (dims <= 0) return;
        int step = partitionSize / dims;
        Span<float> entryBuf = dims <= 64 ? stackalloc float[dims] : new float[dims];
        for (int i = 0; i < step; i++)
        {
            int entry = book.DecodeVector(ref r, entryBuf);
            if (entry < 0) return;
            for (int j = 0; j < dims; j++)
            {
                int idx = offset + i + j * step;
                if ((uint)idx < (uint)vector.Length) vector[idx] += entryBuf[j];
            }
        }
    }

    private static void ResidueType1Decode(
        ref VorbisBitReader r,
        VorbisCodebook book,
        float[] vector,
        int offset,
        int partitionSize)
    {
        int dims = book.Dimensions;
        if (dims <= 0) return;
        int i = 0;
        Span<float> entryBuf = dims <= 64 ? stackalloc float[dims] : new float[dims];
        while (i < partitionSize)
        {
            int entry = book.DecodeVector(ref r, entryBuf);
            if (entry < 0) return;
            for (int j = 0; j < dims; j++)
            {
                int idx = offset + i + j;
                if ((uint)idx < (uint)vector.Length) vector[idx] += entryBuf[j];
            }
            i += dims;
        }
    }
}
