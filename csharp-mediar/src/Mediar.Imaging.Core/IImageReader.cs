namespace Mediar.Imaging;

/// <summary>
/// Common abstraction every image-format reader implements. Implementations
/// are stateful (positioned in the input stream) and are expected to be
/// short-lived: instantiate, enumerate, dispose.
/// </summary>
public interface IImageReader : IDisposable
{
    /// <summary>The concrete format being read.</summary>
    ImageFormat Format { get; }

    /// <summary>Image header (dimensions, bit depth, etc.).</summary>
    ImageInfo Info { get; }

    /// <summary>Title / EXIF / XMP / format-specific metadata.</summary>
    ImageMetadata Metadata { get; }

    /// <summary>True if pixel decoding is implemented for this image's encoding.</summary>
    bool CanDecodePixels { get; }

    /// <summary>
    /// Decode and yield each frame in order. For still images the sequence
    /// emits exactly one frame.
    /// </summary>
    /// <remarks>
    /// Caller must <see cref="IDisposable.Dispose"/> each yielded
    /// <see cref="ImageFrame"/> to return its pooled buffer.
    /// </remarks>
    IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default);
}

/// <summary>
/// Optional capability: an image writer that encodes <see cref="ImageFrame"/>
/// instances back into bytes.
/// </summary>
public interface IImageWriter : IAsyncDisposable
{
    /// <summary>The format being written.</summary>
    ImageFormat Format { get; }

    /// <summary>Append a frame; for still-image writers this must be called exactly once.</summary>
    ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default);

    /// <summary>Finalize the stream (write trailers, patch lengths, etc.).</summary>
    ValueTask FinishAsync(CancellationToken cancellationToken = default);
}

/// <summary>
/// Thrown by <see cref="IImageReader"/> implementations when the source is
/// malformed or claims an unsupported sub-encoding.
/// </summary>
public sealed class ImageFormatException : Exception
{
    /// <summary>Constructs an exception with the given message.</summary>
    public ImageFormatException(string message) : base(message) { }

    /// <summary>Constructs an exception with the given message + inner exception.</summary>
    public ImageFormatException(string message, Exception innerException) : base(message, innerException) { }
}
