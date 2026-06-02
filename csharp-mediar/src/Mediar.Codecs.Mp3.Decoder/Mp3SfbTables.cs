namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// Scalefactor band index tables for MPEG-1/2/2.5 Layer III, per ISO 11172-3
/// Table B.8 and ISO 13818-3 Table 8/9. Each entry is the START offset (in
/// the 576-coefficient spectral array for long blocks, or in one of three
/// 192-coefficient short half-windows for short blocks) of one scalefactor
/// band; the final entry is the implicit "end" sentinel.
/// </summary>
internal static class Mp3SfbTables
{
    public static readonly int[][] LongBands = new int[][]
    {
        new[] { 0, 4, 8, 12, 16, 20, 24, 30, 36, 44, 52, 62, 74, 90, 110, 134, 162, 196, 238, 288, 342, 418, 576 },
        new[] { 0, 4, 8, 12, 16, 20, 24, 30, 36, 42, 50, 60, 72, 88, 106, 128, 156, 190, 230, 276, 330, 384, 576 },
        new[] { 0, 4, 8, 12, 16, 20, 24, 30, 36, 44, 54, 66, 82, 102, 126, 156, 194, 240, 296, 364, 448, 550, 576 },
        new[] { 0, 6, 12, 18, 24, 30, 36, 44, 54, 66, 80, 96, 116, 140, 168, 200, 238, 284, 336, 396, 464, 522, 576 },
        new[] { 0, 6, 12, 18, 24, 30, 36, 44, 54, 66, 80, 96, 114, 136, 162, 194, 232, 278, 332, 394, 464, 540, 576 },
        new[] { 0, 6, 12, 18, 24, 30, 36, 44, 54, 66, 80, 96, 116, 140, 168, 200, 238, 284, 336, 396, 464, 522, 576 },
        new[] { 0, 6, 12, 18, 24, 30, 36, 44, 54, 66, 80, 96, 116, 140, 168, 200, 238, 284, 336, 396, 464, 522, 576 },
        new[] { 0, 6, 12, 18, 24, 30, 36, 44, 54, 66, 80, 96, 116, 140, 168, 200, 238, 284, 336, 396, 464, 522, 576 },
        new[] { 0, 12, 24, 36, 48, 60, 72, 88, 108, 132, 160, 192, 232, 280, 336, 400, 476, 566, 568, 570, 572, 574, 576 },
    };

    public static readonly int[][] ShortBands = new int[][]
    {
        new[] { 0, 4, 8, 12, 16, 22, 30, 40, 52, 66, 84, 106, 136, 192 },
        new[] { 0, 4, 8, 12, 16, 22, 28, 38, 50, 64, 80, 100, 126, 192 },
        new[] { 0, 4, 8, 12, 16, 22, 30, 42, 58, 78, 104, 138, 180, 192 },
        new[] { 0, 4, 8, 12, 18, 24, 32, 42, 56, 74, 100, 132, 174, 192 },
        new[] { 0, 4, 8, 12, 18, 26, 36, 48, 62, 80, 104, 136, 180, 192 },
        new[] { 0, 4, 8, 12, 18, 26, 36, 48, 62, 80, 104, 134, 174, 192 },
        new[] { 0, 4, 8, 12, 18, 24, 32, 42, 56, 74, 100, 132, 174, 192 },
        new[] { 0, 4, 8, 12, 18, 24, 32, 42, 56, 74, 100, 132, 174, 192 },
        new[] { 0, 8, 16, 24, 36, 52, 72, 96, 124, 160, 162, 164, 166, 192 },
    };

    /// <summary>
    /// Pick the band-table row for (mpeg_version, sample_rate_index):
    /// row indices: 0..2 = MPEG-1 (44.1, 48, 32), 3..5 = MPEG-2 LSF (22.05, 24, 16),
    /// 6..8 = MPEG-2.5 (11.025, 12, 8).
    /// </summary>
    public static int Row(int version, int sampleRateIndex)
    {
        int @base = version switch { 1 => 0, 2 => 3, 25 => 6, _ => 0 };
        return @base + sampleRateIndex;
    }
}
