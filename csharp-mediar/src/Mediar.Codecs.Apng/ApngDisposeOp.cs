namespace Mediar.Codecs.Apng;

/// <summary>
/// APNG fcTL <c>dispose_op</c> values per the APNG specification. Determines
/// how the canvas is mutated at the end of the current frame's delay,
/// before the next frame is rendered.
/// </summary>
/// <remarks>
/// The numeric values match the on-disk encoding of the <c>fcTL</c> chunk
/// (<see href="https://wiki.mozilla.org/APNG_Specification" />).
/// </remarks>
public enum ApngDisposeOp : byte
{
    /// <summary>No disposal; the current contents of the canvas are kept as-is.</summary>
    None = 0,

    /// <summary>The frame's region of the canvas is cleared to fully transparent black.</summary>
    Background = 1,

    /// <summary>The frame's region of the canvas is reverted to the state before this frame was rendered.</summary>
    Previous = 2,
}
