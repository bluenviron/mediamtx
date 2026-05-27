using System.Runtime.CompilerServices;
using System.Runtime.InteropServices;
using System.Runtime.Intrinsics;
using System.Runtime.Intrinsics.X86;

namespace Mediar.Acceleration.X86;

/// <summary>
/// SSE2 implementation of <see cref="IByteSaturator"/>. Processes 16
/// integers per iteration using a packsswb/packssdw pipeline, then
/// scalar-clamps the tail.
/// </summary>
public sealed class Sse2ByteSaturator : IByteSaturator
{
    /// <summary>Singleton instance.</summary>
    public static Sse2ByteSaturator Instance { get; } = new();

    /// <summary>True when the host CPU supports the SSE2 instruction set.</summary>
    public static bool IsSupported => Sse2.IsSupported;

    /// <inheritdoc/>
    public AccelerationTier IsaTier => AccelerationTier.Sse2;

    /// <inheritdoc/>
    public unsafe void Saturate(ReadOnlySpan<int> source, Span<byte> destination)
    {
        if (destination.Length < source.Length)
        {
            throw new ArgumentException("destination span is shorter than source.", nameof(destination));
        }
        if (!Sse2.IsSupported)
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
                Vector128<int> a = Sse2.LoadVector128(srcPtr + i);
                Vector128<int> b = Sse2.LoadVector128(srcPtr + i + 4);
                Vector128<int> c = Sse2.LoadVector128(srcPtr + i + 8);
                Vector128<int> d = Sse2.LoadVector128(srcPtr + i + 12);

                // packssdw: signed-saturate i32 -> i16 (clamps to [-32768, 32767]).
                Vector128<short> ab = Sse2.PackSignedSaturate(a, b);
                Vector128<short> cd = Sse2.PackSignedSaturate(c, d);

                // packuswb: unsigned-saturate i16 -> u8 (clamps to [0, 255]).
                Vector128<byte> packed = Sse2.PackUnsignedSaturate(ab, cd);

                Sse2.Store(dstPtr + i, packed);
            }
        }

        for (; i < len; i++)
        {
            int v = Unsafe.Add(ref src, i);
            Unsafe.Add(ref dst, i) = v < 0 ? (byte)0 : v > 255 ? (byte)255 : (byte)v;
        }
    }
}
