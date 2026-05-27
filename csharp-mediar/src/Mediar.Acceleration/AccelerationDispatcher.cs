using System.Diagnostics.CodeAnalysis;

namespace Mediar.Acceleration;

/// <summary>
/// Identifies the CPU instruction-set family providing a hardware kernel.
/// </summary>
/// <remarks>
/// Returned by <see cref="IAcceleratedKernel.IsaTier"/> and used by
/// <see cref="AccelerationDispatcher"/> to pick the highest available
/// implementation registered for a given kernel contract.
/// </remarks>
public enum AccelerationTier
{
    /// <summary>Portable, fully managed scalar fallback. Always available.</summary>
    Scalar = 0,

    /// <summary>x86 SSE2 baseline (Sse / Sse2).</summary>
    Sse2 = 10,

    /// <summary>x86 SSSE3 (Ssse3).</summary>
    Ssse3 = 11,

    /// <summary>x86 SSE4.1 (Sse41).</summary>
    Sse41 = 12,

    /// <summary>x86 AVX2 (Avx, Avx2).</summary>
    Avx2 = 20,

    /// <summary>x86 AVX-512 foundation + BW + VL (Avx512F, Avx512BW, Avx512VL).</summary>
    Avx512 = 30,

    /// <summary>Arm AdvSimd / NEON.</summary>
    Neon = 40,

    /// <summary>Arm SVE (Scalable Vector Extension).</summary>
    Sve = 50,
}

/// <summary>
/// Marker contract for every kernel registered with
/// <see cref="AccelerationDispatcher"/>. Carries the instruction-set tier
/// used to pick the highest-ranked implementation at startup.
/// </summary>
public interface IAcceleratedKernel
{
    /// <summary>Instruction-set tier that this kernel requires at runtime.</summary>
    AccelerationTier IsaTier { get; }
}

/// <summary>
/// Static dispatcher for hardware-accelerated kernels. Hardware-specific
/// assemblies (e.g. <c>Mediar.Acceleration.X86</c>) register their
/// implementations via <see cref="System.Runtime.CompilerServices.ModuleInitializerAttribute"/>
/// so the dispatcher selects the best available kernel without any
/// runtime reflection.
/// </summary>
public static class AccelerationDispatcher
{
    private static readonly Dictionary<Type, object> s_kernels = [];
    private static readonly Lock s_gate = new();

    /// <summary>
    /// Registers a hardware-specific implementation of a kernel contract.
    /// </summary>
    /// <typeparam name="TKernel">The kernel contract interface.</typeparam>
    /// <param name="kernel">A concrete implementation of <typeparamref name="TKernel"/>.</param>
    /// <remarks>
    /// Called from <c>ModuleInitializer</c> in hardware-specific assemblies.
    /// If multiple registrations occur for the same contract, the one with
    /// the highest <see cref="AccelerationTier"/> wins.
    /// </remarks>
    public static void Register<TKernel>(TKernel kernel)
        where TKernel : class, IAcceleratedKernel
    {
        ArgumentNullException.ThrowIfNull(kernel);
        lock (s_gate)
        {
            if (s_kernels.TryGetValue(typeof(TKernel), out var existing)
                && existing is IAcceleratedKernel current
                && current.IsaTier >= kernel.IsaTier)
            {
                return;
            }
            s_kernels[typeof(TKernel)] = kernel;
        }
    }

    /// <summary>
    /// Resolves the highest-tier kernel registered for
    /// <typeparamref name="TKernel"/>, falling back to
    /// <paramref name="scalarFallback"/> when no hardware implementation
    /// is present.
    /// </summary>
    public static TKernel Resolve<TKernel>(TKernel scalarFallback)
        where TKernel : class, IAcceleratedKernel
    {
        ArgumentNullException.ThrowIfNull(scalarFallback);
        lock (s_gate)
        {
            if (s_kernels.TryGetValue(typeof(TKernel), out var k) && k is TKernel typed)
            {
                return typed;
            }
            return scalarFallback;
        }
    }

    /// <summary>
    /// Attempts to look up a hardware kernel without falling back to a scalar.
    /// </summary>
    public static bool TryResolve<TKernel>([NotNullWhen(true)] out TKernel? kernel)
        where TKernel : class, IAcceleratedKernel
    {
        lock (s_gate)
        {
            if (s_kernels.TryGetValue(typeof(TKernel), out var k) && k is TKernel typed)
            {
                kernel = typed;
                return true;
            }
            kernel = null;
            return false;
        }
    }

    /// <summary>
    /// Test-only / shutdown helper: clears all registered kernels.
    /// </summary>
    public static void Reset()
    {
        lock (s_gate)
        {
            s_kernels.Clear();
        }
    }
}
