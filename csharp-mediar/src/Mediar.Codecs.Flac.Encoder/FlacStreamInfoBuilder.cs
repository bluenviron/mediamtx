using System.Buffers.Binary;
using System.Security.Cryptography;

namespace Mediar.Codecs.Flac.Encoder;

/// <summary>
/// Incremental builder for the 34-byte FLAC STREAMINFO block (RFC 9639 §8.2).
/// Tracks min/max block size, min/max frame size, total sample count, and the
/// MD5 of the unencoded interleaved PCM as required by the spec. The block
/// returned by <see cref="ToBytes"/> reflects the state of every
/// <see cref="ObserveFrame"/> call made so far.
/// </summary>
public sealed class FlacStreamInfoBuilder
{
    private const int Bytes = 34;
    private const ushort MinBlockSizeSpec = 16;
    private const long MaxTotalSamples = (1L << 36) - 1;

    private readonly FlacEncoderParameters _parameters;
    private readonly IncrementalHash _md5;
    private readonly int _bytesPerSampleMd5;

    private int _minBlockSize = ushort.MaxValue;
    private int _maxBlockSize;
    private int _minFrameSize = int.MaxValue;
    private int _maxFrameSize;
    private long _totalSamples;
    private byte[]? _frozenMd5;

    /// <summary>Create a new builder for <paramref name="parameters"/>.</summary>
    /// <param name="parameters">Stream parameters; must match those passed to the frame encoder.</param>
    public FlacStreamInfoBuilder(FlacEncoderParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        parameters.Validate();
        _parameters = parameters;
        _md5 = IncrementalHash.CreateHash(HashAlgorithmName.MD5);
        _bytesPerSampleMd5 = (parameters.BitsPerSample + 7) / 8;
    }

    /// <summary>Total samples per channel observed so far.</summary>
    public long TotalSamples => _totalSamples;

    /// <summary>Number of frames observed.</summary>
    public int FrameCount { get; private set; }

    /// <summary>Smallest observed frame size in bytes, or 0 if no frames yet.</summary>
    public int MinFrameSize => _minFrameSize == int.MaxValue ? 0 : _minFrameSize;

    /// <summary>Largest observed frame size in bytes.</summary>
    public int MaxFrameSize => _maxFrameSize;

    /// <summary>
    /// Observe one encoded frame: updates min/max block size, frame size,
    /// total samples and the MD5 of the unencoded PCM.
    /// </summary>
    /// <param name="interleavedSamples">The samples the encoder consumed
    /// (same layout as the encoder input).</param>
    /// <param name="samplesPerChannel">Block size of THIS frame.</param>
    /// <param name="frameSizeInBytes">Encoded frame size returned by
    /// <see cref="FlacFrameEncoder.EncodeFrame"/>.</param>
    public void ObserveFrame(ReadOnlySpan<int> interleavedSamples, int samplesPerChannel, int frameSizeInBytes)
    {
        if (_frozenMd5 is not null)
        {
            throw new InvalidOperationException("Builder has been frozen by ToBytes().");
        }
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(samplesPerChannel);
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(frameSizeInBytes);
        int needed = checked(samplesPerChannel * _parameters.Channels);
        if (interleavedSamples.Length < needed)
        {
            throw new ArgumentException("Not enough samples for the declared block size.", nameof(interleavedSamples));
        }

        if (samplesPerChannel < _minBlockSize) _minBlockSize = samplesPerChannel;
        if (samplesPerChannel > _maxBlockSize) _maxBlockSize = samplesPerChannel;
        if (frameSizeInBytes < _minFrameSize) _minFrameSize = frameSizeInBytes;
        if (frameSizeInBytes > _maxFrameSize) _maxFrameSize = frameSizeInBytes;

        _totalSamples = checked(_totalSamples + samplesPerChannel);
        if (_totalSamples > MaxTotalSamples)
        {
            throw new InvalidOperationException("FLAC total sample count exceeds 36-bit field.");
        }
        FrameCount++;

        AppendMd5(interleavedSamples[..needed]);
    }

