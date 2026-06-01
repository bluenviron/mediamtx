namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// High-level streaming AAC frame decoder: combines raw_data_block
/// parsing with the per-speaker PCM dispatcher into a single
/// stateful entry point. Holds the per-speaker filterbank state
/// across consecutive frame calls so the spec-mandated 50 % MDCT
/// overlap-add is preserved.
/// </summary>
/// <remarks>
/// <para>
/// One instance per AAC elementary stream. Construct from a parsed
/// <see cref="AudioSpecificConfig"/> plus the caller-supplied
/// Huffman codebooks (scale-factor + spectral). Call
/// <see cref="DecodeFrame(ReadOnlySpan{byte})"/> once per
/// raw_data_block payload.
/// </para>
/// <para>
/// Only channelConfiguration values 1..7 are supported by this
/// facade; PCE-described streams (channelConfiguration == 0) must
/// use <see cref="AacPceRawDataBlockDecoder"/> directly with the
/// parsed <see cref="AacProgramConfigurationElement"/>.
/// </para>
/// <para>
/// The default PRNG factory seeds a fresh PNS sequence per
/// channel-instance per frame call; provide a custom factory to
/// override (e.g., for deterministic test vectors).
/// </para>
/// </remarks>
public sealed class AacFrameDecoder
{
    private readonly AudioSpecificConfig _config;
    private readonly AacRawDataBlockContext _context;
    private readonly Func<AacPnsRandom> _prngFactory;
    private readonly Dictionary<AacSpeaker, AacSynthesisFilterbank> _filterbanks;
    private readonly bool _applyTns;
    private readonly AacAudioObjectType _aotForTns;

    /// <summary>Speaker ordering produced by <see cref="DecodeFrame(ReadOnlySpan{byte})"/>.</summary>
    public IReadOnlyList<AacSpeaker> Speakers { get; }

    /// <summary>Resolved AAC core sample rate (Hz) from the supplied config.</summary>
    public int SampleRate => _context.SampleRate;

    /// <summary>Constructed <see cref="AudioSpecificConfig"/> used by this decoder.</summary>
    public AudioSpecificConfig Config => _config;

    /// <summary>
    /// Build a frame decoder for a constant-config AAC stream.
    /// </summary>
    /// <param name="config">Parsed AudioSpecificConfig from an esds box or ADTS bridge.</param>
    /// <param name="scaleFactorCodebook">Annex 4.A.2.1 scale-factor codebook.</param>
    /// <param name="spectralCodebooks">Spectral codebook lookup table.</param>
    /// <param name="prngFactory">
    /// Optional PNS PRNG factory. When omitted, each invocation
    /// returns a fresh <c>new AacPnsRandom(seed: 1u)</c>; this is
    /// deterministic but couples PNS noise across frame boundaries.
    /// Real decoders should pass a stateful factory.
    /// </param>
    /// <exception cref="ArgumentNullException">Any required argument is null.</exception>
    /// <exception cref="ArgumentException">
    /// <paramref name="config"/> has a channelConfiguration outside [1, 7]
    /// (PCE-described streams must use <see cref="AacPceRawDataBlockDecoder"/>),
    /// or its sample rate does not match a supported AAC core rate.
    /// </exception>
    public AacFrameDecoder(
        AudioSpecificConfig config,
        AacHuffmanCodebook scaleFactorCodebook,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        Func<AacPnsRandom>? prngFactory = null)
    {
        ArgumentNullException.ThrowIfNull(config);
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        ArgumentNullException.ThrowIfNull(spectralCodebooks);

        if (config.ChannelConfiguration is < 1 or > 7)
        {
            throw new ArgumentException(
                $"AacFrameDecoder supports channelConfiguration 1..7 only; got {config.ChannelConfiguration}. " +
                $"PCE-described streams (channelConfiguration == 0) must use AacPceRawDataBlockDecoder.",
                nameof(config));
        }

        _config = config;
        _context = AacRawDataBlockContext.FromAudioSpecificConfig(config, scaleFactorCodebook, spectralCodebooks);
        _prngFactory = prngFactory ?? DefaultPrngFactory;
        _filterbanks = AacRawDataBlockDecoder.CreateFilterbanks(config.ChannelConfiguration);
        Speakers = AacRawDataBlockDecoder.GetExpectedSpeakers(config.ChannelConfiguration);

        var aot = config.ObjectTypeEnum;
        // TNS is currently wired for AAC-Main / AAC-LC / AAC-LTP /
        // AAC-SSR via the long-window AOT-aware composer overloads.
        // Any other AOT skips TNS (matches the base overload).
        _applyTns = aot is AacAudioObjectType.AacMain
            or AacAudioObjectType.AacLc
            or AacAudioObjectType.AacLtp
            or AacAudioObjectType.AacSsr;
        _aotForTns = _applyTns ? aot : default;
    }

