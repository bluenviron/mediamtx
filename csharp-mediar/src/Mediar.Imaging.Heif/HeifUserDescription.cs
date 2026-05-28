using System.Text;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view of the HEIF <c>udes</c> property (User Description) per
/// ISO/IEC 23008-12 §6.5.20. The property carries four null-terminated
/// UTF-8 strings — language tag, human-readable name, free-form
/// description, and a comma-separated tag list — and is most commonly
/// attached to image collections (e.g. cover-image item or album item)
/// to convey author-provided titles and descriptions.
/// </summary>
public sealed record HeifUserDescription
{
    /// <summary>BCP-47 language tag (e.g. "en", "en-US"); empty when unspecified.</summary>
    public required string Lang { get; init; }

    /// <summary>Human-readable name / title.</summary>
    public required string Name { get; init; }

    /// <summary>Free-form description.</summary>
    public required string Description { get; init; }

    /// <summary>Comma-separated tag list.</summary>
    public required string Tags { get; init; }

    /// <summary>
    /// Decodes a raw <c>udes</c> payload: 4-byte FullBox header
    /// (version + flags) followed by four null-terminated UTF-8
    /// strings (lang, name, description, tags).
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifUserDescription record)
    {
        record = null!;
        if (data.Length < 4) return false;

        byte version = data[0];
        if (version != 0) return false; // only version 0 defined.

        int pos = 4; // skip version + 3 flag bytes.
        if (!TryReadCString(data, ref pos, out var lang)) return false;
        if (!TryReadCString(data, ref pos, out var name)) return false;
        if (!TryReadCString(data, ref pos, out var description)) return false;
        if (!TryReadCString(data, ref pos, out var tags)) return false;

        record = new HeifUserDescription
        {
            Lang = lang,
            Name = name,
            Description = description,
            Tags = tags,
        };
        return true;
    }

    private static bool TryReadCString(ReadOnlySpan<byte> data, ref int pos, out string value)
    {
        value = "";
        if (pos >= data.Length) return false;
        int start = pos;
        while (pos < data.Length && data[pos] != 0) pos++;
        if (pos >= data.Length) return false; // missing null terminator.
        value = Encoding.UTF8.GetString(data.Slice(start, pos - start));
        pos++; // consume the null terminator.
        return true;
    }
}
