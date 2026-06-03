namespace Mediar.Acceleration;

/// <summary>
/// One-stop accessor for hardware-accelerated kernels. Each property
/// resolves the best available implementation registered with
/// <see cref="AccelerationDispatcher"/>, falling back to a scalar
/// implementation when no hardware kernel is present.
/// </summary>
/// <remarks>
/// Hardware-specific assemblies — <c>Mediar.Acceleration.X86</c> and
/// <c>Mediar.Acceleration.Arm</c> — register their kernels via a
/// <c>ModuleInitializer</c>, so simply referencing the assembly is
/// enough to activate it. Callers do not need to inspect ISA support
/// themselves.
/// </remarks>
public static class Kernels
{
    /// <summary>The active byte-saturation kernel for the current CPU.</summary>
    public static IByteSaturator ByteSaturator
        => AccelerationDispatcher.Resolve<IByteSaturator>(ScalarByteSaturator.Instance);

    /// <summary>
    /// The active 8×8 inverse-DCT kernel. All registered backends are
    /// bit-exact with <see cref="ScalarIdct8x8"/> by construction (same
    /// integer Loeffler constants and rounding).
    /// </summary>
    public static IIdct8x8 Idct8x8
        => AccelerationDispatcher.Resolve<IIdct8x8>(ScalarIdct8x8.Instance);
}
