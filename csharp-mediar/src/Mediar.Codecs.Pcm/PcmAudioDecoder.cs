using System.Buffers;

namespace Mediar.Codecs.Pcm;

/// <summary>
/// <see cref="IAudioDecoder"/> facade over <see cref="PcmConverter"/> that
/// turns interleaved PCM byte payloads into normalized float
/// <see cref="DecodedAudioFrame"/>s.
/// </summary>
public sealed class PcmAudioDecoder : IAudioDecoder
{
    /// <inheritdoc/>
    public CodecId Codec { get; }

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>Create a PCM decoder for the given codec parameters.</summary>
    public PcmAudioDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec.Kind() != StreamKind.Audio)
        {
            throw new ArgumentException("PcmAudioDecoder only handles audio codecs.", nameof(parameters));
        }
        Codec = parameters.Codec;
        Parameters = parameters;
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;
        int channels = Math.Max(1, Parameters.Channels);
        int bytesPerSample = PcmConverter.BytesPerSample(Codec);
        int totalSamples = encoded.Length / bytesPerSample;
        if (totalSamples == 0) return default;
        int samplesPerChannel = totalSamples / channels;

        var owner = MemoryPool<float>.Shared.Rent(totalSamples);
        var floats = owner.Memory.Span[..totalSamples];
        PcmConverter.ToFloat(Codec, encoded, floats);

        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = Parameters.SampleRate,
            SamplesPerChannel = samplesPerChannel,
            Pts = pts,
            Samples = owner.Memory[..totalSamples],
            Owner = owner,
        };
    }

    /// <inheritdoc/>
    public void Reset() { /* stateless */ }

    /// <inheritdoc/>
    public void Dispose() { /* no resources */ }
}

/// <summary>Factory that creates <see cref="PcmAudioDecoder"/> instances.</summary>
public sealed class PcmAudioDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) =>
        codec is CodecId.PcmS16Le or CodecId.PcmS16Be or CodecId.PcmS24Le
        or CodecId.PcmS32Le or CodecId.PcmF32Le;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new PcmAudioDecoder(parameters);
}
