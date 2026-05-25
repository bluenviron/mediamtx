using System.Runtime.CompilerServices;
using System.Runtime.Intrinsics;
using System.Runtime.Intrinsics.X86;

namespace Mediar.Bitstream;

/// <summary>
/// Scans a byte buffer for H.264 / H.265 Annex-B start codes
/// (0x000001 or 0x00000001) and yields the byte ranges of the NAL
/// payloads in between.
/// </summary>
/// <remarks>
/// <para>
/// This is purely a byte-level operation: no NAL-unit semantics are
/// interpreted. The scanner is the foundation for Annex-B ↔ length-prefixed
/// (AVCC / HVCC) conversions and for muxing raw H.264 / H.265 elementary
/// streams into MP4 / Matroska.
/// </para>
/// <para>
/// Uses a vectorized search via <see cref="Vector256"/> when AVX2 is
/// available; otherwise falls back to a scalar three-byte rolling match.
/// </para>
/// </remarks>
public static class AnnexBScanner
{
    /// <summary>
    /// Locate every Annex-B NAL inside <paramref name="data"/>.
    /// Returns offset/length pairs that point into the original buffer,
    /// excluding the leading start code.
    /// </summary>
    public static List<NalRange> FindNalUnits(ReadOnlySpan<byte> data)
    {
        var nals = new List<NalRange>(8);
        int n = data.Length;
        int i = 0;
        int lastNalStart = -1;

        while (i + 2 < n)
        {
            int idx = FindNextStartCode(data, i);
            if (idx < 0) break;
            int startCodeLen = (idx > 0 && data[idx - 1] == 0) ? 4 : 3;
            int nalStart = idx + 3;
            if (lastNalStart >= 0)
            {
                int nalEnd = idx - (startCodeLen - 3);
                nals.Add(new NalRange(lastNalStart, nalEnd - lastNalStart));
            }
            lastNalStart = nalStart;
            i = nalStart;
        }
        if (lastNalStart >= 0 && lastNalStart < n)
        {
            nals.Add(new NalRange(lastNalStart, n - lastNalStart));
        }
        return nals;
    }

    /// <summary>
    /// Scan forward from <paramref name="start"/> for the next
    /// occurrence of <c>0x 00 00 01</c>. Returns the index of the
    /// first 0x00 byte, or -1 if none is found.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int FindNextStartCode(ReadOnlySpan<byte> data, int start)
    {
        int n = data.Length;
        int i = start;
        if (Avx2.IsSupported && n - i >= 34)
        {
            unsafe
            {
                fixed (byte* p = data)
                {
                    Vector256<byte> zero = Vector256<byte>.Zero;
                    Vector256<byte> one = Vector256.Create((byte)1);
                    int simdEnd = n - 34;
                    while (i <= simdEnd)
                    {
                        var v0 = Vector256.Load(p + i);
                        var v1 = Vector256.Load(p + i + 1);
                        var v2 = Vector256.Load(p + i + 2);
                        var m0 = Vector256.Equals(v0, zero);
                        var m1 = Vector256.Equals(v1, zero);
                        var m2 = Vector256.Equals(v2, one);
                        var matches = (m0 & m1 & m2).ExtractMostSignificantBits();
                        if (matches != 0)
                        {
                            int bit = System.Numerics.BitOperations.TrailingZeroCount(matches);
                            return i + bit;
                        }
                        i += 32;
                    }
                }
            }
        }
        while (i + 2 < n)
        {
            if (data[i] == 0 && data[i + 1] == 0 && data[i + 2] == 1) return i;
            i++;
        }
        return -1;
    }
}

/// <summary>A single NAL unit's location inside a buffer.</summary>
public readonly record struct NalRange(int Offset, int Length);
