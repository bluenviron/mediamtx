namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Streaming AAC decoder for ADTS-framed elementary streams
/// (ISO/IEC 13818-7 §6.2). Combines an inline ADTS header parser
/// with the per-stream <see cref="AacFrameDecoder"/> facade so a
/// caller can feed one ADTS frame at a time (header + raw_data_block
/// payload) and receive decoded per-speaker PCM.
/// </summary>
/// <remarks>
/// <para>
/// The decoder owns a lazily-constructed <see cref="AacFrameDecoder"/>;
/// it (re)builds it on the first frame and whenever a subsequent
/// header signals a different audio object type, sampling-frequency
/// index, or channel configuration. Filterbank overlap-add state is
/// preserved across consecutive frames with a matching configuration
/// — a config change forces a fresh underlying decoder.
/// </para>
/// <para>
/// Both single-block and multi-block ADTS frames are supported.
/// Single-block frames (<c>number_of_raw_data_blocks_in_frame == 0</c>)
/// can be processed with <see cref="DecodeFrame(ReadOnlySpan{byte})"/>
/// which returns the one decoded block. Multi-block frames must use
/// <see cref="DecodeBlocks(ReadOnlySpan{byte}, FrameSink)"/>, which
/// invokes the sink once per contained raw_data_block; calling
/// <see cref="DecodeFrame(ReadOnlySpan{byte})"/> on a multi-block
/// frame throws <see cref="NotSupportedException"/>.
/// </para>
/// <para>
/// CRC bytes, when present (protection_absent == 0), are skipped
/// without verification — callers that need CRC validation should
/// check the bytes themselves before calling. For protected
/// multi-block frames, the 16-bit per-block crc_check trailer
/// after each raw_data_block is also skipped, and the multi-block
/// raw_data_block_position[] pointers in the header are ignored
/// in favour of the byte-aligned cursor walk used by the
/// unprotected path.
/// </para>
/// <para>
/// PCE-described streams (channelConfig == 0) cannot be carried by
/// ADTS in the first place; the inline parser rejects them.
/// </para>
/// </remarks>
public sealed class AacAdtsFrameDecoder
{
    private readonly AacHuffmanCodebook _scaleFactorCodebook;
    private readonly IReadOnlyList<AacHuffmanCodebook?> _spectralCodebooks;
    private readonly Func<AacPnsRandom>? _prngFactory;

    private AacFrameDecoder? _inner;
    private AudioSpecificConfig? _config;

    /// <summary>
    /// Build a streaming ADTS decoder. The underlying
    /// <see cref="AacFrameDecoder"/> is created lazily on the first
    /// <see cref="DecodeFrame(ReadOnlySpan{byte})"/> call so that
    /// callers don't have to know the stream's format in advance.
    /// </summary>
    /// <param name="scaleFactorCodebook">Annex 4.A.2.1 scale-factor codebook.</param>
    /// <param name="spectralCodebooks">Spectral codebook lookup table.</param>
    /// <param name="prngFactory">
    /// Optional PNS PRNG factory passed through to the underlying
    /// <see cref="AacFrameDecoder"/>. When omitted, the default
    /// deterministic seed-1 factory is used.
    /// </param>
    public AacAdtsFrameDecoder(
        AacHuffmanCodebook scaleFactorCodebook,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        Func<AacPnsRandom>? prngFactory = null)
    {
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        ArgumentNullException.ThrowIfNull(spectralCodebooks);

        _scaleFactorCodebook = scaleFactorCodebook;
        _spectralCodebooks = spectralCodebooks;
        _prngFactory = prngFactory;
    }

    /// <summary>
    /// Total number of decoded <c>raw_data_block</c>s since
    /// construction (or since the last <see cref="ResetState"/>).
    /// For single-block ADTS streams this equals the number of ADTS
    /// frames; for multi-block streams it counts each contained
    /// block separately because each block represents one frame of
    /// PCM (1024 samples per channel).
    /// </summary>
    public long FrameCount { get; private set; }

    /// <summary>
    /// The <see cref="AudioSpecificConfig"/> synthesised from the
    /// most recently decoded ADTS header, or <c>null</c> before the
    /// first successful <see cref="DecodeFrame(ReadOnlySpan{byte})"/>.
    /// </summary>
    public AudioSpecificConfig? CurrentConfig => _config;

    /// <summary>
    /// Speaker ordering produced by the underlying frame decoder, or
    /// <c>null</c> before the first successful frame.
    /// </summary>
    public IReadOnlyList<AacSpeaker>? CurrentSpeakers => _inner?.Speakers;

    /// <summary>
    /// Sample rate of the currently-active configuration in Hz, or 0
    /// before the first frame.
    /// </summary>
    public int CurrentSampleRate => _inner?.SampleRate ?? 0;

