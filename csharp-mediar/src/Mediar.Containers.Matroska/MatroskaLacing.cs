namespace Mediar.Containers.Matroska;

/// <summary>
/// Matroska Block / SimpleBlock lacing mode. The numeric values match the
/// two-bit lacing field in the block flags byte (bits 1-2).
/// </summary>
public enum MatroskaLacing
{
    /// <summary>One frame per block (default).</summary>
    None = 0,
    /// <summary>Xiph lacing — per-frame sizes encoded as 0xFF run-lengths.</summary>
    Xiph = 1,
    /// <summary>Fixed lacing — all frames have the same size; no per-frame headers.</summary>
    Fixed = 2,
    /// <summary>EBML lacing — first size as unsigned VINT, subsequent as signed-delta VINTs.</summary>
    Ebml = 3,
}
