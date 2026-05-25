using Mediar.IO;

namespace Mediar.Codecs.Flac.Decoder;

/// <summary>
/// Subframe decoder for a single FLAC channel. Implements RFC 9639 §10.3:
/// CONSTANT, VERBATIM, FIXED (orders 0-4) and LPC (orders 1-32) subframes
/// plus the Rice / Rice2 residual coding (RFC 9639 §10.3.5).
/// </summary>
internal static class FlacSubframeDecoder
{
    // Fixed predictor coefficients for orders 0..4 (RFC 9639 §10.3.3 Table 3).
    // Order 0: x[i] = 0
    // Order 1: x[i] = x[i-1]
    // Order 2: x[i] = 2*x[i-1] -   x[i-2]
    // Order 3: x[i] = 3*x[i-1] - 3*x[i-2] +   x[i-3]
    // Order 4: x[i] = 4*x[i-1] - 6*x[i-2] + 4*x[i-3] -   x[i-4]
    private static readonly int[][] FixedCoefficients =
    {
        [],
        new[] {  1 },
        new[] {  2, -1 },
        new[] {  3, -3,  1 },
        new[] {  4, -6,  4, -1 },
    };

    public static void Decode(ref BitReader br, Span<int> samples, int blockSize, int bps)
    {
        // Subframe header: 1 reserved (0) + 6 type + 1 wasted-bits flag (+ unary)
        if (br.ReadBit()) throw new InvalidDataException("Reserved bit set in FLAC subframe header.");
        int type = (int)br.ReadBits(6);
        int wasted = 0;
        if (br.ReadBit())
        {
            // Unary-encoded "k": k zeros then a 1, wasted = k + 1
            wasted = 1;
            while (!br.ReadBit()) wasted++;
        }
        int effectiveBps = bps - wasted;
        if (effectiveBps <= 0) throw new InvalidDataException("FLAC subframe wastes all bits per sample.");

        if (type == 0)
        {
            // CONSTANT — one sample replicated across the whole block.
            int v = ReadSigned(ref br, effectiveBps);
            for (int i = 0; i < blockSize; i++) samples[i] = v;
        }
        else if (type == 1)
        {
            // VERBATIM — blockSize samples encoded straight.
            for (int i = 0; i < blockSize; i++) samples[i] = ReadSigned(ref br, effectiveBps);
        }
        else if ((type & 0b111110) == 0b000010)
        {
            throw new InvalidDataException("Reserved FLAC subframe type.");
        }
        else if ((type & 0b111100) == 0b000100)
        {
            throw new InvalidDataException("Reserved FLAC subframe type.");
        }
        else if ((type & 0b111000) == 0b001000)
        {
            int order = type & 0b000111;
            if (order > 4) throw new InvalidDataException("Reserved fixed predictor order.");
            DecodeFixed(ref br, samples, blockSize, effectiveBps, order);
        }
        else if ((type & 0b100000) != 0)
        {
            int order = (type & 0b011111) + 1; // 1..32
            DecodeLpc(ref br, samples, blockSize, effectiveBps, order);
        }
        else
        {
            throw new InvalidDataException($"Reserved FLAC subframe type 0x{type:X2}.");
        }

        if (wasted > 0)
        {
            for (int i = 0; i < blockSize; i++) samples[i] <<= wasted;
        }
    }

    private static void DecodeFixed(ref BitReader br, Span<int> samples, int blockSize, int bps, int order)
    {
        for (int i = 0; i < order; i++) samples[i] = ReadSigned(ref br, bps);
        DecodeResidual(ref br, samples, blockSize, order);

        if (order == 0) return;
        var coeffs = FixedCoefficients[order];
        for (int i = order; i < blockSize; i++)
        {
            long acc = 0;
            for (int k = 0; k < order; k++) acc += (long)coeffs[k] * samples[i - 1 - k];
            samples[i] = samples[i] + (int)acc;
        }
    }

    private static void DecodeLpc(ref BitReader br, Span<int> samples, int blockSize, int bps, int order)
    {
        for (int i = 0; i < order; i++) samples[i] = ReadSigned(ref br, bps);

        int precision = (int)br.ReadBits(4) + 1;
        if (precision == 16) throw new InvalidDataException("Invalid FLAC LPC coefficient precision.");
        int shift = (int)br.ReadBits(5);
        if ((shift & 0x10) != 0) shift |= unchecked((int)0xFFFFFFE0); // sign-extend 5 bits
        if (shift < 0) throw new InvalidDataException("Negative LPC shift is not allowed by the FLAC spec.");

        Span<int> coeffs = stackalloc int[order];
        for (int i = 0; i < order; i++) coeffs[i] = ReadSigned(ref br, precision);

        DecodeResidual(ref br, samples, blockSize, order);

        for (int i = order; i < blockSize; i++)
        {
            long acc = 0;
            for (int k = 0; k < order; k++) acc += (long)coeffs[k] * samples[i - 1 - k];
            samples[i] = samples[i] + (int)(acc >> shift);
        }
    }

    private static void DecodeResidual(ref BitReader br, Span<int> samples, int blockSize, int predictorOrder)
    {
        int method = (int)br.ReadBits(2);
        int paramBits;
        int escapeValue;
        if (method == 0) { paramBits = 4; escapeValue = 0b1111; }
        else if (method == 1) { paramBits = 5; escapeValue = 0b11111; }
        else throw new InvalidDataException("Reserved FLAC residual coding method.");

        int partitionOrder = (int)br.ReadBits(4);
        int partitions = 1 << partitionOrder;
        int partitionSamples = blockSize >> partitionOrder;
        if (partitionSamples * partitions != blockSize)
            throw new InvalidDataException("FLAC partition order incompatible with block size.");

        int sampleIndex = predictorOrder;
        for (int p = 0; p < partitions; p++)
        {
            int n = partitionSamples;
            if (p == 0) n -= predictorOrder;
            int param = (int)br.ReadBits(paramBits);
            if (param == escapeValue)
            {
                int rawBits = (int)br.ReadBits(5);
                for (int i = 0; i < n; i++)
                {
                    samples[sampleIndex++] = rawBits == 0 ? 0 : ReadSigned(ref br, rawBits);
                }
            }
            else
            {
                for (int i = 0; i < n; i++)
                {
                    samples[sampleIndex++] = ReadRice(ref br, param);
                }
            }
        }
    }

    private static int ReadRice(ref BitReader br, int param)
    {
        int q = 0;
        while (!br.ReadBit()) q++;
        uint remainder = param == 0 ? 0u : br.ReadBits(param);
        uint folded = ((uint)q << param) | remainder;
        return (folded & 1) != 0 ? -(int)((folded >> 1) + 1) : (int)(folded >> 1);
    }

    private static int ReadSigned(ref BitReader br, int bits)
    {
        if (bits == 0) return 0;
        uint raw = bits <= 32 ? br.ReadBits(bits) : throw new InvalidDataException("Sample width > 32 bits.");
        // Sign-extend
        if (bits == 32) return (int)raw;
        int signBit = 1 << (bits - 1);
        int mask = (1 << bits) - 1;
        int v = (int)(raw & (uint)mask);
        return (v & signBit) != 0 ? v - (1 << bits) : v;
    }
}
