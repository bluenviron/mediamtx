using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Result of validating a <see cref="HeifReader"/> against a specific
/// HEIF / AVIF profile. <see cref="Issues"/> is empty when
/// <see cref="IsConformant"/> is true.
/// </summary>
public sealed record HeifConformanceResult
{
    /// <summary>Human-readable profile name (e.g. "AVIF", "HEIC Main",
    /// "MIAF").</summary>
    public required string ProfileName { get; init; }

    /// <summary>True iff the file satisfies every required rule for
    /// the named profile.</summary>
    public required bool IsConformant { get; init; }

    /// <summary>Human-readable description of each rule violation. An
    /// empty list always accompanies <see cref="IsConformant"/> true.</summary>
    public required ImmutableArray<string> Issues { get; init; }
}

/// <summary>
/// Static brand-conformance validators for the most common HEIF /
/// AVIF profiles. Each validator inspects the existing
/// <see cref="HeifReader"/> view (ftyp brands, primary item, item
/// list, properties, ipma associations) and reports the set of
/// required-rule violations.
/// </summary>
public static class HeifBrandConformance
{
    /// <summary>Validates against the AVIF specification baseline
    /// profile: <c>avif</c> brand, an AV1-encoded primary item (or an
    /// AVIF derivation), and the mandatory ispe + av1C properties on
    /// every AV1 image item.</summary>
    public static HeifConformanceResult ValidateAvif(HeifReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);
        var issues = ImmutableArray.CreateBuilder<string>();

        if (!reader.BrandInfo.HasBrand("avif"))
        {
            issues.Add("ftyp does not advertise the 'avif' brand.");
        }

        ValidatePrimaryItem(reader, issues, "av01", out var primaryItem);

        foreach (var item in reader.Items)
        {
            if (item.Type != "av01") continue;
            EnsureProperty(reader, item.Id, "ispe", issues);
            EnsureProperty(reader, item.Id, "av1C", issues);
        }

        // Conformance trivially passes when there are no AV1 items AND no primary item resolved.
        if (primaryItem is null && CountItemsOfType(reader, "av01") == 0 && issues.Count == 0)
        {
            issues.Add("No AV1 image items present and no primary item resolved.");
        }

