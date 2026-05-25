using System.Buffers.Binary;
using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;

namespace Mediar.Codecs.Pcm;

/// <summary>
/// SIMD-friendly PCM sample-format converters. All conversions go through
/// normalized 32-bit float (range <c>[-1.0, 1.0]</c>), which is the standard
/// internal format for audio DSP. The methods are deliberately allocation-free
/// — the caller owns both buffers.
/// </summary>
public static class PcmConverter
{
    private const float InvInt16 = 1.0f / 32768.0f;
    private const float InvInt24 = 1.0f / 8388608.0f;
    private const float InvInt32 = 1.0f / 2147483648.0f;

    /// <summary>Convert interleaved signed 16-bit little-endian PCM to normalized float.</summary>
    public static void S16LeToFloat(ReadOnlySpan<byte> source, Span<float> destination)
    {
        int sampleCount = source.Length / 2;
        if (destination.Length < sampleCount) throw new ArgumentException("Destination too small.", nameof(destination));
        var src16 = MemoryMarshal.Cast<byte, short>(source);
        // On little-endian hosts, no swap needed.
        if (BitConverter.IsLittleEndian)
        {
            for (int i = 0; i < sampleCount; i++) destination[i] = src16[i] * InvInt16;
        }
        else
        {
            for (int i = 0; i < sampleCount; i++)
            {
                short s = BinaryPrimitives.ReverseEndianness(src16[i]);
                destination[i] = s * InvInt16;
            }
        }
    }

    /// <summary>Convert interleaved signed 16-bit big-endian PCM to normalized float.</summary>
    public static void S16BeToFloat(ReadOnlySpan<byte> source, Span<float> destination)
    {
        int sampleCount = source.Length / 2;
        if (destination.Length < sampleCount) throw new ArgumentException("Destination too small.", nameof(destination));
        var src16 = MemoryMarshal.Cast<byte, short>(source);
        if (BitConverter.IsLittleEndian)
        {
            for (int i = 0; i < sampleCount; i++)
            {
                short s = BinaryPrimitives.ReverseEndianness(src16[i]);
                destination[i] = s * InvInt16;
            }
        }
        else
        {
            for (int i = 0; i < sampleCount; i++) destination[i] = src16[i] * InvInt16;
        }
    }

    /// <summary>Convert interleaved signed 24-bit little-endian packed PCM to normalized float.</summary>
    public static void S24LeToFloat(ReadOnlySpan<byte> source, Span<float> destination)
    {
        int sampleCount = source.Length / 3;
        if (destination.Length < sampleCount) throw new ArgumentException("Destination too small.", nameof(destination));
        for (int i = 0; i < sampleCount; i++)
        {
            int o = i * 3;
            int v = source[o] | (source[o + 1] << 8) | (source[o + 2] << 16);
            // sign-extend the 24-bit value
            if ((v & 0x00800000) != 0) v |= unchecked((int)0xFF000000);
            destination[i] = v * InvInt24;
        }
    }

    /// <summary>Convert interleaved signed 32-bit little-endian PCM to normalized float.</summary>
    public static void S32LeToFloat(ReadOnlySpan<byte> source, Span<float> destination)
    {
        int sampleCount = source.Length / 4;
        if (destination.Length < sampleCount) throw new ArgumentException("Destination too small.", nameof(destination));
        var src32 = MemoryMarshal.Cast<byte, int>(source);
        if (BitConverter.IsLittleEndian)
        {
            for (int i = 0; i < sampleCount; i++) destination[i] = src32[i] * InvInt32;
        }
        else
        {
            for (int i = 0; i < sampleCount; i++)
            {
                int s = BinaryPrimitives.ReverseEndianness(src32[i]);
                destination[i] = s * InvInt32;
            }
        }
    }

    /// <summary>Reinterpret IEEE 754 32-bit little-endian floats (the most common float PCM format).</summary>
    public static void F32LeToFloat(ReadOnlySpan<byte> source, Span<float> destination)
    {
        int sampleCount = source.Length / 4;
        if (destination.Length < sampleCount) throw new ArgumentException("Destination too small.", nameof(destination));
        var srcF = MemoryMarshal.Cast<byte, float>(source);
        if (BitConverter.IsLittleEndian)
        {
            srcF[..sampleCount].CopyTo(destination);
        }
        else
        {
            var srcI = MemoryMarshal.Cast<byte, uint>(source);
            for (int i = 0; i < sampleCount; i++)
            {
                uint bits = BinaryPrimitives.ReverseEndianness(srcI[i]);
                destination[i] = BitConverter.UInt32BitsToSingle(bits);
            }
        }
    }

