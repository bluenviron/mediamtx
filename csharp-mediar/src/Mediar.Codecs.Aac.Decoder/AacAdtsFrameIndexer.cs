namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// One entry in the index produced by
/// <see cref="AacAdtsFrameIndexer.BuildIndex"/>: identifies a
/// single ADTS frame by byte offset and reports the PCM sample
/// position at its start so seek operations can map (or bracket)
/// a target sample to the right byte offset without re-parsing
/// the whole file.
/// </summary>
public sealed record AacAdtsFrameIndexEntry
{
    /// <summary>Byte offset of the frame's syncword in the source stream.</summary>
    public required long ByteOffset { get; init; }

    /// <summary>Total frame length in bytes (header + payload).</summary>
    public required int FrameLength { get; init; }

    /// <summary>
    /// Number of raw_data_blocks the frame carries
    /// (<c>number_of_raw_data_blocks_in_frame + 1</c>; 1 for
    /// typical single-block frames).
    /// </summary>
    public required int BlockCount { get; init; }

    /// <summary>
    /// Cumulative PCM sample position (per channel) at the start
    /// of this frame, counted across all preceding indexed frames.
    /// The first entry has <c>SampleOffset == 0</c>.
    /// </summary>
    public required long SampleOffset { get; init; }

    /// <summary>Sampling frequency in Hz advertised by the ADTS header.</summary>
    public required int SampleRate { get; init; }

    /// <summary>
    /// Channel configuration field (1..7 for the standard
    /// configurations; 0 means PCE-described, which ADTS does
    /// not actually carry but the index reports it verbatim for
    /// diagnostic purposes).
    /// </summary>
    public required int ChannelConfiguration { get; init; }
}

/// <summary>
/// Builds an index over the ADTS frames in a source stream
/// without decoding any audio data. Each entry records the
/// byte offset, frame length, embedded block count, cumulative
/// per-channel PCM sample position, and the advertised sample
/// rate / channel configuration so seek-by-time can be
/// implemented as a binary search over the returned array.
/// </summary>
/// <remarks>
/// <para>
/// The scanner reads the input stream sequentially from its
/// current position. It does NOT skip a leading ID3v2 tag — pass
/// a stream already positioned at the first ADTS sync, or wrap
/// the file with <see cref="AacAdtsStreamReader"/> when ID3
/// skipping is needed.
/// </para>
/// <para>
/// Each raw_data_block produces 1024 PCM samples per channel
/// (the AAC long-window frame length). Multi-block frames
/// contribute <c>BlockCount * 1024</c> samples to the cumulative
/// offset.
/// </para>
/// <para>
/// On lost sync the scanner throws
/// <see cref="InvalidDataException"/>; resynchronisation is the
/// caller's responsibility.
/// </para>
/// </remarks>
public static class AacAdtsFrameIndexer
{
    private const int SamplesPerBlock = 1024;

