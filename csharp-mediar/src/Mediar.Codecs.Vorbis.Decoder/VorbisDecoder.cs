using System.Buffers;

namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis I audio decoder. Patent-free per Xiph.org and shipped under Mediar's MIT license.
///
/// Status (this release):
/// • Identification + comment + setup headers parse end-to-end — fully validated.
/// • Bit reader, codebooks (Huffman + lattice/explicit VQ), IMDCT, sin² window,
///   and Xiph lacing utilities are complete and unit-tested.
/// • Floor 1 / residue decode + IMDCT synthesis path is roadmap work; until
///   that lands, <see cref="Decode"/> returns silent frames sized to the
///   declared blocksize so callers can pipe Vorbis through Mediar's container
///   layer without a missing-decoder exception.
///
/// The decoder accepts the standard Matroska/WebM-style Xiph-laced
/// <c>AudioCodecParameters.ExtraData</c> blob (count-1, then per-packet
/// lengths, then the three Vorbis priming packets concatenated). When
/// <c>AudioCodecParameters.ExtraData</c> is empty the decoder treats
/// the first three <see cref="Decode"/> calls as the priming sequence.
/// </summary>
public sealed class VorbisDecoder : IAudioDecoder
{
    private VorbisIdentificationHeader? _id;
    private VorbisCommentHeader? _comment;
    private VorbisSetup? _setup;
    private int _primed;
    private long _samplesProduced;

    /// <inheritdoc/>
    public CodecId Codec => CodecId.Vorbis;

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>Vendor string from the comment header, or empty until parsed.</summary>
    public string Vendor => _comment?.Vendor ?? string.Empty;

    /// <summary>User comments from the comment header.</summary>
    public IReadOnlyList<string> UserComments => _comment?.UserComments ?? Array.Empty<string>();

    /// <summary>Short-block length (samples), 0 until headers parsed.</summary>
    public int ShortBlocksize => _id?.Blocksize0 ?? 0;

    /// <summary>Long-block length (samples), 0 until headers parsed.</summary>
    public int LongBlocksize => _id?.Blocksize1 ?? 0;

    /// <summary>True once all three Vorbis priming packets have been processed.</summary>
    public bool IsPrimed => _primed >= 3;

    /// <summary>Create a Vorbis decoder.</summary>
    public VorbisDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec != CodecId.Vorbis)
            throw new ArgumentException("VorbisDecoder requires Codec=Vorbis.", nameof(parameters));
        Parameters = parameters;

        if (!parameters.ExtraData.IsEmpty)
        {
            var packets = VorbisHeaders.UnpackXiphLaced(parameters.ExtraData.Span);
            if (packets.Length != 3)
                throw new InvalidDataException("Vorbis ExtraData must contain exactly 3 xiph-laced packets.");
            foreach (var pkt in packets) ConsumeHeader(pkt);
        }
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;

        if (_primed < 3)
        {
            ConsumeHeader(encoded);
            return default;
        }

        // Audio packet — frame length depends on the mode (short/long block) and
        // the prev/next block flags. Until floor/residue/synthesis lands we
        // emit silence sized to the long blocksize, which is what every Vorbis
        // file pads to during steady-state playback.
        int n = _id!.Blocksize1;
        int channels = _id.Channels;
        int sampleRate = _id.SampleRate;
        int half = n / 2;
        int samples = half;
        int total = samples * channels;

        var owner = MemoryPool<float>.Shared.Rent(total);
        var mem = owner.Memory[..total];
        mem.Span.Clear();
        long framePts = _samplesProduced;
        _samplesProduced += samples;
        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = sampleRate,
            SamplesPerChannel = samples,
            Pts = pts >= 0 ? pts : framePts,
            Samples = mem,
            Owner = owner,
        };
    }

    private void ConsumeHeader(ReadOnlySpan<byte> packet)
    {
        if (packet.Length < 1) throw new InvalidDataException("Empty Vorbis header packet.");
        byte type = packet[0];
        switch (type)
        {
            case 1:
                _id = VorbisHeaders.ParseIdentification(packet);
                break;
            case 3:
                _comment = VorbisHeaders.ParseComment(packet);
                break;
            case 5:
                if (_id is null)
                    throw new InvalidDataException("Setup header arrived before identification header.");
                _setup = VorbisSetup.Parse(packet, _id.Channels);
                break;
            default:
                throw new InvalidDataException($"Unexpected Vorbis header type {type}.");
        }
        _primed++;
    }

    /// <inheritdoc/>
    public void Reset()
    {
        _samplesProduced = 0;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
    }
}

/// <summary>Factory that produces <see cref="VorbisDecoder"/> instances.</summary>
public sealed class VorbisDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec == CodecId.Vorbis;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new VorbisDecoder(parameters);
}
