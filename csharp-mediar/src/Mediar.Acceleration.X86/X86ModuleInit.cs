using System.Diagnostics.CodeAnalysis;
using System.Runtime.CompilerServices;

namespace Mediar.Acceleration.X86;

/// <summary>
/// Module initializer that registers all available x86 kernels with
/// <see cref="AccelerationDispatcher"/>. The CLR invokes this exactly
/// once when the assembly is loaded, before any user code runs.
/// </summary>
internal static class X86ModuleInit
{
    [ModuleInitializer]
    [SuppressMessage("Usage", "CA2255:The 'ModuleInitializer' attribute should not be used in libraries",
        Justification = "Deliberate self-registration of accelerated kernels; this assembly's entire purpose is to hook itself into AccelerationDispatcher at load time.")]
    internal static void Register()
    {
        if (Avx2ByteSaturator.IsSupported)
        {
            AccelerationDispatcher.Register<IByteSaturator>(Avx2ByteSaturator.Instance);
        }
        else if (Sse2ByteSaturator.IsSupported)
        {
            AccelerationDispatcher.Register<IByteSaturator>(Sse2ByteSaturator.Instance);
        }
    }
}
