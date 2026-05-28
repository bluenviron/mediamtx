namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Parsed MPEG-4 AudioSpecificConfig (ISO/IEC 14496-3 §1.6.2.1). Carries
/// everything a raw_data_block decoder needs to bootstrap: the audio object
/// type, the resolved sample rate (post-SBR), the channel count, and the SBR
/// extension state (HE-AAC v1) when signalled explicitly. Stored in MP4 as
/// the <c>esds</c> DecoderSpecificInfo payload and reconstructable from
/// ADTS headers via <see cref="AacAdtsBridge.BuildAudioSpecificConfig"/>.
/// </summary>
public sealed record AudioSpecificConfig
{
    /// <summary>Numeric audio object type (1..31, then optional 32+ via escape).</summary>
    public required int AudioObjectType { get; init; }

    /// <summary>Strongly-typed view of <see cref="AudioObjectType"/> when in range.</summary>
    public AacAudioObjectType ObjectTypeEnum =>
        AudioObjectType >= 0 && AudioObjectType <= 31
            ? (AacAudioObjectType)AudioObjectType
            : AacAudioObjectType.Null;

    /// <summary>Sampling-frequency-index field (0..14, or 15 for inline frequency).</summary>
    public required int SamplingFrequencyIndex { get; init; }

    /// <summary>Resolved sample rate in Hz (post-SBR if SBR signalling is present).</summary>
    public required int SamplingFrequency { get; init; }

    /// <summary>Channel configuration field (0..7 typical; 0 means PCE-described).</summary>
    public required int ChannelConfiguration { get; init; }

    /// <summary>Effective speaker count derived from <see cref="ChannelConfiguration"/>.</summary>
    public required int ChannelCount { get; init; }

    /// <summary>True when an explicit SBR extension was signalled.</summary>
    public bool SbrPresent { get; init; }

    /// <summary>True when an explicit Parametric Stereo extension was signalled.</summary>
    public bool PsPresent { get; init; }

    /// <summary>Extension sampling-frequency-index when SBR is signalled, else -1.</summary>
    public int ExtensionSamplingFrequencyIndex { get; init; } = -1;

    /// <summary>Extension sample rate in Hz when SBR is signalled, else 0.</summary>
    public int ExtensionSamplingFrequency { get; init; }

    /// <summary>Object type carried after an SBR/PS extension marker, or 0 when not present.</summary>
    public int ExtensionAudioObjectType { get; init; }

    /// <summary>
    /// Parse an MPEG-4 AudioSpecificConfig from <paramref name="data"/>. Returns
    /// false on truncation, unrecognised object types, or invalid sample-rate
    /// indices. The parser currently supports the AAC family (object types
    /// 1..7, 17, 19, 20, 22, 23, 29) plus the SBR/PS extension blocks; other
    /// object types are parsed up to the AOT field and then surfaced verbatim
    /// without bit-level interpretation of the GASpecificConfig.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out AudioSpecificConfig? config)
    {
        config = null;
        if (data.IsEmpty) return false;

        try
        {
            var reader = new BitReader(data);
            int aot = ReadAudioObjectType(ref reader);
            if (aot <= 0) return false;

            int sfIndex = (int)reader.ReadBits(4);
            int sampleRate;
            if (sfIndex == AacSampleRates.EscapeIndex)
            {
                if (reader.Remaining < 24) return false;
                sampleRate = (int)reader.ReadBits(24);
                if (sampleRate <= 0) return false;
            }
            else
            {
                sampleRate = AacSampleRates.FromIndex(sfIndex);
                if (sampleRate <= 0) return false;
            }

            if (reader.Remaining < 4) return false;
            int channelConfig = (int)reader.ReadBits(4);

            bool sbrPresent = false;
            bool psPresent = false;
            int extAot = 0;
            int extSfIndex = -1;
            int extSampleRate = 0;

            if (aot == (int)AacAudioObjectType.Sbr || aot == (int)AacAudioObjectType.Ps)
            {
                sbrPresent = true;
                psPresent = aot == (int)AacAudioObjectType.Ps;
                extAot = aot;

                if (reader.Remaining < 4) return false;
                extSfIndex = (int)reader.ReadBits(4);
                if (extSfIndex == AacSampleRates.EscapeIndex)
                {
                    if (reader.Remaining < 24) return false;
                    extSampleRate = (int)reader.ReadBits(24);
                }
                else
                {
                    extSampleRate = AacSampleRates.FromIndex(extSfIndex);
                    if (extSampleRate <= 0) return false;
                }
                aot = ReadAudioObjectType(ref reader);
                if (aot <= 0) return false;
            }

            int channelCount = channelConfig == 0 ? 0 : AacChannelConfigurations.SpeakerCount(channelConfig);
            if (channelConfig != 0 && channelCount == 0) return false;

            config = new AudioSpecificConfig
            {
                AudioObjectType = aot,
                SamplingFrequencyIndex = sfIndex,
                SamplingFrequency = sampleRate,
                ChannelConfiguration = channelConfig,
                ChannelCount = channelCount,
                SbrPresent = sbrPresent,
                PsPresent = psPresent,
                ExtensionAudioObjectType = extAot,
                ExtensionSamplingFrequencyIndex = extSfIndex,
                ExtensionSamplingFrequency = extSampleRate,
            };
            return true;
        }
        catch (EndOfStreamException)
        {
            config = null;
            return false;
        }
        catch (ArgumentOutOfRangeException)
        {
            config = null;
            return false;
        }
    }

    /// <summary>
    /// Serialise this configuration back to the AudioSpecificConfig bit layout.
    /// Currently emits the minimal AOT + samplingFrequencyIndex + channelConfiguration
    /// triple (and the inline samplingFrequency / SBR extension blocks when present),
    /// which is the form MP4 demuxers store in <c>esds</c> for AAC-LC / HE-AAC.
    /// </summary>
    public byte[] ToBytes()
    {
        var writer = new BitWriter();
        WriteAudioObjectType(writer, SbrPresent ? ExtensionAudioObjectType : AudioObjectType);
        if (SamplingFrequencyIndex == AacSampleRates.EscapeIndex)
        {
            writer.Write(AacSampleRates.EscapeIndex, 4);
            writer.Write((uint)SamplingFrequency, 24);
        }
        else
        {
            writer.Write(SamplingFrequencyIndex, 4);
        }
        writer.Write(ChannelConfiguration, 4);

        if (SbrPresent)
        {
            if (ExtensionSamplingFrequencyIndex == AacSampleRates.EscapeIndex)
            {
                writer.Write(AacSampleRates.EscapeIndex, 4);
                writer.Write((uint)ExtensionSamplingFrequency, 24);
            }
            else
            {
                writer.Write(ExtensionSamplingFrequencyIndex, 4);
            }
            WriteAudioObjectType(writer, AudioObjectType);
        }

        return writer.ToArray();
    }

    private static int ReadAudioObjectType(ref BitReader reader)
    {
        if (reader.Remaining < 5) return 0;
        int aot = (int)reader.ReadBits(5);
        if (aot == 31)
        {
            if (reader.Remaining < 6) return 0;
            aot = 32 + (int)reader.ReadBits(6);
        }
        return aot;
    }

    private static void WriteAudioObjectType(BitWriter writer, int aot)
    {
        if (aot < 32)
        {
            writer.Write(aot, 5);
        }
        else
        {
            writer.Write(31, 5);
            writer.Write(aot - 32, 6);
        }
    }
}
