namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Synchronous reader that pumps an ADTS-framed AAC
/// <see cref="System.IO.Stream"/> through
/// <see cref="AacAdtsFrameDecoder"/> one frame at a time. Handles
/// the common "raw <c>.aac</c> file with a leading ID3v2 tag"
/// shape automatically.
/// </summary>
/// <remarks>
/// <para>
/// The reader owns a single internal byte buffer that grows on
/// demand to fit the largest ADTS frame seen so far (capped by
/// <see cref="MaxFrameLength"/>). Unconsumed bytes are preserved
/// across refills so partial frames at the buffer boundary are
/// reassembled transparently. The reader does not own the supplied
/// stream unless <c>leaveOpen</c> is <c>false</c> on
/// construction.
/// </para>
/// <para>
/// Multi-block ADTS frames (unprotected) are transparently fanned
/// out one block per <see cref="ReadNextFrame"/> call via a small
/// internal queue. Protected multi-block ADTS frames
/// (<c>protection_absent == 0</c> with multiple raw_data_blocks)
/// surface as <see cref="NotSupportedException"/> via the inner
/// frame decoder — per-block CRC interleaving is not yet wired.
/// For corrupted streams where the sync is lost mid-stream, the
/// reader raises <see cref="InvalidDataException"/>; recovery
/// (skip-forward-and-resync) is the caller's responsibility.
/// </para>
/// </remarks>
public sealed class AacAdtsStreamReader : IDisposable, IAsyncDisposable
{
    /// <summary>Maximum ADTS frame length (13-bit field): 8191 bytes.</summary>
    public const int MaxFrameLength = (1 << 13) - 1;

    /// <summary>Default initial buffer capacity (8 KiB fits a wide range of typical frames).</summary>
    public const int DefaultBufferSize = 8 * 1024;

    private readonly Stream _stream;
    private readonly bool _leaveOpen;
    private readonly AacAdtsFrameDecoder _decoder;

    private byte[] _buffer;
    private int _start;
    private int _end;
    private bool _eof;
    private bool _skippedLeadingId3;
    private bool _disposed;
    private readonly Queue<AacDecodedRawDataBlock> _pending = new();

    /// <summary>Construct a reader over <paramref name="stream"/>.</summary>
    /// <param name="stream">Source stream; must be readable.</param>
    /// <param name="scaleFactorCodebook">Annex 4.A.2.1 scale-factor codebook.</param>
    /// <param name="spectralCodebooks">Spectral codebook lookup table.</param>
    /// <param name="leaveOpen">
    /// When <c>true</c>, <see cref="Dispose"/> leaves
    /// <paramref name="stream"/> open. Default is <c>false</c>.
    /// </param>
    /// <param name="initialBufferSize">
    /// Starting capacity of the internal frame buffer. Grows on
    /// demand up to <see cref="MaxFrameLength"/>.
    /// </param>
    /// <param name="prngFactory">Optional PNS PRNG factory forwarded to the inner frame decoder.</param>
    public AacAdtsStreamReader(
        Stream stream,
        AacHuffmanCodebook scaleFactorCodebook,
        IReadOnlyList<AacHuffmanCodebook?> spectralCodebooks,
        bool leaveOpen = false,
        int initialBufferSize = DefaultBufferSize,
        Func<AacPnsRandom>? prngFactory = null)
    {
        ArgumentNullException.ThrowIfNull(stream);
        ArgumentNullException.ThrowIfNull(scaleFactorCodebook);
        ArgumentNullException.ThrowIfNull(spectralCodebooks);
        if (!stream.CanRead)
        {
            throw new ArgumentException("Source stream must be readable.", nameof(stream));
        }
        if (initialBufferSize < 16)
        {
            throw new ArgumentOutOfRangeException(
                nameof(initialBufferSize),
                "Initial buffer size must be at least 16 bytes.");
        }

        _stream = stream;
        _leaveOpen = leaveOpen;
        _decoder = new AacAdtsFrameDecoder(scaleFactorCodebook, spectralCodebooks, prngFactory);
        _buffer = new byte[initialBufferSize];
    }

    /// <summary>Frame count successfully decoded since construction (or last <see cref="ResetState"/>).</summary>
    public long FrameCount => _decoder.FrameCount;

    /// <summary>Currently-active config as derived from the last decoded ADTS header.</summary>
    public AudioSpecificConfig? CurrentConfig => _decoder.CurrentConfig;

    /// <summary>Speaker ordering produced by the underlying frame decoder.</summary>
    public IReadOnlyList<AacSpeaker>? CurrentSpeakers => _decoder.CurrentSpeakers;

