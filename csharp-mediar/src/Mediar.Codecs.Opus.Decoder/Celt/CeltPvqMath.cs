using System.Numerics;

namespace Mediar.Codecs.Opus.Decoder.Celt;

/// <summary>
/// Bit-exact math helpers and pulse-cache tables for the CELT PVQ layer
/// (Phase 2c.3b). All routines and tables are ports of libopus
/// <c>celt/bands.c</c> + <c>celt/rate.h</c> + <c>celt/static_modes_*.h</c>
/// and produce byte-identical results on every platform — which is
/// required because they steer bit allocation inside
/// <c>compute_allocation</c> and the PVQ split decoder.
/// </summary>
internal static class CeltPvqMath
{
    /// <summary>
    /// Maximum quantised pulse count index used by <see cref="GetPulses"/>.
    /// Matches libopus <c>MAX_PSEUDO</c>.
    /// </summary>
    public const int MaxPseudo = 40;

    /// <summary>
    /// <c>log2(MaxPseudo)</c> rounded up, used to size the inner binary
    /// search in <see cref="Bits2Pulses"/>. Matches libopus
    /// <c>LOG_MAX_PSEUDO</c>.
    /// </summary>
    public const int LogMaxPseudo = 6;

    /// <summary>
    /// Hard cap on the PVQ pulse count any single band can request.
    /// Matches libopus <c>CELT_MAX_PULSES</c>.
    /// </summary>
    public const int CeltMaxPulses = 128;

    /// <summary>
    /// Per-band starting offset into <see cref="CacheBits50"/>, indexed as
    /// <c>(LM+1) * MaxBands + band</c>. A value of -1 means the band is too
    /// small (N=1) for PVQ pulses at this LM. Matches libopus
    /// <c>cache_index50[105]</c> from <c>static_modes_float.h</c>.
    /// </summary>
    public static ReadOnlySpan<short> CacheIndex50 => new short[]
    {
        -1, -1, -1, -1, -1, -1, -1, -1, 0, 0, 0, 0, 41, 41, 41,
        82, 82, 123, 164, 200, 222, 0, 0, 0, 0, 0, 0, 0, 0, 41,
        41, 41, 41, 123, 123, 123, 164, 164, 240, 266, 283, 295, 41, 41, 41,
        41, 41, 41, 41, 41, 123, 123, 123, 123, 240, 240, 240, 266, 266, 305,
        318, 328, 336, 123, 123, 123, 123, 123, 123, 123, 123, 240, 240, 240, 240,
        305, 305, 305, 318, 318, 343, 351, 358, 364, 240, 240, 240, 240, 240, 240,
        240, 240, 305, 305, 305, 305, 343, 343, 343, 351, 351, 370, 376, 382, 387,
    };

    /// <summary>
    /// Flat pulse-cost cache. Each band's slice begins at
    /// <c>CacheIndex50[(LM+1)*MaxBands+band]</c>; the first byte is the
    /// maximum pulse-count index (cap on the binary search), followed by
    /// the cost-in-bits table that <see cref="Pulses2Bits"/> indexes. All
    /// costs are 8-bit unsigned, matching libopus <c>cache_bits50[392]</c>
    /// from <c>static_modes_float.h</c>.
    /// </summary>
    public static ReadOnlySpan<byte> CacheBits50 => new byte[]
    {
        40, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
        7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
        7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 40, 15, 23, 28,
        31, 34, 36, 38, 39, 41, 42, 43, 44, 45, 46, 47, 47, 49, 50,
        51, 52, 53, 54, 55, 55, 57, 58, 59, 60, 61, 62, 63, 63, 65,
        66, 67, 68, 69, 70, 71, 71, 40, 20, 33, 41, 48, 53, 57, 61,
        64, 66, 69, 71, 73, 75, 76, 78, 80, 82, 85, 87, 89, 91, 92,
        94, 96, 98, 101, 103, 105, 107, 108, 110, 112, 114, 117, 119, 121, 123,
        124, 126, 128, 40, 23, 39, 51, 60, 67, 73, 79, 83, 87, 91, 94,
        97, 100, 102, 105, 107, 111, 115, 118, 121, 124, 126, 129, 131, 135, 139,
        142, 145, 148, 150, 153, 155, 159, 163, 166, 169, 172, 174, 177, 179, 35,
        28, 49, 65, 78, 89, 99, 107, 114, 120, 126, 132, 136, 141, 145, 149,
        153, 159, 165, 171, 176, 180, 185, 189, 192, 199, 205, 211, 216, 220, 225,
        229, 232, 239, 245, 251, 21, 33, 58, 79, 97, 112, 125, 137, 148, 157,
        166, 174, 182, 189, 195, 201, 207, 217, 227, 235, 243, 251, 17, 35, 63,
        86, 106, 123, 139, 152, 165, 177, 187, 197, 206, 214, 222, 230, 237, 250,
        25, 31, 55, 75, 91, 105, 117, 128, 138, 146, 154, 161, 168, 174, 180,
        185, 190, 200, 208, 215, 222, 229, 235, 240, 245, 255, 16, 36, 65, 89,
        110, 128, 144, 159, 173, 185, 196, 207, 217, 226, 234, 242, 250, 11, 41,
        74, 103, 128, 151, 172, 191, 209, 225, 241, 255, 9, 43, 79, 110, 138,
        163, 186, 207, 227, 246, 12, 39, 71, 99, 123, 144, 164, 182, 198, 214,
        228, 241, 253, 9, 44, 81, 113, 142, 168, 192, 214, 235, 255, 7, 49,
        90, 127, 160, 191, 220, 247, 6, 51, 95, 134, 170, 203, 234, 7, 47,
        87, 123, 155, 184, 212, 237, 6, 52, 97, 137, 174, 208, 240, 5, 57,
        106, 151, 192, 231, 5, 59, 111, 158, 202, 243, 5, 55, 103, 147, 187,
        224, 5, 60, 113, 161, 206, 248, 4, 65, 122, 175, 224, 4, 67, 127,
        182, 234,
    };

