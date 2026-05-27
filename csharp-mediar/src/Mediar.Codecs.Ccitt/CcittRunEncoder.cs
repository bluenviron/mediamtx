namespace Mediar.Codecs.Ccitt;

/// <summary>
/// Helpers shared between the encoders: emit a run-length using MH white
/// or black codes, splitting into extended-makeup + makeup + terminating
/// triplets as required by T.4 Annex A.
/// </summary>
internal static class CcittRunEncoder
{
    /// <summary>
    /// Emit <paramref name="run"/> consecutive pixels of the chosen
    /// <paramref name="colour"/> (0 = white, 1 = black) using MH codes.
    /// Runs &gt; 2623 are split into multiple makeup codes followed by a
    /// terminating code &lt; 64 as specified by T.4 §2.2.1.
    /// </summary>
    public static void WriteRun(CcittBitWriter writer, int colour, int run)
    {
        ArgumentNullException.ThrowIfNull(writer);
        ArgumentOutOfRangeException.ThrowIfNegative(run);
        var table = colour == 0 ? CcittTables.WhiteEncode : CcittTables.BlackEncode;

        // Extended makeup chunks (1792 .. 2560 step 64).
        while (run >= 2560 + 64)
        {
            EmitCode(writer, table, 2560);
            run -= 2560;
        }
        if (run >= 1792)
        {
            int chunk = Math.Min(run & ~63, 2560);
            EmitCode(writer, table, chunk);
            run -= chunk;
        }

        // Regular makeup (64 .. 1728 step 64).
        if (run >= 64)
        {
            int chunk = run & ~63;
            EmitCode(writer, table, chunk);
            run -= chunk;
        }

        // Terminating (0..63).
        EmitCode(writer, table, run);
    }

    private static void EmitCode(CcittBitWriter writer,
                                 IReadOnlyDictionary<int, (uint Bits, int Length)> table,
                                 int run)
    {
        if (!table.TryGetValue(run, out var code))
        {
            throw new InvalidOperationException($"No MH code for run {run}.");
        }
        writer.Write(code.Bits, code.Length);
    }
}
