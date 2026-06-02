namespace Mediar.Codecs.Opus.Decoder;

/// <summary>
/// Factory that produces <see cref="OpusDecoder"/> instances for
/// <see cref="CodecId.Opus"/>. Register an instance with
/// <see cref="DecoderRegistry"/> so the rest of Mediar (probe + demux +
/// transmux) can resolve Opus tracks transparently.
/// </summary>
public sealed class OpusDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec == CodecId.Opus;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        return new OpusDecoder(parameters);
    }
}
