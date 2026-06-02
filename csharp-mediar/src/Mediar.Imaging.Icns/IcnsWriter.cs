using System.Buffers.Binary;
using System.IO.Compression;

namespace Mediar.Imaging.Icns;

/// <summary>
/// Writer for Apple's ICNS icon container. Accepts one or more
/// <see cref="ImageFrame"/> instances and writes each as a PNG-encoded
/// sub-image inside a tagged icon entry. The tag is chosen automatically
/// from the frame dimensions, picking the standard "ic0X" / "ic1X" tags
/// that Apple uses for 16 / 32 / 64 / 128 / 256 / 512 / 1024 pixel icons;
/// non-standard sizes are rejected.
/// </summary>
public sealed class IcnsWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly List<(string Tag, byte[] Payload)> _entries = [];
    private bool _finished;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Icns;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    public IcnsWriter(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
    }

    /// <summary>Create an ICNS writer for <paramref name="path"/>.</summary>
    public static IcnsWriter Create(string path)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true);

    /// <inheritdoc/>
    public ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_finished) throw new InvalidOperationException("Cannot write frames after FinishAsync.");

        string tag = TagForSize(frame.Width, frame.Height)
            ?? throw new NotSupportedException(
                $"ICNS writer requires one of the standard square sizes (16/32/64/128/256/512/1024); got {frame.Width}x{frame.Height}.");
        byte[] png = EncodeFrameAsPng(frame);
        _entries.Add((tag, png));
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (_finished) return;
        _finished = true;
        if (_entries.Count == 0)
            throw new InvalidOperationException("ICNS file must contain at least one frame.");

        int payloadBytes = 0;
        foreach (var (_, p) in _entries) payloadBytes += 8 + p.Length;
        int total = 8 + payloadBytes;

        byte[] header = new byte[8];
        header[0] = (byte)'i'; header[1] = (byte)'c'; header[2] = (byte)'n'; header[3] = (byte)'s';
        BinaryPrimitives.WriteUInt32BigEndian(header.AsSpan(4), (uint)total);
        await _stream.WriteAsync(header, cancellationToken).ConfigureAwait(false);

        foreach (var (tag, payload) in _entries)
        {
            byte[] entry = new byte[8];
            for (int i = 0; i < 4; i++) entry[i] = (byte)tag[i];
            BinaryPrimitives.WriteUInt32BigEndian(entry.AsSpan(4), (uint)(payload.Length + 8));
            await _stream.WriteAsync(entry, cancellationToken).ConfigureAwait(false);
            await _stream.WriteAsync(payload, cancellationToken).ConfigureAwait(false);
        }
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished && _entries.Count > 0)
        {
            try { await FinishAsync().ConfigureAwait(false); }
            catch { /* swallow on dispose */ }
        }
        await _stream.FlushAsync().ConfigureAwait(false);
        if (_ownsStream) await _stream.DisposeAsync().ConfigureAwait(false);
    }

    /// <summary>
    /// Pick the Apple tag for a square icon. Returns <c>null</c> for any
    /// size outside the standard 16 / 32 / 64 / 128 / 256 / 512 / 1024 set.
    /// </summary>
    private static string? TagForSize(int w, int h) => (w, h) switch
    {
        (16, 16) => "icp4",
        (32, 32) => "icp5",
        (64, 64) => "icp6",
        (128, 128) => "ic07",
        (256, 256) => "ic08",
        (512, 512) => "ic09",
        (1024, 1024) => "ic10",
        _ => null,
    };

    // Mini PNG encoder dedicated to ICNS payloads. We don't take a hard
    // dependency on Mediar.Imaging.Png to keep the project graph shallow;
    // the format-spec is small enough to inline.
    private static byte[] EncodeFrameAsPng(ImageFrame frame)
    {
        using var ms = new MemoryStream();
        ms.Write([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A]);

        (byte bitDepth, byte colorType, int channels) = frame.PixelFormat switch
        {
            PixelFormat.Gray8 => ((byte)8, (byte)0, 1),
            PixelFormat.Rgb24 => ((byte)8, (byte)2, 3),
            PixelFormat.Rgba32 => ((byte)8, (byte)6, 4),
            _ => throw new NotSupportedException(
                "ICNS PNG sub-image must be Gray8, Rgb24, or Rgba32 — got " + frame.PixelFormat),
        };

        byte[] ihdr = new byte[13];
        BinaryPrimitives.WriteUInt32BigEndian(ihdr.AsSpan(0, 4), (uint)frame.Width);
        BinaryPrimitives.WriteUInt32BigEndian(ihdr.AsSpan(4, 4), (uint)frame.Height);
        ihdr[8] = bitDepth;
        ihdr[9] = colorType;
        WriteChunk(ms, "IHDR"u8, ihdr);

        int rowBytes = frame.Width * channels;
        using var raw = new MemoryStream(checked((rowBytes + 1) * frame.Height));
        ReadOnlySpan<byte> px = frame.Pixels.Span;
        for (int y = 0; y < frame.Height; y++)
        {
            raw.WriteByte(0);
            raw.Write(px.Slice(y * frame.Stride, rowBytes));
        }
        WriteChunk(ms, "IDAT"u8, ZlibCompress(raw.ToArray()));
        WriteChunk(ms, "IEND"u8, []);
        return ms.ToArray();
    }

    private static void WriteChunk(Stream s, ReadOnlySpan<byte> type, ReadOnlySpan<byte> payload)
    {
        Span<byte> len = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(len, (uint)payload.Length);
        s.Write(len);
        s.Write(type);
        s.Write(payload);
        uint crc = Crc32(0xFFFFFFFFu, type);
        crc = Crc32(crc, payload);
        crc ^= 0xFFFFFFFFu;
        Span<byte> c = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(c, crc);
        s.Write(c);
    }

    private static readonly uint[] s_crcTable = BuildCrcTable();
    private static uint[] BuildCrcTable()
    {
        var t = new uint[256];
        for (uint n = 0; n < 256; n++)
        {
            uint c = n;
            for (int k = 0; k < 8; k++) c = (c & 1) != 0 ? 0xEDB88320u ^ (c >> 1) : c >> 1;
            t[n] = c;
        }
        return t;
    }
    private static uint Crc32(uint seed, ReadOnlySpan<byte> data)
    {
        uint c = seed;
        for (int i = 0; i < data.Length; i++) c = s_crcTable[(c ^ data[i]) & 0xFF] ^ (c >> 8);
        return c;
    }

    private static byte[] ZlibCompress(byte[] raw)
    {
        using var ms = new MemoryStream();
        using (var zs = new ZLibStream(ms, CompressionLevel.Optimal, leaveOpen: true))
        {
            zs.Write(raw, 0, raw.Length);
        }
        return ms.ToArray();
    }
}
