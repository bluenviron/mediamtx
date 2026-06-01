using Mediar.Playlists;
using Xunit;

namespace Mediar.Tests;

public sealed class PlaylistTests
{
    private static Playlist Sample() => new()
    {
        Title = "My Mix",
        Entries =
        [
            new("track1.mp3", Title: "Hello", Duration: TimeSpan.FromSeconds(180), Artist: "Artist A"),
            new("https://stream.example/live.ogg", Title: "Stream"),
            new("track3.flac", Title: "Goodbye", Duration: TimeSpan.FromSeconds(240)),
        ],
    };

    [Fact]
    public void M3u_RoundTrips_Title_Duration_Artist()
    {
        var original = Sample();
        string text = M3uPlaylist.Write(original);
        Assert.Contains("#EXTM3U", text);
        Assert.Contains("#PLAYLIST:My Mix", text);
        Assert.Contains("#EXTINF:180,Artist A - Hello", text);

        Playlist rt = M3uPlaylist.Read(text);
        Assert.Equal("My Mix", rt.Title);
        Assert.Equal(3, rt.Entries.Count);

        Assert.Equal("track1.mp3", rt.Entries[0].Uri);
        Assert.Equal("Hello", rt.Entries[0].Title);
        Assert.Equal("Artist A", rt.Entries[0].Artist);
        Assert.Equal(TimeSpan.FromSeconds(180), rt.Entries[0].Duration);

        Assert.Equal("https://stream.example/live.ogg", rt.Entries[1].Uri);
        Assert.Equal("Stream", rt.Entries[1].Title);

        Assert.Equal("track3.flac", rt.Entries[2].Uri);
        Assert.Equal(TimeSpan.FromSeconds(240), rt.Entries[2].Duration);
    }

    [Fact]
    public void M3u_Handles_Comments_And_Empty_Lines()
    {
        const string text =
            """
            #EXTM3U

            # this is a comment
            #EXTINF:120,Artist - Title

            song.mp3
            """;
        Playlist p = M3uPlaylist.Read(text);
        var e = Assert.Single(p.Entries);
        Assert.Equal("song.mp3", e.Uri);
        Assert.Equal("Title", e.Title);
        Assert.Equal("Artist", e.Artist);
    }

    [Fact]
    public void Pls_RoundTrips_With_Numbered_Entries()
    {
        var original = Sample();
        string text = PlsPlaylist.Write(original);
        Assert.Contains("[playlist]", text);
        Assert.Contains("File1=track1.mp3", text);
        Assert.Contains("Title1=Hello", text);
        Assert.Contains("Length1=180", text);
        Assert.Contains("NumberOfEntries=3", text);
        Assert.Contains("Version=2", text);

        Playlist rt = PlsPlaylist.Read(text);
        Assert.Equal(3, rt.Entries.Count);
        Assert.Equal("track1.mp3", rt.Entries[0].Uri);
        Assert.Equal("Hello", rt.Entries[0].Title);
        Assert.Equal(TimeSpan.FromSeconds(180), rt.Entries[0].Duration);
    }

    [Fact]
    public void Xspf_RoundTrips()
    {
        var original = Sample();
        string xml = XmlPlaylist.WriteXspf(original);
        Assert.Contains("xspf.org", xml);
        Assert.Contains("<title>My Mix</title>", xml);
        Assert.Contains("<trackList>", xml);

        Playlist rt = XmlPlaylist.ReadXspf(xml);
        Assert.Equal("My Mix", rt.Title);
        Assert.Equal(3, rt.Entries.Count);
        Assert.Equal("track1.mp3", rt.Entries[0].Uri);
        Assert.Equal("Hello", rt.Entries[0].Title);
        Assert.Equal("Artist A", rt.Entries[0].Artist);
        Assert.Equal(TimeSpan.FromMilliseconds(180000), rt.Entries[0].Duration);
    }

    [Fact]
    public void Wpl_RoundTrips()
    {
        var original = Sample();
        string xml = XmlPlaylist.WriteWpl(original);
        Assert.Contains("<smil>", xml);
        Assert.Contains("<seq>", xml);
        Assert.Contains("src=\"track1.mp3\"", xml);

        Playlist rt = XmlPlaylist.ReadWpl(xml);
        Assert.Equal(3, rt.Entries.Count);
        Assert.Equal("track1.mp3", rt.Entries[0].Uri);
        Assert.Equal("Hello", rt.Entries[0].Title);
    }

    [Fact]
    public void M3u_Write_Without_Title_Skips_Playlist_Header()
    {
        var p = new Playlist { Entries = [new("a.mp3")] };
        string text = M3uPlaylist.Write(p);
        Assert.Contains("#EXTM3U", text);
        Assert.DoesNotContain("#PLAYLIST", text);
        // No EXTINF when entry has no title/duration/artist
        Assert.DoesNotContain("#EXTINF", text);
        Assert.Contains("a.mp3", text);
    }

