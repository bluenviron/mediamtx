namespace Mediar.Subtitles.Srt;

/// <summary>
/// A parsed cue from a SubRip (<c>.srt</c>) document.
/// </summary>
public readonly record struct SrtCue(int Index, TimeSpan Start, TimeSpan End, string Text);
