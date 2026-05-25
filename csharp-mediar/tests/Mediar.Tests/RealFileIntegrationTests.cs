using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Real-file smoke tests. These probe an external fixtures directory
/// containing Creative-Commons / public-domain media files and run every
/// demuxer end-to-end. The fixtures are NOT committed to the repository
/// (size + licensing). Point the <c>MEDIAR_FIXTURES</c> environment
/// variable at a directory of files named <c>sample.&lt;ext&gt;</c>; when
/// the variable is unset the tests are skipped.
///
/// To regenerate the fixtures locally, see
/// <c>tests/Mediar.Tests/FixtureSources.md</c>.
/// </summary>
public sealed class RealFileIntegrationTests
{
    private static string? FixturesDir => Environment.GetEnvironmentVariable("MEDIAR_FIXTURES");

    public static TheoryData<string, string> FixturesWithExpectedFormat()
    {
        var d = new TheoryData<string, string>();
        d.Add("sample.wav",  "wav");
        d.Add("sample.mp3",  "mp3");
        d.Add("sample.flac", "flac");
        d.Add("sample.ogg",  "ogg");
        d.Add("sample.opus", "ogg");
        d.Add("sample.aac",  "aac");
        d.Add("sample.mp4",  "mp4");
        d.Add("sample.m4a",  "mp4");
        d.Add("sample.mkv",  "matroska");
        d.Add("sample.webm", "matroska");
        d.Add("sample.aiff", "aiff");
        d.Add("sample.avi",  "avi");
        return d;
    }

    [Theory]
    [MemberData(nameof(FixturesWithExpectedFormat))]
    public async Task Probe_RealFile_Succeeds(string fileName, string expectedFormat)
    {
        if (FixturesDir is null) return; // soft skip
        string path = Path.Combine(FixturesDir, fileName);
        if (!File.Exists(path)) return; // soft skip if a single fixture is missing

        var info = await MediarOperations.ProbeAsync(path);
        Assert.Equal(expectedFormat, info.Format);
        Assert.NotEmpty(info.Tracks);

        // Every demuxer must produce at least one valid track with a non-default time-base.
        foreach (var t in info.Tracks)
        {
            Assert.True(t.TimeBase.Denominator > 0, $"track #{t.Index} has invalid time-base");
        }
    }

    [Fact]
    public async Task ReadSamples_RealMp4_ProducesNonZeroFrames()
    {
        if (FixturesDir is null) return;
        string path = Path.Combine(FixturesDir, "sample.mp4");
        if (!File.Exists(path)) return;

        await using var dx = MediarOperations.Open(path);
        int sampleCount = 0;
        long totalBytes = 0;
        await foreach (var s in dx.ReadSamplesAsync(default))
        {
            sampleCount++;
            totalBytes += s.Data.Length;
            s.Owner?.Dispose();
            if (sampleCount > 1000) break;
        }
        Assert.True(sampleCount > 0, "demuxer produced no samples");
        Assert.True(totalBytes > 0, "demuxer produced empty samples");
    }

    [Fact]
    public async Task ReadSamples_RealFlac_ProducesNonZeroFrames()
    {
        if (FixturesDir is null) return;
        string path = Path.Combine(FixturesDir, "sample.flac");
        if (!File.Exists(path)) return;

        await using var dx = MediarOperations.Open(path);
        Assert.NotNull(dx.Metadata.Title);
        int frames = 0;
        await foreach (var s in dx.ReadSamplesAsync(default))
        {
            frames++;
            s.Owner?.Dispose();
            if (frames > 50) break;
        }
        Assert.True(frames > 0);
    }

    [Fact]
    public async Task ExtractAudio_FromRealMp4_RoundTrips()
    {
        if (FixturesDir is null) return;
        string src = Path.Combine(FixturesDir, "sample.mp4");
        if (!File.Exists(src)) return;

        string dst = Path.Combine(Path.GetTempPath(), $"mediar-extract-{Guid.NewGuid():N}.m4a");
        try
        {
            await MediarOperations.ExtractAudioAsync(src, dst);
            Assert.True(new FileInfo(dst).Length > 100);
            // Round-trip read back.
            await using var dx = MediarOperations.Open(dst);
            Assert.Contains(dx.Tracks, t => t.Kind == StreamKind.Audio);
        }
        finally
        {
            if (File.Exists(dst)) File.Delete(dst);
        }
    }

    [Fact]
    public async Task Transmux_RealMp4_ToMkv_RoundTrips()
    {
        if (FixturesDir is null) return;
        string src = Path.Combine(FixturesDir, "sample.mp4");
        if (!File.Exists(src)) return;

        string dst = Path.Combine(Path.GetTempPath(), $"mediar-tx-{Guid.NewGuid():N}.mkv");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            Assert.True(new FileInfo(dst).Length > 1024);
            await using var dx = MediarOperations.Open(dst);
            Assert.True(dx.Tracks.Count >= 2);
            Assert.Contains(dx.Tracks, t => t.Codec.Codec == CodecId.H264);
            Assert.Contains(dx.Tracks, t => t.Codec.Codec == CodecId.Aac);
        }
        finally
        {
            if (File.Exists(dst)) File.Delete(dst);
        }
    }
}