    /// <summary>
    /// Channel count of the currently-active configuration, or 0
    /// before the first frame.
    /// </summary>
    public int CurrentChannelCount => _config?.ChannelCount ?? 0;

    /// <summary>
    /// Peek the <c>frame_length</c> field from an ADTS header
    /// without doing any decoding. Useful for streaming callers that
    /// need to slice the next frame out of a buffer before passing
    /// it to <see cref="DecodeFrame(ReadOnlySpan{byte})"/>.
    /// </summary>
    /// <param name="header">
    /// The first bytes of a potential ADTS frame; only the first 6
    /// are inspected so a 7-byte buffer is always sufficient.
    /// </param>
    /// <param name="frameLength">
    /// On success, the total frame length in bytes (including the
    /// 7- or 9-byte ADTS header).
    /// </param>
    public static bool TryParseFrameLength(ReadOnlySpan<byte> header, out int frameLength)
    {
        frameLength = 0;
        if (header.Length < 6) return false;

        // 12-bit syncword 0xFFF.
        if (header[0] != 0xFF || (header[1] & 0xF0) != 0xF0) return false;

        // Layer field must be 0.
        if ((header[1] & 0x06) != 0) return false;

        int length =
            ((header[3] & 0x03) << 11) |
            (header[4] << 3) |
            ((header[5] >> 5) & 0x07);

        if (length < 7) return false;
        frameLength = length;
        return true;
    }

    /// <summary>
    /// Decode one ADTS frame (header + raw_data_block payload) to
    /// per-speaker PCM. The first call builds the underlying
    /// <see cref="AacFrameDecoder"/>; subsequent calls reuse it if
    /// the header still describes the same AOT / sample rate /
    /// channel configuration, otherwise a fresh decoder is built.
    /// </summary>
    /// <param name="adtsFrame">
    /// Exactly one ADTS frame, i.e. <c>frame_length</c> bytes starting
    /// at the syncword. Use <see cref="TryParseFrameLength"/> first
    /// to slice the buffer.
    /// </param>
    /// <exception cref="ArgumentException">
    /// <paramref name="adtsFrame"/> is shorter than the declared
    /// frame length, or the header fails to parse.
    /// </exception>
    /// <exception cref="NotSupportedException">
    /// The header signals <c>number_of_raw_data_blocks_in_frame > 0</c>
    /// (multi-block frames are not yet supported by this facade).
    /// </exception>
    /// <exception cref="InvalidDataException">
    /// Forwarded from <see cref="AacFrameDecoder.DecodeFrame(ReadOnlySpan{byte})"/>
    /// when the embedded raw_data_block is malformed or truncated.
    /// </exception>
    public AacDecodedRawDataBlock DecodeFrame(ReadOnlySpan<byte> adtsFrame)
    {
        if (!TryParseAdtsHeader(adtsFrame, out var parsed))
        {
            throw new ArgumentException(
                "Input does not start with a valid ADTS header.",
                nameof(adtsFrame));
        }

        if (adtsFrame.Length < parsed.FrameLength)
        {
            throw new ArgumentException(
                $"ADTS frame buffer is shorter than the declared frame_length " +
                $"({adtsFrame.Length} < {parsed.FrameLength}).",
                nameof(adtsFrame));
        }

        if (parsed.RawDataBlockCount > 1)
        {
            throw new NotSupportedException(
                "ADTS frame has multiple raw_data_blocks " +
                $"(number_of_raw_data_blocks_in_frame = {parsed.RawDataBlockCount - 1}); " +
                "use DecodeBlocks(ReadOnlySpan<byte>, FrameSink) to receive each block.");
        }

        if (parsed.ChannelConfig is < 1 or > 7)
        {
            // ADTS cannot carry a PCE-described stream; channel_configuration == 0
            // would force an inline PCE which the spec disallows.
            throw new InvalidDataException(
                $"ADTS header declares unsupported channel_configuration {parsed.ChannelConfig}.");
        }

        EnsureFrameDecoder(parsed);

        int payloadOffset = parsed.HeaderSize;
        int payloadLength = parsed.FrameLength - parsed.HeaderSize;
        var payload = adtsFrame.Slice(payloadOffset, payloadLength);

        var block = _inner!.DecodeFrame(payload);
        FrameCount++;
        return block;
    }

    /// <summary>
    /// Sink callback invoked once per decoded raw_data_block by
    /// <see cref="DecodeFrames(ReadOnlySpan{byte}, FrameSink)"/> and
    /// <see cref="DecodeBlocks(ReadOnlySpan{byte}, FrameSink)"/>.
    /// </summary>
    /// <param name="block">The decoded raw_data_block.</param>
    public delegate void FrameSink(AacDecodedRawDataBlock block);