    /// <summary>
    /// Decode one AAC raw_data_block to per-speaker PCM. Filterbank
    /// overlap-add state is carried across consecutive calls.
    /// </summary>
    /// <param name="rawDataBlockBytes">
    /// One complete <c>raw_data_block()</c> payload (without the
    /// outer ADTS / LATM framing). For ADTS, this is the bytes
    /// between the header and the end of the frame.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="rawDataBlockBytes"/> is empty.
    /// </exception>
    /// <exception cref="InvalidDataException">
    /// The block fails to parse, or terminates without reaching the
    /// END sentinel (which means the payload was truncated or its
    /// element layout is invalid for the configured channel count).
    /// </exception>
    public AacDecodedRawDataBlock DecodeFrame(ReadOnlySpan<byte> rawDataBlockBytes)
    {
        var decoded = DecodeRawDataBlock(rawDataBlockBytes, out _);
        return decoded;
    }

    /// <summary>
    /// Decode the next AAC <c>raw_data_block</c> starting at bit
    /// position 0 of <paramref name="bytes"/> and report how many
    /// bits were consumed (excluding any trailing
    /// <c>byte_alignment()</c>). Filterbank overlap-add state is
    /// carried across consecutive calls.
    /// </summary>
    /// <remarks>
    /// Lower-level companion of
    /// <see cref="DecodeFrame(ReadOnlySpan{byte})"/> used by
    /// streaming containers that pack multiple raw_data_blocks into
    /// one outer frame (e.g. multi-block ADTS,
    /// <c>number_of_raw_data_blocks_in_frame &gt; 0</c>). Each block
    /// is byte-aligned at its end inside the outer frame, so
    /// callers should advance their byte cursor by
    /// <c>(bitsConsumed + 7) / 8</c> after every call.
    /// </remarks>
    /// <param name="bytes">
    /// Buffer that contains at least one full <c>raw_data_block()</c>
    /// terminated by an END (id=7) sentinel. Trailing bytes are not
    /// inspected — they belong to the next block (or padding).
    /// </param>
    /// <param name="bitsConsumed">
    /// Bit offset of the bit just after the END sentinel. The next
    /// raw_data_block in a multi-block container starts at the
    /// next byte boundary (round up by 7).
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="bytes"/> is empty.
    /// </exception>
    /// <exception cref="InvalidDataException">
    /// The block fails to parse, or terminates without reaching the
    /// END sentinel (truncated or malformed payload).
    /// </exception>
    public AacDecodedRawDataBlock DecodeRawDataBlock(
        ReadOnlySpan<byte> bytes,
        out int bitsConsumed)
    {
        if (bytes.IsEmpty)
        {
            throw new ArgumentException("bytes is empty.", nameof(bytes));
        }

        if (!AacRawDataBlock.TryParse(bytes, _context, out var block) || block is null)
        {
            throw new InvalidDataException(
                "AAC raw_data_block failed to parse; payload is truncated, malformed, " +
                "or contains an element kind unsupported by this decoder.");
        }

        if (!block.TerminatedByEnd)
        {
            throw new InvalidDataException(
                "AAC raw_data_block did not reach the END (id=7) sentinel before the buffer ended.");
        }

        bitsConsumed = block.BitsConsumed;

        if (_applyTns)
        {
            return AacRawDataBlockDecoder.DecodeToSamples(
                block,
                _config.ChannelConfiguration,
                _context.SampleRate,
                _prngFactory,
                _aotForTns,
                _filterbanks);
        }

        return AacRawDataBlockDecoder.DecodeToSamples(
            block,
            _config.ChannelConfiguration,
            _context.SampleRate,
            _prngFactory,
            _filterbanks);
    }

    /// <summary>
    /// Reset the internal filterbank overlap-add state. Call after
    /// a seek or when starting a new logical stream so the next
    /// <see cref="DecodeFrame(ReadOnlySpan{byte})"/> begins with
    /// the spec-correct half-frame silent ramp-up rather than
    /// inheriting overlap from a prior frame.
    /// </summary>
    public void ResetState()
    {
        var fresh = AacRawDataBlockDecoder.CreateFilterbanks(_config.ChannelConfiguration);
        // Mutate the existing dictionary so external callers that
        // captured a reference (e.g., for diagnostics) keep seeing
        // the live filterbank set.
        _filterbanks.Clear();
        foreach (var kvp in fresh)
        {
            _filterbanks[kvp.Key] = kvp.Value;
        }
    }

    private static AacPnsRandom DefaultPrngFactory() => new(seed: 1u);
}
