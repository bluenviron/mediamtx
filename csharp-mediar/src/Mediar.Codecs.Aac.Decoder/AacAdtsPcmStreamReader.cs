namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One ADTS-decoded PCM frame, ready for handoff to a sink
/// (audio device, file writer, resampler, ...) without further
/// per-channel handling.
/// </summary>
/// <remarks>
/// <para>
/// <see cref="Samples"/> is interleaved
/// <c>[s0_ch0, s0_ch1, ..., s0_chN-1, s1_ch0, ...]</c>; channel
/// order matches the underlying
/// <see cref="AacAdtsStreamReader.CurrentSpeakers"/> snapshot.
/// </para>
/// <para>
/// One ADTS raw_data_block always produces 1024 samples per
/// channel (long window) or 8x128=1024 samples per channel
/// (EightShort). The exact count is reported by
/// <see cref="SamplesPerChannel"/>.
/// </para>
/// </remarks>
public sealed record AacPcmFrame
{
    /// <summary>Interleaved PCM samples; length = ChannelCount * SamplesPerChannel.</summary>
    public required float[] Samples { get; init; }

    /// <summary>Number of channels in the interleaved layout.</summary>
    public required int ChannelCount { get; init; }

    /// <summary>Number of samples per channel produced for this frame.</summary>
    public required int SamplesPerChannel { get; init; }

    /// <summary>Sample rate in Hz, taken from the ADTS header active when the frame decoded.</summary>
    public required int SampleRate { get; init; }

    /// <summary>Speaker order for the interleaved layout, in canonical mapping order.</summary>
    public required IReadOnlyList<AacSpeaker> Speakers { get; init; }
}

/// <summary>
/// High-level facade that combines
/// <see cref="AacAdtsStreamReader"/> with
/// <see cref="AacChannelInterleaver"/> to deliver each
/// raw_data_block as an interleaved-float
/// <see cref="AacPcmFrame"/> ready for output sinks.
/// </summary>
/// <remarks>
/// <para>
/// This is the recommended entry point for "open an .aac file and
/// hand PCM to my audio device" workflows. It owns its
/// <see cref="AacAdtsStreamReader"/>, transparently fans out
/// multi-block ADTS frames, and skips a leading ID3v2 tag.
/// </para>
/// <para>
/// All AAC decoder behaviour (state ownership, PNS PRNG factory,
/// protected-multi-block <see cref="NotSupportedException"/>,
/// <see cref="InvalidDataException"/> on lost sync) is forwarded
/// from the inner reader.
/// </para>
/// </remarks>
public sealed class AacAdtsPcmStreamReader : IDisposable
{
    private readonly AacAdtsStreamReader _reader;
    private bool _disposed;

    /// <summary>Construct a PCM stream reader over <paramref name="stream"/>.</summary>
    /// <param name="stream">Source stream; must be readable.</param>
    /// <param name="scaleFactorCodebook">Annex 4.A.2.1 scale-factor codebook.</param>
    /// <param name="spectralCodebooks">Spectral codebook lookup table.</param>
    /// <param name="leaveOpen">
    /// When <c>true</c>, <see cref="Dispose"/> leaves
    /// <paramref name="stream"/> open. Default is <c>false</c>.
    /// </param>
    /// <param name="initialBufferSize">Initial frame-buffer capacity.</param>
    /// <param name="prngFactory">Optional PNS PRNG factory.</param>
    public AacAdtsPcmStreamReader(
        Stream stream,
        AacHuffmanCodebook scaleFactorCodebook,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        bool leaveOpen = false,
        int initialBufferSize = AacAdtsStreamReader.DefaultBufferSize,
        Func<AacPnsRandom>? prngFactory = null)
    {
        _reader = new AacAdtsStreamReader(
            stream,
            scaleFactorCodebook,
            spectralCodebooks,
            leaveOpen,
            initialBufferSize,
            prngFactory);
    }

    /// <summary>Frame count successfully decoded since construction (or last <see cref="ResetState"/>).</summary>
    public long FrameCount => _reader.FrameCount;

    /// <summary>Currently-active config as derived from the last decoded ADTS header.</summary>
    public AudioSpecificConfig? CurrentConfig => _reader.CurrentConfig;

    /// <summary>Speaker ordering produced by the inner frame decoder.</summary>
    public IReadOnlyList<AacSpeaker>? CurrentSpeakers => _reader.CurrentSpeakers;

    /// <summary>Sample rate of the currently-active configuration in Hz.</summary>
    public int CurrentSampleRate => _reader.CurrentSampleRate;

    /// <summary>Channel count of the currently-active configuration.</summary>
    public int CurrentChannelCount => _reader.CurrentChannelCount;