    /// <summary>
    /// Decode every raw_data_block inside one ADTS frame and invoke
    /// <paramref name="sink"/> for each one in order. Handles both
    /// single-block (<c>number_of_raw_data_blocks_in_frame == 0</c>)
    /// and multi-block frames. Each subsequent raw_data_block starts
    /// at the next byte boundary after the previous block's END
    /// sentinel.
    /// </summary>
    /// <param name="adtsFrame">
    /// Exactly one ADTS frame, i.e. <c>frame_length</c> bytes
    /// starting at the syncword.
    /// </param>
    /// <param name="sink">Invoked once per decoded raw_data_block.</param>
    /// <returns>Number of raw_data_blocks decoded and dispatched.</returns>
    /// <exception cref="ArgumentNullException"><paramref name="sink"/> is null.</exception>
    /// <exception cref="ArgumentException">
    /// The header fails to parse or the buffer is shorter than
    /// <c>frame_length</c>.
    /// </exception>
    /// <exception cref="InvalidDataException">
    /// Forwarded from <see cref="AacFrameDecoder.DecodeRawDataBlock(ReadOnlySpan{byte}, out int)"/>
    /// for malformed blocks, or raised here when the ADTS header
    /// signals <c>channel_configuration == 0</c> (which ADTS cannot
    /// carry), or when a protected multi-block frame is missing
    /// the trailing 16-bit per-block crc_check.
    /// </exception>
    public int DecodeBlocks(ReadOnlySpan<byte> adtsFrame, FrameSink sink)
    {
        ArgumentNullException.ThrowIfNull(sink);

        if (!TryParseAdtsHeader(adtsFrame, out var parsed))
        {
            throw new ArgumentException(
                "Input does not start with a valid ADTS header.",
                nameof(adtsFrame));
        }

        if (adtsFrame.Length < parsed.FrameLength)
        {
            throw new ArgumentException(
                $"ADTS frame buffer is shorter than the declared frame_length " +
                $"({adtsFrame.Length} < {parsed.FrameLength}).",
                nameof(adtsFrame));
        }

        if (parsed.ChannelConfig is < 1 or > 7)
        {
            throw new InvalidDataException(
                $"ADTS header declares unsupported channel_configuration {parsed.ChannelConfig}.");
        }

        int blockCount = parsed.RawDataBlockCount;

        // Protected multi-block frames have a 16-bit CRC trailer
        // after every raw_data_block. The blocks themselves are
        // still byte-aligned, so we walk them with the same cursor
        // logic and skip 2 extra bytes per block. CRCs are not
        // verified (the spec considers verification optional and
        // recoverable streams are usually re-validated upstream).
        bool hasPerBlockCrc = blockCount > 1 && !parsed.ProtectionAbsent;

        EnsureFrameDecoder(parsed);

        int payloadOffset = parsed.HeaderSize;
        int payloadEnd = parsed.FrameLength;
        int cursor = payloadOffset;

        for (int i = 0; i < blockCount; i++)
        {
            if (cursor >= payloadEnd)
            {
                throw new InvalidDataException(
                    $"ADTS frame ran out of payload at block {i + 1}/{blockCount}.");
            }

            var slice = adtsFrame[cursor..payloadEnd];
            var decoded = _inner!.DecodeRawDataBlock(slice, out int bitsConsumed);
            sink(decoded);
            FrameCount++;

            // Each raw_data_block ends with byte_alignment(); round up.
            int blockBytes = (bitsConsumed + 7) >> 3;
            cursor += blockBytes;
            if (hasPerBlockCrc)
            {
                // Skip the 16-bit per-block crc_check trailer without
                // validation.
                if (cursor + 2 > payloadEnd)
                {
                    throw new InvalidDataException(
                        $"Protected ADTS frame is missing the 16-bit CRC trailer for block {i + 1}/{blockCount}.");
                }
                cursor += 2;
            }
        }

        return blockCount;
    }