    [Fact]
    public void M3u_Read_Negative_Duration_Is_Null()
    {
        const string text = """
            #EXTM3U
            #EXTINF:-1,Streaming Track
            stream.mp3
            """;
        Playlist p = M3uPlaylist.Read(text);
        var e = Assert.Single(p.Entries);
        Assert.Null(e.Duration);
        Assert.Equal("Streaming Track", e.Title);
    }

    [Fact]
    public void M3u_Read_Extinf_Without_Comma()
    {
        const string text = """
            #EXTM3U
            #EXTINF:60
            song.mp3
            """;
        Playlist p = M3uPlaylist.Read(text);
        var e = Assert.Single(p.Entries);
        Assert.Equal(TimeSpan.FromSeconds(60), e.Duration);
        Assert.Equal("", e.Title);
    }

    [Fact]
    public void M3u_Read_Without_Extm3u_Marker_Still_Reads_Entries()
    {
        const string text = "song.mp3\nanother.mp3\n";
        Playlist p = M3uPlaylist.Read(text);
        Assert.Equal(2, p.Entries.Count);
    }

    [Fact]
    public void M3u_Read_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => M3uPlaylist.Read(null!));
    }

    [Fact]
    public void M3u_Write_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => M3uPlaylist.Write(null!));
    }

    [Fact]
    public void M3u_ReadFile_NullOrEmpty_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => M3uPlaylist.ReadFile(null!));
        Assert.Throws<ArgumentException>(() => M3uPlaylist.ReadFile(""));
    }

    [Fact]
    public void M3u_WriteFile_RoundTrips_With_M3u8_Encoding()
    {
        string dir = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName());
        Directory.CreateDirectory(dir);
        try
        {
            string path = Path.Combine(dir, "list.m3u8");
            var original = Sample();
            M3uPlaylist.WriteFile(path, original);
            Assert.True(File.Exists(path));
            Playlist rt = M3uPlaylist.ReadFile(path);
            Assert.Equal(original.Title, rt.Title);
            Assert.Equal(3, rt.Entries.Count);
        }
        finally { Directory.Delete(dir, recursive: true); }
    }

    [Fact]
    public void M3u_WriteFile_Default_Extension_Uses_Utf8()
    {
        string dir = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName());
        Directory.CreateDirectory(dir);
        try
        {
            string path = Path.Combine(dir, "list.m3u");
            M3uPlaylist.WriteFile(path, Sample());
            Playlist rt = M3uPlaylist.ReadFile(path);
            Assert.Equal(3, rt.Entries.Count);
        }
        finally { Directory.Delete(dir, recursive: true); }
    }

    [Fact]
    public void Pls_Read_Ignores_Comments_And_Section_Lines()
    {
        const string text = """
            ; this is a comment
            [playlist]
            File1=a.mp3
            Title1=Alpha
            Length1=60
            NumberOfEntries=1
            Version=2
            """;
        Playlist p = PlsPlaylist.Read(text);
        var e = Assert.Single(p.Entries);
        Assert.Equal("a.mp3", e.Uri);
        Assert.Equal(TimeSpan.FromSeconds(60), e.Duration);
    }

    [Fact]
    public void Pls_Read_Non_Sequential_Indices_Sorted()
    {
        const string text = """
            [playlist]
            File3=c.mp3
            File1=a.mp3
            File2=b.mp3
            """;
        Playlist p = PlsPlaylist.Read(text);
        Assert.Equal(3, p.Entries.Count);
        Assert.Equal("a.mp3", p.Entries[0].Uri);
        Assert.Equal("b.mp3", p.Entries[1].Uri);
        Assert.Equal("c.mp3", p.Entries[2].Uri);
    }

    [Fact]
    public void Pls_Negative_Length_Becomes_Null_Duration()
    {
        const string text = """
            [playlist]
            File1=a.mp3
            Length1=-1
            """;
        Playlist p = PlsPlaylist.Read(text);
        Assert.Null(Assert.Single(p.Entries).Duration);
    }

    [Fact]
    public void Pls_Read_Skips_Lines_Without_Equals()
    {
        const string text = """
            [playlist]
            this-line-has-no-equals
            File1=a.mp3
            """;
        Playlist p = PlsPlaylist.Read(text);
        var e = Assert.Single(p.Entries);
        Assert.Equal("a.mp3", e.Uri);
    }

    [Fact]
    public void Pls_Write_Without_Title_Skips_Title_Key()
    {
        var p = new Playlist { Entries = [new("only.mp3")] };
        string text = PlsPlaylist.Write(p);
        Assert.Contains("File1=only.mp3", text);
        Assert.DoesNotContain("Title1=", text);
        Assert.Contains("NumberOfEntries=1", text);
    }

    [Fact]
    public void Pls_Read_And_Write_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => PlsPlaylist.Read(null!));
        Assert.Throws<ArgumentNullException>(() => PlsPlaylist.Write(null!));
    }

    [Fact]
    public void Pls_ReadFile_NullOrEmpty_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => PlsPlaylist.ReadFile(null!));
        Assert.Throws<ArgumentException>(() => PlsPlaylist.ReadFile(""));
    }

    [Fact]
    public void Pls_WriteFile_RoundTrips()
    {
        string dir = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName());
        Directory.CreateDirectory(dir);
        try
        {
            string path = Path.Combine(dir, "list.pls");
            PlsPlaylist.WriteFile(path, Sample());
            Playlist rt = PlsPlaylist.ReadFile(path);
            Assert.Equal(3, rt.Entries.Count);
        }
        finally { Directory.Delete(dir, recursive: true); }
    }

    [Fact]
    public void Xspf_Without_Title_Or_Duration()
    {
        var p = new Playlist { Entries = [new("u.mp3", Title: "T")] };
        string xml = XmlPlaylist.WriteXspf(p);
        Assert.DoesNotContain("<duration>", xml);
        Playlist rt = XmlPlaylist.ReadXspf(xml);
        var e = Assert.Single(rt.Entries);
        Assert.Equal("u.mp3", e.Uri);
        Assert.Equal("T", e.Title);
        Assert.Null(e.Duration);
        Assert.Null(rt.Title);
    }

    [Fact]
    public void Xspf_Read_Skips_Track_Without_Location()
    {
        const string xml = """
            <?xml version="1.0" encoding="utf-8"?>
            <playlist xmlns="http://xspf.org/ns/0/" version="1">
              <trackList>
                <track><title>Orphan</title></track>
                <track><location>good.mp3</location></track>
              </trackList>
            </playlist>
            """;
        Playlist rt = XmlPlaylist.ReadXspf(xml);
        var e = Assert.Single(rt.Entries);
        Assert.Equal("good.mp3", e.Uri);
    }

    [Fact]
    public void Xspf_Read_And_Write_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => XmlPlaylist.ReadXspf(null!));
        Assert.Throws<ArgumentNullException>(() => XmlPlaylist.WriteXspf(null!));
    }

    [Fact]
    public void Xspf_ReadFile_NullOrEmpty_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => XmlPlaylist.ReadXspfFile(null!));
        Assert.Throws<ArgumentException>(() => XmlPlaylist.ReadXspfFile(""));
    }

    [Fact]
    public void Xspf_WriteFile_RoundTrips()
    {
        string dir = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName());
        Directory.CreateDirectory(dir);
        try
        {
            string path = Path.Combine(dir, "list.xspf");
            XmlPlaylist.WriteXspfFile(path, Sample());
            Playlist rt = XmlPlaylist.ReadXspfFile(path);
            Assert.Equal(3, rt.Entries.Count);
        }
        finally { Directory.Delete(dir, recursive: true); }
    }

    [Fact]
    public void Wpl_Read_Skips_Media_Without_Src()
    {
        const string xml = """
            <smil><head><title>X</title></head><body><seq>
              <media trackTitle="No source" />
              <media src="ok.mp3" trackTitle="OK" />
            </seq></body></smil>
            """;
        Playlist rt = XmlPlaylist.ReadWpl(xml);
        var e = Assert.Single(rt.Entries);
        Assert.Equal("ok.mp3", e.Uri);
    }

    [Fact]
    public void Wpl_Read_And_Write_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => XmlPlaylist.ReadWpl(null!));
        Assert.Throws<ArgumentNullException>(() => XmlPlaylist.WriteWpl(null!));
    }

    [Fact]
    public void Wpl_ReadFile_NullOrEmpty_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => XmlPlaylist.ReadWplFile(null!));
        Assert.Throws<ArgumentException>(() => XmlPlaylist.ReadWplFile(""));
    }

    [Fact]
    public void Wpl_WriteFile_RoundTrips()
    {
        string dir = Path.Combine(Path.GetTempPath(), Path.GetRandomFileName());
        Directory.CreateDirectory(dir);
        try
        {
            string path = Path.Combine(dir, "list.wpl");
            XmlPlaylist.WriteWplFile(path, Sample());
            Playlist rt = XmlPlaylist.ReadWplFile(path);
            Assert.Equal(3, rt.Entries.Count);
        }
        finally { Directory.Delete(dir, recursive: true); }
    }

    [Fact]
    public void Empty_Playlist_RoundTrips_All_Formats()
    {
        var empty = new Playlist { Entries = [] };
        Assert.Empty(M3uPlaylist.Read(M3uPlaylist.Write(empty)).Entries);
        Assert.Empty(PlsPlaylist.Read(PlsPlaylist.Write(empty)).Entries);
        Assert.Empty(XmlPlaylist.ReadXspf(XmlPlaylist.WriteXspf(empty)).Entries);
        Assert.Empty(XmlPlaylist.ReadWpl(XmlPlaylist.WriteWpl(empty)).Entries);
    }
}