    /// <summary>Sample rate of the currently-active configuration in Hz.</summary>
    public int CurrentSampleRate => _decoder.CurrentSampleRate;

    /// <summary>Channel count of the currently-active configuration.</summary>
    public int CurrentChannelCount => _decoder.CurrentChannelCount;

    /// <summary>
    /// When <c>true</c>, <see cref="ReadNextFrame"/> /
    /// <see cref="ReadNextFrameAsync"/> do not throw on lost ADTS
    /// sync, impossible advertised frame_length, or a frame body
    /// that runs past end-of-stream — instead they skip the bad
    /// byte(s) and resynchronise to the next valid syncword, or
    /// return <c>null</c> when there is nothing left to recover.
    /// Decoder-level errors raised from inside a header-valid
    /// frame body are not recovered from and still propagate.
    /// Defaults to <c>false</c> to preserve the strict behaviour.
    /// </summary>
    public bool RecoverFromLostSync { get; set; }

    /// <summary>
    /// Read the next decoded raw_data_block from the stream, or
    /// <c>null</c> when end-of-stream is reached on a clean frame
    /// boundary. Multi-block ADTS frames are transparently
    /// fanned out — successive calls return the next block until
    /// the queue drains, then the reader pulls the next frame
    /// from the stream.
    /// </summary>
    /// <exception cref="InvalidDataException">
    /// Mid-stream sync was lost or the stream ended in the middle
    /// of a frame.
    /// </exception>
    /// <exception cref="NotSupportedException">
    /// A protected multi-block ADTS frame
    /// (<c>protection_absent == 0</c> with multiple raw_data_blocks)
    /// was encountered. Per-block CRC interleaving is not yet
    /// supported.
    /// </exception>
    public AacDecodedRawDataBlock? ReadNextFrame()
    {
        ThrowIfDisposed();

        if (_pending.Count > 0)
        {
            return _pending.Dequeue();
        }

        if (!_skippedLeadingId3)
        {
            _skippedLeadingId3 = true;
            SkipLeadingId3v2();
        }

        int frameLength;
        while (true)
        {
            // Make sure we have at least the 6 bytes needed to read frame_length.
            if (!EnsureBuffered(minBytes: 6))
            {
                int leftover = _end - _start;
                if (leftover == 0) return null;
                if (RecoverFromLostSync) return null;
                throw new InvalidDataException(
                    $"Stream ended with {leftover} unconsumed bytes before a complete ADTS header.");
            }

            if (!AacAdtsFrameDecoder.TryParseFrameLength(_buffer.AsSpan(_start, _end - _start), out frameLength))
            {
                if (RecoverFromLostSync)
                {
                    _start++;
                    continue;
                }
                throw new InvalidDataException(
                    "Lost ADTS sync at the next-frame boundary; resynchronisation is the caller's responsibility.");
            }

            if (frameLength > _buffer.Length)
            {
                if (frameLength > MaxFrameLength)
                {
                    if (RecoverFromLostSync)
                    {
                        _start++;
                        continue;
                    }
                    throw new InvalidDataException(
                        $"ADTS header advertised an impossible frame_length of {frameLength} (max {MaxFrameLength}).");
                }
                GrowBuffer(frameLength);
            }

            if (!EnsureBuffered(minBytes: frameLength))
            {
                if (RecoverFromLostSync) return null;
                throw new InvalidDataException(
                    $"Stream ended after only {(_end - _start)} bytes of a declared {frameLength}-byte ADTS frame.");
            }

            break;
        }

        var frame = _buffer.AsSpan(_start, frameLength);
        _decoder.DecodeBlocks(frame, b => _pending.Enqueue(b));
        _start += frameLength;

        // DecodeBlocks always produces at least one block for a
        // syntactically valid frame; a zero-yield outcome would
        // indicate a parser bug rather than malformed input.
        return _pending.Count > 0 ? _pending.Dequeue() : null;
    }

    /// <summary>
    /// Iterator-style wrapper: yields each frame in turn until
    /// end-of-stream. Equivalent to calling
    /// <see cref="ReadNextFrame"/> in a loop.
    /// </summary>
    public IEnumerable<AacDecodedRawDataBlock> ReadFrames()
    {
        while (true)
        {
            var frame = ReadNextFrame();
            if (frame is null) yield break;
            yield return frame;
        }
    }

