using Mediar.Subtitles.Ass;
using Xunit;

namespace Mediar.Tests;

public sealed class AssTests
{
    private const string Sample =
        "[Script Info]\nScriptType: v4.00+\nPlayResX: 1280\nPlayResY: 720\n\n" +
        "[V4+ Styles]\nFormat: Name, Fontname, Fontsize\nStyle: Default,Arial,28\n\n" +
        "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
        "Dialogue: 0,0:00:01.00,0:00:03.50,Default,Speaker,0000,0000,0000,,Hello, world\n" +
        "Dialogue: 1,0:01:02.34,0:01:05.00,Default,,0010,0010,0050,,Second line, with comma\n";

    [Fact]
    public void Parses_All_Events()
    {
        var script = AssReader.ReadString(Sample);
        Assert.Equal(2, script.Events.Count);
        Assert.Equal("Hello, world", script.Events[0].Text);
        Assert.Equal(TimeSpan.FromSeconds(1), script.Events[0].Start);
        Assert.Equal(new TimeSpan(0, 0, 0, 3, 500), script.Events[0].End);
        Assert.Equal("Second line, with comma", script.Events[1].Text);
        Assert.Equal(1, script.Events[1].Layer);
    }

    [Fact]
    public void Preserves_Script_Info()
    {
        var script = AssReader.ReadString(Sample);
        Assert.Contains(script.ScriptInfo, kv => kv.Key == "PlayResX" && kv.Value == "1280");
        Assert.Contains(script.ScriptInfo, kv => kv.Key == "PlayResY" && kv.Value == "720");
    }

    [Fact]
    public void RoundTrips_Through_Write()
    {
        var script = AssReader.ReadString(Sample);
        var text = AssWriter.WriteString(script);
        var script2 = AssReader.ReadString(text);
        Assert.Equal(script.Events.Count, script2.Events.Count);
        for (int i = 0; i < script.Events.Count; i++)
        {
            Assert.Equal(script.Events[i].Text, script2.Events[i].Text);
            Assert.Equal(script.Events[i].Start, script2.Events[i].Start);
            Assert.Equal(script.Events[i].End, script2.Events[i].End);
        }
    }
}
