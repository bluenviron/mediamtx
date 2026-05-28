using System.Collections.Frozen;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Codec hint inferred from a HEIF brand 4-CC. The HEIF spec attaches
/// the codec to the brand only indirectly via the image / image-sequence
/// profile brands, so this represents the most-likely tile-payload codec
/// rather than a hard guarantee.
/// </summary>
public enum HeifCodec
{
    /// <summary>The brand does not identify a codec.</summary>
    Unknown,
    /// <summary>HEVC (ISO/IEC 23008-2) per ISO/IEC 23008-12 §10.3-10.4.</summary>
    Hevc,
    /// <summary>AV1 (AOMedia) per the AVIF specification.</summary>
    Av1,
    /// <summary>VVC (ISO/IEC 23090-3) per ISO/IEC 23008-12 amendment.</summary>
    Vvc,
    /// <summary>Uncompressed image (<c>unif</c>).</summary>
    Uncompressed,
    /// <summary>Canon Raw v3 (<c>crx </c>) - tile payload is RAW sensor data.</summary>
    CanonRaw,
    /// <summary>Tone-mapped (gain map) image (<c>tmap</c>).</summary>
    ToneMapped,
}

/// <summary>
/// High-level structure of the HEIF container, derived from the brand
/// list. An image-sequence brand on its own does not preclude a still
/// image being present; both kinds can coexist.
/// </summary>
public enum HeifContainerKind
{
    /// <summary>No structural classification could be derived.</summary>
    Unknown,
    /// <summary>Single still image (<c>mif1</c>, <c>mif2</c>, <c>mif3</c>, <c>heic</c>, <c>heix</c>, <c>avif</c>, <c>vvic</c>, ...).</summary>
    SingleImage,
    /// <summary>Image sequence / burst (<c>msf1</c>, <c>hevc</c>, <c>hevx</c>, <c>heis</c>, <c>avis</c>, <c>vvis</c>, ...).</summary>
    ImageSequence,
}

/// <summary>
/// Typed view over the <c>ftyp</c> box brands of an ISO-BMFF image
/// container (HEIF / HEIC / AVIF / CR3 / VVC images). Encapsulates the
/// per-brand profile + codec lookup rules from ISO/IEC 23008-12 §10
/// and AV1 / AVIF + ISO/IEC 23090-3 amendments so callers no longer
/// need to maintain their own 4-CC tables.
/// </summary>
public sealed record HeifBrandInfo
{
    /// <summary>Major brand from the <c>ftyp</c> box (e.g. <c>"heic"</c>).</summary>
    public required string MajorBrand { get; init; }

    /// <summary>Compatible brands declared by <c>ftyp</c> (excludes the major brand).</summary>
    public required ImmutableArray<string> CompatibleBrands { get; init; }

    /// <summary>Most-specific codec the brand list identifies. <see cref="HeifCodec.Unknown"/> when no codec-bearing brand is present.</summary>
    public required HeifCodec PrimaryCodec { get; init; }

    /// <summary>High-level container structure derived from the brand list.</summary>
    public required HeifContainerKind ContainerKind { get; init; }

    /// <summary>True iff the brand list contains any image-sequence brand (<c>msf1</c>, <c>hevc</c>, <c>hevx</c>, <c>heis</c>, <c>avis</c>, <c>vvis</c>).</summary>
    public required bool IsImageSequence { get; init; }

    /// <summary>True iff the brand list contains a multilayer HEVC profile (<c>heim</c>, <c>heis</c>).</summary>
    public required bool IsMultilayer { get; init; }

    /// <summary>True iff the brand list contains a range-extended HEVC profile (<c>heix</c>, <c>hevx</c>).</summary>
    public required bool IsRangeExtended { get; init; }

    /// <summary>True iff the brand list contains the tone-map image brand (<c>tmap</c>).</summary>
    public required bool IsToneMapped { get; init; }

    /// <summary>True iff the brand list contains an Apple multi-image variant (<c>MA1A</c>, <c>MA1B</c>).</summary>
    public required bool IsAppleMultiImage { get; init; }

    /// <summary>True iff the brand list contains <paramref name="fourCC"/> as either the major brand or one of the compatible brands.</summary>
    public bool HasBrand(string fourCC)
    {
        if (MajorBrand == fourCC) return true;
        foreach (var b in CompatibleBrands)
        {
            if (b == fourCC) return true;
        }
        return false;
    }