    /// <summary>
    /// Read the next decoded raw_data_block from the stream,
    /// interleave its channels, and return an
    /// <see cref="AacPcmFrame"/>; or <c>null</c> on clean
    /// end-of-stream.
    /// </summary>
    public AacPcmFrame? ReadNextPcmFrame()
    {
        ThrowIfDisposed();

        var block = _reader.ReadNextFrame();
        if (block is null) return null;

        int channelCount = block.Channels.Count;
        if (channelCount == 0)
        {
            throw new InvalidDataException(
                "Decoded raw_data_block contained no channels — refusing to emit an empty PCM frame.");
        }
        int samplesPerChannel = block.Channels[0].Samples.Length;

        var interleaved = AacChannelInterleaver.Interleave(block);

        var speakers = new AacSpeaker[channelCount];
        for (int i = 0; i < channelCount; i++)
        {
            speakers[i] = block.Channels[i].Speaker;
        }

        return new AacPcmFrame
        {
            Samples = interleaved,
            ChannelCount = channelCount,
            SamplesPerChannel = samplesPerChannel,
            SampleRate = _reader.CurrentSampleRate,
            Speakers = speakers,
        };
    }

    /// <summary>
    /// Iterator-style wrapper that yields each PCM frame until
    /// end-of-stream.
    /// </summary>
    public IEnumerable<AacPcmFrame> ReadPcmFrames()
    {
        while (true)
        {
            var frame = ReadNextPcmFrame();
            if (frame is null) yield break;
            yield return frame;
        }
    }

    /// <summary>
    /// Read the next raw_data_block and return its interleaved
    /// samples as PCM-S16 (the audio-device-friendly integer
    /// format). Equivalent to
    /// <c>AacPcmFrameConverter.ToInt16Frame(ReadNextPcmFrame())</c>
    /// when the inner reader still has data.
    /// </summary>
    public AacPcmInt16Frame? ReadNextInt16Frame()
    {
        var floatFrame = ReadNextPcmFrame();
        return floatFrame is null ? null : AacPcmFrameConverter.ToInt16Frame(floatFrame);
    }

    /// <summary>
    /// Iterator-style wrapper over <see cref="ReadNextInt16Frame"/>.
    /// </summary>
    public IEnumerable<AacPcmInt16Frame> ReadInt16Frames()
    {
        while (true)
        {
            var frame = ReadNextInt16Frame();
            if (frame is null) yield break;
            yield return frame;
        }
    }

    /// <summary>
    /// Asynchronous counterpart of <see cref="ReadNextPcmFrame"/>.
    /// Awaits the inner reader's
    /// <see cref="AacAdtsStreamReader.ReadNextFrameAsync"/>, then
    /// interleaves the resulting raw_data_block without blocking
    /// the calling thread.
    /// </summary>
    public async Task<AacPcmFrame?> ReadNextPcmFrameAsync(
        CancellationToken cancellationToken = default)
    {
        ThrowIfDisposed();

        var block = await _reader.ReadNextFrameAsync(cancellationToken).ConfigureAwait(false);
        if (block is null) return null;

        int channelCount = block.Channels.Count;
        if (channelCount == 0)
        {
            throw new InvalidDataException(
                "Decoded raw_data_block contained no channels — refusing to emit an empty PCM frame.");
        }
        int samplesPerChannel = block.Channels[0].Samples.Length;

        var interleaved = AacChannelInterleaver.Interleave(block);

        var speakers = new AacSpeaker[channelCount];
        for (int i = 0; i < channelCount; i++)
        {
            speakers[i] = block.Channels[i].Speaker;
        }

        return new AacPcmFrame
        {
            Samples = interleaved,
            ChannelCount = channelCount,
            SamplesPerChannel = samplesPerChannel,
            SampleRate = _reader.CurrentSampleRate,
            Speakers = speakers,
        };
    }

    /// <summary>
    /// Asynchronous counterpart of <see cref="ReadPcmFrames"/>.
    /// </summary>
    public async IAsyncEnumerable<AacPcmFrame> ReadPcmFramesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        while (true)
        {
            var frame = await ReadNextPcmFrameAsync(cancellationToken).ConfigureAwait(false);
            if (frame is null) yield break;
            yield return frame;
        }
    }

    /// <summary>
    /// Asynchronous counterpart of <see cref="ReadNextInt16Frame"/>.
    /// </summary>
    public async Task<AacPcmInt16Frame?> ReadNextInt16FrameAsync(
        CancellationToken cancellationToken = default)
    {
        var floatFrame = await ReadNextPcmFrameAsync(cancellationToken).ConfigureAwait(false);
        return floatFrame is null ? null : AacPcmFrameConverter.ToInt16Frame(floatFrame);
    }

    /// <summary>
    /// Asynchronous counterpart of <see cref="ReadInt16Frames"/>.
    /// </summary>
    public async IAsyncEnumerable<AacPcmInt16Frame> ReadInt16FramesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        while (true)
        {
            var frame = await ReadNextInt16FrameAsync(cancellationToken).ConfigureAwait(false);
            if (frame is null) yield break;
            yield return frame;
        }
    }

    /// <summary>
    /// Drop the underlying decoder state and clear any buffered
    /// bytes. Use after seeking the underlying stream so the next
    /// read resynchronises and rebuilds filterbank state.
    /// </summary>
    public void ResetState()
    {
        ThrowIfDisposed();
        _reader.ResetState();
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        _reader.Dispose();
    }

    private void ThrowIfDisposed()
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
    }
}
