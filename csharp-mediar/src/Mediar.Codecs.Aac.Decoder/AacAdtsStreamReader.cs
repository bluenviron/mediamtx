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
/// For multi-block ADTS frames, the underlying frame decoder
/// throws <see cref="NotSupportedException"/>. For corrupted
/// streams where the sync is lost mid-stream, the reader raises
/// <see cref="InvalidDataException"/>; recovery (skip-forward-and-
/// resync) is the caller's responsibility.
/// </para>
/// </remarks>
public sealed class AacAdtsStreamReader : IDisposable
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
    /// Read the next decoded frame from the stream, or <c>null</c>
    /// when end-of-stream is reached on a clean frame boundary.
    /// </summary>
    /// <exception cref="InvalidDataException">
    /// Mid-stream sync was lost or the stream ended in the middle
    /// of a frame.
    /// </exception>
    /// <exception cref="NotSupportedException">
    /// A multi-raw_data_block ADTS frame was encountered.
    /// </exception>
    public AacDecodedRawDataBlock? ReadNextFrame()
    {
        ThrowIfDisposed();

        if (!_skippedLeadingId3)
        {
            _skippedLeadingId3 = true;
            SkipLeadingId3v2();
        }

        // Make sure we have at least the 6 bytes needed to read frame_length.
        if (!EnsureBuffered(minBytes: 6))
        {
            int leftover = _end - _start;
            if (leftover == 0) return null;
            throw new InvalidDataException(
                $"Stream ended with {leftover} unconsumed bytes before a complete ADTS header.");
        }

        if (!AacAdtsFrameDecoder.TryParseFrameLength(_buffer.AsSpan(_start, _end - _start), out int frameLength))
        {
            throw new InvalidDataException(
                "Lost ADTS sync at the next-frame boundary; resynchronisation is the caller's responsibility.");
        }

        if (frameLength > _buffer.Length)
        {
            if (frameLength > MaxFrameLength)
            {
                throw new InvalidDataException(
                    $"ADTS header advertised an impossible frame_length of {frameLength} (max {MaxFrameLength}).");
            }
            GrowBuffer(frameLength);
        }

        if (!EnsureBuffered(minBytes: frameLength))
        {
            throw new InvalidDataException(
                $"Stream ended after only {(_end - _start)} bytes of a declared {frameLength}-byte ADTS frame.");
        }

        var frame = _buffer.AsSpan(_start, frameLength);
        var block = _decoder.DecodeFrame(frame);
        _start += frameLength;
        return block;
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
        // ID3v2 only sits at file start; after a seek we deliberately
        // do NOT re-scan for it (callers should seek past their own
        // ID3 tag if any).
        _skippedLeadingId3 = true;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_leaveOpen) _stream.Dispose();
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
}