    /// <summary>
    /// Decode the brand list of an ISO-BMFF image container into a
    /// typed <see cref="HeifBrandInfo"/>. Brands are normalised
    /// lexically only - the caller is responsible for trimming the
    /// 4-CC to exactly 4 ASCII bytes (e.g. <c>"crx "</c> with the
    /// trailing space).
    /// </summary>
    public static HeifBrandInfo From(string majorBrand, ImmutableArray<string> compatibleBrands)
    {
        var all = new List<string>(compatibleBrands.Length + 1) { majorBrand };
        foreach (var b in compatibleBrands)
        {
            if (b != majorBrand) all.Add(b);
        }

        var set = all.ToFrozenSet();

        bool hasHevcImage = set.Contains("heic") || set.Contains("heix") || set.Contains("heim");
        bool hasHevcSeq = set.Contains("hevc") || set.Contains("hevx") || set.Contains("heis");
        bool hasAv1Image = set.Contains("avif");
        bool hasAv1Seq = set.Contains("avis");
        bool hasVvcImage = set.Contains("vvic");
        bool hasVvcSeq = set.Contains("vvis");
        bool hasUncompressed = set.Contains("unif");
        bool hasCanonRaw = set.Contains("crx ");
        bool hasToneMap = set.Contains("tmap");
        bool hasMif = set.Contains("mif1") || set.Contains("mif2") || set.Contains("mif3");
        bool hasMsf = set.Contains("msf1");

        bool isImageSequence = hasHevcSeq || hasAv1Seq || hasVvcSeq || hasMsf;
        bool isMultilayer = set.Contains("heim") || set.Contains("heis");
        bool isRangeExtended = set.Contains("heix") || set.Contains("hevx");
        bool isAppleMultiImage = set.Contains("MA1A") || set.Contains("MA1B");

        HeifCodec codec = (majorBrand, hasHevcImage, hasHevcSeq, hasAv1Image, hasAv1Seq, hasVvcImage, hasVvcSeq, hasUncompressed, hasCanonRaw, hasToneMap) switch
        {
            // Tone-mapped takes precedence only when explicitly the major brand.
            ("tmap", _, _, _, _, _, _, _, _, _) => HeifCodec.ToneMapped,
            ("crx ", _, _, _, _, _, _, _, _, _) => HeifCodec.CanonRaw,
            ("unif", _, _, _, _, _, _, _, _, _) => HeifCodec.Uncompressed,
            (_, true, _, _, _, _, _, _, _, _) => HeifCodec.Hevc,
            (_, _, true, _, _, _, _, _, _, _) => HeifCodec.Hevc,
            (_, _, _, true, _, _, _, _, _, _) => HeifCodec.Av1,
            (_, _, _, _, true, _, _, _, _, _) => HeifCodec.Av1,
            (_, _, _, _, _, true, _, _, _, _) => HeifCodec.Vvc,
            (_, _, _, _, _, _, true, _, _, _) => HeifCodec.Vvc,
            (_, _, _, _, _, _, _, true, _, _) => HeifCodec.Uncompressed,
            (_, _, _, _, _, _, _, _, true, _) => HeifCodec.CanonRaw,
            (_, _, _, _, _, _, _, _, _, true) => HeifCodec.ToneMapped,
            _ => HeifCodec.Unknown,
        };

        HeifContainerKind kind = (isImageSequence, hasMif, hasHevcImage, hasAv1Image, hasVvcImage, hasCanonRaw, hasUncompressed, hasToneMap) switch
        {
            (true, _, _, _, _, _, _, _) => HeifContainerKind.ImageSequence,
            (_, true, _, _, _, _, _, _) => HeifContainerKind.SingleImage,
            (_, _, true, _, _, _, _, _) => HeifContainerKind.SingleImage,
            (_, _, _, true, _, _, _, _) => HeifContainerKind.SingleImage,
            (_, _, _, _, true, _, _, _) => HeifContainerKind.SingleImage,
            (_, _, _, _, _, true, _, _) => HeifContainerKind.SingleImage,
            (_, _, _, _, _, _, true, _) => HeifContainerKind.SingleImage,
            (_, _, _, _, _, _, _, true) => HeifContainerKind.SingleImage,
            _ => HeifContainerKind.Unknown,
        };

        return new HeifBrandInfo
        {
            MajorBrand = majorBrand,
            CompatibleBrands = compatibleBrands,
            PrimaryCodec = codec,
            ContainerKind = kind,
            IsImageSequence = isImageSequence,
            IsMultilayer = isMultilayer,
            IsRangeExtended = isRangeExtended,
            IsToneMapped = hasToneMap,
            IsAppleMultiImage = isAppleMultiImage,
        };
    }
}
