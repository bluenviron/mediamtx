using BenchmarkDotNet.Attributes;
using Mediar.Containers.Mp3;

namespace Mediar.Bench;

[MemoryDiagnoser]
public class Mp3HeaderBenchmarks
{
    private byte[] _header = null!;

    [GlobalSetup]
    public void Setup()
    {
        _header = new byte[] { 0xFF, 0xFB, 0x90, 0x00 };
    }

    [Benchmark]
    public int Parse_1000_Headers()
    {
        int valid = 0;
        for (int i = 0; i < 1000; i++)
        {
            if (Mp3FrameHeader.TryParse(_header, out var h))
            {
                valid += h.FrameSize;
            }
        }
        return valid;
    }
}
