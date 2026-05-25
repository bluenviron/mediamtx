using System.Buffers;
using Mediar.IO;

namespace Mediar.Codecs.Flac.Decoder;

/// <summary>
/// FLAC (RFC 9639) audio decoder. Each <see cref="Decode"/> call expects one
/// complete FLAC frame (sync code through CRC-16 footer) — the boundary that
/// <c>Mediar.Containers.Flac.FlacDemuxer</c> already produces.
/// </summary>
public sealed class FlacDecoder : IAudioDecoder
{
    private readonly int _defaultSampleRate;
    private readonly int _defaultChannels;
    private readonly int _defaultBitsPerSample;
    private readonly int _maxBlockSize;

    private int[][] _channelBuffers = [];

    /// <inheritdoc/>
    public CodecId Codec => CodecId.Flac;

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>Create a FLAC decoder. <paramref name="parameters"/>.ExtraData must contain the 34-byte STREAMINFO body.</summary>
    public FlacDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec != CodecId.Flac)
            throw new ArgumentException("FlacDecoder requires Codec=Flac.", nameof(parameters));
        Parameters = parameters;

        ParseStreamInfo(
            parameters.ExtraData.Span,
            parameters.SampleRate,
            parameters.Channels,
            parameters.BitsPerSample,
            out _defaultSampleRate,
            out _defaultChannels,
            out _defaultBitsPerSample,
            out _maxBlockSize);
    }

    private static void ParseStreamInfo(
        ReadOnlySpan<byte> extra,
        int paramSampleRate,
        int paramChannels,
        int paramBps,
        out int sampleRate,
        out int channels,
        out int bps,
        out int maxBlockSize)
    {
        sampleRate = paramSampleRate;
        channels = paramChannels;
        bps = paramBps;
        maxBlockSize = 4096;

        if (extra.Length < 18) return;
        // STREAMINFO body (RFC 9639 §8.2): 16/16 min/max block size, 24/24 min/max frame size,
        // 20/3/5/36 sample rate / channels-1 / bps-1 / total samples, 128 MD5.
        maxBlockSize = (extra[2] << 8) | extra[3];

        var br = new BitReader(extra[10..]);
        sampleRate = (int)br.ReadBits(20);
        channels = (int)br.ReadBits(3) + 1;
        bps = (int)br.ReadBits(5) + 1;

        if (paramSampleRate > 0) sampleRate = paramSampleRate;
        if (paramChannels > 0) channels = paramChannels;
        if (paramBps > 0) bps = paramBps;
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;
        if (encoded.Length < 6) throw new InvalidDataException("FLAC frame too short.");

        if (!FlacFrameHeaderParser.TryParse(encoded, _defaultSampleRate, _defaultBitsPerSample, out var header))
        {
            throw new InvalidDataException("Invalid FLAC frame header.");
        }

        int channels = header.Channels;
        int blockSize = header.BlockSize;
        int bps = header.BitsPerSample;

        if (blockSize > _maxBlockSize && _maxBlockSize > 0)
        {
            // STREAMINFO max-block-size is advisory only; resize the workspace if needed.
        }
        EnsureWorkspace(channels, Math.Max(blockSize, _maxBlockSize));

        var br = new BitReader(encoded[header.HeaderSize..]);

        for (int c = 0; c < channels; c++)
        {
            int subframeBps = bps;
            if (header.ChannelMode == FlacChannelMode.LeftSide && c == 1) subframeBps++;
            else if (header.ChannelMode == FlacChannelMode.SideRight && c == 0) subframeBps++;
            else if (header.ChannelMode == FlacChannelMode.MidSide && c == 1) subframeBps++;

            FlacSubframeDecoder.Decode(ref br, _channelBuffers[c].AsSpan(0, blockSize), blockSize, subframeBps);
        }

        br.AlignToByte();

        // Frame footer: 2-byte CRC-16-IBM over everything before the footer.
        int frameLen = header.HeaderSize + ((int)br.BitPosition / 8);
        if (encoded.Length < frameLen + 2) throw new InvalidDataException("FLAC frame missing CRC-16 footer.");
        ushort storedCrc = (ushort)((encoded[frameLen] << 8) | encoded[frameLen + 1]);
        ushort computedCrc = FlacCrc.Crc16(encoded[..frameLen]);
        if (storedCrc != computedCrc) throw new InvalidDataException("FLAC frame CRC-16 mismatch.");

        // Stereo decorrelation
        if (header.ChannelMode == FlacChannelMode.LeftSide)
        {
            var l = _channelBuffers[0].AsSpan(0, blockSize);
            var s = _channelBuffers[1].AsSpan(0, blockSize);
            for (int i = 0; i < blockSize; i++) s[i] = l[i] - s[i];
        }
        else if (header.ChannelMode == FlacChannelMode.SideRight)
        {
            var s = _channelBuffers[0].AsSpan(0, blockSize);
            var r = _channelBuffers[1].AsSpan(0, blockSize);
            // ch0 was side, ch1 was right. After: ch0 = L = R + side, ch1 = R (unchanged).
            for (int i = 0; i < blockSize; i++) s[i] = r[i] + s[i];
        }
        else if (header.ChannelMode == FlacChannelMode.MidSide)
        {
            var mid = _channelBuffers[0].AsSpan(0, blockSize);
            var side = _channelBuffers[1].AsSpan(0, blockSize);
            for (int i = 0; i < blockSize; i++)
            {
                int m = mid[i];
                int sd = side[i];
                int mPrime = (m << 1) | (sd & 1);
                int L = (mPrime + sd) >> 1;
                int R = (mPrime - sd) >> 1;
                mid[i] = L;
                side[i] = R;
            }
        }

        // Pack into interleaved normalized float frame.
        int total = blockSize * channels;
        var owner = MemoryPool<float>.Shared.Rent(total);
        var floats = owner.Memory.Span[..total];
        float scale = 1f / (1L << (bps - 1));
        for (int i = 0; i < blockSize; i++)
        {
            for (int c = 0; c < channels; c++)
            {
                floats[i * channels + c] = _channelBuffers[c][i] * scale;
            }
        }

        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = header.SampleRate,
            SamplesPerChannel = blockSize,
            Pts = pts,
            Samples = owner.Memory[..total],
            Owner = owner,
        };
    }

    private void EnsureWorkspace(int channels, int blockSize)
    {
        if (_channelBuffers.Length < channels)
        {
            var bigger = new int[channels][];
            Array.Copy(_channelBuffers, bigger, _channelBuffers.Length);
            _channelBuffers = bigger;
        }
        for (int i = 0; i < channels; i++)
        {
            if (_channelBuffers[i] == null || _channelBuffers[i].Length < blockSize)
            {
                _channelBuffers[i] = new int[blockSize];
            }
        }
    }

    /// <inheritdoc/>
    public void Reset() { /* FLAC frames are independent — nothing to reset */ }

    /// <inheritdoc/>
    public void Dispose() { /* no resources */ }
}

/// <summary>Factory that creates <see cref="FlacDecoder"/> instances.</summary>
public sealed class FlacDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec == CodecId.Flac;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new FlacDecoder(parameters);
}
