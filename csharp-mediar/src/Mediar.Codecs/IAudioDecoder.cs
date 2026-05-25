using System.Buffers;

namespace Mediar.Codecs;

/// <summary>
/// Decoded audio frame. <see cref="Samples"/> is a normalized float buffer in
/// <c>[-1.0, 1.0]</c>. Layout is interleaved: <c>L0 R0 L1 R1 ...</c> for stereo.
/// The buffer is rented from an <see cref="ArrayPool{T}"/> or
/// <see cref="MemoryPool{T}"/>; the caller must dispose <see cref="Owner"/>
/// when done with the frame.
/// </summary>
public readonly struct DecodedAudioFrame : IDisposable
{
    /// <summary>Number of channels (1 = mono, 2 = stereo, ...).</summary>
    public int Channels { get; init; }

    /// <summary>Samples per second per channel.</summary>
    public int SampleRate { get; init; }

    /// <summary>Number of samples PER CHANNEL in this frame (frame length).</summary>
    public int SamplesPerChannel { get; init; }

    /// <summary>Presentation timestamp in track time-base units, or -1 if unknown.</summary>
    public long Pts { get; init; }

    /// <summary>Interleaved normalized float samples; length = <see cref="Channels"/> * <see cref="SamplesPerChannel"/>.</summary>
    public ReadOnlyMemory<float> Samples { get; init; }

    /// <summary>Pool-rented owner; dispose when frame consumption is complete.</summary>
    public IDisposable? Owner { get; init; }

    /// <summary>Release the underlying pooled buffer (if any).</summary>
    public void Dispose() => Owner?.Dispose();
}

/// <summary>
/// Streaming audio decoder. Implementations consume compressed encoded packets
/// (one <see cref="MediaSample"/> per call) and emit zero or more decoded
/// frames. Decoders are not thread-safe; each track needs its own instance.
/// </summary>
public interface IAudioDecoder : IDisposable
{
    /// <summary>Codec identifier this decoder implements.</summary>
    CodecId Codec { get; }

    /// <summary>Codec parameters this decoder was initialized with.</summary>
    AudioCodecParameters Parameters { get; }

    /// <summary>
    /// Decode one encoded packet and return the decoded audio frame.
    /// Some codecs (Vorbis, Opus) need priming packets — for those, the first
    /// few calls may return <c>default</c> (an empty frame). End-of-stream is
    /// signalled by passing <paramref name="encoded"/> = empty.
    /// </summary>
    DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts);

    /// <summary>Reset internal decoder state (after a seek).</summary>
    void Reset();
}

/// <summary>
/// Factory that knows how to instantiate decoders for one or more codecs.
/// Used by <see cref="DecoderRegistry"/> so callers can register their own
/// decoders for codecs Mediar cannot ship (e.g. AAC, H.264).
/// </summary>
public interface IAudioDecoderFactory
{
    /// <summary>True if this factory can instantiate a decoder for <paramref name="codec"/>.</summary>
    bool Supports(CodecId codec);

    /// <summary>Create a decoder for the given codec parameters.</summary>
    IAudioDecoder Create(AudioCodecParameters parameters);
}