    /// <summary>
    /// Asynchronous counterpart of <see cref="ReadNextFrame"/>.
    /// Performs the same buffer-refill / parse / decode pipeline but
    /// uses <see cref="System.IO.Stream.ReadAsync(System.Memory{byte}, CancellationToken)"/>
    /// so the thread is never blocked while waiting on network or
    /// file IO. Multi-block fan-out is shared with the sync path via
    /// the same pending queue; subsequent <see cref="ReadNextFrameAsync"/>
    /// (or <see cref="ReadNextFrame"/>) calls drain it before any new
    /// stream read.
    /// </summary>
    public async Task<AacDecodedRawDataBlock?> ReadNextFrameAsync(
        CancellationToken cancellationToken = default)
    {
        ThrowIfDisposed();
        cancellationToken.ThrowIfCancellationRequested();

        if (_pending.Count > 0)
        {
            return _pending.Dequeue();
        }

        if (!_skippedLeadingId3)
        {
            _skippedLeadingId3 = true;
            await SkipLeadingId3v2Async(cancellationToken).ConfigureAwait(false);
        }

        int frameLength;
        while (true)
        {
            if (!await EnsureBufferedAsync(minBytes: 6, cancellationToken).ConfigureAwait(false))
            {
                int leftover = _end - _start;
                if (leftover == 0) return null;
                if (RecoverFromLostSync) return null;
                throw new InvalidDataException(
                    $"Stream ended with {leftover} unconsumed bytes before a complete ADTS header.");
            }

            if (!AacAdtsFrameDecoder.TryParseFrameLength(_buffer.AsSpan(_start, _end - _start), out frameLength))
            {
                if (RecoverFromLostSync)
                {
                    _start++;
                    continue;
                }
                throw new InvalidDataException(
                    "Lost ADTS sync at the next-frame boundary; resynchronisation is the caller's responsibility.");
            }

            if (frameLength > _buffer.Length)
            {
                if (frameLength > MaxFrameLength)
                {
                    if (RecoverFromLostSync)
                    {
                        _start++;
                        continue;
                    }
                    throw new InvalidDataException(
                        $"ADTS header advertised an impossible frame_length of {frameLength} (max {MaxFrameLength}).");
                }
                GrowBuffer(frameLength);
            }

            if (!await EnsureBufferedAsync(minBytes: frameLength, cancellationToken).ConfigureAwait(false))
            {
                if (RecoverFromLostSync) return null;
                throw new InvalidDataException(
                    $"Stream ended after only {(_end - _start)} bytes of a declared {frameLength}-byte ADTS frame.");
            }

            break;
        }

        var frame = _buffer.AsSpan(_start, frameLength);
        _decoder.DecodeBlocks(frame, b => _pending.Enqueue(b));
        _start += frameLength;

        return _pending.Count > 0 ? _pending.Dequeue() : null;
    }

