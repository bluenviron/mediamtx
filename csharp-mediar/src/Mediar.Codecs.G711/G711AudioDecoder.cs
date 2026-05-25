using System.Buffers;

namespace Mediar.Codecs.G711;

/// <summary>
/// <see cref="IAudioDecoder"/> implementation for ITU-T G.711 µ-law and A-law.
/// G.711 is one byte in, one 16-bit linear PCM sample out — there is no
/// inter-sample state to manage.
/// </summary>
public sealed class G711AudioDecoder : IAudioDecoder
{
    /// <inheritdoc/>
    public CodecId Codec { get; }

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>Create a G.711 decoder for the given codec parameters.</summary>
    public G711AudioDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec is not (CodecId.G711MuLaw or CodecId.G711ALaw))
        {
            throw new ArgumentException($"G711AudioDecoder does not support {parameters.Codec}.", nameof(parameters));
        }
        Codec = parameters.Codec;
        Parameters = parameters;
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;
        int channels = Parameters.Channels <= 0 ? 1 : Parameters.Channels;
        int totalSamples = encoded.Length;
        int samplesPerChannel = totalSamples / channels;
        if (samplesPerChannel == 0) return default;

        var owner = MemoryPool<float>.Shared.Rent(totalSamples);
        var floats = owner.Memory.Span[..totalSamples];

        if (Codec == CodecId.G711MuLaw)
            G711.DecodeMuLaw(encoded, floats);
        else
            G711.DecodeALaw(encoded, floats);

        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = Parameters.SampleRate <= 0 ? 8000 : Parameters.SampleRate,
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

/// <summary>Factory that creates <see cref="G711AudioDecoder"/> instances.</summary>
public sealed class G711AudioDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec is CodecId.G711MuLaw or CodecId.G711ALaw;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new G711AudioDecoder(parameters);
}
