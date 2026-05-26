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
}
