namespace Mediar.Codecs.Apng;

/// <summary>
/// APNG fcTL <c>blend_op</c> values per the APNG specification. Determines
/// how the frame's pixels are written to the canvas.
/// </summary>
/// <remarks>
/// The numeric values match the on-disk encoding of the <c>fcTL</c> chunk
/// (<see href="https://wiki.mozilla.org/APNG_Specification" />).
/// </remarks>
public enum ApngBlendOp : byte
{
    /// <summary>All channels of the frame (including alpha) overwrite the corresponding canvas region.</summary>
    Source = 0,

    /// <summary>The frame is alpha-blended onto the canvas using a standard SRC-OVER operation.</summary>
    Over = 1,
}
