using System.Diagnostics;
using Mediar.Codecs.Mp3.Decoder;
using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Performance and memory characteristics of <see cref="Mp3Decoder"/> exercised
/// against the bundled <c>TestData/Mp3/anime-wow-sound-effect.mp3</c> fixture.
/// Always-on tests assert deterministic allocation/heap behavior. The wall-clock
/// throughput test is opt-in via the <c>MEDIAR_MP3_PERF</c> environment variable
/// because microbenchmarks are too noisy for default CI.
/// </summary>
[Collection("Mp3DecoderPerformance")]
public sealed class Mp3DecoderPerformanceTests
{
    private const string FixtureRelativePath = "TestData/Mp3/anime-wow-sound-effect.mp3";

    /// <summary>Per-frame allocation budget (in bytes) once side-info is reused.</summary>
    /// <remarks>
    /// Each <see cref="Mp3Decoder.Decode"/> call rents a PCM buffer from
    /// <see cref="System.Buffers.MemoryPool{T}.Shared"/>; the underlying float
    /// array is pooled but the <c>IMemoryOwner&lt;float&gt;</c> wrapper
    /// allocates per rent (~280 B steady-state on .NET 8/10's
    /// <c>SharedArrayPoolBuffer</c>). 1024 B/frame is the steady-state CI
    /// ceiling — anything beyond that means a new heap allocation has been
    /// (re)introduced into the hot decode path. Switching all audio decoders
    /// from <c>MemoryPool&lt;float&gt;.Shared</c> to a pooled
    /// <see cref="System.Buffers.IMemoryOwner{T}"/> wrapper backed by
    /// <see cref="System.Buffers.ArrayPool{T}.Shared"/> is the path to
    /// further reduce this baseline.
    /// </remarks>
    private const long PerFrameAllocationBudgetBytes = 1024;

    /// <summary>Total managed-memory growth budget after 10 full decode passes.</summary>
    private const long MultiPassMemoryGrowthBudgetBytes = 4L * 1024 * 1024;

    private static string GetFixturePath() =>
        Path.Combine(AppContext.BaseDirectory, "TestData", "Mp3", "anime-wow-sound-effect.mp3");

    private static bool FixtureExists() => File.Exists(GetFixturePath());

    /// <summary>
    /// Preload all demuxed MPEG frame payloads into immutable byte arrays.
    /// Lets the measurement loop be 100% synchronous and free of demuxer/
    /// IO/async allocations.
    /// </summary>
    private static async Task<List<byte[]>> PreloadFramesAsync()
    {
        var frames = new List<byte[]>();
        using var demux = Mp3Demuxer.Open(GetFixturePath());
        await foreach (var sample in demux.ReadSamplesAsync())
        {
            try
            {
                frames.Add(sample.Data.ToArray());
            }
            finally
            {
                sample.Owner?.Dispose();
            }
        }
        return frames;
    }

    private static AudioCodecParameters GetCodecParameters()
    {
        using var demux = Mp3Demuxer.Open(GetFixturePath());
        return (AudioCodecParameters)demux.Tracks[0].Codec;
    }

    private static void DecodeAll(List<byte[]> frames, Mp3Decoder decoder)
    {
        for (int i = 0; i < frames.Count; i++)
        {
            using var f = decoder.Decode(frames[i], i);
        }
    }

    [Fact]
    public async Task Decode_PerFrame_Allocations_Stay_Within_Budget()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        var frames = await PreloadFramesAsync();
        Assert.NotEmpty(frames);

        var codec = GetCodecParameters();
        using var decoder = new Mp3Decoder(codec);

        // Warm up: prime decoder state, JIT all hot paths, fill MemoryPool buckets.
        DecodeAll(frames, decoder);
        decoder.Reset();
        DecodeAll(frames, decoder);

        decoder.Reset();
        // Settle the GC so the measurement starts on a clean baseline.
        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();

        long before = GC.GetAllocatedBytesForCurrentThread();
        DecodeAll(frames, decoder);
        long after = GC.GetAllocatedBytesForCurrentThread();

        long totalAllocated = after - before;
        double bytesPerFrame = totalAllocated / (double)frames.Count;

