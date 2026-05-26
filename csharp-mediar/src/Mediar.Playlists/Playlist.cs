namespace Mediar.Playlists;

/// <summary>
/// A single entry in a media playlist.
/// </summary>
/// <param name="Uri">Resource location (file path or URL).</param>
/// <param name="Title">Optional display title.</param>
/// <param name="Duration">
/// Optional duration. <c>null</c> means the playlist did not declare a duration.
/// </param>
/// <param name="Artist">Optional artist hint (some formats expose this separately).</param>
public readonly record struct PlaylistEntry(
    string Uri,
    string? Title = null,
    TimeSpan? Duration = null,
    string? Artist = null);

/// <summary>
/// Strongly typed playlist document. Maintained as an immutable record so it
/// can be shared across threads safely.
/// </summary>
public sealed record Playlist
{
    /// <summary>Optional human-readable playlist title.</summary>
    public string? Title { get; init; }
    /// <summary>Entries in playback order.</summary>
    public required IReadOnlyList<PlaylistEntry> Entries { get; init; }
}
