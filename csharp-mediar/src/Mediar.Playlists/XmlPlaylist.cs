using System.Globalization;
using System.Text;
using System.Xml;

namespace Mediar.Playlists;

/// <summary>
/// Reader / writer for the XML Shareable Playlist Format
/// (<see href="https://xspf.org/">xspf.org</see>) and Microsoft's WPL
/// (Windows Media Player) playlist, which is a SMIL XML dialect with very
/// similar semantics.
/// </summary>
public static class XmlPlaylist
{
    /// <summary>Read an .xspf playlist from disk.</summary>
    public static Playlist ReadXspfFile(string path)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        return ReadXspf(File.ReadAllText(path, Encoding.UTF8));
    }

    /// <summary>Parse an .xspf playlist already loaded into memory.</summary>
    public static Playlist ReadXspf(string xml)
    {
        ArgumentNullException.ThrowIfNull(xml);
        var doc = new XmlDocument();
        doc.LoadXml(xml);
        string? title = SelectInnerText(doc.DocumentElement, "title");
        var entries = new List<PlaylistEntry>();
        var trackNodes = doc.GetElementsByTagName("track");
        foreach (XmlNode trackNode in trackNodes)
        {
            string? loc = SelectInnerText(trackNode, "location");
            if (string.IsNullOrEmpty(loc)) continue;
            string? trkTitle = SelectInnerText(trackNode, "title");
            string? artist = SelectInnerText(trackNode, "creator");
            TimeSpan? duration = null;
            string? dur = SelectInnerText(trackNode, "duration");
            if (dur is not null && long.TryParse(dur, NumberStyles.Integer, CultureInfo.InvariantCulture, out var ms))
            {
                duration = TimeSpan.FromMilliseconds(ms);
            }
            entries.Add(new PlaylistEntry(loc, trkTitle, duration, artist));
        }
        return new Playlist { Title = title, Entries = entries };
    }

    /// <summary>Write an .xspf playlist to disk.</summary>
    public static void WriteXspfFile(string path, Playlist playlist)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        File.WriteAllText(path, WriteXspf(playlist), new UTF8Encoding(false));
    }

    /// <summary>Serialise to XSPF.</summary>
    public static string WriteXspf(Playlist playlist)
    {
        ArgumentNullException.ThrowIfNull(playlist);
        var sb = new StringBuilder();
        var settings = new XmlWriterSettings
        {
            Indent = true,
            Encoding = new UTF8Encoding(false),
            OmitXmlDeclaration = false,
        };
        using var writer = XmlWriter.Create(sb, settings);
        writer.WriteStartDocument();
        writer.WriteStartElement("playlist", "http://xspf.org/ns/0/");
        writer.WriteAttributeString("version", "1");
        if (!string.IsNullOrEmpty(playlist.Title))
            writer.WriteElementString("title", playlist.Title);
        writer.WriteStartElement("trackList");
        foreach (var e in playlist.Entries)
        {
            writer.WriteStartElement("track");
            writer.WriteElementString("location", e.Uri);
            if (!string.IsNullOrEmpty(e.Title)) writer.WriteElementString("title", e.Title);
            if (!string.IsNullOrEmpty(e.Artist)) writer.WriteElementString("creator", e.Artist);
            if (e.Duration.HasValue)
                writer.WriteElementString("duration",
                    ((long)e.Duration.Value.TotalMilliseconds).ToString(CultureInfo.InvariantCulture));
            writer.WriteEndElement();
        }
        writer.WriteEndElement();
        writer.WriteEndElement();
        writer.WriteEndDocument();
        writer.Flush();
        return sb.ToString();
    }

    /// <summary>Read a Windows Media Player .wpl playlist from disk.</summary>
    public static Playlist ReadWplFile(string path)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        return ReadWpl(File.ReadAllText(path, Encoding.UTF8));
    }

    /// <summary>Parse a .wpl playlist already in memory.</summary>
    public static Playlist ReadWpl(string xml)
    {
        ArgumentNullException.ThrowIfNull(xml);
        var doc = new XmlDocument();
        doc.LoadXml(xml);
        string? title = SelectInnerText(doc.DocumentElement, "title");
        var entries = new List<PlaylistEntry>();
        var nodes = doc.GetElementsByTagName("media");
        foreach (XmlNode node in nodes)
        {
            string? src = node.Attributes?["src"]?.Value;
            if (string.IsNullOrEmpty(src)) continue;
            string? trkTitle = node.Attributes?["trackTitle"]?.Value;
            entries.Add(new PlaylistEntry(src, trkTitle));
        }
        return new Playlist { Title = title, Entries = entries };
    }

    /// <summary>Write a Windows Media Player .wpl playlist to disk.</summary>
    public static void WriteWplFile(string path, Playlist playlist)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        File.WriteAllText(path, WriteWpl(playlist), new UTF8Encoding(false));
    }

    /// <summary>Serialise to WPL (SMIL).</summary>
    public static string WriteWpl(Playlist playlist)
    {
        ArgumentNullException.ThrowIfNull(playlist);
        var sb = new StringBuilder();
        var settings = new XmlWriterSettings
        {
            Indent = true,
            Encoding = new UTF8Encoding(false),
            OmitXmlDeclaration = false,
        };
        using var writer = XmlWriter.Create(sb, settings);
        writer.WriteStartDocument();
        writer.WriteStartElement("smil");
        writer.WriteStartElement("head");
        writer.WriteElementString("title", playlist.Title ?? "");
        writer.WriteEndElement();
        writer.WriteStartElement("body");
        writer.WriteStartElement("seq");
        foreach (var e in playlist.Entries)
        {
            writer.WriteStartElement("media");
            writer.WriteAttributeString("src", e.Uri);
            if (!string.IsNullOrEmpty(e.Title))
                writer.WriteAttributeString("trackTitle", e.Title);
            writer.WriteEndElement();
        }
        writer.WriteEndElement();
        writer.WriteEndElement();
        writer.WriteEndElement();
        writer.WriteEndDocument();
        writer.Flush();
        return sb.ToString();
    }

    private static string? SelectInnerText(XmlNode? parent, string name)
    {
        if (parent is null) return null;
        foreach (XmlNode child in parent.ChildNodes)
        {
            if (child.NodeType == XmlNodeType.Element && child.LocalName.Equals(name, StringComparison.OrdinalIgnoreCase))
                return child.InnerText;
        }
        return null;
    }
}
