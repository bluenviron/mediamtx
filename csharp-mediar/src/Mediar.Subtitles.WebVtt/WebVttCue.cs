namespace Mediar.Subtitles.WebVtt;

/// <summary>A parsed cue from a WebVTT document.</summary>
/// <param name="Identifier">Optional cue identifier (line preceding the timecode); empty if absent.</param>
/// <param name="Start">Cue start time.</param>
/// <param name="End">Cue end time.</param>
/// <param name="Settings">Trailing cue settings string (e.g. <c>align:start position:10%</c>); empty if absent.</param>
/// <param name="Text">Cue payload (may contain raw WebVTT inline tags).</param>
public readonly record struct WebVttCue(string Identifier, TimeSpan Start, TimeSpan End, string Settings, string Text);
