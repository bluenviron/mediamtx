using System.Globalization;
using System.Text;

namespace Mediar.Playlists;

/// <summary>
/// Reader / writer for the SHOUTcast PLS (INI-style) playlist format.
/// </summary>
/// <remarks>
/// PLS playlists are well-formed INI files with a single <c>[playlist]</c>
/// section. Each entry is described by three keys numbered from 1:
/// <c>FileN=…</c>, <c>TitleN=…</c> and <c>LengthN=…</c>. Plus the
/// <c>NumberOfEntries</c> and <c>Version</c> top-level keys.
/// </remarks>
public static class PlsPlaylist
{
    /// <summary>Read a .pls playlist from disk.</summary>
    public static Playlist ReadFile(string path)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        return Read(File.ReadAllText(path, Encoding.UTF8));
    }

    /// <summary>Parse a .pls playlist already loaded into memory.</summary>
    public static Playlist Read(string text)
    {
        ArgumentNullException.ThrowIfNull(text);
        var files = new Dictionary<int, string>();
        var titles = new Dictionary<int, string>();
        var lengths = new Dictionary<int, int>();
        foreach (var rawLine in text.Split('\n'))
        {
            string line = rawLine.Trim('\r', ' ', '\t');
            if (line.Length == 0 || line[0] == ';' || line[0] == '[') continue;
            int eq = line.IndexOf('=');
            if (eq < 0) continue;
            string key = line[..eq].Trim();
            string value = line[(eq + 1)..].Trim();
            if (TryParsePrefixed(key, "File", out var ix)) files[ix] = value;
            else if (TryParsePrefixed(key, "Title", out var ix2)) titles[ix2] = value;
            else if (TryParsePrefixed(key, "Length", out var ix3)
                  && int.TryParse(value, NumberStyles.Integer, CultureInfo.InvariantCulture, out var secs))
            {
                lengths[ix3] = secs;
            }
        }

        var entries = new List<PlaylistEntry>(files.Count);
        foreach (int ix in files.Keys.OrderBy(static k => k))
        {
            TimeSpan? dur = lengths.TryGetValue(ix, out var s) && s > 0
                ? TimeSpan.FromSeconds(s) : null;
            entries.Add(new PlaylistEntry(
                files[ix],
                titles.TryGetValue(ix, out var t) ? t : null,
                dur));
        }
        return new Playlist { Entries = entries };
    }

    /// <summary>Write a playlist to disk in PLS format.</summary>
    public static void WriteFile(string path, Playlist playlist)
    {
        ArgumentException.ThrowIfNullOrEmpty(path);
        ArgumentNullException.ThrowIfNull(playlist);
        File.WriteAllText(path, Write(playlist), new UTF8Encoding(false));
    }

    /// <summary>Serialise a playlist to PLS text.</summary>
    public static string Write(Playlist playlist)
    {
        ArgumentNullException.ThrowIfNull(playlist);
        var sb = new StringBuilder();
        sb.AppendLine("[playlist]");
        for (int i = 0; i < playlist.Entries.Count; i++)
        {
            int n = i + 1;
            var e = playlist.Entries[i];
            sb.Append("File").Append(n.ToString(CultureInfo.InvariantCulture)).Append('=').AppendLine(e.Uri);
            if (!string.IsNullOrEmpty(e.Title))
                sb.Append("Title").Append(n.ToString(CultureInfo.InvariantCulture)).Append('=').AppendLine(e.Title);
            long secs = e.Duration.HasValue ? (long)Math.Round(e.Duration.Value.TotalSeconds) : -1;
            sb.Append("Length").Append(n.ToString(CultureInfo.InvariantCulture)).Append('=')
              .AppendLine(secs.ToString(CultureInfo.InvariantCulture));
        }
        sb.Append("NumberOfEntries=").AppendLine(playlist.Entries.Count.ToString(CultureInfo.InvariantCulture));
        sb.AppendLine("Version=2");
        return sb.ToString();
    }

    private static bool TryParsePrefixed(string key, string prefix, out int index)
    {
        index = 0;
        if (key.Length <= prefix.Length || !key.StartsWith(prefix, StringComparison.OrdinalIgnoreCase))
            return false;
        return int.TryParse(key.AsSpan(prefix.Length), NumberStyles.Integer, CultureInfo.InvariantCulture, out index);
    }
}
