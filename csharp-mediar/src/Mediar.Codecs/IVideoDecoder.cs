namespace Mediar.Codecs;

/// <summary>Pixel layouts produced by video decoders.</summary>
public enum PixelFormat
{
    /// <summary>Unknown / unspecified.</summary>
    Unknown = 0,

    /// <summary>Planar Y'CbCr 4:2:0 8-bit (3 planes, Cb/Cr half-width and half-height).</summary>
    Yuv420p,

    /// <summary>Planar Y'CbCr 4:2:2 8-bit (3 planes, Cb/Cr half-width).</summary>
    Yuv422p,

    /// <summary>Planar Y'CbCr 4:4:4 8-bit (3 planes, full chroma resolution).</summary>
    Yuv444p,

    /// <summary>Packed RGB 8-bit.</summary>
    Rgb24,

    /// <summary>Packed RGBA 8-bit.</summary>
    Rgba32,
}

/// <summary>
/// Decoded video frame. Each plane is a contiguous region of memory; stride
/// may be larger than width to allow SIMD alignment. The owner controls the
/// lifetime of all plane buffers.
/// </summary>
public readonly struct DecodedVideoFrame : IDisposable
{
    /// <summary>Pixel layout.</summary>
    public PixelFormat PixelFormat { get; init; }

    /// <summary>Pixel width.</summary>
    public int Width { get; init; }

    /// <summary>Pixel height.</summary>
    public int Height { get; init; }

    /// <summary>Up to 4 planes of sample data; unused planes are <see cref="ReadOnlyMemory{Byte}.Empty"/>.</summary>
    public ReadOnlyMemory<byte> Plane0 { get; init; }

    /// <summary>Stride (bytes per row) of plane 0; ≥ width * bytes-per-sample.</summary>
    public int Stride0 { get; init; }

    /// <summary>Plane 1 (Cb / U for YUV, unused for RGB).</summary>
    public ReadOnlyMemory<byte> Plane1 { get; init; }

    /// <summary>Stride of plane 1.</summary>
    public int Stride1 { get; init; }

    /// <summary>Plane 2 (Cr / V for YUV).</summary>
    public ReadOnlyMemory<byte> Plane2 { get; init; }

    /// <summary>Stride of plane 2.</summary>
    public int Stride2 { get; init; }

    /// <summary>Presentation timestamp in track time-base units.</summary>
    public long Pts { get; init; }

    /// <summary>True if this is a keyframe.</summary>
    public bool IsKeyFrame { get; init; }

    /// <summary>Owner of the plane buffers; dispose when consumption is complete.</summary>
    public IDisposable? Owner { get; init; }

    /// <inheritdoc/>
    public void Dispose() => Owner?.Dispose();
}

/// <summary>
/// Streaming video decoder. Implementations consume compressed encoded
/// packets (one <see cref="MediaSample"/> per call) and emit zero or more
/// decoded frames. Decoders are not thread-safe.
/// </summary>
/// <remarks>
/// <para>
/// Mediar deliberately does not ship implementations of H.264, H.265, H.266
/// or other patent-encumbered video codecs. This interface exists so that
/// callers can plug their own decoder (or a third-party permissively-licensed
/// one) into Mediar pipelines.
/// </para>
/// </remarks>
public interface IVideoDecoder : IDisposable
{
    /// <summary>Codec identifier this decoder implements.</summary>
    CodecId Codec { get; }

    /// <summary>Codec parameters this decoder was initialized with.</summary>
    VideoCodecParameters Parameters { get; }

    /// <summary>Decode one encoded packet and return the decoded video frame, if available.</summary>
    DecodedVideoFrame Decode(ReadOnlySpan<byte> encoded, long pts, bool isKeyFrame);

    /// <summary>Reset internal decoder state (after a seek).</summary>
    void Reset();
}

/// <summary>Factory abstraction matching <see cref="IAudioDecoderFactory"/> but for video.</summary>
public interface IVideoDecoderFactory
{
    /// <summary>True if this factory can instantiate a decoder for <paramref name="codec"/>.</summary>
    bool Supports(CodecId codec);

    /// <summary>Create a decoder for the given codec parameters.</summary>
    IVideoDecoder Create(VideoCodecParameters parameters);
}