    /// <summary>
    /// Convert a pseudo-pulse index <c>i ∈ [0, MaxPseudo]</c> to the actual
    /// pulse count it represents. Matches libopus
    /// <c>get_pulses(i)</c>: linear in <c>[0, 8)</c>, then geometric
    /// (<c>(8 + i&amp;7) &lt;&lt; ((i&gt;&gt;3)-1)</c>) up to 128 at i=40.
    /// </summary>
    public static int GetPulses(int i) =>
        i < 8 ? i : (8 + (i & 7)) << ((i >> 3) - 1);

    /// <summary>
    /// Bit-exact <c>cos(x * π/16384)</c> approximation. Input is the angle
    /// scaled so a full quarter-turn = 16384. Output is a Q15 cosine
    /// (<c>[1, 32767]</c>). Matches libopus <c>bitexact_cos</c> in
    /// <c>celt/bands.c</c> byte-for-byte across all platforms.
    /// </summary>
    public static short BitexactCos(short x)
    {
        int tmp = (4096 + (int)x * x) >> 13;
        // tmp must fit in opus_int16; libopus asserts <= 32767 here.
        short x2 = (short)tmp;
        int v = (32767 - x2)
                + FracMul16(x2, -7651 + FracMul16(x2, 8277 + FracMul16(-626, x2)));
        return (short)(1 + v);
    }

    /// <summary>
    /// Bit-exact <c>log2(tan(asin(isin) - acos(icos)))</c> approximation
    /// scaled by <c>1 &lt;&lt; 11</c>. Used by the PVQ split decoder to
    /// price intensity / theta angles. Matches libopus
    /// <c>bitexact_log2tan</c> in <c>celt/bands.c</c>.
    /// </summary>
    public static int BitexactLog2Tan(int isin, int icos)
    {
        int lc = Ilog((uint)icos);
        int ls = Ilog((uint)isin);
        icos <<= 15 - lc;
        isin <<= 15 - ls;
        return (ls - lc) * (1 << 11)
            + FracMul16(isin, FracMul16(isin, -2597) + 7932)
            - FracMul16(icos, FracMul16(icos, -2597) + 7932);
    }

    /// <summary>
    /// Convert a target bit budget for a single PVQ band into the closest
    /// pseudo-pulse index the cache supports. Performs a 6-iteration
    /// binary search over <see cref="CacheBits50"/>. Matches libopus
    /// <c>bits2pulses(m, band, LM, bits)</c> in <c>celt/rate.h</c>.
    /// </summary>
    public static int Bits2Pulses(int band, int lm, int bits)
    {
        int cacheStart = CacheIndex50[(lm + 1) * CeltConstants.MaxBands + band];
        var cache = CacheBits50.Slice(cacheStart);
        int lo = 0;
        int hi = cache[0];
        bits--;
        for (int i = 0; i < LogMaxPseudo; i++)
        {
            int mid = (lo + hi + 1) >> 1;
            if (cache[mid] >= bits) hi = mid;
            else lo = mid;
        }
        int loCost = lo == 0 ? -1 : cache[lo];
        if (bits - loCost <= cache[hi] - bits) return lo;
        return hi;
    }

    /// <summary>
    /// Inverse of <see cref="Bits2Pulses"/> — look up the bit cost of a
    /// given pseudo-pulse index. Matches libopus
    /// <c>pulses2bits(m, band, LM, pulses)</c> in <c>celt/rate.h</c>.
    /// </summary>
    public static int Pulses2Bits(int band, int lm, int pulses)
    {
        if (pulses == 0) return 0;
        int cacheStart = CacheIndex50[(lm + 1) * CeltConstants.MaxBands + band];
        return CacheBits50[cacheStart + pulses] + 1;
    }

    private static int FracMul16(int a, int b) =>
        (16384 + (short)a * (short)b) >> 15;

    private static int Ilog(uint v) =>
        v == 0 ? 0 : BitOperations.Log2(v) + 1;
}
