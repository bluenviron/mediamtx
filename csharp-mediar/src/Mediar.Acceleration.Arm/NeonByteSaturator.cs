using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Runtime.Intrinsics;
using System.Runtime.Intrinsics.Arm;

namespace Mediar.Acceleration.Arm;

/// <summary>
/// AdvSimd / NEON implementation of <see cref="IByteSaturator"/>.
/// Processes 16 integers per iteration via a pair of
/// <c>VQMOVN</c> narrowings (i32 -> i16, then i16 -> u8) that
/// match the architectural saturation semantics exactly.
/// </summary>
public sealed class NeonByteSaturator : IByteSaturator
{
    /// <summary>Singleton instance.</summary>
    public static NeonByteSaturator Instance { get; } = new();

    /// <summary>True when the host CPU supports the AdvSimd instruction set.</summary>
    public static bool IsSupported => AdvSimd.IsSupported;

    /// <inheritdoc/>
    public AccelerationTier IsaTier => AccelerationTier.Neon;

    /// <inheritdoc/>
    public unsafe void Saturate(ReadOnlySpan<int> source, Span<byte> destination)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException("destination span is shorter than source.", nameof(destination));
        }
        if (!AdvSimd.IsSupported)
        {
            ScalarByteSaturator.Instance.Saturate(source, destination);
            return;
        }

        int len = source.Length;
        ref int src = ref MemoryMarshal.GetReference(source);
        ref byte dst = ref MemoryMarshal.GetReference(destination);

        int i = 0;
        int vec = len & ~15;
        fixed (int* srcPtr = &src)
        fixed (byte* dstPtr = &dst)
        {
            for (; i < vec; i += 16)
            {
                Vector128<int> a = AdvSimd.LoadVector128(srcPtr + i);
                Vector128<int> b = AdvSimd.LoadVector128(srcPtr + i + 4);
                Vector128<int> c = AdvSimd.LoadVector128(srcPtr + i + 8);
                Vector128<int> d = AdvSimd.LoadVector128(srcPtr + i + 12);

                // VQMOVN: signed-saturate i32 -> i16 (low half of a Vector128<short>).
                Vector64<short> ab_lo = AdvSimd.ExtractNarrowingSaturateLower(a);
                Vector128<short> ab = AdvSimd.ExtractNarrowingSaturateUpper(ab_lo, b);
                Vector64<short> cd_lo = AdvSimd.ExtractNarrowingSaturateLower(c);
                Vector128<short> cd = AdvSimd.ExtractNarrowingSaturateUpper(cd_lo, d);

                // VQMOVUN: signed-saturate i16 -> u8 (treats negative as 0).
                Vector64<byte> abN = AdvSimd.ExtractNarrowingSaturateUnsignedLower(ab);
                Vector128<byte> packed = AdvSimd.ExtractNarrowingSaturateUnsignedUpper(abN, cd);

                AdvSimd.Store(dstPtr + i, packed);
            }
        }

        for (; i < len; i++)
        {
            int v = Unsafe.Add(ref src, i);
            Unsafe.Add(ref dst, i) = v < 0 ? (byte)0 : v > 255 ? (byte)255 : (byte)v;
        }
    }
}
