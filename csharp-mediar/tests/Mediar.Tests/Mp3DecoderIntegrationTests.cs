using Mediar.Codecs;
using Mediar.Codecs.Mp3.Decoder;
using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// End-to-end integration tests that demux a real-world MP3 file from disk and run every
/// frame through <see cref="Mp3Decoder"/>. The fixture
/// (<c>TestData/Mp3/anime-wow-sound-effect.mp3</c>) is copied to the test output by
/// <c>Mediar.Tests.csproj</c>.
/// </summary>
public sealed class Mp3DecoderIntegrationTests
{
    private const string FixtureRelativePath = "TestData/Mp3/anime-wow-sound-effect.mp3";

    /// <summary>Resolve the fixture path next to the test assembly.</summary>
    private static string GetFixturePath()
    {
        var basePath = AppContext.BaseDirectory;
        var combined = Path.Combine(basePath, "TestData", "Mp3", "anime-wow-sound-effect.mp3");
        return combined;
    }

    private static bool FixtureExists() => File.Exists(GetFixturePath());

    [Fact]
    public void Fixture_Is_Copied_To_Output()
    {
        Assert.True(
            FixtureExists(),
            $"MP3 fixture '{FixtureRelativePath}' not found at '{GetFixturePath()}'. " +
            "Make sure Mediar.Tests.csproj copies TestData/** to the output directory.");
    }

    [Fact]
    public async Task Demuxer_Parses_Real_File()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        await using var fs = File.OpenRead(GetFixturePath());
        using var demux = Mp3Demuxer.Open(GetFixturePath());

