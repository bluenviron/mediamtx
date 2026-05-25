using System.Buffers;

namespace Mediar;

/// <summary>
/// A single demuxed access unit (one frame for video, one packet for audio/subtitle).
/// </summary>
/// <remarks>
/// The payload is stored as <see cref="ReadOnlyMemory{Byte}"/> so it can be a pooled
/// buffer, a slice of a memory-mapped file, or freshly allocated. When the sample owns
/// a pooled buffer, the demuxer is responsible for returning it to the pool when the
/// caller has consumed the sample; the recommended pattern is to dispose the
/// <see cref="IMemoryOwner{Byte}"/> after copying out the payload.
/// </remarks>
public sealed record MediaSample
{
    /// <summary>Track index this sample belongs to (matches <see cref="MediaTrack.Index"/>).</summary>
    public required int TrackIndex { get; init; }

    /// <summary>Presentation timestamp in the track's time-base.</summary>
    public required long Pts { get; init; }

    /// <summary>Decode timestamp in the track's time-base. Equals <see cref="Pts"/> for tracks without b-frames.</summary>
    public required long Dts { get; init; }

    /// <summary>Duration of the sample in the track's time-base. 0 if unknown.</summary>
    public required int Duration { get; init; }

    /// <summary>True if this sample is independently decodable (key frame / sync sample).</summary>
    public bool IsKeyFrame { get; init; }

    /// <summary>The raw access-unit bytes (codec-specific layout).</summary>
    public required ReadOnlyMemory<byte> Data { get; init; }

    /// <summary>
    /// Optional owner of the buffer backing <see cref="Data"/>. When non-null, dispose
    /// once the sample's data is no longer needed.
    /// </summary>
    public IMemoryOwner<byte>? Owner { get; init; }

    /// <inheritdoc/>
    public override string ToString() =>
        $"trk={TrackIndex} pts={Pts} dts={Dts} dur={Duration} key={IsKeyFrame} len={Data.Length}";
}
