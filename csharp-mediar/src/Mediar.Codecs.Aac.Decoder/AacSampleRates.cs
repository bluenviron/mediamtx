namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// MPEG-4 sampling-frequency-index table (ISO/IEC 14496-3 Table 1.18).
/// Maps the 4-bit <c>samplingFrequencyIndex</c> field to a sample rate in
/// Hz. Index 0x0F signals that the rate is carried inline as a 24-bit
/// <c>samplingFrequency</c> field.
/// </summary>
public static class AacSampleRates
{
    /// <summary>The sample-frequency-index escape value (15).</summary>
    public const int EscapeIndex = 0x0F;

    private static readonly int[] Table =
    [
        96_000, 88_200, 64_000, 48_000,
        44_100, 32_000, 24_000, 22_050,
        16_000, 12_000, 11_025,  8_000,
         7_350,      0,      0,      0,
    ];

    /// <summary>Return the sample rate in Hz for <paramref name="index"/>, or 0 for reserved / escape.</summary>
    public static int FromIndex(int index)
    {
        if ((uint)index >= (uint)Table.Length) return 0;
        return Table[index];
    }

    /// <summary>Return the index for <paramref name="sampleRate"/>, or <see cref="EscapeIndex"/> when it is not table-resident.</summary>
    public static int ToIndex(int sampleRate)
    {
        for (int i = 0; i < Table.Length; i++)
        {
            if (Table[i] == sampleRate) return i;
        }
        return EscapeIndex;
    }
}

/// <summary>
/// MPEG-4 channel configuration table (ISO/IEC 14496-3 Table 1.19).
/// <c>channelConfiguration</c> is a 4-bit field; the values below cover
/// configurations 1..7. Configuration 0 means the channel layout is
/// described inline by a <c>program_config_element()</c>.
/// </summary>
public static class AacChannelConfigurations
{
    /// <summary>Number of speaker channels rendered for <paramref name="config"/>, or 0 when the layout is PCE-described.</summary>
    public static int SpeakerCount(int config) => config switch
    {
        1 => 1, // C
        2 => 2, // L R
        3 => 3, // C L R
        4 => 4, // C L R Cs
        5 => 5, // C L R Ls Rs
        6 => 6, // C L R Ls Rs LFE
        7 => 8, // C L R Ls Rs LsB RsB LFE (7.1)
        _ => 0,
    };
}