    /// <summary>
    /// Walk a contiguous slice of ADTS-framed bytes, decode every
    /// complete frame found, and invoke <paramref name="sink"/> for
    /// each contained raw_data_block in stream order. Stops at the
    /// first truncated frame — the leftover bytes (if any) should be
    /// kept by the caller and prepended to the next chunk to resume
    /// cleanly.
    /// </summary>
    /// <param name="input">
    /// Bytes starting at an ADTS frame boundary. The decoder is
    /// strict: any byte that fails to match the syncword + layer
    /// preamble immediately aborts the walk by throwing
    /// <see cref="InvalidDataException"/>; resynchronisation is the
    /// caller's responsibility.
    /// </param>
    /// <param name="sink">Callback invoked for every decoded raw_data_block.</param>
    /// <returns>
    /// Number of bytes consumed up to the start of the first
    /// incomplete frame (or end of input). Equal to
    /// <c>input.Length</c> when every frame in the buffer was fully
    /// decoded.
    /// </returns>
    /// <exception cref="ArgumentNullException"><paramref name="sink"/> is null.</exception>
    /// <exception cref="InvalidDataException">
    /// A byte position contained a non-zero buffer that did not
    /// start with a valid ADTS syncword.
    /// </exception>
    public int DecodeFrames(ReadOnlySpan<byte> input, FrameSink sink)
    {
        ArgumentNullException.ThrowIfNull(sink);

        int offset = 0;
        while (offset < input.Length)
        {
            var remaining = input[offset..];

            // We need at least the 6 bytes peek_frame_length looks
            // at to know whether the next frame fits.
            if (!TryParseFrameLength(remaining, out int frameLength))
            {
                if (remaining.Length < 6)
                {
                    // Header is itself incomplete; defer to next call.
                    return offset;
                }

                throw new InvalidDataException(
                    $"Lost ADTS sync at byte offset {offset}; resynchronisation " +
                    "is the caller's responsibility.");
            }

            if (remaining.Length < frameLength)
            {
                // Frame body is truncated; preserve the rest for next call.
                return offset;
            }

            var frame = remaining[..frameLength];
            _ = DecodeBlocks(frame, sink);
            offset += frameLength;
        }

        return offset;
    }

    /// <summary>
    /// Drop the underlying decoder state. The next
    /// <see cref="DecodeFrame(ReadOnlySpan{byte})"/> call will rebuild
    /// the <see cref="AacFrameDecoder"/> from the next ADTS header
    /// and start with fresh (silent) overlap-add buffers. Use this
    /// after a seek or when switching to a different elementary
    /// stream so PCM doesn't inherit overlap from prior content.
    /// </summary>
    public void ResetState()
    {
        _inner = null;
        _config = null;
        FrameCount = 0;
    }

    private void EnsureFrameDecoder(in AdtsParsedHeader parsed)
    {
        if (_inner is not null && _config is not null &&
            _config.AudioObjectType == parsed.AudioObjectType &&
            _config.SamplingFrequencyIndex == parsed.SamplingFrequencyIndex &&
            _config.SamplingFrequency == parsed.SampleRate &&
            _config.ChannelConfiguration == parsed.ChannelConfig)
        {
            return;
        }

        _config = new AudioSpecificConfig
        {
            AudioObjectType = parsed.AudioObjectType,
            SamplingFrequencyIndex = parsed.SamplingFrequencyIndex,
            SamplingFrequency = parsed.SampleRate,
            ChannelConfiguration = parsed.ChannelConfig,
            ChannelCount = AacChannelConfigurations.SpeakerCount(parsed.ChannelConfig),
            SbrPresent = false,
        };

        _inner = new AacFrameDecoder(
            _config,
            _scaleFactorCodebook,
            _spectralCodebooks,
            _prngFactory);
    }

    private readonly record struct AdtsParsedHeader(
        int HeaderSize,
        int FrameLength,
        int AudioObjectType,
        int SamplingFrequencyIndex,
        int SampleRate,
        int ChannelConfig,
        int RawDataBlockCount,
        bool ProtectionAbsent);

    private static bool TryParseAdtsHeader(ReadOnlySpan<byte> data, out AdtsParsedHeader header)
    {
        header = default;
        if (data.Length < 7) return false;

        // 12-bit syncword 0xFFF.
        if (data[0] != 0xFF || (data[1] & 0xF0) != 0xF0) return false;

        // Layer field must be 0.
        if ((data[1] & 0x06) != 0) return false;
        bool protectionAbsent = (data[1] & 0x01) != 0;

        int profile = (data[2] >> 6) & 0x03;
        int sampleRateIndex = (data[2] >> 2) & 0x0F;
        int sampleRate = AacSampleRates.FromIndex(sampleRateIndex);
        if (sampleRate <= 0) return false;

        int channelConfig = ((data[2] & 0x01) << 2) | ((data[3] >> 6) & 0x03);

        int frameLength =
            ((data[3] & 0x03) << 11) |
            (data[4] << 3) |
            ((data[5] >> 5) & 0x07);
        if (frameLength < 7) return false;

        int rdbInFrame = data[6] & 0x03;

        // Header size: 7 bytes always (fixed + variable headers);
        // when protected, add 2 bytes per extra raw_data_block_position
        // entry plus 2 bytes for crc_check trailer.
        int headerSize = protectionAbsent
            ? 7
            : 9 + 2 * rdbInFrame;
        if (frameLength <= headerSize) return false;

        header = new AdtsParsedHeader(
            HeaderSize: headerSize,
            FrameLength: frameLength,
            AudioObjectType: profile + 1,
            SamplingFrequencyIndex: sampleRateIndex,
            SampleRate: sampleRate,
            ChannelConfig: channelConfig,
            RawDataBlockCount: rdbInFrame + 1,
            ProtectionAbsent: protectionAbsent);
        return true;
    }
}
