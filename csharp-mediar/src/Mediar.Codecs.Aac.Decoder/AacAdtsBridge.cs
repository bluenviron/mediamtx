namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Bridge between ADTS frame headers (ISO/IEC 13818-7 §6.2) and the MPEG-4
/// AudioSpecificConfig (ISO/IEC 14496-3 §1.6.2.1) layout that the AAC
/// decoder family expects. ADTS payloads embed their own AAC profile,
/// sample-rate-index, and channel-config fields; transcoding to MP4 or
/// initialising a decoder both require synthesising the 2- or 5-byte
/// AudioSpecificConfig blob from those three fields.
/// </summary>
public static class AacAdtsBridge
{
    /// <summary>
    /// Build an <see cref="AudioSpecificConfig"/> for an AAC stream that
    /// was advertised over ADTS. <paramref name="adtsProfile"/> is the
    /// 2-bit profile field (the audio object type is <c>profile + 1</c>).
    /// </summary>
    public static AudioSpecificConfig FromAdts(int adtsProfile, int samplingFrequencyIndex, int channelConfiguration)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(adtsProfile);
        ArgumentOutOfRangeException.ThrowIfGreaterThan(adtsProfile, 3);
        ArgumentOutOfRangeException.ThrowIfNegative(samplingFrequencyIndex);
        ArgumentOutOfRangeException.ThrowIfGreaterThan(samplingFrequencyIndex, AacSampleRates.EscapeIndex);
        ArgumentOutOfRangeException.ThrowIfNegative(channelConfiguration);
        ArgumentOutOfRangeException.ThrowIfGreaterThan(channelConfiguration, 7);

        int aot = adtsProfile + 1;
        int sampleRate = AacSampleRates.FromIndex(samplingFrequencyIndex);
        int channels = channelConfiguration == 0
            ? 0
            : AacChannelConfigurations.SpeakerCount(channelConfiguration);

        return new AudioSpecificConfig
        {
            AudioObjectType = aot,
            SamplingFrequencyIndex = samplingFrequencyIndex,
            SamplingFrequency = sampleRate,
            ChannelConfiguration = channelConfiguration,
            ChannelCount = channels,
        };
    }

    /// <summary>
    /// Convenience helper that emits the AudioSpecificConfig byte payload
    /// directly. Equivalent to <c>FromAdts(...).ToBytes()</c>.
    /// </summary>
    public static byte[] BuildAudioSpecificConfig(int adtsProfile, int samplingFrequencyIndex, int channelConfiguration)
        => FromAdts(adtsProfile, samplingFrequencyIndex, channelConfiguration).ToBytes();
}
