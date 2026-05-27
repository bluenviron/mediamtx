using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Runtime.Intrinsics;
using System.Runtime.Intrinsics.X86;

namespace Mediar.Acceleration.X86;

/// <summary>
/// AVX2 implementation of <see cref="IByteSaturator"/>. Processes 32
/// integers per iteration. AVX2 pack instructions operate on the two
/// 128-bit lanes of each register independently, so the output must be
/// re-shuffled with a final <c>vpermd</c> to restore source order.
/// </summary>
public sealed class Avx2ByteSaturator : IByteSaturator
{
    /// <summary>Singleton instance.</summary>
    public static Avx2ByteSaturator Instance { get; } = new();

    /// <summary>True when the host CPU supports both AVX and AVX2.</summary>
    public static bool IsSupported => Avx2.IsSupported;

    /// <inheritdoc/>
    public AccelerationTier IsaTier => AccelerationTier.Avx2;

    private static readonly Vector256<int> s_permute = Vector256.Create(0, 4, 1, 5, 2, 6, 3, 7);

    /// <inheritdoc/>
    public unsafe void Saturate(ReadOnlySpan<int> source, Span<byte> destination)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException("destination span is shorter than source.", nameof(destination));
        }
        if (!Avx2.IsSupported)
        {
            Sse2ByteSaturator.Instance.Saturate(source, destination);
            return;
        }

        int len = source.Length;
        ref int src = ref MemoryMarshal.GetReference(source);
        ref byte dst = ref MemoryMarshal.GetReference(destination);

        int i = 0;
        int vec = len & ~31;
        fixed (int* srcPtr = &src)
        fixed (byte* dstPtr = &dst)
        {
            Vector256<int> perm = s_permute;
            for (; i < vec; i += 32)
            {
                Vector256<int> a = Avx.LoadVector256(srcPtr + i);
                Vector256<int> b = Avx.LoadVector256(srcPtr + i + 8);
                Vector256<int> c = Avx.LoadVector256(srcPtr + i + 16);
                Vector256<int> d = Avx.LoadVector256(srcPtr + i + 24);

                // Per-lane signed-saturate i32 -> i16, then unsigned-saturate
                // i16 -> u8.  Both operations are lane-local under AVX2.
                Vector256<short> ab = Avx2.PackSignedSaturate(a, b);
                Vector256<short> cd = Avx2.PackSignedSaturate(c, d);
                Vector256<byte> packed = Avx2.PackUnsignedSaturate(ab, cd);

                // Restore source order across the lane boundary:
                // packed currently interleaves the two 128-bit lanes; a
                // single 32-bit permutation puts everything back in line.
                Vector256<int> reordered = Avx2.PermuteVar8x32(packed.AsInt32(), perm);

                Avx.Store(dstPtr + i, reordered.AsByte());
            }
        }

        for (; i < len; i++)
        {
            int v = Unsafe.Add(ref src, i);
            Unsafe.Add(ref dst, i) = v < 0 ? (byte)0 : v > 255 ? (byte)255 : (byte)v;
        }
    }
}
