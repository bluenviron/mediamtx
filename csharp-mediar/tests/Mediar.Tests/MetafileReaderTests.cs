using System.Buffers.Binary;
using System.IO.Compression;
using Mediar.Imaging;
using Mediar.Imaging.Metafiles;
using Xunit;

namespace Mediar.Tests;

public class MetafileReaderTests
{
    [Fact]
    public void Parses_Emf_Header()
    {
        byte[] file = BuildEmf(left: 0, top: 0, right: 1920, bottom: 1080);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);

        Assert.Equal(ImageFormat.Emf, r.Format);
        Assert.Equal(1920, r.Info.Width);
        Assert.Equal(1080, r.Info.Height);
        Assert.Equal((0, 0, 1920, 1080), r.Bounds);
        Assert.True(r.Records.Length >= 1);
        Assert.Equal(1, r.Records[0].RecordType);  // EMR_HEADER
    }

    [Fact]
    public void Parses_Wmf_Header()
    {
        byte[] file = BuildWmf();
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Wmf, ownsStream: true);

        Assert.Equal(ImageFormat.Wmf, r.Format);
        Assert.False(r.IsPlaceable);
    }

    [Fact]
    public void Parses_Apm_Placeable_Wmf()
    {
        byte[] file = BuildApm(left: 0, top: 0, right: 14400, bottom: 7200, inch: 1440);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Apm, ownsStream: true);

        Assert.True(r.IsPlaceable);
        Assert.Equal(14400, r.Info.Width);
        Assert.Equal(7200, r.Info.Height);
    }

    [Fact]
    public void Unwraps_Emz_Gzipped_Emf()
    {
        byte[] inner = BuildEmf(0, 0, 100, 50);
        using var ms = new MemoryStream();
        using (var gz = new GZipStream(ms, CompressionLevel.Optimal, leaveOpen: true))
            gz.Write(inner);
        ms.Position = 0;

        using var r = MetafileReader.Open(ms, ImageFormat.Emz, ownsStream: true);
        Assert.True(r.WasCompressed);
        Assert.Equal(ImageFormat.Emf, r.Format);
        Assert.Equal(100, r.Info.Width);
    }

    [Fact]
    public async Task ReadFramesAsync_RendersEmptyEmf()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        int frameCount = 0;
        await foreach (var f in r.ReadFramesAsync())
        {
            Assert.True(f.Width > 0);
            Assert.True(f.Height > 0);
            f.Dispose();
            frameCount++;
        }
        Assert.Equal(1, frameCount);
    }

    private static byte[] BuildEmf(int left, int top, int right, int bottom)
    {
        using var ms = new MemoryStream();
        // EMR_HEADER (type 1) — size 88 bytes is the smallest valid header.
        Span<byte> hdr = stackalloc byte[88];
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(0, 4), 1);   // record type
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(4, 4), 88);  // size
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(8, 4), left);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(12, 4), top);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(16, 4), right);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(20, 4), bottom);
        ms.Write(hdr);
        // EMR_EOF (type 14)
        Span<byte> eof = stackalloc byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(eof.Slice(0, 4), 14);
        BinaryPrimitives.WriteUInt32LittleEndian(eof.Slice(4, 4), 20);
        ms.Write(eof);
        return ms.ToArray();
    }

    private static byte[] BuildWmf()
    {
        using var ms = new MemoryStream();
        // META_HEADER (18 bytes minimum)
        Span<byte> hdr = stackalloc byte[18];
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(0, 2), 1);     // Type = MemoryMetafile
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(2, 2), 9);     // HeaderSize in words (18 / 2)
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(4, 2), 0x0300); // Version
        ms.Write(hdr);
        // META_EOF: 3 words = 6 bytes (size in words + function 0)
        Span<byte> eof = stackalloc byte[6];
        BinaryPrimitives.WriteUInt32LittleEndian(eof.Slice(0, 4), 3);
        BinaryPrimitives.WriteUInt16LittleEndian(eof.Slice(4, 2), 0);
        ms.Write(eof);
        return ms.ToArray();
    }

    private static byte[] BuildApm(short left, short top, short right, short bottom, ushort inch)
    {
        using var ms = new MemoryStream();
        // Placeable header (22 bytes)
        Span<byte> ph = stackalloc byte[22];
        BinaryPrimitives.WriteUInt32LittleEndian(ph.Slice(0, 4), 0x9AC6CDD7);
        BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(6, 2), left);
        BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(8, 2), top);
        BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(10, 2), right);
        BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(12, 2), bottom);
        BinaryPrimitives.WriteUInt16LittleEndian(ph.Slice(14, 2), inch);
        ms.Write(ph);
        ms.Write(BuildWmf());
        return ms.ToArray();
    }

    [Fact]
    public void Open_Stream_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => MetafileReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Emf_TooShort_Throws()
    {
        using var ms = new MemoryStream(new byte[40]);
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Emf, ownsStream: true));
    }

    [Fact]
    public void Open_Emf_WrongFirstRecord_Throws()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        // Overwrite the first record type to something other than 1 (EMR_HEADER).
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(0, 4), 5);
        using var ms = new MemoryStream(file);
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Emf, ownsStream: true));
    }

    [Fact]
    public void Open_Wmf_TooShort_Throws()
    {
        using var ms = new MemoryStream(new byte[10]);
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Wmf, ownsStream: true));
    }

    [Fact]
    public void Open_Wmf_WrongType_Throws()
    {
        byte[] file = BuildWmf();
        BinaryPrimitives.WriteUInt16LittleEndian(file.AsSpan(0, 2), 7); // invalid type field
        using var ms = new MemoryStream(file);
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Wmf, ownsStream: true));
    }

    [Fact]
    public void Open_Apm_NoMagicKey_Throws()
    {
        byte[] file = new byte[64]; // empty bytes, no APM magic
        using var ms = new MemoryStream(file);
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Apm, ownsStream: true));
    }

    [Fact]
    public void Open_Apm_BodyTooShort_Throws()
    {
        byte[] file = new byte[30]; // has 22-byte preamble room, but no WMF body
        BinaryPrimitives.WriteUInt32LittleEndian(file.AsSpan(0, 4), 0x9AC6CDD7);
        using var ms = new MemoryStream(file);
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Apm, ownsStream: true));
    }

    [Fact]
    public void Apm_With_Zero_Inch_Has_Zero_Dpi()
    {
        byte[] file = BuildApm(0, 0, 1440, 720, inch: 0);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Apm, ownsStream: true);
        Assert.Equal(0, r.Info.HorizontalDpi);
        Assert.Equal(0, r.Info.VerticalDpi);
    }

    [Fact]
    public void Apm_Dpi_Calculated_From_Inch()
    {
        byte[] file = BuildApm(0, 0, 14400, 7200, inch: 1440);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Apm, ownsStream: true);
        // dx = (14400 - 0) / 1440 = 10 -> 10 * 96 = 960 dpi
        Assert.Equal(960.0, r.Info.HorizontalDpi, precision: 1);
        Assert.Equal(480.0, r.Info.VerticalDpi, precision: 1);
    }

    [Fact]
    public void Emz_Wraps_NonCompressed_Body_NotMarkedCompressed()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        // Reading EMZ format from non-gzip bytes should still parse (not detected as compressed).
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emz, ownsStream: true);
        Assert.False(r.WasCompressed);
    }

    [Fact]
    public void Unwraps_Wmz_Gzipped_Wmf()
    {
        byte[] inner = BuildWmf();
        using var ms = new MemoryStream();
        using (var gz = new GZipStream(ms, CompressionLevel.Optimal, leaveOpen: true))
            gz.Write(inner);
        ms.Position = 0;

        using var r = MetafileReader.Open(ms, ImageFormat.Wmz, ownsStream: true);
        Assert.True(r.WasCompressed);
        Assert.Equal(ImageFormat.Wmf, r.Format);
    }

    [Fact]
    public void Open_Svgz_Throws_When_Treated_As_Metafile()
    {
        byte[] inner = BuildWmf();
        using var ms = new MemoryStream();
        using (var gz = new GZipStream(ms, CompressionLevel.Optimal, leaveOpen: true))
            gz.Write(inner);
        ms.Position = 0;
        Assert.Throws<ImageFormatException>(() => MetafileReader.Open(ms, ImageFormat.Svgz, ownsStream: true));
    }

    [Fact]
    public void Format_And_Metadata_Are_Set()
    {
        byte[] file = BuildEmf(0, 0, 200, 100);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        Assert.Equal(ImageFormat.Emf, r.Info.Format);
        Assert.Equal(1, r.Info.FrameCount);
        Assert.True(r.CanDecodePixels);
        Assert.Equal(ImageMetadata.Empty, r.Metadata);
    }

    [Fact]
    public void RenderAt_NegativeWidth_Throws()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        Assert.Throws<ArgumentOutOfRangeException>(() => r.RenderAt(-1, 100));
        Assert.Throws<ArgumentOutOfRangeException>(() => r.RenderAt(100, 0));
    }

    [Fact]
    public void RenderAt_Wmf_Returns_NonNullFrame()
    {
        byte[] file = BuildApm(0, 0, 14400, 7200, inch: 1440);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Apm, ownsStream: true);
        using var frame = r.RenderAt(64, 32);
        Assert.Equal(64, frame.Width);
        Assert.Equal(32, frame.Height);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        r.Dispose();
        r.Dispose(); // no exception
    }

    [Fact]
    public void OwnsStream_False_Leaves_Source_Open()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        var ms = new MemoryStream(file);
        using (var r = MetafileReader.Open(ms, ImageFormat.Emf, ownsStream: false))
        {
            Assert.Equal(ImageFormat.Emf, r.Format);
        }
        Assert.True(ms.CanRead); // Still open after disposal.
    }

    [Fact]
    public void Records_Includes_Header_And_Eof_For_Emf()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        Assert.True(r.Records.Length >= 2);
        Assert.Equal(1, r.Records[0].RecordType);
        Assert.Equal(14, r.Records[^1].RecordType);
    }

    [Fact]
    public async Task ReadFramesAsync_Cancellation_Honored()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token))
            {
                f.Dispose();
            }
        });
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => MetafileReader.Open((string)null!));
    }

    [Fact]
    public void OwnsStream_True_Disposes_Underlying_Stream()
    {
        byte[] file = BuildEmf(0, 0, 100, 50);
        var inner = new MemoryStream(file, writable: false);
        using (var r = MetafileReader.Open(inner, ImageFormat.Emf, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Emf, r.Format);
        }
        Assert.False(inner.CanRead);
    }
}
