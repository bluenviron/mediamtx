#pragma warning disable CA1711 // The type name mirrors the ISO/IEC 14496-3 syntactic element individual_channel_stream().

namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed view of an AAC <c>individual_channel_stream()</c> body up to
/// (but not including) <c>spectral_data()</c>, per ISO/IEC 14496-3
/// §4.4.2.6 Table 4.44 / Table 4.71. Aggregates the existing
/// element-parsers behind a single entry point:
/// <c>global_gain</c> + optional <c>ics_info()</c> + <c>section_data()</c>
/// + <c>scale_factor_data()</c> + optional
/// <c>pulse_data()</c> + optional <c>tns_data()</c> + the
/// <c>gain_control_data_present</c> bit.
/// </summary>
/// <remarks>
/// <para>
/// The aggregator stops at <c>spectral_data()</c>. The remaining bits
/// in the source span belong to the spectral coefficient stream and
/// are intentionally not consumed here; a future spectral-data
/// walker will read them via the existing
/// <see cref="AacSpectralValueDecoder"/> and the section list captured
/// in <see cref="SectionData"/>.
/// </para>
/// <para>
/// <c>gain_control_data()</c> bodies are SSR-profile-only
/// (audio_object_type == 3) and extremely rare in real-world content.
/// When the <c>gain_control_data_present</c> bit is set the aggregator
/// dispatches to <see cref="AacGainControlData"/> using the active
/// <see cref="AacIcsInfo.WindowSequence"/> for the bit-layout. The
/// parsed payload is exposed via <see cref="GainControlData"/>;
/// <c>gain_control_data_present = 0</c> leaves it <see langword="null"/>.
/// </para>
/// <para>
/// Per spec, <c>pulse_data_present</c> may only be set when the
/// enclosing <see cref="AacIcsInfo.WindowSequence"/> is not
/// <see cref="AacWindowSequence.EightShort"/>; the aggregator
/// enforces this and returns <see langword="false"/> on violation.
/// </para>
/// </remarks>
public sealed record AacIndividualChannelStream
{
    /// <summary>8-bit <c>global_gain</c> anchor for the scale-factor stream.</summary>
    public required int GlobalGain { get; init; }

    /// <summary>
    /// Owned <c>ics_info()</c> when the enclosing element passed
    /// <c>common_window = 0</c>; <see langword="null"/> when the
    /// caller supplied a shared <see cref="AacIcsInfo"/> via
    /// <see cref="TryRead"/>.
    /// </summary>
    public AacIcsInfo? OwnIcsInfo { get; init; }

    /// <summary>
    /// The <c>ics_info()</c> actually used to drive the rest of the
    /// parse - either <see cref="OwnIcsInfo"/> or the caller-supplied
    /// shared instance.
    /// </summary>
    public required AacIcsInfo IcsInfo { get; init; }

    /// <summary>Parsed <c>section_data()</c>.</summary>
    public required AacSectionData SectionData { get; init; }

    /// <summary>Parsed <c>scale_factor_data()</c>.</summary>
    public required AacScaleFactorData ScaleFactorData { get; init; }

    /// <summary><c>pulse_data_present</c> flag (1 bit).</summary>
    public required bool PulseDataPresent { get; init; }

    /// <summary>Parsed <c>pulse_data()</c> when <see cref="PulseDataPresent"/>; otherwise <see langword="null"/>.</summary>
    public AacPulseData? PulseData { get; init; }

    /// <summary><c>tns_data_present</c> flag (1 bit).</summary>
    public required bool TnsDataPresent { get; init; }

    /// <summary>Parsed <c>tns_data()</c> when <see cref="TnsDataPresent"/>; otherwise <see langword="null"/>.</summary>
    public AacTnsData? TnsData { get; init; }

    /// <summary>
    /// <c>gain_control_data_present</c> flag (1 bit). When
    /// <see langword="true"/>, the parsed payload is exposed via
    /// <see cref="GainControlData"/>.
    /// </summary>
    public required bool GainControlDataPresent { get; init; }

    /// <summary>
    /// Parsed <c>gain_control_data()</c> when
    /// <see cref="GainControlDataPresent"/>; otherwise
    /// <see langword="null"/>.
    /// </summary>
    public AacGainControlData? GainControlData { get; init; }

    /// <summary>
    /// Total bits consumed up to and including the
    /// <c>gain_control_data_present</c> trailing flag (when
    /// <c>scale_flag = false</c>). When <c>scale_flag = true</c> the
    /// count stops after <c>scale_factor_data()</c>. The bits that
    /// follow belong to <c>spectral_data()</c>.
    /// </summary>
    public required int BitsConsumed { get; init; }

