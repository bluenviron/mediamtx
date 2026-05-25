using Mediar.Subtitles.Srt;
using Mediar.Subtitles.WebVtt;
using Xunit;

namespace Mediar.Tests;

public sealed class SubtitleTests
{
    [Fact]
    public void Srt_RoundTrip_Preserves_Cues()
    {
        var cues = new[]
        {
            new SrtCue(1, TimeSpan.FromMilliseconds(1500), TimeSpan.FromMilliseconds(4000), "Hello, world!"),
            new SrtCue(2, TimeSpan.FromSeconds(5), TimeSpan.FromSeconds(7), "Second\ncue"),
            new SrtCue(3, new TimeSpan(0, 1, 1, 2, 345), new TimeSpan(0, 1, 1, 5, 678), "Line three"),
        };

        string serialized = SrtWriter.WriteString(cues);
        Assert.Contains("00:00:01,500 --> 00:00:04,000", serialized);
        Assert.Contains("01:01:02,345 --> 01:01:05,678", serialized);

        var parsed = SrtReader.ReadString(serialized).ToArray();
        Assert.Equal(cues.Length, parsed.Length);
        for (int i = 0; i < cues.Length; i++)
        {
            Assert.Equal(cues[i].Index, parsed[i].Index);
            Assert.Equal(cues[i].Start, parsed[i].Start);
            Assert.Equal(cues[i].End, parsed[i].End);
            Assert.Equal(cues[i].Text, parsed[i].Text);
        }
    }

    [Fact]
    public void Srt_Without_Index_Still_Parses()
    {
        const string noIndex = "00:00:00,500 --> 00:00:02,000\nFirst\n\n00:00:02,500 --> 00:00:04,000\nSecond\n";
        var cues = SrtReader.ReadString(noIndex).ToArray();
        Assert.Equal(2, cues.Length);
        Assert.Equal("First", cues[0].Text);
        Assert.Equal("Second", cues[1].Text);
    }

    [Fact]
    public void WebVtt_RoundTrip_Preserves_Cues()
    {
        var cues = new[]
        {
            new WebVttCue("intro", TimeSpan.FromSeconds(1), TimeSpan.FromSeconds(3), "align:start", "Hello, WebVTT"),
            new WebVttCue(string.Empty, TimeSpan.FromSeconds(4), TimeSpan.FromSeconds(6), string.Empty, "Line A\nLine B"),
        };

        string serialized = WebVttWriter.WriteString(cues);
        Assert.StartsWith("WEBVTT", serialized);
        Assert.Contains("00:00:01.000 --> 00:00:03.000 align:start", serialized);

        var parsed = WebVttReader.ReadString(serialized).ToArray();
        Assert.Equal(cues.Length, parsed.Length);
        Assert.Equal(cues[0].Identifier, parsed[0].Identifier);
        Assert.Equal(cues[0].Start, parsed[0].Start);
        Assert.Equal(cues[0].End, parsed[0].End);
        Assert.Equal(cues[0].Settings, parsed[0].Settings);
        Assert.Equal(cues[0].Text, parsed[0].Text);
        Assert.Equal(cues[1].Text, parsed[1].Text);
    }
}
