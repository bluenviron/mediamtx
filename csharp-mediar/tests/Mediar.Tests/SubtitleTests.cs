using Mediar.Subtitles.Srt;
using Mediar.Subtitles.WebVtt;
using Xunit;

namespace Mediar.Tests;

public sealed class SubtitleTests
{
    // -------------------- SRT --------------------

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
        Assert.Equal(1, cues[0].Index);
        Assert.Equal(2, cues[1].Index);
    }

    [Fact]
    public void SrtReader_Read_Null_Reader_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SrtReader.Read(null!).ToArray());
    }

    [Fact]
    public void SrtWriter_WriteString_Null_Cues_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SrtWriter.WriteString(null!));
    }

    [Fact]
    public void SrtWriter_Write_Null_Writer_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => SrtWriter.Write(null!, Array.Empty<SrtCue>()));
    }

    [Fact]
    public void SrtWriter_Write_Null_Cues_Throws()
    {
        using var sw = new StringWriter();
        Assert.Throws<ArgumentNullException>(() => SrtWriter.Write(sw, null!));
    }

    [Fact]
    public void Srt_Empty_String_Yields_No_Cues()
    {
        Assert.Empty(SrtReader.ReadString(string.Empty).ToArray());
    }

    [Fact]
    public void Srt_Bom_Is_Stripped()
    {
        string content = "\uFEFF1\n00:00:00,000 --> 00:00:01,000\nHi\n";
        var cues = SrtReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal("Hi", cues[0].Text);
    }

    [Fact]
    public void Srt_Accepts_Dot_Separator_In_Timecode()
    {
        string content = "1\n00:00:01.250 --> 00:00:02.500\nDot\n";
        var cues = SrtReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal(TimeSpan.FromMilliseconds(1250), cues[0].Start);
        Assert.Equal(TimeSpan.FromMilliseconds(2500), cues[0].End);
    }

    [Fact]
    public void Srt_Timecode_With_Trailing_Position_Attrs_Is_Parsed()
    {
        string content = "1\n00:00:01,000 --> 00:00:02,000 X1:0 X2:100\nPositioned\n";
        var cues = SrtReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal(TimeSpan.FromSeconds(1), cues[0].Start);
        Assert.Equal(TimeSpan.FromSeconds(2), cues[0].End);
    }

    [Fact]
    public void Srt_Multiline_Text_Is_Joined_With_Newlines()
    {
        string content = "1\n00:00:00,000 --> 00:00:02,000\nLine 1\nLine 2\nLine 3\n";
        var cues = SrtReader.ReadString(content).ToArray();
        Assert.Equal("Line 1\nLine 2\nLine 3", cues[0].Text);
    }

    [Fact]
    public void Srt_Writer_Auto_Indexes_When_Cue_Index_Is_Zero()
    {
        var cues = new[]
        {
            new SrtCue(0, TimeSpan.Zero, TimeSpan.FromSeconds(1), "A"),
            new SrtCue(0, TimeSpan.FromSeconds(1), TimeSpan.FromSeconds(2), "B"),
        };
        string s = SrtWriter.WriteString(cues);
        Assert.Contains("\r\n1\r\n", "\r\n" + s);
        Assert.Contains("\r\n2\r\n", s);
    }

    [Fact]
    public void Srt_Writer_Preserves_Explicit_Index()
    {
        var cues = new[]
        {
            new SrtCue(42, TimeSpan.Zero, TimeSpan.FromSeconds(1), "X"),
        };
        string s = SrtWriter.WriteString(cues);
        Assert.StartsWith("42\r\n", s);
    }

    [Fact]
    public void Srt_Writer_Normalises_Crlf_In_Cue_Text_To_Lf_Before_Output()
    {
        var cues = new[] { new SrtCue(1, TimeSpan.Zero, TimeSpan.FromSeconds(1), "A\r\nB\rC") };
        string s = SrtWriter.WriteString(cues);
        // Each logical line in cue text becomes its own CRLF-terminated line.
        Assert.Contains("A\r\nB\r\nC\r\n", s);
    }

    [Fact]
    public void Srt_FormatTimecode_Clamps_Negative_To_Zero()
    {
        Assert.Equal("00:00:00,000", SrtWriter.FormatTimecode(TimeSpan.FromSeconds(-5)));
    }

    [Theory]
    [InlineData(0, 0, 0, 0, "00:00:00,000")]
    [InlineData(0, 0, 0, 7, "00:00:00,007")]
    [InlineData(0, 0, 0, 999, "00:00:00,999")]
    [InlineData(1, 2, 3, 4, "01:02:03,004")]
    [InlineData(99, 59, 59, 999, "99:59:59,999")]
    public void Srt_FormatTimecode_Format_Is_Canonical(int hh, int mm, int ss, int ms, string expected)
    {
        var ts = new TimeSpan(0, hh, mm, ss, ms);
        Assert.Equal(expected, SrtWriter.FormatTimecode(ts));
    }

    [Theory]
    [InlineData("00:00:01,500", true, 1500)]
    [InlineData("00:00:01.500", true, 1500)]
    [InlineData("01:02:03,004", true, 3_723_004)]
    [InlineData("00:00:01",     true, 1000)]
    [InlineData("0:00:00",      false, 0)]
    [InlineData("bad",          false, 0)]
    [InlineData("00-00-00,000", false, 0)]
    [InlineData("00:00:xx,000", false, 0)]
    public void Srt_TryParseTimecode(string input, bool expectedOk, int expectedMs)
    {
        bool ok = SrtReader.TryParseTimecode(input.AsSpan(), out var value);
        Assert.Equal(expectedOk, ok);
        if (expectedOk) Assert.Equal(expectedMs, (int)value.TotalMilliseconds);
    }

    [Fact]
    public void Srt_File_RoundTrip()
    {
        var cues = new[]
        {
            new SrtCue(1, TimeSpan.FromMilliseconds(100), TimeSpan.FromSeconds(1), "Hi"),
        };
        var path = Path.Combine(Path.GetTempPath(), $"mediar-srt-{Guid.NewGuid():N}.srt");
        try
        {
            SrtWriter.WriteFile(path, cues);
            var parsed = SrtReader.ReadFile(path).ToArray();
            Assert.Single(parsed);
            Assert.Equal(cues[0].Text, parsed[0].Text);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Srt_WriteFile_Null_Cues_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-srt-{Guid.NewGuid():N}.srt");
        try
        {
            Assert.Throws<ArgumentNullException>(() => SrtWriter.WriteFile(path, null!));
        }
        finally { if (File.Exists(path)) File.Delete(path); }
    }

    [Fact]
    public void Srt_Stops_On_Malformed_Timecode_Line()
    {
        string content = "1\nnot-a-timecode\nbody\n\n2\n00:00:05,000 --> 00:00:06,000\nlater\n";
        var cues = SrtReader.ReadString(content).ToArray();
        // First cue: index 1, malformed timecode line aborts the iterator → 0 cues.
        Assert.Empty(cues);
    }

    // -------------------- WebVTT --------------------

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

    [Fact]
    public void WebVtt_Missing_Signature_Throws()
    {
        Assert.Throws<InvalidDataException>(() => WebVttReader.ReadString("not-webvtt\n").ToArray());
    }

    [Fact]
    public void WebVtt_Empty_String_Yields_No_Cues()
    {
        // Empty stream → no header line → graceful EOF (yield break, no throw).
        Assert.Empty(WebVttReader.ReadString(string.Empty).ToArray());
    }

    [Fact]
    public void WebVtt_Bom_Before_Signature_Is_Consumed()
    {
        string content = "\uFEFFWEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
    }

    [Fact]
    public void WebVtt_Note_Block_Is_Skipped()
    {
        string content = "WEBVTT\n\nNOTE this is a note\nstill in note\n\n00:00:01.000 --> 00:00:02.000\nHi\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal("Hi", cues[0].Text);
    }

    [Fact]
    public void WebVtt_Style_Block_Is_Skipped()
    {
        string content = "WEBVTT\n\nSTYLE\n::cue { color:red }\n\n00:00:01.000 --> 00:00:02.000\nA\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
    }

    [Fact]
    public void WebVtt_Region_Block_Is_Skipped()
    {
        string content = "WEBVTT\n\nREGION\nid:r1\n\n00:00:01.000 --> 00:00:02.000\nB\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
    }

    [Fact]
    public void WebVtt_Cue_Without_Identifier_Parses()
    {
        string content = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal(string.Empty, cues[0].Identifier);
    }

    [Fact]
    public void WebVtt_Cue_With_Identifier_Parses()
    {
        string content = "WEBVTT\n\nmycue\n00:00:01.000 --> 00:00:02.000\nHello\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal("mycue", cues[0].Identifier);
    }

    [Fact]
    public void WebVtt_Cue_With_Settings_Is_Parsed()
    {
        string content = "WEBVTT\n\n00:00:01.000 --> 00:00:02.000 align:center position:10%\nC\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Equal("align:center position:10%", cues[0].Settings);
    }

    [Fact]
    public void WebVtt_Mm_Ss_Mmm_Format_Is_Accepted()
    {
        // MM:SS.mmm (no hours)
        string content = "WEBVTT\n\n01:23.456 --> 02:34.567\nD\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal(new TimeSpan(0, 0, 1, 23, 456), cues[0].Start);
        Assert.Equal(new TimeSpan(0, 0, 2, 34, 567), cues[0].End);
    }

    [Fact]
    public void WebVtt_Comma_Separator_Is_Accepted()
    {
        string content = "WEBVTT\n\n00:00:01,250 --> 00:00:02,500\nE\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Equal(TimeSpan.FromMilliseconds(1250), cues[0].Start);
    }

    [Fact]
    public void WebVtt_Invalid_Timecode_Skips_Cue()
    {
        string content = "WEBVTT\n\nbadid\nbad-tc\n00:00:01.000 --> 00:00:02.000\nGood\n";
        var cues = WebVttReader.ReadString(content).ToArray();
        Assert.Single(cues);
        Assert.Equal("Good", cues[0].Text);
    }

    [Fact]
    public void WebVttReader_Read_Null_Reader_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => WebVttReader.Read(null!).ToArray());
    }

    [Fact]
    public void WebVttWriter_Write_Null_Writer_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => WebVttWriter.Write(null!, Array.Empty<WebVttCue>()));
    }

    [Fact]
    public void WebVttWriter_Write_Null_Cues_Throws()
    {
        using var sw = new StringWriter();
        Assert.Throws<ArgumentNullException>(() => WebVttWriter.Write(sw, null!));
    }

    [Fact]
    public void WebVtt_File_RoundTrip()
    {
        var cues = new[]
        {
            new WebVttCue("id1", TimeSpan.FromMilliseconds(100), TimeSpan.FromSeconds(1), string.Empty, "Hi"),
        };
        var path = Path.Combine(Path.GetTempPath(), $"mediar-vtt-{Guid.NewGuid():N}.vtt");
        try
        {
            WebVttWriter.WriteFile(path, cues);
            var parsed = WebVttReader.ReadFile(path).ToArray();
            Assert.Single(parsed);
            Assert.Equal("Hi", parsed[0].Text);
            Assert.Equal("id1", parsed[0].Identifier);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void WebVtt_WriteString_Empty_Cues_Yields_Header_Only()
    {
        string s = WebVttWriter.WriteString(Array.Empty<WebVttCue>());
        Assert.Equal("WEBVTT\n\n", s);
    }

    [Fact]
    public void WebVtt_FormatTimecode_Clamps_Negative_To_Zero()
    {
        Assert.Equal("00:00:00.000", WebVttWriter.FormatTimecode(TimeSpan.FromSeconds(-1)));
    }

    [Fact]
    public void WebVtt_FormatTimecode_Has_Dot_Separator()
    {
        var ts = new TimeSpan(0, 1, 2, 3, 4);
        Assert.Equal("01:02:03.004", WebVttWriter.FormatTimecode(ts));
    }

    [Theory]
    [InlineData("00:00:01.500", true, 1500)]
    [InlineData("01:02:03.004", true, 3_723_004)]
    [InlineData("01:23.456",    true, 83_456)]   // mm:ss.mmm
    [InlineData("badbadb",      false, 0)]
    [InlineData("foo:bar.baz",  false, 0)]
    public void WebVtt_TryParseTimecode(string input, bool expectedOk, int expectedMs)
    {
        bool ok = WebVttReader.TryParseTimecode(input.AsSpan(), out var value);
        Assert.Equal(expectedOk, ok);
        if (expectedOk) Assert.Equal(expectedMs, (int)value.TotalMilliseconds);
    }
}
