using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// HEIF image rotation property (<c>irot</c>) per ISO/IEC 23008-12 §6.5.10.
/// Counter-clockwise rotation that should be applied as part of the
/// rendering pipeline.
/// </summary>
public enum HeifImageRotation
{
    /// <summary>No rotation.</summary>
    None = 0,
    /// <summary>Rotate 90° counter-clockwise.</summary>
    Rotate90Ccw = 90,
    /// <summary>Rotate 180°.</summary>
    Rotate180 = 180,
    /// <summary>Rotate 270° counter-clockwise.</summary>
    Rotate270Ccw = 270,
}

/// <summary>
/// HEIF image mirror axis (<c>imir</c>) per ISO/IEC 23008-12 §6.5.12.
/// </summary>
public enum HeifImageMirrorAxis
{
    /// <summary>Mirror around the vertical axis (left ↔ right flip).</summary>
    Vertical = 0,
    /// <summary>Mirror around the horizontal axis (top ↔ bottom flip).</summary>
    Horizontal = 1,
}

/// <summary>
/// HEIF pixel aspect ratio property (<c>pasp</c>) per ISO/IEC 14496-12.
/// </summary>
public sealed record HeifPixelAspectRatio
{
    /// <summary>Horizontal sample spacing.</summary>
    public required uint HorizontalSpacing { get; init; }

    /// <summary>Vertical sample spacing.</summary>
    public required uint VerticalSpacing { get; init; }

    /// <summary>Computed aspect ratio (HorizontalSpacing / VerticalSpacing); NaN when VerticalSpacing is 0.</summary>
    public double Ratio => VerticalSpacing == 0 ? double.NaN : HorizontalSpacing / (double)VerticalSpacing;
}

/// <summary>
/// HEIF pixel information property (<c>pixi</c>) per ISO/IEC 23008-12
/// §6.5.6. Declares the number of channels in the reconstructed image
/// and the bit depth of each channel.
/// </summary>
public sealed record HeifPixelInformation
{
    /// <summary>Per-channel bit depths (e.g. [8, 8, 8] for 8-bit RGB; [10, 10, 10] for 10-bit; [8] for monochrome).</summary>
    public required ImmutableArray<byte> BitDepthsPerChannel { get; init; }

    /// <summary>Number of channels.</summary>
    public int NumberOfChannels => BitDepthsPerChannel.Length;

    /// <summary>Decodes a raw <c>pixi</c> payload (4-byte FullBox header
    /// + 1 byte num_channels + N bytes bit_depth[i]).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifPixelInformation info)
    {
        info = null!;
        if (data.Length < 5) return false;
        byte numChannels = data[4];
        if (5 + numChannels > data.Length) return false;
        var bitDepths = ImmutableArray.CreateBuilder<byte>(numChannels);
        for (int i = 0; i < numChannels; i++) bitDepths.Add(data[5 + i]);
        info = new HeifPixelInformation { BitDepthsPerChannel = bitDepths.ToImmutable() };
        return true;
    }
}

/// <summary>
/// HEIF auxiliary type property (<c>auxC</c>) per ISO/IEC 23008-12 §6.5.8.
/// Declares the semantic meaning of an auxiliary image item (alpha,
/// depth, gain map, etc.).
/// </summary>
public sealed record HeifAuxiliaryType
{
    /// <summary>Auxiliary type URN (e.g. "urn:mpeg:mpegB:cicp:systems:auxiliary:alpha").</summary>
    public required string AuxTypeUrn { get; init; }

    /// <summary>Optional auxiliary subtype bytes carried after the URN.</summary>
    public required ImmutableArray<byte> AuxSubtype { get; init; }

    /// <summary>True when the aux type URN matches the MPEG-B alpha auxiliary.</summary>
    public bool IsAlpha => AuxTypeUrn == "urn:mpeg:mpegB:cicp:systems:auxiliary:alpha";

    /// <summary>True when the aux type URN matches the MPEG-B depth auxiliary.</summary>
    public bool IsDepth => AuxTypeUrn == "urn:mpeg:mpegB:cicp:systems:auxiliary:depth";

    /// <summary>True when the aux type URN matches the ISO 21496-1 gain map auxiliary.</summary>
    public bool IsGainMap => AuxTypeUrn == "urn:iso:std:iso:ts:21496:-1:gainmap";

    /// <summary>Decodes a raw <c>auxC</c> payload (4-byte FullBox header
    /// + NUL-terminated UTF-8 URN + optional subtype bytes).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifAuxiliaryType type)
    {
        type = null!;
        if (data.Length < 4) return false;
        int pos = 4;
        int start = pos;
        while (pos < data.Length && data[pos] != 0) pos++;
        if (pos > data.Length) return false;
        string urn = System.Text.Encoding.UTF8.GetString(data.Slice(start, pos - start));
        if (pos < data.Length) pos++; // skip NUL
        ImmutableArray<byte> subtype = pos < data.Length
            ? ImmutableArray.Create(data[pos..])
            : ImmutableArray<byte>.Empty;
        type = new HeifAuxiliaryType { AuxTypeUrn = urn, AuxSubtype = subtype };
        return true;
    }
}