    private void AppendMd5(ReadOnlySpan<int> samples)
    {
        // libFLAC convention: MD5 over each sample stored as ⌈bps/8⌉ little-endian bytes
        // of the signed value, in interleaved channel order.
        int bytesPerSample = _bytesPerSampleMd5;
        // Buffer up to 4 KiB worth of samples at a time.
        const int ChunkSamples = 1024;
        Span<byte> chunk = stackalloc byte[ChunkSamples * 4];
        int idx = 0;
        while (idx < samples.Length)
        {
            int take = Math.Min(ChunkSamples, samples.Length - idx);
            int outBytes = 0;
            for (int i = 0; i < take; i++)
            {
                int v = samples[idx + i];
                switch (bytesPerSample)
                {
                    case 1:
                        chunk[outBytes++] = (byte)v;
                        break;
                    case 2:
                        BinaryPrimitives.WriteInt16LittleEndian(chunk.Slice(outBytes, 2), (short)v);
                        outBytes += 2;
                        break;
                    case 3:
                        chunk[outBytes++] = (byte)v;
                        chunk[outBytes++] = (byte)(v >> 8);
                        chunk[outBytes++] = (byte)(v >> 16);
                        break;
                    case 4:
                        BinaryPrimitives.WriteInt32LittleEndian(chunk.Slice(outBytes, 4), v);
                        outBytes += 4;
                        break;
                }
            }
            _md5.AppendData(chunk[..outBytes]);
            idx += take;
        }
    }

    /// <summary>
    /// Emit the 34-byte STREAMINFO block reflecting all observed frames. Once
    /// called, further <see cref="ObserveFrame"/> calls will throw.
    /// </summary>
    /// <param name="writeMd5">When true (default), include the MD5 of the
    /// unencoded PCM. When false, leave it as 16 zero bytes (decoders
    /// interpret all-zero as "unknown" per RFC 9639 §8.2).</param>
    public byte[] ToBytes(bool writeMd5 = true)
    {
        if (FrameCount == 0)
        {
            // No frames observed: emit a degenerate-but-valid STREAMINFO. Min/max
            // block size fall back to the configured fixed-blocksize so existing
            // decoders see a sensible range.
            _minBlockSize = _parameters.BlockSize;
            _maxBlockSize = _parameters.BlockSize;
        }
        if (_minBlockSize < MinBlockSizeSpec) _minBlockSize = MinBlockSizeSpec;

        if (_frozenMd5 is null)
        {
            _frozenMd5 = writeMd5 ? _md5.GetHashAndReset() : new byte[16];
            _md5.Dispose();
        }

        byte[] block = new byte[Bytes];
        BinaryPrimitives.WriteUInt16BigEndian(block.AsSpan(0, 2), (ushort)_minBlockSize);
        BinaryPrimitives.WriteUInt16BigEndian(block.AsSpan(2, 2), (ushort)_maxBlockSize);

        // 24-bit big-endian min/max frame size.
        block[4] = (byte)(_minFrameSize == int.MaxValue ? 0 : (_minFrameSize >> 16));
        block[5] = (byte)((_minFrameSize == int.MaxValue ? 0 : _minFrameSize) >> 8);
        block[6] = (byte)(_minFrameSize == int.MaxValue ? 0 : _minFrameSize);
        block[7] = (byte)(_maxFrameSize >> 16);
        block[8] = (byte)(_maxFrameSize >> 8);
        block[9] = (byte)_maxFrameSize;

        // Packed: 20 bits sample_rate || 3 bits channels-1 || 5 bits bps-1 || 36 bits total_samples.
        uint sr = (uint)_parameters.SampleRate;
        uint ch = (uint)(_parameters.Channels - 1);
        uint bps = (uint)(_parameters.BitsPerSample - 1);
        ulong packed = ((ulong)sr & 0xFFFFFUL) << 44;
        packed |= ((ulong)ch & 0x7UL) << 41;
        packed |= ((ulong)bps & 0x1FUL) << 36;
        packed |= (ulong)_totalSamples & 0xFFFFFFFFFUL;
        BinaryPrimitives.WriteUInt64BigEndian(block.AsSpan(10, 8), packed);

        // 16-byte MD5 of unencoded PCM.
        _frozenMd5.AsSpan(0, 16).CopyTo(block.AsSpan(18, 16));
        return block;
    }
}
