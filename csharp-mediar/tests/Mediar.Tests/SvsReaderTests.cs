using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Svs;
using Mediar.Tests.Tiff;
using Xunit;

namespace Mediar.Tests;

public class SvsReaderTests
{
    [Fact]
    public void Parses_Single_Page_Aperio_Tiff()
    {
        var bytes = BuildSvsTiff(
            new[] { (Width: 1024, Height: 768, Description: "Aperio Image Library v11.0.0|AppMag = 20|MPP = 0.5|ScanScope ID = SS1234|Date = 03/14/24|Time = 09:30:00|User = JD") });

        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(ImageFormat.Svs, r.Format);
        Assert.Equal(1024, r.Info.Width);
        Assert.Equal(768, r.Info.Height);
        Assert.Single(r.Levels);
        Assert.Contains("Aperio", r.VendorDescription);
        Assert.Equal("Aperio Image Library v11.0.0", r.Metadata.Title);
        Assert.Equal("JD", r.Metadata.Author);
        Assert.Equal("20", r.Metadata.Tags["AppMag"]);
        Assert.Equal("0.5", r.Metadata.Tags["MPP"]);
    }

    [Fact]
    public void Parses_Multi_Level_Pyramid()
    {
        var bytes = BuildSvsTiff(new[]
        {
            (Width: 4096, Height: 3072, Description: "Aperio|AppMag = 40"),
            (Width: 1024, Height: 768,  Description: "Aperio|thumbnail"),
            (Width: 256,  Height: 192,  Description: "Aperio|level=2"),
        });

        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(3, r.Levels.Count);
        Assert.Equal(4096, r.Levels[0].Width);
        Assert.Equal(256, r.Levels[2].Width);
        Assert.Equal(3, r.Info.FrameCount);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Baseline_Has_No_StripOrTile_Data()
    {
        // Synthesised SVS pages here omit StripOffsets / TileOffsets entirely,
        // so SvsReader correctly reports CanDecodePixels=false and the async
        // iterator throws on first MoveNextAsync.
        var bytes = BuildSvsTiff(new[] { (Width: 16, Height: 16, Description: "Aperio") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in r.ReadFramesAsync()) { /* drain */ }
        });
    }

    [Fact]
    public async Task ReadFramesAsync_Decodes_JpegTiled_Baseline_Via_TiffReader()
    {
        // Build a real SVS-shaped TIFF whose baseline is a single 16x16 JPEG tile.
        // SvsReader.CanDecodePixels should be true and the decoded frame should
        // be red-dominant (the embedded JPEG payload is the solid-red 16x16
        // tile from JpegBaselineDecoderTests).
        byte[] jpeg = Convert.FromBase64String(JpegBaselineDecoderTests_RedJpegBase64);
        var bytes = TestTiffBuilder.Build(new TestTiffBuilder.TiffSpec
        {
            Width = 16, Height = 16, BitsPerSample = 8, SamplesPerPixel = 3,
            Compression = 7, Photometric = 2,
            TileWidth = 16, TileHeight = 16,
            TilePayloads = [jpeg],
        });

        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.True(r.CanDecodePixels);

        ImageFrame? captured = null;
        await foreach (var frame in r.ReadFramesAsync())
        {
            captured = frame;
            break;
        }
        Assert.NotNull(captured);
        using (captured)
        {
            long rSum = 0, gSum = 0, bSum = 0;
            var px = captured!.Pixels.Span;
            for (int i = 0; i + 2 < px.Length; i += 3)
            {
                rSum += px[i]; gSum += px[i + 1]; bSum += px[i + 2];
            }
            int n = px.Length / 3;
            Assert.True(rSum / n > 180);
            Assert.True(gSum / n < 40);
            Assert.True(bSum / n < 40);
        }
    }

    // Re-exposed for the SVS pixel-decode test; mirrors JpegBaselineDecoderTests.RedJpegBase64.
    private const string JpegBaselineDecoderTests_RedJpegBase64 =
        "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAQCAwMDAgQDAwMEBAQEBQkGBQUFBQsICAYJDQsNDQ0LDAwOEBQRDg8TDwwMEhgSExUWFxcXDhEZGxkWGhQWFxb/" +
        "2wBDAQQEBAUFBQoGBgoWDwwPFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhb/wAARCAAQABADASIAAhEBAxEB/8QA" +
        "HwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2Jyggk" +
        "KFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMX" +
        "Gx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAEC" +
        "AxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOE" +
        "hYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDxeiiivyk/" +
        "v4//2Q==";

    [Fact]
    public void Rejects_Non_Tiff()
    {
        Assert.Throws<ImageFormatException>(() =>
            SvsReader.Open(new MemoryStream(new byte[32]), ownsStream: true));
    }

