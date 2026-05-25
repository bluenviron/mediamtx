namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Materialized record for one sample in a track.
/// 32 bytes; keeps the per-track table cache-friendly.
/// </summary>
internal struct SampleRecord
{
    /// <summary>Absolute byte offset of the sample in the source.</summary>
    public long Offset;
    /// <summary>Decode timestamp in the track's time-base.</summary>
    public long Dts;
    /// <summary>Sample size in bytes.</summary>
    public int Size;
    /// <summary>Duration in the track's time-base.</summary>
    public int Duration;
    /// <summary>Composition time offset (PTS = DTS + CtsOffset).</summary>
    public int CtsOffset;
    /// <summary>True if this is a sync (key) sample.</summary>
    public bool IsKey;
}
