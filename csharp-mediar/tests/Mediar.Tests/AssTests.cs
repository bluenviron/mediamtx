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

    // ----- Reader: lines that should be ignored -----

    [Fact]
    public void Reader_SkipsBlankLinesCommentsAndBangPrefixedLines()
    {
        const string s =
            "[Script Info]\n" +
            "; this is a comment\n" +
            "\n" +
            "!: legacy bang line\n" +
            "PlayResX: 320\n";
        var script = AssReader.ReadString(s);
        // Only the real key-value should land in ScriptInfo.
        Assert.Single(script.ScriptInfo);
        Assert.Equal("PlayResX", script.ScriptInfo[0].Key);
        Assert.Equal("320", script.ScriptInfo[0].Value);
    }

    // ----- Reader: V4 (legacy) styles route to the same StyleSection bucket -----

    [Fact]
    public void Reader_AcceptsLegacyV4StylesSection()
    {
        const string s =
            "[V4 Styles]\nFormat: Name, Fontname\nStyle: Default,Arial\n";
        var script = AssReader.ReadString(s);
        Assert.Collection(
            script.StyleSection,
            l => Assert.Equal("Format: Name, Fontname", l),
            l => Assert.Equal("Style: Default,Arial", l));
    }

    // ----- Reader: event "Kind" round-trip (Dialogue vs Comment) -----

    [Fact]
    public void Reader_PreservesEventKind()
    {
        const string s =
            "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
            "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0000,0000,0000,,line one\n" +
            "Comment: 0,0:00:03.00,0:00:04.00,Default,,0000,0000,0000,,annotation\n";
        var script = AssReader.ReadString(s);
        Assert.Equal(2, script.Events.Count);
        Assert.Equal("Dialogue", script.Events[0].Kind);
        Assert.Equal("Comment", script.Events[1].Kind);
    }

    // ----- Reader: alternate Format field order / "Actor" alias -----

    [Fact]
    public void Reader_HonoursCustomFieldOrderAndActorAlias()
    {
        // Different ordering, and uses "Actor" rather than "Name".
        const string s =
            "[Events]\nFormat: Start, End, Text, Actor, Layer\n" +
            "Dialogue: 0:00:05.00,0:00:06.00,hi there,Bob,7\n";
        var script = AssReader.ReadString(s);
        var ev = Assert.Single(script.Events);
        Assert.Equal(TimeSpan.FromSeconds(5), ev.Start);
        Assert.Equal(TimeSpan.FromSeconds(6), ev.End);
        Assert.Equal("hi there", ev.Text);
        Assert.Equal("Bob", ev.Name);
        Assert.Equal(7, ev.Layer);
    }

    // ----- Reader: malformed time stays at default (TryParse returns false) -----

    [Fact]
    public void Reader_LeavesStartUnchangedWhenTimeFailsToParse()
    {
        const string s =
            "[Events]\nFormat: Layer, Start, End, Text\n" +
            "Dialogue: 0,not-a-time,0:00:02.00,abc\n";
        var script = AssReader.ReadString(s);
        var ev = Assert.Single(script.Events);
        Assert.Equal(TimeSpan.Zero, ev.Start);
        Assert.Equal(TimeSpan.FromSeconds(2), ev.End);
        Assert.Equal("abc", ev.Text);
    }

    [Fact]
    public void Reader_RejectsNullReader()
    {
        Assert.Throws<ArgumentNullException>(() => AssReader.Read(null!));
    }

    // ----- Writer: field formatting -----

    [Fact]
    public void Writer_FormatsMarginsAsFourDigits()
    {
        var script = new AssScript();
        script.Events.Add(new AssEvent
        {
            Layer = 2,
            Start = new TimeSpan(0, 1, 30, 0, 250),
            End = new TimeSpan(0, 1, 32, 0, 750),
            Style = "Default",
            Name = "X",
            MarginL = 5,
            MarginR = 12,
            MarginV = 999,
            Effect = "",
            Text = "test, line",
        });
        var text = AssWriter.WriteString(script);
        // D4-formatted margins.
        Assert.Contains(",0005,0012,0999,", text);
        // Time format H:MM:SS.CS.
        Assert.Contains("1:30:00.25", text);
        Assert.Contains("1:32:00.75", text);
    }

    [Fact]
    public void Writer_EmitsDefaultFormatLineWhenNoFieldsSpecified()
    {
        var script = new AssScript();
        var text = AssWriter.WriteString(script);
        Assert.Contains("[Events]", text);
        Assert.Contains("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text", text);
    }

    [Fact]
    public void Writer_RejectsNullArguments()
    {
        Assert.Throws<ArgumentNullException>(() => AssWriter.Write(null!, new AssScript()));
        using var sw = new StringWriter();
        Assert.Throws<ArgumentNullException>(() => AssWriter.Write(sw, null!));
    }

    [Fact]
    public void Writer_SkipsStyleSectionHeaderWhenEmpty()
    {
        var script = new AssScript();
        script.ScriptInfo.Add(new KeyValuePair<string, string>("PlayResX", "640"));
        var text = AssWriter.WriteString(script);
        Assert.DoesNotContain("[V4+ Styles]", text);
    }

    // ----- Round trip via file (covers WriteFile + ReadFile) -----

    [Fact]
    public void File_RoundTrip_PreservesEvents()
    {
        var script = AssReader.ReadString(Sample);
        var path = Path.Combine(Path.GetTempPath(), $"ass-roundtrip-{Guid.NewGuid():N}.ass");
        try
        {
            AssWriter.WriteFile(path, script);
            var loaded = AssReader.ReadFile(path);
            Assert.Equal(script.Events.Count, loaded.Events.Count);
            for (int i = 0; i < script.Events.Count; i++)
            {
                Assert.Equal(script.Events[i].Text, loaded.Events[i].Text);
                Assert.Equal(script.Events[i].Start, loaded.Events[i].Start);
                Assert.Equal(script.Events[i].End, loaded.Events[i].End);
                Assert.Equal(script.Events[i].Layer, loaded.Events[i].Layer);
            }
        }
        finally
        {
            if (File.Exists(path)) File.Delete(path);
        }
    }

    // ----- AssEvent.ToString sanity -----

    [Fact]
    public void Event_ToString_IncludesKindStyleAndTimes()
    {
        var ev = new AssEvent
        {
            Layer = 4,
            Start = new TimeSpan(0, 0, 0, 1, 0),
            End = new TimeSpan(0, 0, 0, 2, 0),
            Style = "Big",
            Text = "hi",
        };
        var s = ev.ToString();
        Assert.Contains("[Dialogue L4]", s);
        Assert.Contains("(Big)", s);
        Assert.Contains("0:00:01.00", s);
        Assert.Contains("0:00:02.00", s);
        Assert.Contains("hi", s);
    }

    [Fact]
    public void Event_HasExpectedDefaults()
    {
        var ev = new AssEvent();
        Assert.Equal("Dialogue", ev.Kind);
        Assert.Equal(0, ev.Layer);
        Assert.Equal(TimeSpan.Zero, ev.Start);
        Assert.Equal(TimeSpan.Zero, ev.End);
        Assert.Equal("Default", ev.Style);
        Assert.Equal(string.Empty, ev.Name);
        Assert.Equal(0, ev.MarginL);
        Assert.Equal(0, ev.MarginR);
        Assert.Equal(0, ev.MarginV);
        Assert.Equal(string.Empty, ev.Effect);
        Assert.Equal(string.Empty, ev.Text);
    }

    [Fact]
    public void Event_Record_Equality_HoldsForEquivalentInstances()
    {
        var a = new AssEvent { Layer = 1, Text = "hi", Start = TimeSpan.FromSeconds(1) };
        var b = new AssEvent { Layer = 1, Text = "hi", Start = TimeSpan.FromSeconds(1) };
        Assert.Equal(a, b);
        Assert.Equal(a.GetHashCode(), b.GetHashCode());

        var c = a with { Text = "bye" };
        Assert.NotEqual(a, c);
        Assert.Equal("bye", c.Text);
    }

    [Fact]
    public void Script_Constructor_StartsWithEmptyCollections()
    {
        var s = new AssScript();
        Assert.Empty(s.ScriptInfo);
        Assert.Empty(s.StyleSection);
        Assert.Empty(s.EventFields);
        Assert.Empty(s.Events);
    }

    [Fact]
    public void Reader_AcceptsEmptyDocument()
    {
        var script = AssReader.ReadString(string.Empty);
        Assert.Empty(script.Events);
        Assert.Empty(script.ScriptInfo);
        Assert.Empty(script.StyleSection);
    }

    [Fact]
    public void Reader_TextWithManyCommas_FullyPreserved()
    {
        const string s =
            "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
            "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0000,0000,0000,,a,b,c,d,e,f,g\n";
        var script = AssReader.ReadString(s);
        var ev = Assert.Single(script.Events);
        Assert.Equal("a,b,c,d,e,f,g", ev.Text);
    }

    [Fact]
    public void Reader_LayerNotAnInt_DefaultsToZero()
    {
        const string s =
            "[Events]\nFormat: Layer, Start, End, Text\n" +
            "Dialogue: not-a-number,0:00:01.00,0:00:02.00,hello\n";
        var script = AssReader.ReadString(s);
        var ev = Assert.Single(script.Events);
        Assert.Equal(0, ev.Layer);
        Assert.Equal("hello", ev.Text);
    }

    [Theory]
    [InlineData("MarginL")]
    [InlineData("MarginR")]
    [InlineData("MarginV")]
    public void Reader_MalformedMargin_DefaultsToZero(string marginField)
    {
        string s =
            "[Events]\nFormat: Layer, Start, End, " + marginField + ", Text\n" +
            "Dialogue: 0,0:00:01.00,0:00:02.00,not-a-num,hello\n";
        var script = AssReader.ReadString(s);
        var ev = Assert.Single(script.Events);
        Assert.Equal(0, marginField switch
        {
            "MarginL" => ev.MarginL,
            "MarginR" => ev.MarginR,
            _ => ev.MarginV,
        });
    }

    [Fact]
    public void Reader_UnknownFieldName_IgnoredButLineStillParses()
    {
        const string s =
            "[Events]\nFormat: Layer, Start, End, Bogus, Text\n" +
            "Dialogue: 5,0:00:01.00,0:00:02.00,ignored,line one\n";
        var script = AssReader.ReadString(s);
        var ev = Assert.Single(script.Events);
        Assert.Equal(5, ev.Layer);
        Assert.Equal("line one", ev.Text);
    }

    [Fact]
    public void Reader_ScriptInfo_ColonInValue_KeepsAfterFirstColon()
    {
        const string s = "[Script Info]\nTitle: Foo: with colon\n";
        var script = AssReader.ReadString(s);
        var kv = Assert.Single(script.ScriptInfo);
        Assert.Equal("Title", kv.Key);
        Assert.Equal("Foo: with colon", kv.Value);
    }

    [Fact]
    public void Reader_RejectsNullForReadFile()
    {
        // ReadFile uses StreamReader(path, ...) which itself throws.
        Assert.Throws<ArgumentNullException>(() => AssReader.ReadFile(null!));
    }

    [Fact]
    public void Reader_LineWithoutFormat_ThrowsOnEventLine()
    {
        // Events section but no Format line — ParseEvent can't size the parts
        // array correctly; current implementation throws when encountering an
        // event line without a preceding Format directive.
        const string s =
            "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0000,0000,0000,,hello\n";
        Assert.ThrowsAny<Exception>(() => AssReader.ReadString(s));
    }

    [Fact]
    public void Writer_PreservesCustomEventFieldsInFormatLine()
    {
        var script = new AssScript();
        script.EventFields.Add("Start");
        script.EventFields.Add("End");
        script.EventFields.Add("Text");
        script.Events.Add(new AssEvent
        {
            Start = TimeSpan.FromSeconds(1),
            End = TimeSpan.FromSeconds(2),
            Text = "hi",
        });
        var text = AssWriter.WriteString(script);
        Assert.Contains("Format: Start, End, Text", text);
        Assert.Contains("Dialogue: 0:00:01.00,0:00:02.00,hi", text);
    }

    [Fact]
    public void Writer_NegativeStart_FormattedAsZero()
    {
        var script = new AssScript();
        script.Events.Add(new AssEvent
        {
            Start = TimeSpan.FromSeconds(-5),
            End = TimeSpan.FromSeconds(1),
            Text = "x",
        });
        var text = AssWriter.WriteString(script);
        Assert.Contains(",0:00:00.00,", text);
    }

    [Fact]
    public void Writer_TimeFormat_CentisecondsFloorOfMilliseconds()
    {
        var script = new AssScript();
        script.Events.Add(new AssEvent
        {
            Start = new TimeSpan(0, 0, 0, 0, 999),
            End = new TimeSpan(0, 0, 0, 0, 100),
            Text = "x",
        });
        var text = AssWriter.WriteString(script);
        Assert.Contains("0:00:00.99", text);
        Assert.Contains("0:00:00.10", text);
    }

    [Fact]
    public void Writer_StyleSection_EmitsSectionWhenPopulated()
    {
        var script = new AssScript();
        script.StyleSection.Add("Format: Name, Fontname");
        script.StyleSection.Add("Style: Default,Arial");
        var text = AssWriter.WriteString(script);
        Assert.Contains("[V4+ Styles]", text);
        Assert.Contains("Format: Name, Fontname", text);
        Assert.Contains("Style: Default,Arial", text);
    }

    [Fact]
    public void Reader_BomPrefixedFile_RoundTrips()
    {
        // ReadFile detects BOM; sanity check that BOM-prefixed UTF-8 files load.
        var path = Path.Combine(Path.GetTempPath(), $"ass-bom-{Guid.NewGuid():N}.ass");
        try
        {
            File.WriteAllText(path, Sample, new System.Text.UTF8Encoding(encoderShouldEmitUTF8Identifier: true));
            var script = AssReader.ReadFile(path);
            Assert.Equal(2, script.Events.Count);
        }
        finally
        {
            if (File.Exists(path)) File.Delete(path);
        }
    }
}

