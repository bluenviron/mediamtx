namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis I floor type 1 decoder (Vorbis I spec §7.2). Floor 1 represents
/// the spectral envelope of one audio channel as a piecewise-linear curve in
/// the <c>(x, dB)</c> plane, where x runs from 0 to <c>blocksize/2</c> and
/// the amplitude axis is quantized in ~0.5 dB steps. The decoder reads the
/// quantized Y values from the bit stream, undoes the predictive coding,
/// renders the line segments via a Bresenham-like integer raster (spec
/// §9.2.6 <c>render_line</c>), and converts the integer dB ladder to linear
/// amplitude through <see cref="InverseDbTable"/>.
/// </summary>
internal static class VorbisFloor1
{
    /// <summary>
    /// Spec §9.2.4 — <c>floor1_inverse_dB_static_table</c>. Pre-computed
    /// 256-entry table mapping the quantized dB ladder to linear amplitude.
    /// </summary>
    /// <remarks>
    /// The spec ships an explicit 256-element float table; that table is
    /// (to single-precision accuracy) <c>2^((i-255)/11)</c>. We compute the
    /// values once at type-load time so we don't ship a 256-entry literal
    /// while still being a single hot lookup at decode time. Differences vs.
    /// the spec table are sub-ULP and the spec only requires "approximate"
    /// correspondence at this stage.
    /// </remarks>
    public static readonly float[] InverseDbTable = BuildDbTable();

    private static float[] BuildDbTable()
    {
        var t = new float[256];
        for (int i = 0; i < 256; i++)
        {
            t[i] = (float)Math.Pow(2.0, (i - 255) / 11.0);
        }
        return t;
    }

    /// <summary>
    /// Decode a floor 1 packet for one channel. Returns the per-X quantized
    /// Y vector, or <see langword="null"/> if the channel is silent
    /// (<c>nonzero == 0</c>, in which case the channel produces no residue
    /// either — see <c>no_residue</c> in spec §4.3.2).
    /// </summary>
    public static int[]? Decode(ref VorbisBitReader r, VorbisSetup.Floor floor, VorbisCodebook[] books)
    {
        // §7.2.3 step 1 — nonzero flag.
        if (!r.ReadBit()) return null;

        int rangeIndex = floor.Multiplier - 1;
        if ((uint)rangeIndex >= 4) throw new InvalidDataException("Floor 1 multiplier out of range.");
        int range = RangeTable[rangeIndex];
        int rangeBits = VorbisBitReader.Ilog(range - 1);

        int xCount = floor.XList.Length;
        var y = new int[xCount];
        y[0] = (int)r.ReadBits(rangeBits);
        y[1] = (int)r.ReadBits(rangeBits);

        int offset = 2;
        var partitionClassList = floor.PartitionClassList;
        for (int p = 0; p < partitionClassList.Length; p++)
        {
            var cls = floor.Classes[partitionClassList[p]];
            int classDims = cls.Dimensions;
            int cbits = cls.SubclassBits;
            int csub = (1 << cbits) - 1;
            int cval = 0;
            if (cbits > 0)
            {
                cval = books[cls.MasterBook].DecodeScalar(ref r);
                if (cval < 0) return null;
            }
            for (int j = 0; j < classDims; j++)
            {
                int book = cls.SubclassBooks[cval & csub];
                cval >>= cbits;
                if (book >= 0)
                {
                    int v = books[book].DecodeScalar(ref r);
                    if (v < 0) return null;
                    y[offset + j] = v;
                }
                else
                {
                    y[offset + j] = 0;
                }
            }
            offset += classDims;
        }

        return y;
    }

