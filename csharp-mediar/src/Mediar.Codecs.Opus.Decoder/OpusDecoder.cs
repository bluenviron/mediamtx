using System.Buffers;

namespace Mediar.Codecs.Opus.Decoder;

/// <summary>
/// Opus audio decoder (RFC 6716). <b>Phased delivery</b> — Phase 1 ships the
/// framing layer:
/// <list type="bullet">
///   <item><description>Parses the TOC byte and walks the per-frame packing for codes 0/1/2/3 (incl. padding + CBR/VBR code-3).</description></item>
///   <item><description>Initialises the range decoder over each frame's payload so the entropy state is observable to tests.</description></item>
///   <item><description>Emits a correctly shaped <see cref="DecodedAudioFrame"/> at 48 kHz with the right number of samples, channels and PTS — but the sample data is all zeros (silence) until subsequent phases land the actual CELT/SILK synthesis.</description></item>
/// </list>
///
/// <para>
/// This intentionally lets callers wire up the full Mediar pipeline now —
/// probe + demux + transmux through Opus tracks — without crashing or
/// producing garbage. As soon as Phase 2 (CELT) and Phase 3 (SILK) ship, the
/// silence is replaced with real decoded audio with no public API change.
/// </para>
/// </summary>
public sealed class OpusDecoder : IAudioDecoder
{
    private static readonly int[] OutputSampleRates = { 8_000, 12_000, 16_000, 24_000, 48_000 };

    private readonly OpusHead _head;
    private readonly bool _hasOpusHead;

    private readonly List<OpusFrameView> _frames = new(capacity: 4);
    private long _samplesProduced;

    /// <inheritdoc/>
    public CodecId Codec => CodecId.Opus;

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>
    /// Output sample rate. Opus always decodes internally at 48 kHz; this
    /// property reports the effective output rate after any future
    /// resampler (Phase 5). Phase 1 always returns 48 000.
    /// </summary>
    public int OutputSampleRate => 48_000;

    /// <summary>
    /// Decoded <see cref="OpusHead"/> from <c>ExtraData</c>, if it was
    /// supplied. Provides <c>PreSkip</c>, <c>OutputGain</c>, and channel-
    /// mapping fields that later phases will apply during synthesis.
    /// </summary>
    public OpusHead Head => _head;

    /// <summary>
    /// True when the constructor received a valid Ogg-form
    /// <c>OpusHead</c> in <c>AudioCodecParameters.ExtraData</c>.
    /// </summary>
    public bool HasHead => _hasOpusHead;

    /// <summary>
    /// Total number of decoded samples per channel produced by this
    /// decoder. Increments by the frame size whenever
    /// <see cref="Decode"/> consumes a packet.
    /// </summary>
    public long SamplesProduced => _samplesProduced;

    /// <summary>
    /// Create an Opus decoder for the given parameters. The decoder accepts
    /// either an empty <c>ExtraData</c> (CAF / live streams) or the
    /// canonical Ogg-form <c>OpusHead</c> bytes that
    /// <c>OggDemuxer</c> / <c>Mp4Demuxer</c> produce.
    /// </summary>
    public OpusDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec != CodecId.Opus)
            throw new ArgumentException("OpusDecoder requires Codec=Opus.", nameof(parameters));
        Parameters = parameters;

        if (!parameters.ExtraData.IsEmpty)
        {
            if (!OpusHead.TryReadOgg(parameters.ExtraData.Span, out _head))
            {
                throw new ArgumentException(
                    "AudioCodecParameters.ExtraData is non-empty but does not contain a valid Ogg-form OpusHead.",
                    nameof(parameters));
            }
            _hasOpusHead = true;
        }
        else
        {
            _head = default;
            _hasOpusHead = false;
        }
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;

        var toc = OpusToc.Parse(encoded[0]);
        OpusFramePacker.Unpack(encoded, toc, _frames);
        if (_frames.Count == 0)
        {
            // No frames in this packet — emit zero samples.
            return default;
        }

        // Walk every frame so that any structural error in the entropy stream
        // surfaces immediately. Phase 1 doesn't *use* the range decoder's
        // output, but constructing it validates that the per-frame bytes are
        // present and that the decoder's invariants hold (the constructor
        // already pulls in the first byte and normalises).
        foreach (var frame in _frames)
        {
            if (frame.Length > 0)
            {
                _ = new OpusRangeDecoder(encoded.Slice(frame.Offset, frame.Length));
            }
        }

        int channels = ResolveChannelCount(toc);
        int samplesPerChannel = toc.SamplesPerFrameAt48k * _frames.Count;
        int totalFloats = samplesPerChannel * channels;

        var owner = MemoryPool<float>.Shared.Rent(totalFloats);
        // Rent gives us a buffer >= requested; we slice down to exact size
        // and zero-fill — Phase 1 always emits silence.
        owner.Memory.Span.Slice(0, totalFloats).Clear();
        _samplesProduced += samplesPerChannel;

        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = OutputSampleRate,
            SamplesPerChannel = samplesPerChannel,
            Pts = pts,
            Samples = owner.Memory.Slice(0, totalFloats),
            Owner = owner,
        };
    }

    /// <inheritdoc/>
    public void Reset()
    {
        _frames.Clear();
        _samplesProduced = 0;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        // No unmanaged resources in Phase 1.
    }

    private int ResolveChannelCount(OpusToc toc)
    {
        int tocChannels = toc.IsStereo ? 2 : 1;
        if (_hasOpusHead)
        {
            // Multistream Opus uses ChannelMappingFamily != 0 and the
            // OpusHead's ChannelCount as the authoritative count (Phase 6
            // will wire that up). For mapping family 0, the OpusHead must
            // hold 1 or 2 channels — match it against the TOC stereo bit.
            if (_head.ChannelMappingFamily != 0)
            {
                return _head.ChannelCount;
            }
            // Family 0: a per-packet stereo bit may differ from the OpusHead
            // (rare but allowed for asymmetric content). Trust the TOC.
        }
        else if (Parameters.Channels > 0)
        {
            // No OpusHead: prefer the codec params if they specify
            // multi-channel, otherwise fall back to the TOC's 1/2.
            if (Parameters.Channels > 2) return Parameters.Channels;
        }
        return tocChannels;
    }
}