        return Build("AVIF", issues);
    }

    /// <summary>Validates against the HEIF Main image profile per
    /// ISO/IEC 23008-12: <c>heic</c>/<c>heix</c>/<c>mif1</c> brand
    /// presence, an HEVC-encoded primary item (or HEIF derivation),
    /// and the mandatory ispe + hvcC properties on every HEVC image
    /// item.</summary>
    public static HeifConformanceResult ValidateHeic(HeifReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);
        var issues = ImmutableArray.CreateBuilder<string>();

        if (!reader.BrandInfo.HasBrand("heic") &&
            !reader.BrandInfo.HasBrand("heix") &&
            !reader.BrandInfo.HasBrand("mif1"))
        {
            issues.Add("ftyp does not advertise any of 'heic', 'heix', or 'mif1' brands.");
        }

        ValidatePrimaryItem(reader, issues, "hvc1", "hev1", out var primaryItem);

        foreach (var item in reader.Items)
        {
            if (item.Type != "hvc1" && item.Type != "hev1") continue;
            EnsureProperty(reader, item.Id, "ispe", issues);
            EnsureProperty(reader, item.Id, "hvcC", issues);
        }

        if (primaryItem is null
            && CountItemsOfType(reader, "hvc1") == 0
            && CountItemsOfType(reader, "hev1") == 0
            && issues.Count == 0)
        {
            issues.Add("No HEVC image items present and no primary item resolved.");
        }

        return Build("HEIC Main", issues);
    }

    /// <summary>Validates against the MIAF profile per ISO/IEC
    /// 23000-22: <c>miaf</c> or <c>mif2</c> brand presence, every
    /// image item carries an ispe property, and every image item
    /// carries at least one of pixi / colr so the colour space is
    /// declared.</summary>
    public static HeifConformanceResult ValidateMiaf(HeifReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);
        var issues = ImmutableArray.CreateBuilder<string>();

        if (!reader.BrandInfo.HasBrand("miaf") && !reader.BrandInfo.HasBrand("mif2"))
        {
            issues.Add("ftyp does not advertise either of 'miaf' or 'mif2' brands.");
        }

        if (reader.PrimaryItemId == 0)
        {
            issues.Add("No primary item declared in pitm.");
        }

        foreach (var item in reader.Items)
        {
            if (!IsImageItemType(item.Type)) continue;
            EnsureProperty(reader, item.Id, "ispe", issues);
            if (!HasProperty(reader, item.Id, "pixi") && !HasProperty(reader, item.Id, "colr"))
            {
                issues.Add($"Image item {item.Id} ({item.Type}) is missing colour info: neither pixi nor colr is associated.");
            }
        }

        return Build("MIAF", issues);
    }

    // ----- helpers -----

    private static void ValidatePrimaryItem(
        HeifReader reader,
        ImmutableArray<string>.Builder issues,
        string expectedType,
        out HeifItem? primaryItem)
        => ValidatePrimaryItem(reader, issues, expectedType, expectedAlt: null, out primaryItem);

    private static void ValidatePrimaryItem(
        HeifReader reader,
        ImmutableArray<string>.Builder issues,
        string expectedType,
        string? expectedAlt,
        out HeifItem? primaryItem)
    {
        primaryItem = null;
        if (reader.PrimaryItemId == 0)
        {
            issues.Add("No primary item declared in pitm.");
            return;
        }

        foreach (var item in reader.Items)
        {
            if (item.Id == reader.PrimaryItemId) { primaryItem = item; break; }
        }

        if (primaryItem is null)
        {
            issues.Add($"Primary item id {reader.PrimaryItemId} is not present in the iinf item list.");
            return;
        }

        bool match = primaryItem.Type == expectedType
                  || (expectedAlt is not null && primaryItem.Type == expectedAlt)
                  || IsAcceptedDerivation(primaryItem.Type);
        if (!match)
        {
            string alt = expectedAlt is null ? "" : $"/{expectedAlt}";
            issues.Add($"Primary item type '{primaryItem.Type}' is not '{expectedType}{alt}' or a recognised derivation (grid / iden / iovl).");
        }
    }

    private static bool IsAcceptedDerivation(string type)
        => type is "grid" or "iden" or "iovl";

    private static bool IsImageItemType(string type)
        => type is "hvc1" or "hev1" or "av01" or "vvc1" or "jpeg" or "j2ki" or "grid" or "iden" or "iovl";

    private static int CountItemsOfType(HeifReader reader, string type)
    {
        int n = 0;
        foreach (var it in reader.Items)
        {
            if (it.Type == type) n++;
        }
        return n;
    }

    private static bool HasProperty(HeifReader reader, uint itemId, string propertyType)
    {
        if (!reader.Associations.TryGetValue(itemId, out var indices)) return false;
        foreach (int idx in indices)
        {
            if (idx <= 0 || idx > reader.Properties.Length) continue;
            if (reader.Properties[idx - 1].Type == propertyType) return true;
        }
        return false;
    }

    private static void EnsureProperty(
        HeifReader reader,
        uint itemId,
        string propertyType,
        ImmutableArray<string>.Builder issues)
    {
        if (!HasProperty(reader, itemId, propertyType))
        {
            issues.Add($"Image item {itemId} is missing required '{propertyType}' property.");
        }
    }

    private static HeifConformanceResult Build(string profileName, ImmutableArray<string>.Builder issues)
        => new()
        {
            ProfileName = profileName,
            IsConformant = issues.Count == 0,
            Issues = issues.ToImmutable(),
        };
}
