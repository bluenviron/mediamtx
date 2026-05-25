namespace Mediar.Codecs.G711;

/// <summary>
/// Pure functional encoder/decoder for the ITU-T G.711 companded audio
/// codecs (µ-law and A-law). Both formats compand 14-/13-bit linear PCM into
/// 8 bits per sample at 8000 Hz; the math is a piecewise log curve. They are
/// commonly used in telephony container formats (.au / WAV format codes
/// 0x06 and 0x07).
/// </summary>
public static class G711
{
    private const short Bias = 0x84;
    private const short Clip = 32635;

    // ---------- µ-law (ITU-T G.711, mu = 255) ----------

    /// <summary>Decode one µ-law byte to signed 16-bit linear PCM.</summary>
    public static short DecodeMuLaw(byte u)
    {
        // Reverse the encoding: invert all bits, sign-extend, decode segment & step.
        u = (byte)~u;
        int sign = (u & 0x80);
        int exponent = (u >> 4) & 0x07;
        int mantissa = u & 0x0F;
        int sample = ((mantissa << 3) + Bias) << exponent;
        sample -= Bias;
        return (short)(sign != 0 ? -sample : sample);
    }

    /// <summary>Encode a signed 16-bit linear PCM sample to µ-law.</summary>
    public static byte EncodeMuLaw(short pcm)
    {
        int sign = (pcm >> 8) & 0x80;
        if (sign != 0) pcm = (short)-pcm;
        if (pcm > Clip) pcm = Clip;
        pcm = (short)(pcm + Bias);

        int exponent = 7;
        for (int mask = 0x4000; (pcm & mask) == 0 && exponent > 0; exponent--, mask >>= 1) { }
        int mantissa = (pcm >> (exponent + 3)) & 0x0F;
        int u = ~(sign | (exponent << 4) | mantissa);
        return (byte)u;
    }

    // ---------- A-law (ITU-T G.711, A = 87.6) ----------

    /// <summary>Decode one A-law byte to signed 16-bit linear PCM.</summary>
    public static short DecodeALaw(byte a)
    {
        a ^= 0x55;
        int sign = a & 0x80;
        int exponent = (a & 0x70) >> 4;
        int mantissa = a & 0x0F;
        int sample;
        if (exponent != 0)
        {
            sample = ((mantissa << 4) | 0x108) << (exponent - 1);
        }
        else
        {
            sample = (mantissa << 4) | 0x008;
        }
        return (short)(sign != 0 ? sample : -sample);
    }

    /// <summary>Encode a signed 16-bit linear PCM sample to A-law.</summary>
    public static byte EncodeALaw(short pcm)
    {
        int sign = ((~pcm) >> 8) & 0x80;
        if (sign == 0) pcm = (short)-pcm;
        if (pcm > Clip) pcm = Clip;
        int exponent;
        int mantissa;
        if (pcm >= 256)
        {
            exponent = 7;
            for (int mask = 0x4000; (pcm & mask) == 0 && exponent > 0; exponent--, mask >>= 1) { }
            mantissa = (pcm >> (exponent + 3)) & 0x0F;
        }
        else
        {
            exponent = 0;
            mantissa = pcm >> 4;
        }
        int a = sign | (exponent << 4) | mantissa;
        return (byte)(a ^ 0x55);
    }

    // ---------- Buffer helpers ----------

    /// <summary>Decode an entire µ-law payload to normalized float in <c>[-1, 1]</c>.</summary>
    public static void DecodeMuLaw(ReadOnlySpan<byte> source, Span<float> destination)
    {
        if (destination.Length < source.Length) throw new ArgumentException("Destination too small.", nameof(destination));
        const float inv = 1.0f / 32768.0f;
        for (int i = 0; i < source.Length; i++)
        {
            destination[i] = DecodeMuLaw(source[i]) * inv;
        }
    }

    /// <summary>Decode an entire A-law payload to normalized float in <c>[-1, 1]</c>.</summary>
    public static void DecodeALaw(ReadOnlySpan<byte> source, Span<float> destination)
    {
        if (destination.Length < source.Length) throw new ArgumentException("Destination too small.", nameof(destination));
        const float inv = 1.0f / 32768.0f;
        for (int i = 0; i < source.Length; i++)
        {
            destination[i] = DecodeALaw(source[i]) * inv;
        }
    }

    /// <summary>Encode a normalized float buffer to a µ-law byte payload.</summary>
    public static void EncodeMuLaw(ReadOnlySpan<float> source, Span<byte> destination)
    {
        if (destination.Length < source.Length) throw new ArgumentException("Destination too small.", nameof(destination));
        for (int i = 0; i < source.Length; i++)
        {
            float v = source[i];
            if (v > 1f) v = 1f;
            else if (v < -1f) v = -1f;
            destination[i] = EncodeMuLaw((short)MathF.Round(v * 32767f));
        }
    }

    /// <summary>Encode a normalized float buffer to an A-law byte payload.</summary>
    public static void EncodeALaw(ReadOnlySpan<float> source, Span<byte> destination)
    {
        if (destination.Length < source.Length) throw new ArgumentException("Destination too small.", nameof(destination));
        for (int i = 0; i < source.Length; i++)
        {
            float v = source[i];
            if (v > 1f) v = 1f;
            else if (v < -1f) v = -1f;
            destination[i] = EncodeALaw((short)MathF.Round(v * 32767f));
        }
    }
}