    /// <summary>
    /// Asynchronous counterpart of <see cref="ReadFrames"/>. Yields
    /// each decoded frame until end-of-stream, awaiting underlying
    /// IO without blocking the calling thread.
    /// </summary>
    public async IAsyncEnumerable<AacDecodedRawDataBlock> ReadFramesAsync(
        [System.Runtime.CompilerServices.EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        while (true)
        {
            var frame = await ReadNextFrameAsync(cancellationToken).ConfigureAwait(false);
            if (frame is null) yield break;
            yield return frame;
        }
    }

    /// <summary>
    /// Non-throwing variant of <see cref="ReadNextFrame"/>.
    /// Returns <c>true</c> with a non-null <paramref name="frame"/> on success,
    /// <c>true</c> with <c>null</c> on clean end-of-stream, and <c>false</c> with
    /// <c>null</c> when an <see cref="InvalidDataException"/> is raised (lost sync,
    /// impossible advertised frame length, or a frame body that ends before the
    /// stream does).
    /// </summary>
    /// <remarks>
    /// <see cref="RecoverFromLostSync"/> is orthogonal: when <c>true</c>, the reader
    /// already skips bad bytes and resynchronises internally, so this method would
    /// only return <c>false</c> for decoder-level errors inside a structurally valid
    /// frame body.
    /// </remarks>
    public bool TryReadNextFrame(out AacDecodedRawDataBlock? frame)
    {
        try
        {
            frame = ReadNextFrame();
            return true;
        }
        catch (InvalidDataException)
        {
            frame = null;
            return false;
        }
    }

    /// <summary>
    /// Asynchronous, non-throwing variant of <see cref="ReadNextFrameAsync"/>.
    /// Returns <c>(true, block)</c> on success, <c>(true, null)</c> on clean
    /// end-of-stream, and <c>(false, null)</c> on an
    /// <see cref="InvalidDataException"/>.
    /// </summary>
    /// <remarks>
    /// See <see cref="TryReadNextFrame"/> for the interaction with
    /// <see cref="RecoverFromLostSync"/>.
    /// </remarks>
    public async ValueTask<(bool Success, AacDecodedRawDataBlock? Frame)> TryReadNextFrameAsync(
        CancellationToken cancellationToken = default)
    {
        try
        {
            var frame = await ReadNextFrameAsync(cancellationToken).ConfigureAwait(false);
            return (true, frame);
        }
        catch (InvalidDataException)
        {
            return (false, null);
        }
    }

    /// <summary>
    /// Drop the underlying decoder state and clear any buffered
    /// bytes. Use after seeking the underlying stream so the next
    /// <see cref="ReadNextFrame"/> resynchronises from the stream's
    /// new position and rebuilds the inner decoder.
    /// </summary>
    public void ResetState()
    {
        ThrowIfDisposed();
        _decoder.ResetState();
        _start = 0;
        _end = 0;
        _eof = false;
        _pending.Clear();
        // ID3v2 only sits at file start; after a seek we deliberately
        // do NOT re-scan for it (callers should seek past their own
        // ID3 tag if any).
        _skippedLeadingId3 = true;
    }

    /// <summary>
    /// Reports whether the underlying stream supports the seek-based
    /// overloads (<see cref="SeekToFrame(AacAdtsFrameIndexEntry)"/>
    /// and <see cref="SeekToFrame(System.Collections.Generic.IReadOnlyList{AacAdtsFrameIndexEntry}, long)"/>).
    /// </summary>
    public bool CanSeek => _stream.CanSeek;

    /// <summary>
    /// Seek the underlying stream to the byte offset recorded in
    /// <paramref name="entry"/>, then clear all decoder + buffer
    /// state so the next <see cref="ReadNextFrame"/> (or
    /// <see cref="ReadNextFrameAsync"/>) call returns the
    /// raw_data_block at that frame. Requires
    /// <see cref="CanSeek"/> to be <c>true</c>.
    /// </summary>
    public void SeekToFrame(AacAdtsFrameIndexEntry entry)
    {
        ThrowIfDisposed();
        ArgumentNullException.ThrowIfNull(entry);
        if (!_stream.CanSeek)
        {
            throw new NotSupportedException(
                "Underlying stream is not seekable; seek is unsupported on this reader.");
        }
        if (entry.ByteOffset < 0)
        {
            throw new ArgumentOutOfRangeException(
                nameof(entry),
                "Index entry byte offset must be non-negative.");
        }

        _stream.Seek(entry.ByteOffset, SeekOrigin.Begin);
        ResetState();
    }

    /// <summary>
    /// Convenience wrapper: looks up <paramref name="sampleTarget"/>
    /// in <paramref name="index"/> via
    /// <see cref="AacAdtsFrameIndexer.FindFrameAtSample"/>, then
    /// delegates to <see cref="SeekToFrame(AacAdtsFrameIndexEntry)"/>.
    /// Returns the entry that was seeked to, or <c>null</c> when the
    /// index is empty or the target falls before the first entry.
    /// </summary>
    public AacAdtsFrameIndexEntry? SeekToFrame(
        IReadOnlyList<AacAdtsFrameIndexEntry> index,
        long sampleTarget)
    {
        int position = AacAdtsFrameIndexer.FindFrameAtSample(index, sampleTarget);
        if (position < 0) return null;
        var entry = index[position];
        SeekToFrame(entry);
        return entry;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_leaveOpen) _stream.Dispose();
    }

    /// <summary>
    /// Asynchronously releases the reader and, unless constructed
    /// with <c>leaveOpen: true</c>, asynchronously disposes the
    /// underlying stream via
    /// <see cref="System.IO.Stream.DisposeAsync"/>. Idempotent and
    /// safe to interleave with synchronous <see cref="Dispose"/>.
    /// </summary>
    public ValueTask DisposeAsync()
    {
        if (_disposed) return ValueTask.CompletedTask;
        _disposed = true;
        return _leaveOpen ? ValueTask.CompletedTask : _stream.DisposeAsync();
    }

