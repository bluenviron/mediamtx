using System.Diagnostics.CodeAnalysis;
using System.Runtime.CompilerServices;

namespace Mediar.Acceleration.Arm;

/// <summary>
/// Module initializer that registers all available Arm kernels with
/// <see cref="AccelerationDispatcher"/>. The CLR invokes this exactly
/// once when the assembly is loaded.
/// </summary>
internal static class ArmModuleInit
{
    [ModuleInitializer]
    [SuppressMessage("Usage", "CA2255:The 'ModuleInitializer' attribute should not be used in libraries",
        Justification = "Deliberate self-registration of accelerated kernels; this assembly's entire purpose is to hook itself into AccelerationDispatcher at load time.")]
    internal static void Register()
    {
        if (NeonByteSaturator.IsSupported)
        {
            AccelerationDispatcher.Register<IByteSaturator>(NeonByteSaturator.Instance);
        }
    }
}
