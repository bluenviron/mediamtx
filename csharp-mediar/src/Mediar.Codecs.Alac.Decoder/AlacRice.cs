using System.Numerics;
using Mediar.IO;

namespace Mediar.Codecs.Alac.Decoder;

/// <summary>
/// ALAC adaptive Golomb / Rice decoder. Reads <see cref="DecodeBlock"/>
/// signed residuals out of an MSB-first <see cref="BitReader"/>, using the
/// adaptive history-driven parameter scheme documented in Apple's ALAC
/// Apache-2.0 reference (codec/ag_dec.c).
/// </summary>
/// <remarks>
/// Clean-room implementation derived from the published Apple reference.
/// The decoder maintains a running mean <c>mb</c> that drives the rice
/// parameter <c>k</c> for each symbol; when <c>mb</c> drops below a
/// threshold the decoder enters a "zero-run" mode that emits a block of
/// zeros and applies a sign-modifier to the next non-zero symbol.
/// </remarks>
internal static class AlacRice
{
    private const int QbShift = 9;
    private const int Qb = 1 << QbShift;
    private const int MmulShift = 2;
    private const int MdenShift = QbShift - MmulShift - 1;
    private const int Moff = 1 << (MdenShift - 2);
    private const int Bitoff = 24;
    private const int MaxPrefix32 = 9;
    private const int MaxPrefix16 = 9;
    private const int MaxDataTypeBits16 = 16;
    private const int NMaxMeanClamp = 0xFFFF;
    private const int NMeanClampVal = 0xFFFF;

    /// <summary>
    /// Decode <paramref name="numSamples"/> rice-coded signed residuals out
    /// of <paramref name="br"/>, using the per-channel rice parameters
    /// <paramref name="mbInitial"/>, <paramref name="pb"/>,
    /// <paramref name="kb"/>, and the maximum residual width
    /// <paramref name="maxSize"/> (= chanBits).
    /// </summary>
    public static void DecodeBlock(
        ref BitReader br,
        Span<int> residuals,
        int numSamples,
        int mbInitial,
        int pb,
        int kb,
        int maxSize)
    {
        if (residuals.Length < numSamples)
            throw new ArgumentException("residuals buffer too small.", nameof(residuals));

        int mb = mbInitial;
        int wb = (1 << kb) - 1;
        int zmode = 0;

        int c = 0;
        while (c < numSamples)
        {
            // Per-sample k = clamp(lg3a(mb >> QBSHIFT), 0, kb).
            int m = mb >> QbShift;
            int k = Lg3a(m);
            if (k > kb) k = kb;
            m = (1 << k) - 1;

            uint n = DynGet32Bit(ref br, m, k, maxSize);

            // Sign-fold: even codes are positive, odd codes are negative.
            uint nDecode = n + (uint)zmode;
            int multiplier = -(int)(nDecode & 1u) | 1; // -1 if odd, +1 if even
            int del = (int)((nDecode + 1u) >> 1) * multiplier;
            residuals[c++] = del;

            // Update mean tracking.
            mb = pb * (int)(n + (uint)zmode) + mb - ((pb * mb) >> QbShift);
            if (n > NMaxMeanClamp) mb = NMeanClampVal;

            zmode = 0;

            // Zero-run mode triggers when the running mean falls below QB / (2^MMULSHIFT).
            if (((mb << MmulShift) < Qb) && (c < numSamples))
            {
                zmode = 1;
                int leadBits = Lead((uint)mb);
                int kz = leadBits - Bitoff + ((mb + Moff) >> MdenShift);
                if (kz < 0) kz = 0;
                int mz = ((1 << kz) - 1) & wb;

                uint runLen = DynGet16Bit(ref br, mz, kz);

                if (c + (int)runLen > numSamples)
                    throw new InvalidDataException("ALAC zero-run overruns block.");

                for (int j = 0; j < runLen; j++) residuals[c++] = 0;

                if (runLen >= 65535) zmode = 0;
                mb = 0;
            }
        }
    }

    // dyn_get_32bit: rice-coded value, MAX_PREFIX_32 escape, raw fallback width = maxbits.
    // Mirrors Apple's ag_dec.c (dyn_get_32bit) byte-for-byte.
    private static uint DynGet32Bit(ref BitReader br, int m, int k, int maxbits)
    {
        int prefix = ReadUnaryOnesInclTerm(ref br, MaxPrefix32);

        if (prefix >= MaxPrefix32)
        {
            // Escape: prefix-only marker is 9 ones (already consumed), followed
            // by `maxbits` raw bits.
            return br.ReadBits(maxbits);
        }

        if (k == 1)
        {
            return (uint)prefix;
        }

        // Read k bits as v. The encoder's k-bit suffix is in [2, m] when the
        // unary prefix carried information, or 0 when it didn't. When v < 2
        // the high (k-1) bits of v are zero so the encoder only meaningfully
        // wrote (k-1) bits and the decoder must rewind the unused LSB so it
        // becomes the start of the next code.
        uint v = br.ReadBits(k);
        uint result = (uint)prefix * (uint)m;
        if (v >= 2)
        {
            result += v - 1;
        }
        else
        {
            RewindBits(ref br, 1);
        }
        return result;
    }

    // dyn_get_16bit: zero-run variant — same shape, but the escape raw width
    // is MAX_DATATYPE_BITS_16 (=16) instead of `maxbits`.
    private static uint DynGet16Bit(ref BitReader br, int m, int k)
    {
        int prefix = ReadUnaryOnesInclTerm(ref br, MaxPrefix16);

        if (prefix >= MaxPrefix16)
        {
            return br.ReadBits(MaxDataTypeBits16);
        }

        if (k == 1)
        {
            return (uint)prefix;
        }

        uint v = br.ReadBits(k);
        uint result = (uint)prefix * (uint)m;
        if (v >= 2)
        {
            result += v - 1;
        }
        else
        {
            RewindBits(ref br, 1);
        }
        return result;
    }

    // Read leading 1s up to (and including) the terminating 0, capped at
    // <paramref name="maxPrefix"/>. Returns the count of 1s; when the count
    // hits the cap the terminator is NOT consumed (it doesn't exist — this
    // is the "escape" marker), and when the count is below the cap the
    // terminating 0 has been consumed.
    private static int ReadUnaryOnesInclTerm(ref BitReader br, int maxPrefix)
    {
        int count = 0;
        while (count < maxPrefix)
        {
            if (!br.CanRead(1)) throw new EndOfStreamException("ALAC bitstream truncated in unary prefix.");
            if (br.ReadBit())
            {
                count++;
            }
            else
            {
                return count; // consumed terminator
            }
        }
        return count; // escape — no terminator was present
    }

    private static void RewindBits(ref BitReader br, int count)
    {
        long newPos = br.BitPosition - count;
        if (newPos < 0) throw new InvalidOperationException("Cannot rewind before bit 0.");
        br.SeekToBit(newPos);
    }

    // lg3a(x) = floor(log2(x + 3)). For x in [0, 2^31), result in [1, 31].
    internal static int Lg3a(int x)
    {
        long v = (long)x + 3;
        if (v <= 0) return 0;
        int leadingZeros = BitOperations.LeadingZeroCount((uint)v);
        return 31 - leadingZeros;
    }

    // Count leading zero bits in a 32-bit word, 1..32.
    internal static int Lead(uint x)
    {
        if (x == 0) return 32;
        return BitOperations.LeadingZeroCount(x);
    }
}