    /// <summary>
    /// Walk every ADTS frame in <paramref name="stream"/> from its
    /// current position to end-of-stream and return the index.
    /// </summary>
    /// <param name="stream">Source stream; must be readable.</param>
    /// <param name="initialBufferSize">
    /// Initial read buffer capacity. Grows on demand to fit the
    /// largest frame seen (max 8191 bytes).
    /// </param>
    public static IReadOnlyList<AacAdtsFrameIndexEntry> BuildIndex(
        Stream stream,
        int initialBufferSize = AacAdtsStreamReader.DefaultBufferSize)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanRead) throw new ArgumentException("Stream must be readable.", nameof(stream));
        if (initialBufferSize < 16)
        {
            throw new ArgumentOutOfRangeException(
                nameof(initialBufferSize),
                "Initial buffer size must be at least 16 bytes.");
        }

        return BuildIndexCore(
            stream,
            initialBufferSize,
            asyncReader: null,
            cancellationToken: default).GetAwaiter().GetResult();
    }

    /// <summary>
    /// Asynchronous overload of <see cref="BuildIndex"/>. Useful when
    /// indexing large remote / cloud-backed streams where the file
    /// IO would otherwise block a thread for non-trivial wall time.
    /// </summary>
    public static Task<IReadOnlyList<AacAdtsFrameIndexEntry>> BuildIndexAsync(
        Stream stream,
        int initialBufferSize = AacAdtsStreamReader.DefaultBufferSize,
        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanRead) throw new ArgumentException("Stream must be readable.", nameof(stream));
        if (initialBufferSize < 16)
        {
            throw new ArgumentOutOfRangeException(
                nameof(initialBufferSize),
                "Initial buffer size must be at least 16 bytes.");
        }

        return BuildIndexCore(
            stream,
            initialBufferSize,
            asyncReader: (buf, offset, count, ct) => stream.ReadAsync(buf.AsMemory(offset, count), ct),
            cancellationToken: cancellationToken);
    }

    private static async Task<IReadOnlyList<AacAdtsFrameIndexEntry>> BuildIndexCore(
        Stream stream,
        int initialBufferSize,
        Func<byte[], int, int, CancellationToken, ValueTask<int>>? asyncReader,
        CancellationToken cancellationToken)
    {
        var index = new List<AacAdtsFrameIndexEntry>();
        byte[] buf = new byte[initialBufferSize];
        int start = 0, end = 0;
        bool eof = false;
        long streamOffset = 0;
        long sampleOffset = 0;

        while (true)
        {
            cancellationToken.ThrowIfCancellationRequested();

            // Ensure 7 bytes (full fixed + variable header) for parsing.
            while (end - start < 7 && !eof)
            {
                if (start > 0)
                {
                    int leftover = end - start;
                    if (leftover > 0) Buffer.BlockCopy(buf, start, buf, 0, leftover);
                    end = leftover;
                    start = 0;
                }
                int free = buf.Length - end;
                if (free == 0)
                {
                    int newCap = Math.Min(buf.Length * 2, AacAdtsStreamReader.MaxFrameLength);
                    if (newCap == buf.Length)
                    {
                        // Should not happen — 7 bytes always fits.
                        break;
                    }
                    var grown = new byte[newCap];
                    Buffer.BlockCopy(buf, 0, grown, 0, end);
                    buf = grown;
                    free = buf.Length - end;
                }
                int read = asyncReader is null
                    ? stream.Read(buf, end, free)
                    : await asyncReader(buf, end, free, cancellationToken).ConfigureAwait(false);
                if (read <= 0) eof = true;
                else end += read;
            }

            int avail = end - start;
            if (avail == 0) break;
            if (avail < 7)
            {
                throw new InvalidDataException(
                    $"Stream ended with {avail} unconsumed bytes before a complete ADTS header at byte offset {streamOffset}.");
            }

            var header = buf.AsSpan(start, avail);
            if (!TryParseIndexHeader(header, out var parsed))
            {
                throw new InvalidDataException(
                    $"Lost ADTS sync at byte offset {streamOffset}.");
            }

            // Ensure the entire frame body sits in the buffer.
            if (avail < parsed.FrameLength)
            {
                if (parsed.FrameLength > buf.Length)
                {
                    if (parsed.FrameLength > AacAdtsStreamReader.MaxFrameLength)
                    {
                        throw new InvalidDataException(
                            $"ADTS header at offset {streamOffset} advertised an impossible frame_length of {parsed.FrameLength}.");
                    }
                    var grown = new byte[parsed.FrameLength];
                    int leftover = end - start;
                    Buffer.BlockCopy(buf, start, grown, 0, leftover);
                    buf = grown;
                    end = leftover;
                    start = 0;
                }
                else if (start > 0)
                {
                    int leftover = end - start;
                    Buffer.BlockCopy(buf, start, buf, 0, leftover);
                    end = leftover;
                    start = 0;
                }
                while (end - start < parsed.FrameLength && !eof)
                {
                    int free = buf.Length - end;
                    int read = asyncReader is null
                        ? stream.Read(buf, end, free)
                        : await asyncReader(buf, end, free, cancellationToken).ConfigureAwait(false);
                    if (read <= 0) eof = true;
                    else end += read;
                }
                if (end - start < parsed.FrameLength)
                {
                    throw new InvalidDataException(
                        $"Stream ended after only {(end - start)} bytes of a declared {parsed.FrameLength}-byte ADTS frame at offset {streamOffset}.");
                }
            }

            index.Add(new AacAdtsFrameIndexEntry
            {
                ByteOffset = streamOffset,
                FrameLength = parsed.FrameLength,
                BlockCount = parsed.BlockCount,
                SampleOffset = sampleOffset,
                SampleRate = parsed.SampleRate,
                ChannelConfiguration = parsed.ChannelConfig,
            });

            sampleOffset += (long)parsed.BlockCount * SamplesPerBlock;
            start += parsed.FrameLength;
            streamOffset += parsed.FrameLength;
        }

        return index;
    }

    private readonly record struct IndexedHeader(
        int FrameLength,
        int BlockCount,
        int SampleRate,
        int ChannelConfig);

    private static bool TryParseIndexHeader(ReadOnlySpan<byte> data, out IndexedHeader header)
    {
        header = default;
        if (data.Length < 7) return false;
        if (data[0] != 0xFF || (data[1] & 0xF0) != 0xF0) return false;
        if ((data[1] & 0x06) != 0) return false;

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

        header = new IndexedHeader(
            FrameLength: frameLength,
            BlockCount: rdbInFrame + 1,
            SampleRate: sampleRate,
            ChannelConfig: channelConfig);
        return true;
    }

    /// <summary>
    /// Binary-searches the (sample-sorted) <paramref name="index"/>
    /// and returns the position of the frame whose sample range
    /// covers <paramref name="sampleTarget"/>. The returned value is
    /// the index of the entry with the largest <c>SampleOffset</c>
    /// less than or equal to <paramref name="sampleTarget"/>.
    /// </summary>
    /// <param name="index">
    /// Frame index produced by <see cref="BuildIndex(Stream, int)"/>.
    /// Must be non-null and sorted by <c>SampleOffset</c> (the
    /// indexer always produces this ordering).
    /// </param>
    /// <param name="sampleTarget">
    /// Target sample position (per channel), counted from the start
    /// of the stream. Negative values throw.
    /// </param>
    /// <returns>
    /// Position of the matching entry, or <c>-1</c> when the index
    /// is empty or <paramref name="sampleTarget"/> falls strictly
    /// before the first entry's <c>SampleOffset</c>. Targets past
    /// the last entry clamp to the last entry's index.
    /// </returns>
    public static int FindFrameAtSample(
        IReadOnlyList<AacAdtsFrameIndexEntry> index,
        long sampleTarget)
    {
        ArgumentNullException.ThrowIfNull(index);
        if (sampleTarget < 0)
        {
            throw new ArgumentOutOfRangeException(
                nameof(sampleTarget),
                "Sample target must be non-negative.");
        }

        int count = index.Count;
        if (count == 0) return -1;
        if (sampleTarget < index[0].SampleOffset) return -1;
        if (sampleTarget >= index[count - 1].SampleOffset) return count - 1;

        int lo = 0;
        int hi = count - 1;
        while (lo < hi)
        {
            int mid = lo + ((hi - lo + 1) >> 1);
            if (index[mid].SampleOffset <= sampleTarget) lo = mid;
            else hi = mid - 1;
        }
        return lo;
    }

    /// <summary>
    /// Convenience wrapper over <see cref="FindFrameAtSample"/>
    /// that converts a wall-clock <paramref name="time"/> into a
    /// sample target using <paramref name="sampleRate"/> before
    /// dispatching the binary search.
    /// </summary>
    /// <param name="index">Frame index; see <see cref="FindFrameAtSample"/>.</param>
    /// <param name="time">Target playback time. Negative values throw.</param>
    /// <param name="sampleRate">
    /// Sampling frequency (Hz) used to convert <paramref name="time"/>
    /// to a sample offset. Must be positive. When all frames share a
    /// single sample rate, callers typically pass
    /// <c>index[0].SampleRate</c>.
    /// </param>
    public static int FindFrameAtTime(
        IReadOnlyList<AacAdtsFrameIndexEntry> index,
        TimeSpan time,
        int sampleRate)
    {
        ArgumentNullException.ThrowIfNull(index);
        if (time < TimeSpan.Zero)
        {
            throw new ArgumentOutOfRangeException(
                nameof(time),
                "Target time must be non-negative.");
        }
        if (sampleRate <= 0)
        {
            throw new ArgumentOutOfRangeException(
                nameof(sampleRate),
                "Sample rate must be positive.");
        }

        long sampleTarget = (long)Math.Floor(time.TotalSeconds * sampleRate);
        return FindFrameAtSample(index, sampleTarget);
    }
}