    private void ThrowIfDisposed()
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
    }

    private bool EnsureBuffered(int minBytes)
    {
        while (_end - _start < minBytes)
        {
            if (_eof) return false;

            CompactBuffer();
            int free = _buffer.Length - _end;
            if (free == 0)
            {
                // Need more headroom but the buffer is full and compaction
                // already happened (start was 0 → nothing freed). Grow.
                GrowBuffer(_buffer.Length * 2);
                free = _buffer.Length - _end;
            }

            int read = _stream.Read(_buffer, _end, free);
            if (read <= 0)
            {
                _eof = true;
            }
            else
            {
                _end += read;
            }
        }

        return true;
    }

    private async ValueTask<bool> EnsureBufferedAsync(int minBytes, CancellationToken cancellationToken)
    {
        while (_end - _start < minBytes)
        {
            if (_eof) return false;
            cancellationToken.ThrowIfCancellationRequested();

            CompactBuffer();
            int free = _buffer.Length - _end;
            if (free == 0)
            {
                GrowBuffer(_buffer.Length * 2);
                free = _buffer.Length - _end;
            }

            int read = await _stream
                .ReadAsync(_buffer.AsMemory(_end, free), cancellationToken)
                .ConfigureAwait(false);
            if (read <= 0)
            {
                _eof = true;
            }
            else
            {
                _end += read;
            }
        }

        return true;
    }

    private void CompactBuffer()
    {
        if (_start == 0) return;
        int leftover = _end - _start;
        if (leftover > 0)
        {
            Buffer.BlockCopy(_buffer, _start, _buffer, 0, leftover);
        }
        _end = leftover;
        _start = 0;
    }

    private void GrowBuffer(int minCapacity)
    {
        int newCapacity = _buffer.Length;
        while (newCapacity < minCapacity)
        {
            newCapacity = Math.Min(newCapacity * 2, MaxFrameLength);
            if (newCapacity == _buffer.Length) break;
        }
        if (newCapacity < minCapacity) newCapacity = minCapacity;

        var grown = new byte[newCapacity];
        int leftover = _end - _start;
        if (leftover > 0)
        {
            Buffer.BlockCopy(_buffer, _start, grown, 0, leftover);
        }
        _buffer = grown;
        _start = 0;
        _end = leftover;
    }

    private void SkipLeadingId3v2()
    {
        if (!EnsureBuffered(minBytes: 10)) return;
        var head = _buffer.AsSpan(_start, 10);
        if (head[0] != (byte)'I' || head[1] != (byte)'D' || head[2] != (byte)'3') return;

        // 28-bit synchsafe size at bytes 6..9.
        int size =
            ((head[6] & 0x7F) << 21) |
            ((head[7] & 0x7F) << 14) |
            ((head[8] & 0x7F) << 7) |
            (head[9] & 0x7F);
        int totalSkip = 10 + size;

        // Skip what's already buffered then drain the rest from the
        // stream without exposing it to the decoder.
        int skipFromBuffer = Math.Min(totalSkip, _end - _start);
        _start += skipFromBuffer;
        int remaining = totalSkip - skipFromBuffer;

        while (remaining > 0)
        {
            int free = _buffer.Length - _end;
            if (free == 0)
            {
                CompactBuffer();
                free = _buffer.Length - _end;
            }

            int want = Math.Min(free, remaining);
            int read = _stream.Read(_buffer, _end, want);
            if (read <= 0)
            {
                _eof = true;
                return;
            }
            _end += read;
            // Consume those bytes (they're part of the ID3 tag).
            _start += read;
            remaining -= read;
        }
    }

    private async ValueTask SkipLeadingId3v2Async(CancellationToken cancellationToken)
    {
        if (!await EnsureBufferedAsync(minBytes: 10, cancellationToken).ConfigureAwait(false)) return;
        var head = _buffer.AsSpan(_start, 10);
        if (head[0] != (byte)'I' || head[1] != (byte)'D' || head[2] != (byte)'3') return;

        int size =
            ((head[6] & 0x7F) << 21) |
            ((head[7] & 0x7F) << 14) |
            ((head[8] & 0x7F) << 7) |
            (head[9] & 0x7F);
        int totalSkip = 10 + size;

        int skipFromBuffer = Math.Min(totalSkip, _end - _start);
        _start += skipFromBuffer;
        int remaining = totalSkip - skipFromBuffer;

        while (remaining > 0)
        {
            cancellationToken.ThrowIfCancellationRequested();

            int free = _buffer.Length - _end;
            if (free == 0)
            {
                CompactBuffer();
                free = _buffer.Length - _end;
            }

            int want = Math.Min(free, remaining);
            int read = await _stream
                .ReadAsync(_buffer.AsMemory(_end, want), cancellationToken)
                .ConfigureAwait(false);
            if (read <= 0)
            {
                _eof = true;
                return;
            }
            _end += read;
            _start += read;
            remaining -= read;
        }
    }
}
