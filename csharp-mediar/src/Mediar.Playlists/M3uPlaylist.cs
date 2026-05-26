using System.Globalization;
using System.Text;

namespace Mediar.Playlists;

/// <summary>
/// Reader / writer for the M3U and M3U8 playlist formats. M3U files use the
/// system default ANSI encoding (we fall back to UTF-8) while M3U8 files are
/// always UTF-8 by definition. Extended M3U directives <c>#EXTM3U</c>,
/// <c>#EXTINF</c> and <c>#PLAYLIST</c> are recognised when present.
/// </summary>
public static class M3uPlaylist
{
    /// <summary>Read an .m3u / .m3u8 playlist from disk.</summary>
    public static Playlist ReadFile(string path)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        Encoding enc = path.EndsWith(".m3u8", StringComparison.OrdinalIgnoreCase)
            ? new UTF8Encoding(encoderShouldEmitUTF8Identifier: false)
            : Encoding.UTF8; // permissive default
        string text = File.ReadAllText(path, enc);
        return Read(text);
    }

    /// <summary>Parse an M3U/M3U8 playlist already loaded into a string.</summary>
    public static Playlist Read(string text)
    {
        ArgumentNullException.ThrowIfNull(text);
        var entries = new List<PlaylistEntry>();
        string? playlistTitle = null;
        string? pendingTitle = null;
        string? pendingArtist = null;
        TimeSpan? pendingDuration = null;

        foreach (var rawLine in text.Split('\n'))
        {
            string line = rawLine.TrimEnd('\r').Trim();
            if (line.Length == 0) continue;

            if (line.StartsWith('#'))
            {
                if (line.StartsWith("#EXTM3U", StringComparison.OrdinalIgnoreCase))
                {
                    continue;
                }
                if (line.StartsWith("#PLAYLIST:", StringComparison.OrdinalIgnoreCase))
                {
                    playlistTitle = line["#PLAYLIST:".Length..].Trim();
                }
                else if (line.StartsWith("#EXTINF:", StringComparison.OrdinalIgnoreCase))
                {
                    string body = line["#EXTINF:".Length..];
                    int comma = body.IndexOf(',');
                    string durPart = comma < 0 ? body : body[..comma];
                    string rest = comma < 0 ? "" : body[(comma + 1)..];

                    if (long.TryParse(durPart, NumberStyles.Integer, CultureInfo.InvariantCulture, out var secs))
                    {
                        pendingDuration = secs > 0 ? TimeSpan.FromSeconds(secs) : null;
                    }
                    int dash = rest.IndexOf(" - ", StringComparison.Ordinal);
                    if (dash > 0)
                    {
                        pendingArtist = rest[..dash].Trim();
                        pendingTitle = rest[(dash + 3)..].Trim();
                    }
                    else
                    {
                        pendingTitle = rest.Trim();
                    }
                }
                continue;
            }
            entries.Add(new PlaylistEntry(line, pendingTitle, pendingDuration, pendingArtist));
            pendingTitle = null;
            pendingArtist = null;
            pendingDuration = null;
        }
        return new Playlist { Title = playlistTitle, Entries = entries };
    }

    /// <summary>Write a playlist to disk. The file extension picks the encoding.</summary>
    public static void WriteFile(string path, Playlist playlist)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        ArgumentNullException.ThrowIfNull(playlist);
        Encoding enc = path.EndsWith(".m3u8", StringComparison.OrdinalIgnoreCase)
            ? new UTF8Encoding(encoderShouldEmitUTF8Identifier: false)
            : Encoding.UTF8;
        File.WriteAllText(path, Write(playlist), enc);
    }

    /// <summary>Serialise a playlist to extended-M3U text.</summary>
    public static string Write(Playlist playlist)
    {
        ArgumentNullException.ThrowIfNull(playlist);
        var sb = new StringBuilder();
        sb.AppendLine("#EXTM3U");
        if (!string.IsNullOrWhiteSpace(playlist.Title))
        {
            sb.Append("#PLAYLIST:").AppendLine(playlist.Title);
        }
        foreach (var e in playlist.Entries)
        {
            if (e.Duration.HasValue || !string.IsNullOrEmpty(e.Title) || !string.IsNullOrEmpty(e.Artist))
            {
                long secs = e.Duration.HasValue ? (long)Math.Round(e.Duration.Value.TotalSeconds) : -1;
                sb.Append("#EXTINF:").Append(secs.ToString(CultureInfo.InvariantCulture)).Append(',');
                if (!string.IsNullOrEmpty(e.Artist))
                {
                    sb.Append(e.Artist).Append(" - ");
                }
                sb.AppendLine(e.Title ?? "");
            }
            sb.AppendLine(e.Uri);
        }
        return sb.ToString();
    }
}
