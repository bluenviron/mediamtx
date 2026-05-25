namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Per-channel overlap-add accumulator for Vorbis MDCT block synthesis.
///
/// Vorbis emits time-domain blocks whose IMDCT outputs are sin²-windowed and
/// overlap-added with neighbouring blocks. Block centers are stride
/// <c>(n_prev + n_curr)/4</c> apart in audio time (Vorbis I §1.3.2.4 + the
/// stride implied by §4.3); a single decoded packet produces
/// <c>(n_prev + n_curr)/4</c> finalized output samples per channel.
///
/// This class keeps a small per-channel accumulator buffer that holds samples
/// in flight (windowed contributions that haven't yet been emitted) and a
/// running absolute audio position so that mixed long/short transitions
/// align without losing TDAC. The first audio packet produces zero output
/// (it only populates the accumulator); every subsequent packet returns
/// <c>(n_prev + n_curr)/4</c> samples.
/// </summary>
internal sealed class VorbisLap
{
    private readonly int _channels;
    private readonly int _blocksize1;
    private readonly float[][] _accum;
    private readonly int _accumSize;
    private int _accumStart;
    private int _prevCenter;
    private int _prevN;
    private bool _hasPrev;

    public VorbisLap(int channels, int blocksize1)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(channels);
        if (blocksize1 <= 0 || (blocksize1 & (blocksize1 - 1)) != 0)
            throw new ArgumentException("blocksize1 must be a positive power of two.", nameof(blocksize1));
        _channels = channels;
        _blocksize1 = blocksize1;
        _accumSize = 2 * blocksize1;
        _accum = new float[channels][];
        for (int c = 0; c < channels; c++) _accum[c] = new float[_accumSize];
        _accumStart = 0;
        _prevCenter = 0;
        _prevN = 0;
        _hasPrev = false;
    }

    public int Channels => _channels;

    public int Blocksize1 => _blocksize1;

    /// <summary>
    /// Return the sample count that <see cref="Commit"/> will produce given
    /// the supplied current block size. Lets callers size the output buffer
    /// before committing.
    /// </summary>
    public int PeekEmit(int currN) => _hasPrev ? (_prevN + currN) / 4 : 0;

    /// <summary>
    /// Add the windowed time-domain block <paramref name="block"/> for channel
    /// <paramref name="channel"/> at the current write position. Must be
    /// called once per channel per packet, after which <see cref="Commit"/>
    /// finalizes the packet and returns the emitted sample count.
    /// </summary>
    public void Accumulate(int channel, ReadOnlySpan<float> block, int currN)
    {
        if ((uint)channel >= (uint)_channels) throw new ArgumentOutOfRangeException(nameof(channel));
        if (block.Length < currN) throw new ArgumentException("Block shorter than currN.", nameof(block));

        int currCenter = _hasPrev
            ? _prevCenter + (_prevN + currN) / 4
            : currN / 2;
        int currStart = currCenter - currN / 2;
        int offset = _hasPrev ? currStart - _accumStart : 0;

        // For the very first packet we initialise accumStart so the block
        // lands at accum[0..currN) — that's the implicit "zero history"
        // boundary condition (Vorbis I §4.3.6 "the first audio packet
        // primes the lap").
        if (!_hasPrev) _accumStart = currStart;

        var accum = _accum[channel];
        int writeStart = Math.Max(0, offset);
        int writeEnd = Math.Min(_accumSize, offset + currN);
        for (int idx = writeStart; idx < writeEnd; idx++)
        {
            accum[idx] += block[idx - offset];
        }
    }

    /// <summary>
    /// Commit the in-flight packet, populate <paramref name="output"/> with
    /// the finalized samples (one channel-major Memory per channel) and
    /// shift the accumulator. Returns the emitted sample count per channel.
    /// </summary>
    public int Commit(Memory<float>[] output, int currN)
    {
        int currCenter = _hasPrev
            ? _prevCenter + (_prevN + currN) / 4
            : currN / 2;
        int emit = _hasPrev ? (_prevN + currN) / 4 : 0;

        if (emit > 0)
        {
            if (output.Length < _channels) throw new ArgumentException("output too small.", nameof(output));
            int emitStart = _prevCenter - _accumStart;
            for (int ch = 0; ch < _channels; ch++)
            {
                _accum[ch].AsSpan(emitStart, emit).CopyTo(output[ch].Span);
            }
            int shift = emitStart + emit;
            for (int ch = 0; ch < _channels; ch++)
            {
                var arr = _accum[ch];
                Array.Copy(arr, shift, arr, 0, _accumSize - shift);
                Array.Clear(arr, _accumSize - shift, shift);
            }
            _accumStart += shift;
        }
        _prevCenter = currCenter;
        _prevN = currN;
        _hasPrev = true;
        return emit;
    }

    /// <summary>Reset the accumulator (call on seek).</summary>
    public void Reset()
    {
        for (int c = 0; c < _channels; c++) Array.Clear(_accum[c]);
        _accumStart = 0;
        _prevCenter = 0;
        _prevN = 0;
        _hasPrev = false;
    }
}
