namespace Mediar.Imaging.Jpeg;

/// <summary>
/// Chroma-subsampling pattern used by <see cref="JpegBaselineEncoder"/>.
/// Naming follows the conventional <c>J:a:b</c> notation (CCIR-601-style).
/// </summary>
public enum JpegSubsampling
{
    /// <summary>4:4:4 — no chroma subsampling. Highest fidelity, largest file.</summary>
    Yuv444 = 0,

    /// <summary>4:2:2 — chroma horizontally halved.</summary>
    Yuv422 = 1,

    /// <summary>4:2:0 — chroma halved on both axes. Web / camera default.</summary>
    Yuv420 = 2,
}

/// <summary>
/// Options for <see cref="JpegBaselineEncoder"/> and
/// <see cref="JpegWriter"/>. Inputs are validated and clamped to safe
/// ranges; defaults match libjpeg's quality-90 4:2:0 baseline.
/// </summary>
public sealed record JpegEncodeOptions
{
    /// <summary>Output quality, 1..100 (Annex K scaling).</summary>
    public int Quality { get; init; } = 90;

    /// <summary>Chroma-subsampling pattern.</summary>
    public JpegSubsampling Subsampling { get; init; } = JpegSubsampling.Yuv420;

    /// <summary>
    /// DRI restart interval in MCUs (0 = no restart markers).
    /// Useful for parallel decoders and bitstream resilience.
    /// </summary>
    public int RestartInterval { get; init; }

    /// <summary>
    /// When <c>true</c>, run a first pass over the image to gather symbol
    /// frequencies and emit custom DHT segments instead of the standard
    /// Annex K tables. Yields ~2-5 % smaller files at a modest CPU cost.
    /// </summary>
    public bool OptimisedHuffman { get; init; }

    /// <summary>
    /// Optional EXIF / TIFF tag map written into an APP1 segment via
    /// <see cref="JpegMetadataWriter"/>. <c>null</c> = no EXIF.
    /// </summary>
    public IReadOnlyDictionary<string, string>? Exif { get; init; }

    /// <summary>Optional ICC profile bytes written into one or more APP2 segments.</summary>
    public ReadOnlyMemory<byte> IccProfile { get; init; }

    /// <summary>Optional XMP packet written into an APP1 segment.</summary>
    public string? Xmp { get; init; }
}