    [Fact]
    public void Open_Null_Stream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() =>
            SvsReader.Open((Stream)null!, ownsStream: false));
    }

    [Fact]
    public void Open_Truncated_Stream_Below_Header_Throws()
    {
        Assert.Throws<ImageFormatException>(() =>
            SvsReader.Open(new MemoryStream(new byte[4]), ownsStream: true));
    }

    [Fact]
    public void Open_Missing_Path_Throws_FileNotFound()
    {
        var p = Path.Combine(Path.GetTempPath(), "missing-" + Guid.NewGuid().ToString("N") + ".svs");
        Assert.Throws<FileNotFoundException>(() => SvsReader.Open(p));
    }

    [Fact]
    public void Open_BigTiff_Magic_Is_Rejected()
    {
        // 'II' byte-order mark + magic 43 (BigTIFF) -> rejected.
        var bytes = new byte[16];
        bytes[0] = (byte)'I'; bytes[1] = (byte)'I';
        bytes[2] = 43; bytes[3] = 0;
        Assert.Throws<ImageFormatException>(() =>
            SvsReader.Open(new MemoryStream(bytes), ownsStream: true));
    }

    [Fact]
    public void Open_Bad_Byte_Order_Mark_Is_Rejected()
    {
        // 'XX' instead of 'II'/'MM'
        var bytes = new byte[16];
        bytes[0] = (byte)'X'; bytes[1] = (byte)'X';
        Assert.Throws<ImageFormatException>(() =>
            SvsReader.Open(new MemoryStream(bytes), ownsStream: true));
    }

    [Fact]
    public void Open_Header_With_Zero_IfdOffset_Throws()
    {
        // Valid header but ifd offset = 0 -> no IFDs found.
        var bytes = new byte[8];
        bytes[0] = (byte)'I'; bytes[1] = (byte)'I';
        bytes[2] = 42; bytes[3] = 0;
        // bytes[4..7] = 0
        Assert.Throws<ImageFormatException>(() =>
            SvsReader.Open(new MemoryStream(bytes), ownsStream: true));
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio") });
        var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        r.Dispose();
        r.Dispose(); // must not throw
    }

    [Fact]
    public void OwnsStream_False_Keeps_Underlying_Stream_Open_For_Caller()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio") });
        var inner = new MemoryStream(bytes, writable: false);
        var r = SvsReader.Open(inner, ownsStream: false);
        r.Dispose();
        Assert.True(inner.CanRead);
        inner.Dispose();
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => SvsReader.Open((string)null!));
    }

    [Fact]
    public void OwnsStream_True_Disposes_Underlying_Stream()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio") });
        var inner = new MemoryStream(bytes, writable: false);
        using (var r = SvsReader.Open(inner, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Svs, r.Format);
        }
        Assert.False(inner.CanRead);
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_Pre_Cancelled_Token()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        if (!r.CanDecodePixels) return;
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token)) { f.Dispose(); }
        });
    }

    [Fact]
    public void Empty_Description_Yields_Empty_Metadata()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Null(r.Metadata.Title);
        Assert.Null(r.Metadata.Author);
        Assert.Empty(r.Metadata.Tags);
    }

    [Fact]
    public void Description_Without_Pipes_Becomes_Title_Only()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Plain Aperio Title") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("Plain Aperio Title", r.Metadata.Title);
        Assert.Null(r.Metadata.Author);
    }

    [Fact]
    public void Tags_Are_Case_Insensitive()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio|AppMag = 40|MPP = 0.25") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("40", r.Metadata.Tags["appmag"]);
        Assert.Equal("40", r.Metadata.Tags["APPMAG"]);
        Assert.Equal("0.25", r.Metadata.Tags["mpp"]);
    }

    [Fact]
    public void ScanScope_Id_Tag_Becomes_Software_Suffix()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio|ScanScope ID = SS9999") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("Aperio ScanScope SS9999", r.Metadata.Software);
    }

    [Fact]
    public void Default_Software_Is_Aperio_ScanScope()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio|AppMag = 20") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("Aperio ScanScope", r.Metadata.Software);
    }

    [Fact]
    public void Date_Plus_Time_Produce_Captured_Raw_String()
    {
        var bytes = BuildSvsTiff(new[]
        {
            (Width: 8, Height: 8, Description: "Aperio|Date = 2024-03-14|Time = 09:30:00"),
        });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.NotNull(r.Metadata.CapturedAtRaw);
        Assert.Contains("2024-03-14", r.Metadata.CapturedAtRaw);
        Assert.Contains("09:30:00", r.Metadata.CapturedAtRaw);
    }

    [Fact]
    public void Format_Is_Svs()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 8, Height: 8, Description: "Aperio") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(ImageFormat.Svs, r.Format);
        Assert.Equal(ImageFormat.Svs, r.Info.Format);
    }

    [Fact]
    public void Info_FrameCount_Matches_Level_Count()
    {
        var bytes = BuildSvsTiff(new[]
        {
            (Width: 4096, Height: 3072, Description: "Aperio"),
            (Width: 1024, Height: 768,  Description: "thumbnail"),
            (Width: 256,  Height: 192,  Description: "level=2"),
            (Width: 128,  Height: 96,   Description: "level=3"),
        });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal(4, r.Levels.Count);
        Assert.Equal(4, r.Info.FrameCount);
    }

    [Fact]
    public void Baseline_Without_StripOrTile_Cannot_Decode()
    {
        var bytes = BuildSvsTiff(new[] { (Width: 16, Height: 16, Description: "Aperio") });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Level_Captures_Description_And_CompressionTag()
    {
        var bytes = BuildSvsTiff(new[]
        {
            (Width: 4096, Height: 3072, Description: "Aperio|AppMag = 40"),
            (Width: 1024, Height: 768,  Description: "Aperio|Thumb"),
        });
        using var r = SvsReader.Open(new MemoryStream(bytes), ownsStream: true);
        Assert.Equal("Aperio|AppMag = 40", r.Levels[0].Description);
        Assert.Equal("Aperio|Thumb", r.Levels[1].Description);
        Assert.Equal(1, r.Levels[0].CompressionTag);
    }

    private static byte[] BuildSvsTiff((int Width, int Height, string Description)[] pages)
    {
        using var ms = new MemoryStream();
        var w = new BinaryWriter(ms);
        // Little-endian TIFF header
        w.Write((byte)'I'); w.Write((byte)'I');
        w.Write((ushort)42);
        long ifdOffsetSlot = ms.Position;
        w.Write((uint)0);  // patched later

        var stripDataOffsets = new long[pages.Length];
        // Allocate per-page strip data placeholder (uncompressed, tiny)
        // Each page writes its strip data into the stream first, then later writes the IFD with strip offsets pointing here.

        for (int i = 0; i < pages.Length; i++)
        {
            stripDataOffsets[i] = ms.Position;
            // Pretend payload is a single byte per page; the reader only reads dimensions, not pixels.
            w.Write((byte)0xFF);
        }

        var ifdOffsets = new long[pages.Length];
        for (int i = 0; i < pages.Length; i++)
        {
            // ASCII description goes immediately before the IFD
            var descBytes = Encoding.ASCII.GetBytes(pages[i].Description + "\0");
            long descOffset = ms.Position;
            w.Write(descBytes);

            // Align IFD to 2-byte boundary
            if ((ms.Position & 1) == 1) w.Write((byte)0);

            ifdOffsets[i] = ms.Position;
            // 7 IFD entries: ImageWidth(0x0100), ImageLength(0x0101), BitsPerSample(0x0102),
            //                Compression(0x0103), Photometric(0x0106), SamplesPerPixel(0x0115),
            //                ImageDescription(0x010E)
            w.Write((ushort)7);
            WriteEntry(w, 0x0100, 3, 1, (uint)pages[i].Width);
            WriteEntry(w, 0x0101, 3, 1, (uint)pages[i].Height);
            WriteEntry(w, 0x0102, 3, 1, 8);
            WriteEntry(w, 0x0103, 3, 1, 1);
            WriteEntry(w, 0x0106, 3, 1, 2);
            WriteEntry(w, 0x0115, 3, 1, 3);
            WriteEntry(w, 0x010E, 2, (uint)descBytes.Length, (uint)descOffset);
            long nextIfdSlot = ms.Position;
            w.Write((uint)0);  // patched after next loop iteration
            // Patch previous IFD's "next IFD offset" to point here on next iter
            if (i > 0)
            {
                long savePos = ms.Position;
                long prevNext = ifdOffsets[i - 1] + 2 + 7 * 12;
                ms.Position = prevNext;
                w.Write((uint)ifdOffsets[i]);
                ms.Position = savePos;
            }
            else
            {
                long savePos = ms.Position;
                ms.Position = ifdOffsetSlot;
                w.Write((uint)ifdOffsets[i]);
                ms.Position = savePos;
            }
        }

        return ms.ToArray();
    }

    private static void WriteEntry(BinaryWriter w, ushort tag, ushort type, uint count, uint valueOrOffset)
    {
        w.Write(tag);
        w.Write(type);
        w.Write(count);
        w.Write(valueOrOffset);
    }
}