    /// <summary>
    /// Read an <c>individual_channel_stream()</c> body up to the
    /// <c>spectral_data()</c> boundary.
    /// </summary>
    /// <param name="reader">Bit reader positioned at <c>global_gain</c>.</param>
    /// <param name="sharedIcsInfo">
    /// Caller-supplied shared ics_info when the enclosing element
    /// passed <c>common_window = 1</c>; <see langword="null"/> when the
    /// body should parse its own ics_info.
    /// </param>
    /// <param name="scaleFlag">
    /// Scalable-coding flag from the enclosing scalable_extension_data().
    /// Always <see langword="false"/> for AAC-LC content. When
    /// <see langword="true"/>, the optional <c>pulse_data()</c> /
    /// <c>tns_data()</c> / <c>gain_control_data_present</c> block is
    /// skipped.
    /// </param>
    /// <param name="scaleFactorCodebook">121-symbol scale-factor Huffman codebook.</param>
    /// <param name="stream">Populated on success; <see langword="null"/> otherwise.</param>
    /// <returns>
    /// <see langword="true"/> when every sub-element parsed cleanly;
    /// <see langword="false"/> on stream underflow or on a
    /// <c>pulse_data_present = 1</c> with an EIGHT_SHORT ics_info.
    /// </returns>
    internal static bool TryRead(
        scoped ref BitReader reader,
        AacIcsInfo? sharedIcsInfo,
        bool scaleFlag,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacIndividualChannelStream? stream)
    {
        stream = null;
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);

        int startBits = reader.Position;
        if (reader.Remaining < 8) return false;
        int globalGain = (int)reader.ReadBits(8);

        AacIcsInfo? ownIcsInfo = null;
        AacIcsInfo icsInfo;
        if (sharedIcsInfo is null)
        {
            if (!AacIcsInfo.TryParse(ref reader, out var parsed) || parsed is null)
            {
                return false;
            }
            ownIcsInfo = parsed;
            icsInfo = parsed;
        }
        else
        {
            icsInfo = sharedIcsInfo;
        }

        if (!AacSectionData.TryParse(ref reader, icsInfo, out var sectionData) || sectionData is null)
        {
            return false;
        }

        if (!AacScaleFactorData.TryRead(ref reader, sectionData, scaleFactorCodebook, out var scaleFactorData)
            || scaleFactorData is null)
        {
            return false;
        }

        bool pulseDataPresent = false;
        AacPulseData? pulseData = null;
        bool tnsDataPresent = false;
        AacTnsData? tnsData = null;
        bool gainControlDataPresent = false;
        AacGainControlData? gainControlData = null;

        if (!scaleFlag)
        {
            if (reader.Remaining < 1) return false;
            pulseDataPresent = reader.ReadBit();
            if (pulseDataPresent)
            {
                if (icsInfo.WindowSequence == AacWindowSequence.EightShort)
                {
                    // pulse_data() is invalid when window_sequence == EIGHT_SHORT.
                    return false;
                }
                if (!AacPulseData.TryRead(ref reader, out pulseData) || pulseData is null)
                {
                    return false;
                }
            }

            if (reader.Remaining < 1) return false;
            tnsDataPresent = reader.ReadBit();
            if (tnsDataPresent)
            {
                if (!AacTnsData.TryRead(ref reader, icsInfo.WindowSequence, out tnsData) || tnsData is null)
                {
                    return false;
                }
            }

            if (reader.Remaining < 1) return false;
            gainControlDataPresent = reader.ReadBit();
            if (gainControlDataPresent)
            {
                if (!AacGainControlData.TryRead(ref reader, icsInfo.WindowSequence, out gainControlData)
                    || gainControlData is null)
                {
                    return false;
                }
            }
        }

        stream = new AacIndividualChannelStream
        {
            GlobalGain = globalGain,
            OwnIcsInfo = ownIcsInfo,
            IcsInfo = icsInfo,
            SectionData = sectionData,
            ScaleFactorData = scaleFactorData,
            PulseDataPresent = pulseDataPresent,
            PulseData = pulseData,
            TnsDataPresent = tnsDataPresent,
            TnsData = tnsData,
            GainControlDataPresent = gainControlDataPresent,
            GainControlData = gainControlData,
            BitsConsumed = reader.Position - startBits,
        };
        return true;
    }

    /// <summary>
    /// Parses a contiguous <c>individual_channel_stream()</c> body
    /// from <paramref name="bytes"/> starting at the first bit. See
    /// <see cref="TryRead"/> for the parameter semantics.
    /// </summary>
    public static bool TryParse(
        ReadOnlySpan<byte> bytes,
        AacIcsInfo? sharedIcsInfo,
        bool scaleFlag,
        AacHuffmanCodebook scaleFactorCodebook,
        out AacIndividualChannelStream? stream)
    {
        var reader = new BitReader(bytes);
        return TryRead(ref reader, sharedIcsInfo, scaleFlag, scaleFactorCodebook, out stream);
    }
}
