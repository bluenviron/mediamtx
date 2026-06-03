namespace Mediar.Imaging.Jpeg;

/// <summary>
/// <see cref="IImageWriter"/> wrapper around <see cref="JpegBaselineEncoder"/>.
/// Buffers exactly one frame, then commits it to the backing stream on
/// <see cref="FinishAsync(CancellationToken)"/>.
/// </summary>
/// <remarks>
/// Exposes quality, chroma subsampling, restart-interval and optimised-
/// Huffman flag knobs via <see cref="JpegEncodeOptions"/>. Metadata
/// (EXIF / ICC / XMP) round-trip is delegated to
/// <see cref="JpegMetadataWriter"/>.
/// </remarks>
public sealed class JpegWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly JpegEncodeOptions _options;
    private ImageFrame? _pending;
    private bool _finished;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Jpeg;

    /// <summary>
    /// Construct a writer over <paramref name="stream"/> with the given encode options.
    /// </summary>
    public JpegWriter(Stream stream, JpegEncodeOptions? options = null, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
        _options = options ?? new JpegEncodeOptions();
    }

    /// <inheritdoc/>
    public ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_pending is not null)
        {
            throw new InvalidOperationException("JpegWriter is a still-image writer; only one frame is allowed.");
        }
        cancellationToken.ThrowIfCancellationRequested();
        _pending = frame;
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (_finished) return ValueTask.CompletedTask;
        _finished = true;
        if (_pending is null)
        {
            throw new InvalidOperationException("JpegWriter.FinishAsync called without any frame.");
        }
        cancellationToken.ThrowIfCancellationRequested();
        JpegBaselineEncoder.Encode(_pending, _stream, _options);
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (!_finished)
        {
            await FinishAsync().ConfigureAwait(false);
        }
        if (_ownsStream)
        {
            await _stream.DisposeAsync().ConfigureAwait(false);
        }
    }
}