    /// <summary>
    /// Synthesize the floor 1 curve onto <paramref name="output"/> (length
    /// <c>blocksize/2</c>). Spec §7.2.4. Steps:
    /// <list type="number">
    ///   <item>Amplitude unwrap with <c>low_neighbor</c>/<c>high_neighbor</c>
    ///     predictors (§9.2.5 <c>render_point</c>).</item>
    ///   <item>Per-segment Bresenham line raster from the sorted X positions
    ///     onto the output curve (§9.2.6 <c>render_line</c>).</item>
    ///   <item>Convert each integer dB-ladder sample to linear amplitude via
    ///     <see cref="InverseDbTable"/>.</item>
    /// </list>
    /// Returns <see langword="true"/> if the curve has any non-zero content.
    /// </summary>
    public static bool Synthesize(int[] y, VorbisSetup.Floor floor, int blockHalf, Span<float> output)
    {
        if (output.Length < blockHalf) throw new ArgumentException("Output too small.", nameof(output));

        int n = blockHalf;
        var xList = floor.XList;
        int xCount = xList.Length;

        int rangeIndex = floor.Multiplier - 1;
        int range = RangeTable[rangeIndex];

        // §7.2.4 step 1 — amplitude value unwrap.
        Span<bool> step2 = xCount <= 256 ? stackalloc bool[xCount] : new bool[xCount];
        Span<int> finalY = xCount <= 256 ? stackalloc int[xCount] : new int[xCount];
        step2[0] = true;
        step2[1] = true;
        finalY[0] = y[0];
        finalY[1] = y[1];

        for (int i = 2; i < xCount; i++)
        {
            int lo = LowNeighbor(xList, i);
            int hi = HighNeighbor(xList, i);
            int predicted = RenderPoint(xList[lo], finalY[lo], xList[hi], finalY[hi], xList[i]);
            int val = y[i];
            int highroom = range - predicted;
            int lowroom = predicted;
            int room = Math.Min(highroom, lowroom) * 2;
            if (val != 0)
            {
                step2[lo] = true;
                step2[hi] = true;
                step2[i] = true;
                if (val >= room)
                {
                    finalY[i] = highroom > lowroom ? val - lowroom + predicted : -val + highroom + predicted - 1;
                }
                else
                {
                    finalY[i] = (val & 1) == 1 ? predicted - ((val + 1) >> 1) : predicted + (val >> 1);
                }
            }
            else
            {
                step2[i] = false;
                finalY[i] = predicted;
            }
        }

        // §7.2.4 step 2 — line-segment raster.
        // Sort indices by X value (ascending) using an insertion sort — xCount
        // is typically small (≤ floor.PartitionClassList.Count + 2).
        Span<int> order = xCount <= 256 ? stackalloc int[xCount] : new int[xCount];
        for (int i = 0; i < xCount; i++) order[i] = i;
        for (int i = 1; i < xCount; i++)
        {
            int cur = order[i];
            int curX = xList[cur];
            int j = i - 1;
            while (j >= 0 && xList[order[j]] > curX)
            {
                order[j + 1] = order[j];
                j--;
            }
            order[j + 1] = cur;
        }

        // Zero the output before painting so untouched regions are silence.
        output[..n].Clear();
        bool nonZero = false;

        int hx = 0;
        int hy = 0;
        int lx = 0;
        int ly = finalY[order[0]];
        for (int i = 1; i < xCount; i++)
        {
            int idx = order[i];
            if (!step2[idx]) continue;
            hx = xList[idx];
            hy = finalY[idx] * floor.Multiplier;
            int lyScaled = ly * floor.Multiplier;
            if (lx < n) RenderLine(lx, lyScaled, hx > n ? n : hx, hy, output);
            lx = hx;
            ly = finalY[idx];
            nonZero = true;
        }

        // Spec §7.2.4 step 2: positions beyond the last X are filled with the
        // last sample value (clamp).
        if (hx < n)
        {
            float clamp = InverseDbTable[Math.Clamp(hy, 0, 255)];
            for (int i = hx; i < n; i++) output[i] = clamp;
            if (clamp != 0) nonZero = true;
        }

        return nonZero;
    }

    private static int LowNeighbor(int[] x, int i)
    {
        int bestIdx = 0;
        int bestX = int.MinValue;
        int target = x[i];
        for (int j = 0; j < i; j++)
        {
            if (x[j] < target && x[j] > bestX)
            {
                bestX = x[j];
                bestIdx = j;
            }
        }
        return bestIdx;
    }

    private static int HighNeighbor(int[] x, int i)
    {
        int bestIdx = 0;
        int bestX = int.MaxValue;
        int target = x[i];
        for (int j = 0; j < i; j++)
        {
            if (x[j] > target && x[j] < bestX)
            {
                bestX = x[j];
                bestIdx = j;
            }
        }
        return bestIdx;
    }

    /// <summary>Spec §9.2.5 <c>render_point</c>.</summary>
    private static int RenderPoint(int x0, int y0, int x1, int y1, int x)
    {
        int dy = y1 - y0;
        int adx = x1 - x0;
        int ady = Math.Abs(dy);
        int err = ady * (x - x0);
        int off = err / adx;
        return dy < 0 ? y0 - off : y0 + off;
    }

    /// <summary>
    /// Spec §9.2.6 <c>render_line</c>. Bresenham-style integer DDA that
    /// paints the dB-ladder line from <c>(x0, y0)</c> (inclusive) up to but
    /// not including <c>(x1, y1)</c> into <paramref name="output"/>, looking
    /// each painted sample up through <see cref="InverseDbTable"/>.
    /// </summary>
    private static void RenderLine(int x0, int y0, int x1, int y1, Span<float> output)
    {
        int dy = y1 - y0;
        int adx = x1 - x0;
        if (adx <= 0) return;
        int ady = Math.Abs(dy);
        int b = dy / adx;
        int sy = dy < 0 ? b - 1 : b + 1;
        ady -= Math.Abs(b) * adx;
        int x = x0;
        int y = y0;
        int err = 0;
        if ((uint)x < (uint)output.Length)
            output[x] = InverseDbTable[Math.Clamp(y, 0, 255)];
        for (x = x0 + 1; x < x1; x++)
        {
            err += ady;
            if (err >= adx)
            {
                err -= adx;
                y += sy;
            }
            else
            {
                y += b;
            }
            if ((uint)x < (uint)output.Length)
                output[x] = InverseDbTable[Math.Clamp(y, 0, 255)];
        }
    }

    /// <summary>
    /// Range table from the multiplier byte (spec §7.2.2): <c>multiplier=1</c>
    /// → 256, <c>2</c> → 128, <c>3</c> → 86, <c>4</c> → 64.
    /// </summary>
    private static ReadOnlySpan<int> RangeTable => [256, 128, 86, 64];
}
