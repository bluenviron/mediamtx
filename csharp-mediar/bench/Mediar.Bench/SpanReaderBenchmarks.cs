using BenchmarkDotNet.Attributes;
using Mediar.IO;

namespace Mediar.Bench;

[MemoryDiagnoser]
public class SpanReaderBenchmarks
{
    private byte[] _buffer = null!;

    [GlobalSetup]
    public void Setup()
    {
        _buffer = new byte[64 * 1024];
        new Random(42).NextBytes(_buffer);
    }

    [Benchmark]
    public ulong BigEndian_Read_64KiB()
    {
        var r = new BigEndianSpanReader(_buffer);
        ulong acc = 0;
        int iterations = _buffer.Length / 8;
        for (int i = 0; i < iterations; i++)
        {
            acc ^= r.ReadUInt32();
            acc ^= r.ReadUInt32();
        }
        return acc;
    }

    [Benchmark]
    public ulong LittleEndian_Read_64KiB()
    {
        var r = new LittleEndianSpanReader(_buffer);
        ulong acc = 0;
        int iterations = _buffer.Length / 8;
        for (int i = 0; i < iterations; i++)
        {
            acc ^= r.ReadUInt32();
            acc ^= r.ReadUInt32();
        }
        return acc;
    }

    [Benchmark]
    public ulong BitReader_Read1Bit_64KiB()
    {
        var r = new BitReader(_buffer);
        ulong acc = 0;
        int bits = _buffer.Length * 8;
        for (int i = 0; i < bits; i++)
        {
            acc ^= r.ReadBits(1);
        }
        return acc;
    }

    [Benchmark]
    public ulong BitReader_Read16Bit_64KiB()
    {
        var r = new BitReader(_buffer);
        ulong acc = 0;
        int iterations = (_buffer.Length * 8) / 16;
        for (int i = 0; i < iterations; i++)
        {
            acc ^= r.ReadBits(16);
        }
        return acc;
    }
}
