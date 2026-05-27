using System.Buffers.Binary;
using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Icns;

/// <summary>
/// Reader for the Apple ICNS icon container. Emits one
/// <see cref="ImageFrame"/> per sub-image; PNG/JPEG2000 sub-images are
/// emitted as raw payload frames (caller must decode separately).
/// </summary>
public sealed class IcnsReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly byte[] _bytes;
    private readonly IcnsEntry[] _entries;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Icns;

    /// <inheritdoc/>
    public ImageInfo Info { get; }

    /// <inheritdoc/>
    public ImageMetadata Metadata => ImageMetadata.Empty;

    /// <inheritdoc/>
    public bool CanDecodePixels => _entries.Length > 0;

    /// <summary>The raw list of icon sub-images contained in this file.</summary>
    public IReadOnlyList<IcnsEntry> Entries => _entries;

    private IcnsReader(Stream s, bool owns, byte[] b, IcnsEntry[] entries, ImageInfo info)
    {
        _stream = s; _ownsStream = owns; _bytes = b; _entries = entries; Info = info;
    }

    /// <summary>Open an ICNS file by path.</summary>
    public static IcnsReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read,
                                FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open an ICNS from a stream.</summary>
    public static IcnsReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        var bytes = ms.ToArray();
        if (bytes.Length < 8 ||
            bytes[0] != (byte)'i' || bytes[1] != (byte)'c' ||
            bytes[2] != (byte)'n' || bytes[3] != (byte)'s')
        {
            throw new ImageFormatException("Not an ICNS file.");
        }
        uint total = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(4));
        if (total > bytes.Length) total = (uint)bytes.Length;

        var entries = new List<IcnsEntry>();
        int p = 8;
        while (p + 8 <= total)
        {
            string tag = System.Text.Encoding.ASCII.GetString(bytes, p, 4);
            int len = (int)BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(p + 4));
            if (len < 8 || p + len > total) break;
            entries.Add(new IcnsEntry(tag, p + 8, len - 8, ClassifyTag(tag)));
            p += len;
        }

        int widest = 0;
        foreach (var e in entries) if (e.Size.W > widest) widest = e.Size.W;

        var biggest = widest > 0 ? entries.First(e => e.Size.W == widest) : default;
        var info = new ImageInfo
        {
            Width = biggest.Size.W,
            Height = biggest.Size.H,
            BitsPerPixel = 32,
            ChannelCount = 4,
            PixelFormat = PixelFormat.Rgba32,
            Format = ImageFormat.Icns,
            HasAlpha = true,
            FrameCount = entries.Count,
        };

        return new IcnsReader(stream, ownsStream, bytes, [.. entries], info);
    }

    /// <inheritdoc/>
    public async IAsyncEnumerable<ImageFrame> ReadFramesAsync(
        [EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        await Task.CompletedTask.ConfigureAwait(false);
        cancellationToken.ThrowIfCancellationRequested();
        foreach (var e in _entries)
        {
            if (e.Size.W <= 0) continue;
            int w = e.Size.W, h = e.Size.H;
            // PNG / JP2 sub-images: detect by magic.
            ReadOnlySpan<byte> payload = _bytes.AsSpan(e.Offset, e.Length);
            if (payload.Length >= 8 && payload[0] == 0x89 && payload[1] == 0x50 &&
                payload[2] == 0x4E && payload[3] == 0x47)
            {
                // Emit as a 1×N pass-through with the raw PNG payload in the buffer.
                yield return EmitRawPayload(payload, ImageFormat.Png);
                continue;
            }
            if (payload.Length >= 8 &&
                payload[0] == 0x00 && payload[1] == 0x00 &&
                payload[2] == 0x00 && payload[3] == 0x0C &&
                payload[4] == (byte)'j' && payload[5] == (byte)'P' &&
                payload[6] == (byte)' ' && payload[7] == (byte)' ')
            {
                yield return EmitRawPayload(payload, ImageFormat.Jp2);
                continue;
            }
            // Otherwise: 32-bit ARGB icon (icp4/icp5/icp6/ic07-…) is the only
            // legacy path implemented here, treated as ARGB raw of w*h*4 bytes.
            if (payload.Length == w * h * 4)
            {
                int stride = w * 4;
                var (frame, buf) = ImageFrame.Rent(w, h, PixelFormat.Argb32, stride);
                Buffer.BlockCopy(_bytes, e.Offset, buf, 0, payload.Length);
                yield return frame;
            }
        }
    }

    private ImageFrame EmitRawPayload(ReadOnlySpan<byte> payload, ImageFormat subFmt)
    {
        var buf = new byte[Math.Max(1, payload.Length)];
        payload.CopyTo(buf);
        // Width/height = encoded payload size in a 1-row layout — caller is
        // expected to inspect via the metadata channel.
        return new ImageFrame(payload.Length, 1, PixelFormat.Unknown,
            payload.Length, buf, buf)
        {
            OffsetX = 0,
            OffsetY = (int)subFmt,
        };
    }

    private static (int W, int H) ClassifyTag(string tag) => tag switch
    {
        "ICON" => (32, 32),
        "ICN#" => (32, 32),
        "icm#" or "icm4" or "icm8" => (16, 12),
        "ics#" or "ics4" or "ics8" or "is32" or "s8mk" => (16, 16),
        "icl4" or "icl8" or "il32" or "l8mk" => (32, 32),
        "ich#" or "ich4" or "ich8" or "ih32" or "h8mk" => (48, 48),
        "it32" or "t8mk" => (128, 128),
        "icp4" => (16, 16),
        "icp5" => (32, 32),
        "icp6" => (64, 64),
        "ic07" => (128, 128),
        "ic08" => (256, 256),
        "ic09" => (512, 512),
        "ic10" => (1024, 1024),
        "ic11" => (32, 32),
        "ic12" => (64, 64),
        "ic13" => (256, 256),
        "ic14" => (512, 512),
        _ => (0, 0),
    };

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }
}

/// <summary>One sub-image entry inside an ICNS file.</summary>
/// <param name="Tag">The 4-character tag (e.g. "ic08").</param>
/// <param name="Offset">Byte offset of the payload in the file.</param>
/// <param name="Length">Payload length, in bytes.</param>
/// <param name="Size">Nominal (width, height) of this sub-image.</param>
public readonly record struct IcnsEntry(string Tag, int Offset, int Length, (int W, int H) Size);
