using AacBitReader = Mediar.Codecs.Aac.Decoder.BitReader;
using AacBitWriter = Mediar.Codecs.Aac.Decoder.BitWriter;
using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class AudioSpecificConfigTests
{
    [Fact]
    public void AacSampleRates_KnownRatesRoundTrip()
    {
        Assert.Equal(96_000, AacSampleRates.FromIndex(0));
        Assert.Equal(48_000, AacSampleRates.FromIndex(3));
        Assert.Equal(44_100, AacSampleRates.FromIndex(4));
        Assert.Equal(8_000, AacSampleRates.FromIndex(11));
        Assert.Equal(0, AacSampleRates.FromIndex(13));
        Assert.Equal(0, AacSampleRates.FromIndex(99));

        Assert.Equal(3, AacSampleRates.ToIndex(48_000));
        Assert.Equal(4, AacSampleRates.ToIndex(44_100));
        Assert.Equal(AacSampleRates.EscapeIndex, AacSampleRates.ToIndex(123_456));
    }

    [Fact]
    public void AacChannelConfigurations_KnownSpeakerCounts()
    {
        Assert.Equal(0, AacChannelConfigurations.SpeakerCount(0));
        Assert.Equal(1, AacChannelConfigurations.SpeakerCount(1));
        Assert.Equal(2, AacChannelConfigurations.SpeakerCount(2));
        Assert.Equal(6, AacChannelConfigurations.SpeakerCount(6));
        Assert.Equal(8, AacChannelConfigurations.SpeakerCount(7));
        Assert.Equal(0, AacChannelConfigurations.SpeakerCount(8));
    }

    [Fact]
    public void TryParse_AacLc_44100_Stereo()
    {
        // AOT=2 (LC), sfIndex=4 (44100), channels=2 â†’ 0x12, 0x10
        byte[] bytes = [0x12, 0x10];
        Assert.True(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.NotNull(cfg);
        Assert.Equal(2, cfg!.AudioObjectType);
        Assert.Equal(AacAudioObjectType.AacLc, cfg.ObjectTypeEnum);
        Assert.Equal(4, cfg.SamplingFrequencyIndex);
        Assert.Equal(44_100, cfg.SamplingFrequency);
        Assert.Equal(2, cfg.ChannelConfiguration);
        Assert.Equal(2, cfg.ChannelCount);
        Assert.False(cfg.SbrPresent);
        Assert.False(cfg.PsPresent);
    }

    [Fact]
    public void TryParse_AacLc_48000_Mono()
    {
        // AOT=2, sfIndex=3 (48000), channels=1 â†’ 0001 0 0011 0001 (3 bits padding)
        byte[] bytes = [0b00010_001, 0b1_0001_000];
        Assert.True(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.NotNull(cfg);
        Assert.Equal(2, cfg!.AudioObjectType);
        Assert.Equal(48_000, cfg.SamplingFrequency);
        Assert.Equal(1, cfg.ChannelConfiguration);
        Assert.Equal(1, cfg.ChannelCount);
    }

    [Fact]
    public void TryParse_Heaac_ExplicitSbr_Surfaces_ExtensionFields()
    {
        // Explicit SBR signalling: AOT=5 (SBR), sfIndex=8 (16000), channels=2,
        // extension sfIndex=4 (44100), extension AOT=2 (AAC-LC).
        // Bit layout: 00101 1000 0010 0100 00010
        var writer = new AacBitWriter();
        writer.Write(5, 5); writer.Write(8, 4); writer.Write(2, 4);
        writer.Write(4, 4); writer.Write(2, 5);
        byte[] bytes = writer.ToArray();

        Assert.True(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.NotNull(cfg);
        Assert.Equal(2, cfg!.AudioObjectType);
        Assert.Equal(5, cfg.ExtensionAudioObjectType);
        Assert.True(cfg.SbrPresent);
        Assert.False(cfg.PsPresent);
        Assert.Equal(16_000, cfg.SamplingFrequency);
        Assert.Equal(4, cfg.ExtensionSamplingFrequencyIndex);
        Assert.Equal(44_100, cfg.ExtensionSamplingFrequency);
        Assert.Equal(2, cfg.ChannelConfiguration);
    }

    [Fact]
    public void TryParse_Heaacv2_ExplicitPs_SetsPsFlag()
    {
        var writer = new AacBitWriter();
        writer.Write(29, 5); // AOT=PS
        writer.Write(8, 4);  // sfIndex=8 (16000)
        writer.Write(2, 4);  // channels=2
        writer.Write(4, 4);  // ext sfIndex=4 (44100)
        writer.Write(2, 5);  // AAC-LC core
        byte[] bytes = writer.ToArray();

        Assert.True(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.NotNull(cfg);
        Assert.True(cfg!.SbrPresent);
        Assert.True(cfg.PsPresent);
        Assert.Equal(2, cfg.AudioObjectType);
    }

    [Fact]
    public void TryParse_InlineSampleRate_HandlesEscapeIndex()
    {
        var writer = new AacBitWriter();
        writer.Write(2, 5);            // AOT=AAC-LC
        writer.Write(AacSampleRates.EscapeIndex, 4);
        writer.Write(96_001u, 24);     // inline frequency
        writer.Write(2, 4);            // channels=2
        byte[] bytes = writer.ToArray();

        Assert.True(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.NotNull(cfg);
        Assert.Equal(AacSampleRates.EscapeIndex, cfg!.SamplingFrequencyIndex);
        Assert.Equal(96_001, cfg.SamplingFrequency);
        Assert.Equal(2, cfg.ChannelCount);
    }

    [Fact]
    public void TryParse_ExtendedAot_UsesEscape()
    {
        var writer = new AacBitWriter();
        writer.Write(31, 5); writer.Write(7, 6); // AOT = 32 + 7 = 39
        writer.Write(4, 4);
        writer.Write(2, 4);
        byte[] bytes = writer.ToArray();

        Assert.True(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.NotNull(cfg);
        Assert.Equal(39, cfg!.AudioObjectType);
        Assert.Equal(AacAudioObjectType.Null, cfg.ObjectTypeEnum); // outside 0..31 enum range
        Assert.Equal(44_100, cfg.SamplingFrequency);
    }

    [Fact]
    public void TryParse_Rejects_AotZero()
    {
        var writer = new AacBitWriter();
        writer.Write(0, 5); writer.Write(4, 4); writer.Write(2, 4);
        Assert.False(AudioSpecificConfig.TryParse(writer.ToArray(), out var cfg));
        Assert.Null(cfg);
    }

    [Fact]
    public void TryParse_Rejects_ReservedSampleRateIndex()
    {
        var writer = new AacBitWriter();
        writer.Write(2, 5); writer.Write(13, 4); writer.Write(2, 4);
        Assert.False(AudioSpecificConfig.TryParse(writer.ToArray(), out var cfg));
        Assert.Null(cfg);
    }

    [Fact]
    public void TryParse_Rejects_TruncatedInput()
    {
        byte[] bytes = [0x12];
        Assert.False(AudioSpecificConfig.TryParse(bytes, out var cfg));
        Assert.Null(cfg);
    }

    [Fact]
    public void TryParse_Rejects_EmptyInput()
    {
        Assert.False(AudioSpecificConfig.TryParse([], out var cfg));
        Assert.Null(cfg);
    }

    [Fact]
    public void ToBytes_RoundTripsAacLc()
    {
        var cfg = new AudioSpecificConfig
        {
            AudioObjectType = 2,
            SamplingFrequencyIndex = 4,
            SamplingFrequency = 44_100,
            ChannelConfiguration = 2,
            ChannelCount = 2,
        };
        byte[] bytes = cfg.ToBytes();
        Assert.True(AudioSpecificConfig.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);
        Assert.Equal(cfg.AudioObjectType, parsed!.AudioObjectType);
        Assert.Equal(cfg.SamplingFrequency, parsed.SamplingFrequency);
        Assert.Equal(cfg.ChannelConfiguration, parsed.ChannelConfiguration);
    }

    [Fact]
    public void ToBytes_RoundTripsInlineSampleRate()
    {
        var cfg = new AudioSpecificConfig
        {
            AudioObjectType = 2,
            SamplingFrequencyIndex = AacSampleRates.EscapeIndex,
            SamplingFrequency = 22_222,
            ChannelConfiguration = 1,
            ChannelCount = 1,
        };
        byte[] bytes = cfg.ToBytes();
        Assert.True(AudioSpecificConfig.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);
        Assert.Equal(22_222, parsed!.SamplingFrequency);
        Assert.Equal(AacSampleRates.EscapeIndex, parsed.SamplingFrequencyIndex);
        Assert.Equal(1, parsed.ChannelConfiguration);
    }

    [Fact]
    public void ToBytes_RoundTripsExplicitSbr()
    {
        var cfg = new AudioSpecificConfig
        {
            AudioObjectType = 2,
            SamplingFrequencyIndex = 8,
            SamplingFrequency = 16_000,
            ChannelConfiguration = 2,
            ChannelCount = 2,
            SbrPresent = true,
            ExtensionAudioObjectType = 5,
            ExtensionSamplingFrequencyIndex = 4,
            ExtensionSamplingFrequency = 44_100,
        };
        byte[] bytes = cfg.ToBytes();
        Assert.True(AudioSpecificConfig.TryParse(bytes, out var parsed));
        Assert.NotNull(parsed);
        Assert.True(parsed!.SbrPresent);
        Assert.Equal(2, parsed.AudioObjectType);
        Assert.Equal(5, parsed.ExtensionAudioObjectType);
        Assert.Equal(44_100, parsed.ExtensionSamplingFrequency);
    }

    [Fact]
    public void AacAdtsBridge_BuildsLc44100Stereo()
    {
        // profile=1 (AAC-LC), sfIndex=4 (44100), channels=2
        byte[] asc = AacAdtsBridge.BuildAudioSpecificConfig(1, 4, 2);
        Assert.Equal([0x12, 0x10], asc);
    }

    [Fact]
    public void AacAdtsBridge_FromAdts_PopulatesAllFields()
    {
        var cfg = AacAdtsBridge.FromAdts(adtsProfile: 1, samplingFrequencyIndex: 3, channelConfiguration: 6);
        Assert.Equal(2, cfg.AudioObjectType);
        Assert.Equal(48_000, cfg.SamplingFrequency);
        Assert.Equal(6, cfg.ChannelConfiguration);
        Assert.Equal(6, cfg.ChannelCount);
        Assert.False(cfg.SbrPresent);
    }

    [Fact]
    public void AacAdtsBridge_Rejects_OutOfRangeProfile()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => AacAdtsBridge.FromAdts(adtsProfile: 4, 4, 2));
    }

    [Fact]
    public void AacAdtsBridge_Rejects_OutOfRangeChannelConfig()
    {
        Assert.Throws<ArgumentOutOfRangeException>(() => AacAdtsBridge.FromAdts(adtsProfile: 1, 4, channelConfiguration: 8));
    }

    [Fact]
    public void BitReader_ReadsAcrossByteBoundaries()
    {
        byte[] bytes = [0b10110011, 0b01010000];
        var reader = new AacBitReader(bytes);
        Assert.Equal(0b1011u, reader.ReadBits(4));
        Assert.Equal(0b0011010u, reader.ReadBits(7));
        Assert.Equal(0b10000u, reader.ReadBits(5));
    }

    [Fact]
    public void BitReader_ThrowsOnUnderflow()
    {
        Assert.Throws<EndOfStreamException>(static () =>
        {
            byte[] bytes = [0x00];
            var reader = new AacBitReader(bytes);
            reader.ReadBits(8);
            reader.ReadBits(1);
        });
    }

    [Fact]
    public void BitWriter_RoundTripsThroughReader()
    {
        var writer = new AacBitWriter();
        writer.Write(0b101u, 3);
        writer.Write(0b1111_0000u, 8);
        writer.Write(0b01u, 2);
        byte[] bytes = writer.ToArray();
        var reader = new AacBitReader(bytes);
        Assert.Equal(0b101u, reader.ReadBits(3));
        Assert.Equal(0b1111_0000u, reader.ReadBits(8));
        Assert.Equal(0b01u, reader.ReadBits(2));
    }
}
