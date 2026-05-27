using System.Globalization;

namespace Mediar;

/// <summary>
/// Loudness-normalisation metadata extracted from container tags.
/// Aggregates ReplayGain 2.0 (REPLAYGAIN_* Vorbis-comment keys) and
/// the Opus R128 (RFC 7845 § 5.2) loudness fields into a single typed
/// record so callers do not have to parse the textual representations
/// themselves.
/// </summary>
/// <remarks>
/// Gains, ranges and reference loudness are expressed in decibels (dB).
/// Peaks are expressed in unitless linear sample-domain amplitude with
/// 1.0 corresponding to digital-full-scale (a peak of 1.5 is therefore
/// 50% above full-scale and would clip without attenuation).
/// </remarks>
public sealed record LoudnessInfo
{
    /// <summary>ReplayGain 2.0 track gain in dB.</summary>
    public double? TrackGainDb { get; init; }

    /// <summary>ReplayGain 2.0 album gain in dB.</summary>
    public double? AlbumGainDb { get; init; }

    /// <summary>ReplayGain 2.0 track peak as linear sample amplitude (1.0 = full scale).</summary>
    public double? TrackPeak { get; init; }

    /// <summary>ReplayGain 2.0 album peak as linear sample amplitude (1.0 = full scale).</summary>
    public double? AlbumPeak { get; init; }

    /// <summary>ReplayGain 2.0 track range (LRA) in dB.</summary>
    public double? TrackRangeDb { get; init; }

    /// <summary>ReplayGain 2.0 album range (LRA) in dB.</summary>
    public double? AlbumRangeDb { get; init; }

    /// <summary>ReplayGain 2.0 reference loudness in dB (typically 89 dB SPL).</summary>
    public double? ReferenceLoudnessDb { get; init; }

    /// <summary>
    /// Opus R128 track gain in dB. Decoded from the Q7.8 fixed-point
    /// integer per RFC 7845 § 5.2 (the raw value divided by 256.0).
    /// </summary>
    public double? R128TrackGainDb { get; init; }

    /// <summary>
    /// Opus R128 album gain in dB. Decoded from the Q7.8 fixed-point
    /// integer per RFC 7845 § 5.2 (the raw value divided by 256.0).
    /// </summary>
    public double? R128AlbumGainDb { get; init; }

    /// <summary>True when no loudness fields are populated.</summary>
    public bool IsEmpty =>
        TrackGainDb is null && AlbumGainDb is null &&
        TrackPeak is null && AlbumPeak is null &&
        TrackRangeDb is null && AlbumRangeDb is null &&
        ReferenceLoudnessDb is null &&
        R128TrackGainDb is null && R128AlbumGainDb is null;

    /// <summary>
    /// Parse a ReplayGain dB value of the form <c>"-7.89 dB"</c> or
    /// <c>"+0.00 dB"</c> (the <c>dB</c> suffix and leading sign are
    /// optional). Returns <see langword="false"/> when the value is
    /// malformed or out of the valid <c>[-60, +60]</c> range.
    /// </summary>
    public static bool TryParseReplayGainDb(string? value, out double db)
    {
        db = 0;
        if (string.IsNullOrWhiteSpace(value)) return false;
        ReadOnlySpan<char> s = value.AsSpan().Trim();
        if (s.EndsWith("dB", StringComparison.OrdinalIgnoreCase))
            s = s[..^2].TrimEnd();
        else if (s.EndsWith("DB", StringComparison.Ordinal))
            s = s[..^2].TrimEnd();
        if (!double.TryParse(s, NumberStyles.Float, CultureInfo.InvariantCulture, out var parsed))
            return false;
        if (double.IsNaN(parsed) || double.IsInfinity(parsed)) return false;
        if (parsed is < -60 or > 60) return false;
        db = parsed;
        return true;
    }

    /// <summary>
    /// Parse a ReplayGain peak amplitude (unitless linear, e.g.
    /// <c>"0.987654"</c>). Returns <see langword="false"/> when the
    /// value is malformed or non-positive. Peaks above 1.0 are
    /// preserved (the source is louder than digital-full-scale and
    /// would clip without attenuation).
    /// </summary>
    public static bool TryParseReplayGainPeak(string? value, out double peak)
    {
        peak = 0;
        if (string.IsNullOrWhiteSpace(value)) return false;
        if (!double.TryParse(value.Trim(), NumberStyles.Float, CultureInfo.InvariantCulture, out var parsed))
            return false;
        if (double.IsNaN(parsed) || double.IsInfinity(parsed)) return false;
        if (parsed < 0 || parsed > 10) return false;
        peak = parsed;
        return true;
    }

    /// <summary>
    /// Parse an Opus R128 Q7.8 fixed-point gain (e.g. <c>"-2304"</c>
    /// representing −9.0 dB). Per RFC 7845 § 5.2 the on-tape value is
    /// a signed 16-bit integer in units of 1/256 dB. Returns
    /// <see langword="false"/> when malformed or out of the valid
    /// <c>[-32768, 32767]</c> range.
    /// </summary>
    public static bool TryParseR128Q78(string? value, out double db)
    {
        db = 0;
        if (string.IsNullOrWhiteSpace(value)) return false;
        if (!int.TryParse(value.Trim(), NumberStyles.Integer, CultureInfo.InvariantCulture, out var raw))
            return false;
        if (raw is < -32768 or > 32767) return false;
        db = raw / 256.0;
        return true;
    }
}