        Assert.Single(demux.Tracks);
        var track = demux.Tracks[0];
        var codec = Assert.IsType<AudioCodecParameters>(track.Codec);
        Assert.Equal(CodecId.Mp3, codec.Codec);
        Assert.True(codec.SampleRate > 0, $"SampleRate should be positive, was {codec.SampleRate}");
        Assert.InRange(codec.Channels, 1, 2);
    }

    /// <summary>
    /// Walk every frame in the file through the decoder and assert basic correctness
    /// properties: no exceptions, channel/sample-rate consistency, monotonic PTS,
    /// expected per-frame sample count, finite samples in [-1, 1], and aggregate
    /// duration matching the demuxer's per-sample duration metadata.
    /// </summary>
    [Fact]
    public async Task Decodes_Every_Frame_Without_Errors()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        using var demux = Mp3Demuxer.Open(GetFixturePath());
        var track = demux.Tracks[0];
        var codec = (AudioCodecParameters)track.Codec;
        var expectedChannels = codec.Channels;
        var expectedSampleRate = codec.SampleRate;

        using var decoder = new Mp3Decoder(codec);

        int frameCount = 0;
        long totalDecodedSamplesPerChannel = 0;
        long totalDemuxerDurationSamples = 0;
        long? previousPts = null;
        float minSample = 0f;
        float maxSample = 0f;
        bool sawNaN = false;
        bool sawInf = false;

        await foreach (var sample in demux.ReadSamplesAsync())
        {
            Assert.True(sample.Data.Length >= 4, $"Frame {frameCount} too small ({sample.Data.Length} bytes)");
            Assert.Equal(0xFF, sample.Data.Span[0]);
            Assert.Equal(0xE0, sample.Data.Span[1] & 0xE0);

            DecodedAudioFrame decoded = default;
            try
            {
                decoded = decoder.Decode(sample.Data.Span, sample.Pts);
            }
            catch (Exception ex)
            {
                Assert.Fail($"Decode threw at frame {frameCount}, pts={sample.Pts}: {ex}");
            }

            try
            {
                Assert.Equal(expectedChannels, decoded.Channels);
                Assert.Equal(expectedSampleRate, decoded.SampleRate);
                Assert.True(decoded.SamplesPerChannel > 0, $"Frame {frameCount} produced no samples.");

                // Per-frame sample count must match what the demuxer says.
                Assert.Equal(sample.Duration, decoded.SamplesPerChannel);

                // Interleaved buffer length sanity.
                Assert.Equal(decoded.SamplesPerChannel * decoded.Channels, decoded.Samples.Length);

                // Monotonic PTS (demuxer pts is per-sample at SampleRate; verify never goes backwards).
                if (previousPts.HasValue)
                {
                    Assert.True(sample.Pts >= previousPts.Value, $"PTS regressed at frame {frameCount}: {previousPts} → {sample.Pts}");
                }
                previousPts = sample.Pts;

                var span = decoded.Samples.Span;
                for (int i = 0; i < span.Length; i++)
                {
                    float v = span[i];
                    if (float.IsNaN(v)) sawNaN = true;
                    else if (float.IsInfinity(v)) sawInf = true;
                    else
                    {
                        if (v < minSample) minSample = v;
                        if (v > maxSample) maxSample = v;
                    }
                }

                totalDecodedSamplesPerChannel += decoded.SamplesPerChannel;
                totalDemuxerDurationSamples += sample.Duration;
                frameCount++;
            }
            finally
            {
                decoded.Dispose();
                sample.Owner?.Dispose();
            }
        }

        Assert.True(frameCount > 0, "Expected at least one MP3 frame in the fixture.");
        Assert.False(sawNaN, "Decoder produced NaN samples.");
        Assert.False(sawInf, "Decoder produced infinite samples.");

        // Output range guard. PCM samples must remain within [-1, 1].
        Assert.True(minSample >= -1.0f, $"Min sample {minSample} below -1.0");
        Assert.True(maxSample <= 1.0f, $"Max sample {maxSample} above 1.0");

        // Sum of per-frame durations from the demuxer must exactly equal the total
        // samples-per-channel that came out of the decoder.
        Assert.Equal(totalDemuxerDurationSamples, totalDecodedSamplesPerChannel);

        // The fixture is "anime-wow-sound-effect.mp3" — a short SFX of order ~1-5 seconds.
        // Verify the decoded length is in a plausible range to catch regressions where the
        // demuxer or decoder aborts after only a handful of frames.
        var decodedSeconds = (double)totalDecodedSamplesPerChannel / expectedSampleRate;
        Assert.InRange(decodedSeconds, 0.5, 30.0);
    }

    /// <summary>
    /// Reset() must clear the bit reservoir and overlap-add state so a second pass over
    /// the same file produces identical decoded samples.
    /// </summary>
    [Fact]
    public async Task Reset_Allows_Replay_With_Identical_Output()
    {
        Assert.True(FixtureExists(), $"Fixture missing: {GetFixturePath()}");

        using var demux1 = Mp3Demuxer.Open(GetFixturePath());
        var codec = (AudioCodecParameters)demux1.Tracks[0].Codec;

        var firstRun = await DecodeAll(demux1, codec);

        // Re-open and decode again through a fresh decoder for a clean baseline.
        using var demux2 = Mp3Demuxer.Open(GetFixturePath());
        var secondRun = await DecodeAll(demux2, codec);

        Assert.Equal(firstRun.FrameCount, secondRun.FrameCount);
        Assert.Equal(firstRun.TotalSamples, secondRun.TotalSamples);
        Assert.Equal(firstRun.Checksum, secondRun.Checksum);

        // Now decode with one decoder, Reset, and decode again. Results must still match.
        using var demux3 = Mp3Demuxer.Open(GetFixturePath());
        using var decoder = new Mp3Decoder(codec);
        var pass1 = await DecodeAllWithDecoder(demux3, decoder);

        decoder.Reset();

        using var demux4 = Mp3Demuxer.Open(GetFixturePath());
        var pass2 = await DecodeAllWithDecoder(demux4, decoder);

        Assert.Equal(pass1.FrameCount, pass2.FrameCount);
        Assert.Equal(pass1.TotalSamples, pass2.TotalSamples);
        Assert.Equal(pass1.Checksum, pass2.Checksum);
    }

    private readonly record struct RunSummary(int FrameCount, long TotalSamples, long Checksum);

    private static async Task<RunSummary> DecodeAll(Mp3Demuxer demux, AudioCodecParameters codec)
    {
        using var decoder = new Mp3Decoder(codec);
        return await DecodeAllWithDecoder(demux, decoder);
    }

    private static async Task<RunSummary> DecodeAllWithDecoder(Mp3Demuxer demux, Mp3Decoder decoder)
    {
        int frames = 0;
        long total = 0;
        long checksum = 0;

        await foreach (var sample in demux.ReadSamplesAsync())
        {
            try
            {
                using var decoded = decoder.Decode(sample.Data.Span, sample.Pts);
                frames++;
                total += decoded.SamplesPerChannel;

                var span = decoded.Samples.Span;
                // Fold every sample into a simple FNV-1a-style 64-bit hash over float bit-patterns.
                for (int i = 0; i < span.Length; i++)
                {
                    uint bits = (uint)BitConverter.SingleToInt32Bits(span[i]);
                    checksum ^= bits;
                    checksum = unchecked((long)((ulong)checksum * 1099511628211UL));
                }
            }
            finally
            {
                sample.Owner?.Dispose();
            }
        }

        return new RunSummary(frames, total, checksum);
    }
}