        Assert.True(
            bytesPerFrame < PerFrameAllocationBudgetBytes,
            $"Decoder allocated {totalAllocated} bytes across {frames.Count} frames " +
            $"({bytesPerFrame:F1} B/frame), exceeding the {PerFrameAllocationBudgetBytes} B/frame budget. " +
            "A heap allocation has been (re)introduced into the hot decode path.");
    }

    [Fact]
    public void Reset_Is_Allocation_Free()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        var codec = GetCodecParameters();
        using var decoder = new Mp3Decoder(codec);

        // Warmup so JIT and any one-shot Reset-related allocations are done.
        for (int i = 0; i < 16; i++) decoder.Reset();

        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();

        long before = GC.GetAllocatedBytesForCurrentThread();
        for (int i = 0; i < 10_000; i++) decoder.Reset();
        long after = GC.GetAllocatedBytesForCurrentThread();

        // No per-call heap allocation is expected — clearing pooled buffers is in-place.
        long delta = after - before;
        Assert.True(delta < 1024,
            $"Mp3Decoder.Reset() allocated {delta} bytes across 10,000 calls; expected ~0.");
    }

    [Fact]
    public async Task Repeated_Full_Decode_Passes_Do_Not_Grow_Heap_Unboundedly()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        var frames = await PreloadFramesAsync();
        var codec = GetCodecParameters();

        // Warmup pass: stabilize JIT + MemoryPool retention.
        using (var warm = new Mp3Decoder(codec))
        {
            for (int pass = 0; pass < 3; pass++)
            {
                DecodeAll(frames, warm);
                warm.Reset();
            }
        }

        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();
        long before = GC.GetTotalMemory(forceFullCollection: true);

        for (int pass = 0; pass < 10; pass++)
        {
            using var dec = new Mp3Decoder(codec);
            DecodeAll(frames, dec);
        }

        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();
        long after = GC.GetTotalMemory(forceFullCollection: true);

        long growth = after - before;
        Assert.True(growth < MultiPassMemoryGrowthBudgetBytes,
            $"Managed heap grew {growth:N0} bytes across 10 decode passes; budget is {MultiPassMemoryGrowthBudgetBytes:N0} bytes. " +
            "This may indicate a leak — buffers being held alive beyond a single decoder lifetime.");
    }

    [Fact]
    public async Task Many_Decoder_Instances_Can_Be_Created_And_Disposed_Without_Leak()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        var codec = GetCodecParameters();
        var frames = await PreloadFramesAsync();

        // Warmup.
        for (int i = 0; i < 8; i++)
        {
            using var d = new Mp3Decoder(codec);
            using var f = d.Decode(frames[0], 0);
        }

        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();
        long before = GC.GetTotalMemory(forceFullCollection: true);

        const int instances = 256;
        for (int i = 0; i < instances; i++)
        {
            using var d = new Mp3Decoder(codec);
            using var f = d.Decode(frames[0], 0);
        }

        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();
        long after = GC.GetTotalMemory(forceFullCollection: true);

        long growth = after - before;
        // Each Mp3Decoder owns ~10-15 KB of state (V-buffers, overlap buffers, reservoir).
        // 256 instances created and disposed should leave essentially zero net retention.
        long budget = 1L * 1024 * 1024;
        Assert.True(growth < budget,
            $"Creating and disposing {instances} Mp3Decoder instances grew the heap by {growth:N0} bytes; expected < {budget:N0}.");
    }

    [Fact]
    public async Task Independent_Decoders_Run_Concurrently_Without_Interference()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        var codec = GetCodecParameters();
        var frames = await PreloadFramesAsync();

        // Baseline: single-threaded checksum.
        long baselineChecksum = ComputeChecksum(frames, codec);

        int workers = Math.Min(4, Environment.ProcessorCount);
        var tasks = new Task<long>[workers];
        for (int t = 0; t < workers; t++)
        {
            tasks[t] = Task.Run(() => ComputeChecksum(frames, codec));
        }
        var results = await Task.WhenAll(tasks);

        for (int i = 0; i < results.Length; i++)
        {
            Assert.Equal(baselineChecksum, results[i]);
        }
    }

    [Fact]
    public async Task Throughput_Exceeds_Realtime_When_Opt_In_Set()
    {
        if (string.IsNullOrEmpty(Environment.GetEnvironmentVariable("MEDIAR_MP3_PERF")))
        {
            // Opt-in via env var. Wall-clock perf assertions are too noisy for default CI.
            return;
        }

        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        var frames = await PreloadFramesAsync();
        var codec = GetCodecParameters();
        int sampleRate = codec.SampleRate;

        using var decoder = new Mp3Decoder(codec);

        // Warmup: enough passes to amortize JIT and pool warmup.
        for (int i = 0; i < 4; i++)
        {
            DecodeAll(frames, decoder);
            decoder.Reset();
        }

        long totalSamplesDecoded = 0;
        const int measuredPasses = 8;

        var sw = Stopwatch.StartNew();
        for (int pass = 0; pass < measuredPasses; pass++)
        {
            for (int i = 0; i < frames.Count; i++)
            {
                using var f = decoder.Decode(frames[i], i);
                totalSamplesDecoded += f.SamplesPerChannel;
            }
            decoder.Reset();
        }
        sw.Stop();

        double seconds = sw.Elapsed.TotalSeconds;
        double samplesPerSecond = totalSamplesDecoded / seconds;
        double realtimeMultiple = samplesPerSecond / sampleRate;

        Assert.True(realtimeMultiple > 1.0,
            $"Decoder ran at {realtimeMultiple:F2}x realtime ({samplesPerSecond:N0} samples/s vs {sampleRate:N0} samples/s); " +
            "expected to be faster than realtime.");
    }

    private static long ComputeChecksum(List<byte[]> frames, AudioCodecParameters codec)
    {
        using var decoder = new Mp3Decoder(codec);
        long checksum = 0;
        for (int i = 0; i < frames.Count; i++)
        {
            using var f = decoder.Decode(frames[i], i);
            var span = f.Samples.Span;
            for (int s = 0; s < span.Length; s++)
            {
                uint bits = (uint)BitConverter.SingleToInt32Bits(span[s]);
                checksum ^= bits;
                checksum = unchecked((long)((ulong)checksum * 1099511628211UL));
            }
        }
        return checksum;
    }
}
