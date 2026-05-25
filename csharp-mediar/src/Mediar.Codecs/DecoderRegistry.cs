namespace Mediar.Codecs;

/// <summary>
/// Lookup of registered decoder factories per <see cref="CodecId"/>. Callers
/// can register their own factory for codecs Mediar does not ship (AAC, H.264,
/// HEVC, AV1) and the rest of the Mediar pipeline will pick them up
/// automatically.
/// </summary>
public sealed class DecoderRegistry
{
    private readonly List<IAudioDecoderFactory> _audio = new();
    private readonly List<IVideoDecoderFactory> _video = new();

    /// <summary>The default registry; built-in audio decoders register themselves here on first use.</summary>
    public static DecoderRegistry Default { get; } = new();

    /// <summary>Register an audio decoder factory.</summary>
    public void Register(IAudioDecoderFactory factory)
    {
        ArgumentNullException.ThrowIfNull(factory);
        _audio.Add(factory);
    }

    /// <summary>Register a video decoder factory.</summary>
    public void Register(IVideoDecoderFactory factory)
    {
        ArgumentNullException.ThrowIfNull(factory);
        _video.Add(factory);
    }

    /// <summary>Try to create an audio decoder for the given parameters.</summary>
    public bool TryCreate(AudioCodecParameters parameters, out IAudioDecoder decoder)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        foreach (var f in _audio)
        {
            if (f.Supports(parameters.Codec))
            {
                decoder = f.Create(parameters);
                return true;
            }
        }
        decoder = null!;
        return false;
    }

    /// <summary>Try to create a video decoder for the given parameters.</summary>
    public bool TryCreate(VideoCodecParameters parameters, out IVideoDecoder decoder)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        foreach (var f in _video)
        {
            if (f.Supports(parameters.Codec))
            {
                decoder = f.Create(parameters);
                return true;
            }
        }
        decoder = null!;
        return false;
    }
}
