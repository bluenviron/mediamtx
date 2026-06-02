using System.Buffers.Binary;
using System.IO.Compression;
using Mediar.Codecs.Lzw;
using Mediar.Codecs.PackBits;

namespace Mediar.Imaging.Tiff;

/// <summary>Compression mode used by <see cref="TiffWriter"/>.</summary>
public enum TiffWriterCompression
{
    /// <summary>No compression — Compression tag 1.</summary>
    None = 1,
    /// <summary>LZW compression — Compression tag 5 (TIFF dialect).</summary>
    Lzw = 5,
    /// <summary>Deflate / Zlib — Compression tag 8.</summary>
    Deflate = 8,
    /// <summary>PackBits — Compression tag 32773.</summary>
    PackBits = 32773,
}

/// <summary>
/// Writer for a single-page, single-strip, little-endian baseline TIFF.
/// Supports <see cref="PixelFormat.Gray8"/>, <see cref="PixelFormat.Gray16"/>,
/// <see cref="PixelFormat.Rgb24"/>, and <see cref="PixelFormat.Rgba32"/> input
/// with optional Deflate, LZW, or PackBits compression. Output layout:
/// 8-byte header → IFD → side tables (BitsPerSample, XResolution, YResolution)
/// → raw / encoded pixel strip.
/// </summary>
public sealed class TiffWriter : IImageWriter
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly TiffWriterCompression _compression;
    private bool _wrote;
    private bool _finished;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Tiff;

    /// <summary>
    /// Construct a writer over <paramref name="stream"/>.
    /// <paramref name="compression"/> controls the strip encoding; default is
    /// <see cref="TiffWriterCompression.Deflate"/> which keeps files small
    /// while remaining universally supported.
    /// </summary>
    public TiffWriter(Stream stream,
                      bool ownsStream = false,
                      TiffWriterCompression compression = TiffWriterCompression.Deflate)
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        _stream = stream;
        _ownsStream = ownsStream;
        _compression = compression;
    }

    /// <summary>Create a writer that emits to <paramref name="path"/>.</summary>
    public static TiffWriter Create(string path,
                                    TiffWriterCompression compression = TiffWriterCompression.Deflate)
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true, compression);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_wrote) throw new InvalidOperationException("TIFF writer currently supports a single frame per file.");
        _wrote = true;

        (int spp, int bps, int photometric, bool hasAlpha) = frame.PixelFormat switch
        {
            PixelFormat.Gray8 => (1, 8, 1, false),
            PixelFormat.Gray16 => (1, 16, 1, false),
            PixelFormat.Rgb24 => (3, 8, 2, false),
            PixelFormat.Rgba32 => (4, 8, 2, true),
            _ => throw new NotSupportedException(
                "TIFF writer supports Gray8, Gray16, Rgb24, and Rgba32 — got " + frame.PixelFormat),
        };

        int rowBytes = frame.Width * spp * (bps / 8);
        byte[] raw = new byte[rowBytes * frame.Height];
        ReadOnlySpan<byte> src = frame.Pixels.Span;
        for (int y = 0; y < frame.Height; y++)
        {
            src.Slice(y * frame.Stride, rowBytes).CopyTo(raw.AsSpan(y * rowBytes));
        }

        byte[] strip = _compression switch
        {
            TiffWriterCompression.None => raw,
            TiffWriterCompression.Deflate => DeflateCompress(raw),
            TiffWriterCompression.PackBits => PackBitsByRow(raw, rowBytes, frame.Height),
            TiffWriterCompression.Lzw => LzwEncoder.EncodeTiff(raw),
            _ => throw new InvalidOperationException("Unknown TIFF writer compression: " + _compression),
        };

        // Compose IFD entries — keep the tag order ascending which is a
        // baseline-TIFF MUST. Any value that exceeds 4 bytes lives in a
        // side table written immediately after the IFD.
        bool needBpsTable = spp > 1; // BitsPerSample[1] fits inline, [N>1] needs offset.
        int bpsTableSize = needBpsTable ? spp * 2 : 0;
        const int rationalSize = 8; // 2x4 bytes per rational.
        int extraSamplesInline = hasAlpha ? 1 : 0; // ExtraSamples = [2] fits in 4 bytes (short).

        int entryCount = 12 + (hasAlpha ? 1 : 0); // base entries + optional ExtraSamples
        int ifdSize = 2 + (entryCount * 12) + 4;
        int sideTablesSize = bpsTableSize + (rationalSize * 2); // XRes + YRes
        int headerSize = 8;
        int stripOffset = headerSize + ifdSize + sideTablesSize;

        await _stream.WriteAsync(BuildHeader(headerSize, ifdSize, sideTablesSize, entryCount,
                                              frame.Width, frame.Height, bps, spp, photometric,
                                              strip.Length, stripOffset, needBpsTable, hasAlpha),
                                  cancellationToken).ConfigureAwait(false);
        await _stream.WriteAsync(strip, cancellationToken).ConfigureAwait(false);
        _ = extraSamplesInline; // referenced for clarity above
    }

    /// <inheritdoc/>
    public ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        _finished = true;
        return ValueTask.CompletedTask;
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        if (!_finished) await FinishAsync().ConfigureAwait(false);
        await _stream.FlushAsync().ConfigureAwait(false);
        if (_ownsStream) await _stream.DisposeAsync().ConfigureAwait(false);
    }

    private byte[] BuildHeader(int headerSize, int ifdSize, int sideTablesSize, int entryCount,
                               int width, int height, int bps, int spp, int photometric,
                               int stripLen, int stripOffset, bool needBpsTable, bool hasAlpha)
    {
        byte[] buf = new byte[headerSize + ifdSize + sideTablesSize];
        var span = buf.AsSpan();

        // Header: little-endian magic + IFD pointer.
        span[0] = (byte)'I'; span[1] = (byte)'I';
        BinaryPrimitives.WriteUInt16LittleEndian(span.Slice(2), 42);
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(4), (uint)headerSize);

        // IFD lives at offset 8 (= headerSize).
        int p = headerSize;
        BinaryPrimitives.WriteUInt16LittleEndian(span.Slice(p), (ushort)entryCount); p += 2;

        // Side tables come after the IFD.
        int bpsOffset = headerSize + ifdSize;
        int xresOffset = bpsOffset + (needBpsTable ? spp * 2 : 0);
        int yresOffset = xresOffset + 8;

        WriteEntry(span, ref p, 0x0100, type: 3, count: 1, value: (uint)width);            // ImageWidth (SHORT)
        WriteEntry(span, ref p, 0x0101, type: 3, count: 1, value: (uint)height);           // ImageLength (SHORT)

        // BitsPerSample (SHORT[spp]) — inline if spp==1, else offset to table.
        if (needBpsTable)
            WriteEntry(span, ref p, 0x0102, type: 3, count: (uint)spp, value: (uint)bpsOffset);
        else
            WriteEntryShortInline(span, ref p, 0x0102, (ushort)bps);

        WriteEntry(span, ref p, 0x0103, type: 3, count: 1, value: (uint)_compression);     // Compression
        WriteEntry(span, ref p, 0x0106, type: 3, count: 1, value: (uint)photometric);      // Photometric
        WriteEntry(span, ref p, 0x0111, type: 4, count: 1, value: (uint)stripOffset);      // StripOffsets (LONG)
        WriteEntry(span, ref p, 0x0115, type: 3, count: 1, value: (uint)spp);              // SamplesPerPixel
        WriteEntry(span, ref p, 0x0116, type: 3, count: 1, value: (uint)height);           // RowsPerStrip
        WriteEntry(span, ref p, 0x0117, type: 4, count: 1, value: (uint)stripLen);         // StripByteCounts (LONG)
        WriteEntry(span, ref p, 0x011A, type: 5, count: 1, value: (uint)xresOffset);       // XResolution (RATIONAL)
        WriteEntry(span, ref p, 0x011B, type: 5, count: 1, value: (uint)yresOffset);       // YResolution (RATIONAL)
        WriteEntry(span, ref p, 0x0128, type: 3, count: 1, value: 2);                      // ResolutionUnit = inch
        if (hasAlpha)
            WriteEntryShortInline(span, ref p, 0x0152, value: 2);                          // ExtraSamples = unassociated alpha

        // next-IFD pointer
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(p), 0u); p += 4;

        // Side tables
        if (needBpsTable)
        {
            for (int i = 0; i < spp; i++)
                BinaryPrimitives.WriteUInt16LittleEndian(span.Slice(p + i * 2), (ushort)bps);
            p += spp * 2;
        }
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(p + 0), 72);
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(p + 4), 1);
        p += 8;
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(p + 0), 72);
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(p + 4), 1);

        return buf;
    }

    private static void WriteEntry(Span<byte> dst, ref int p, ushort tag, ushort type, uint count, uint value)
    {
        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(p + 0), tag);
        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(p + 2), type);
        BinaryPrimitives.WriteUInt32LittleEndian(dst.Slice(p + 4), count);
        BinaryPrimitives.WriteUInt32LittleEndian(dst.Slice(p + 8), value);
        p += 12;
    }

    private static void WriteEntryShortInline(Span<byte> dst, ref int p, ushort tag, ushort value)
    {
        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(p + 0), tag);
        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(p + 2), 3);     // SHORT
        BinaryPrimitives.WriteUInt32LittleEndian(dst.Slice(p + 4), 1u);
        // SHORT inline lives in the low 2 bytes of the value field; pad the rest.
        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(p + 8), value);
        BinaryPrimitives.WriteUInt16LittleEndian(dst.Slice(p + 10), 0);
        p += 12;
    }

    private static byte[] DeflateCompress(byte[] raw)
    {
        using var ms = new MemoryStream();
        using (var z = new ZLibStream(ms, CompressionLevel.Optimal, leaveOpen: true))
        {
            z.Write(raw, 0, raw.Length);
        }
        return ms.ToArray();
    }

    private static byte[] PackBitsByRow(byte[] raw, int rowBytes, int height)
    {
        // TIFF/PackBits requires per-row packets so the decoder can resync on
        // any row boundary even when strips are broken into chunks.
        using var ms = new MemoryStream(raw.Length);
        for (int y = 0; y < height; y++)
        {
            byte[] enc = PackBitsCodec.Encode(raw.AsSpan(y * rowBytes, rowBytes));
            ms.Write(enc, 0, enc.Length);
        }
        return ms.ToArray();
    }
}
