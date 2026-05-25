using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// A parsed box header describing an ISO BMFF box's location in the source.
/// </summary>
/// <param name="Type">The FourCC box type.</param>
/// <param name="HeaderOffset">Absolute offset of the box header in the source.</param>
/// <param name="PayloadOffset">Absolute offset of the box payload (post-header).</param>
/// <param name="PayloadLength">Length of the payload in bytes.</param>
/// <remarks>
/// The total box length is <c>PayloadOffset - HeaderOffset + PayloadLength</c>.
/// For a top-level <c>mdat</c> with the special size==0 marker the payload runs to EOF.
/// </remarks>
public readonly record struct BoxHeader(
    FourCc Type,
    long HeaderOffset,
    long PayloadOffset,
    long PayloadLength)
{
    /// <summary>The byte just past the end of this box.</summary>
    public long EndOffset => PayloadOffset + PayloadLength;

    /// <summary>The total length of the box header in bytes (8, 16, or 24 for a uuid box).</summary>
    public int HeaderLength => (int)(PayloadOffset - HeaderOffset);
}