    /// <summary>Convert normalized float PCM (clamped to <c>[-1, 1]</c>) to signed 16-bit little-endian.</summary>
    public static void FloatToS16Le(ReadOnlySpan<float> source, Span<byte> destination)
    {
        if (destination.Length < source.Length * 2) throw new ArgumentException("Destination too small.", nameof(destination));
        var dst = MemoryMarshal.Cast<byte, short>(destination);
        for (int i = 0; i < source.Length; i++)
        {
            float v = source[i];
            if (v > 1f) v = 1f;
            else if (v < -1f) v = -1f;
            int scaled = (int)MathF.Round(v * 32767f);
            short s = (short)scaled;
            dst[i] = BitConverter.IsLittleEndian ? s : BinaryPrimitives.ReverseEndianness(s);
        }
    }

    /// <summary>Convert normalized float PCM to signed 24-bit little-endian packed bytes.</summary>
    public static void FloatToS24Le(ReadOnlySpan<float> source, Span<byte> destination)
    {
        if (destination.Length < source.Length * 3) throw new ArgumentException("Destination too small.", nameof(destination));
        for (int i = 0; i < source.Length; i++)
        {
            float v = source[i];
            if (v > 1f) v = 1f;
            else if (v < -1f) v = -1f;
            int scaled = (int)MathF.Round(v * 8388607f);
            int o = i * 3;
            destination[o] = (byte)(scaled & 0xFF);
            destination[o + 1] = (byte)((scaled >> 8) & 0xFF);
            destination[o + 2] = (byte)((scaled >> 16) & 0xFF);
        }
    }

    /// <summary>Convert normalized float PCM to signed 32-bit little-endian.</summary>
    public static void FloatToS32Le(ReadOnlySpan<float> source, Span<byte> destination)
    {
        if (destination.Length < source.Length * 4) throw new ArgumentException("Destination too small.", nameof(destination));
        var dst = MemoryMarshal.Cast<byte, int>(destination);
        for (int i = 0; i < source.Length; i++)
        {
            float v = source[i];
            if (v > 1f) v = 1f;
            else if (v < -1f) v = -1f;
            long scaled = (long)Math.Round(v * 2147483647.0);
            int s = (int)Math.Clamp(scaled, int.MinValue, int.MaxValue);
            dst[i] = BitConverter.IsLittleEndian ? s : BinaryPrimitives.ReverseEndianness(s);
        }
    }

    /// <summary>
    /// Convert from any of Mediar's recognized PCM <see cref="CodecId"/> values to normalized float.
    /// Returns the number of float samples written.
    /// </summary>
    public static int ToFloat(CodecId codec, ReadOnlySpan<byte> source, Span<float> destination)
    {
        switch (codec)
        {
            case CodecId.PcmS16Le: S16LeToFloat(source, destination); return source.Length / 2;
            case CodecId.PcmS16Be: S16BeToFloat(source, destination); return source.Length / 2;
            case CodecId.PcmS24Le: S24LeToFloat(source, destination); return source.Length / 3;
            case CodecId.PcmS32Le: S32LeToFloat(source, destination); return source.Length / 4;
            case CodecId.PcmF32Le: F32LeToFloat(source, destination); return source.Length / 4;
            default: throw new ArgumentException($"Unsupported PCM codec {codec}.", nameof(codec));
        }
    }

    /// <summary>
    /// Convert normalized float to the wire format identified by <paramref name="codec"/>.
    /// Returns the number of bytes written.
    /// </summary>
    public static int FromFloat(CodecId codec, ReadOnlySpan<float> source, Span<byte> destination)
    {
        switch (codec)
        {
            case CodecId.PcmS16Le: FloatToS16Le(source, destination); return source.Length * 2;
            case CodecId.PcmS24Le: FloatToS24Le(source, destination); return source.Length * 3;
            case CodecId.PcmS32Le: FloatToS32Le(source, destination); return source.Length * 4;
            case CodecId.PcmF32Le:
                if (BitConverter.IsLittleEndian)
                {
                    MemoryMarshal.AsBytes(source).CopyTo(destination);
                }
                else
                {
                    var dst32 = MemoryMarshal.Cast<byte, uint>(destination);
                    for (int i = 0; i < source.Length; i++)
                    {
                        dst32[i] = BinaryPrimitives.ReverseEndianness(BitConverter.SingleToUInt32Bits(source[i]));
                    }
                }
                return source.Length * 4;
            default: throw new ArgumentException($"Unsupported PCM codec {codec}.", nameof(codec));
        }
    }

    /// <summary>
    /// Compute the byte size of a single normalized float sample after
    /// conversion to <paramref name="codec"/>.
    /// </summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public static int BytesPerSample(CodecId codec) => codec switch
    {
        CodecId.PcmS16Le or CodecId.PcmS16Be => 2,
        CodecId.PcmS24Le => 3,
        CodecId.PcmS32Le or CodecId.PcmF32Le => 4,
        _ => throw new ArgumentException($"Unsupported PCM codec {codec}.", nameof(codec)),
    };
}
