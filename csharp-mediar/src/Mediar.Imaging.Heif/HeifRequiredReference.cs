using System.Collections.Immutable;
using System.Text;

namespace Mediar.Imaging.Heif;

/// <summary>
/// HEIF Required Reference Types property (<c>rref</c>) per ISO/IEC
/// 23008-12. When associated with a derived image item, declares the
/// set of reference types (4-character codes from the <c>iref</c>
/// graph) whose source items must be successfully resolved for the
/// derived item to be rendered. A reader that cannot resolve any of
/// the listed reference types should treat the derived item as
/// unavailable rather than rendering a partial result.
/// </summary>
public sealed record HeifRequiredReference
{
    /// <summary>The list of required reference type 4-character codes
    /// (e.g. "dimg", "thmb", "auxl"). Order is preserved from the
    /// underlying box.</summary>
    public required ImmutableArray<string> ReferenceTypes { get; init; }

    /// <summary>Returns true when <paramref name="referenceType"/> is
    /// listed as required by this property.</summary>
    public bool Requires(string referenceType)
    {
        if (string.IsNullOrEmpty(referenceType)) return false;
        foreach (var rt in ReferenceTypes)
        {
            if (rt == referenceType) return true;
        }
        return false;
    }

    /// <summary>Decodes a raw <c>rref</c> payload (4-byte FullBox
    /// header + 1 byte reference_type_count + N*4 bytes of 4-character
    /// reference type codes).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifRequiredReference required)
    {
        required = null!;
        if (data.Length < 5) return false;
        if (data[0] != 0) return false; // only version 0 is defined

        byte count = data[4];
        int needed = 5 + count * 4;
        if (data.Length < needed) return false;

        var builder = ImmutableArray.CreateBuilder<string>(count);
        int pos = 5;
        for (int i = 0; i < count; i++)
        {
            string code = Encoding.ASCII.GetString(data.Slice(pos, 4));
            builder.Add(code);
            pos += 4;
        }
        required = new HeifRequiredReference { ReferenceTypes = builder.ToImmutable() };
        return true;
    }
}
