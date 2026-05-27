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
    public async Task ReadFramesAsync_Throws()
    {
        byte[] file = BuildEmf(0, 0, 10, 10);
        using var r = MetafileReader.Open(new MemoryStream(file), ImageFormat.Emf, ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
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
}
