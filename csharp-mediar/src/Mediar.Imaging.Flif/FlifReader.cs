namespace Mediar.Imaging.Flif;

/// <summary>
/// Reader for FLIF (Free Lossless Image Format) files. Parses the FLIF
/// main header: signature, interlacing flag, channel count, bit depth,
/// and the variable-length-encoded width / height / frame count. MANIAC
/// tree / range-coded entropy decode is not implemented.
/// </summary>
public sealed class FlifReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Flif;
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }
    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>True if the FLIF stream is interlaced (per the header letter case).</summary>
    public bool IsInterlaced { get; }
    /// <summary>Bit depth code: 0=unknown, 1=8-bit, 2=16-bit.</summary>
    public int BitDepthCode { get; }
    /// <summary>Channel count (1=Gray, 3=RGB, 4=RGBA).</summary>
    public int Channels { get; }
    /// <summary>Frame count for animated FLIFs (1 for stills).</summary>
    public int NumFrames { get; }

    private FlifReader(Stream s, bool owns, ImageInfo info, ImageMetadata meta,
                       bool interlaced, int bitDepth, int channels, int numFrames)
    {
        _stream = s; _ownsStream = owns;
        Info = info; Metadata = meta;
        IsInterlaced = interlaced; BitDepthCode = bitDepth;
        Channels = channels; NumFrames = numFrames;
    }

    /// <summary>Open a FLIF file from a path.</summary>
    public static FlifReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a FLIF file from a stream.</summary>
    public static FlifReader Open(Stream stream, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] b = ms.ToArray();

        if (b.Length < 6 || b[0] != (byte)'F' || b[1] != (byte)'L' || b[2] != (byte)'I' || b[3] != (byte)'F')
            throw new ImageFormatException("Not a FLIF file (missing 'FLIF' signature).");

        byte flags = b[4];
        // High nibble: I=interlaced ('A'-'P' = non-animated, 'Q'-'Z' = animated, etc.)
        // Per spec: byte 5 = animation+channels packed; high nibble = animation, low = channels
        bool interlaced = (flags & 0x20) == 0;  // ASCII 'A'..'P' have bit 5 set when uppercase
        int channels = flags & 0x0F;
        if (channels == 0) channels = 3;

        byte bdByte = b[5];
        int bdCode = bdByte switch
        {
            (byte)'0' => 0,
            (byte)'1' => 1,
            (byte)'2' => 2,
            _ => 1,
        };

        int p = 6;
        int w = ReadVarInt(b, ref p) + 1;
        int h = ReadVarInt(b, ref p) + 1;
        int frames = ReadVarInt(b, ref p) + 1;

        var info = new ImageInfo
        {
            Width = w,
            Height = h,
            BitsPerPixel = (bdCode == 2 ? 16 : 8) * channels,
            ChannelCount = channels,
            HasAlpha = channels == 4 || channels == 2,
            Format = ImageFormat.Flif,
            FrameCount = frames,
            IsAnimated = frames > 1,
        };
        return new FlifReader(stream, ownsStream, info, ImageMetadata.Empty,
                               interlaced, bdCode, channels, frames);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            "FLIF MANIAC tree and entropy decoding are not implemented in this Mediar release. " +
            "Header (dimensions, channels, bit-depth, frame count) is exposed for inspection.");

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    private static int ReadVarInt(byte[] b, ref int p)
    {
        int v = 0;
        while (p < b.Length)
        {
            byte by = b[p++];
            v = (v << 7) | (by & 0x7F);
            if ((by & 0x80) == 0) break;
        }
        return v;
    }
}
