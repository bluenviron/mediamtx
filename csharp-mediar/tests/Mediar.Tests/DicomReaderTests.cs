using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dicom;
using Xunit;

namespace Mediar.Tests;

public sealed class DicomReaderTests
{
    [Fact]
    public async Task ReadsExplicitVrLittleEndian_Monochrome2_8bit_GradientFrame()
    {
        const int w = 8, h = 4;
        var pixels = BuildGradient8(w, h);
        var bytes = BuildExplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "MONOCHROME2", pixels);

        await using var stream = new MemoryStream(bytes);
        using var reader = DicomReader.Open(stream, ownsStream: false);

        Assert.Equal(ImageFormat.Dicom, reader.Format);
        Assert.Equal(w, reader.Info.Width);
        Assert.Equal(h, reader.Info.Height);
        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);
        Assert.True(reader.CanDecodePixels);

        var frame = await EnumerateOne(reader);
        Assert.Equal(w, frame.Width);
        Assert.Equal(h, frame.Height);
        Assert.Equal(PixelFormat.Gray8, frame.PixelFormat);
        Assert.Equal(pixels, frame.Pixels.Slice(0, w * h).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task ReadsImplicitVrLittleEndian_Monochrome2_8bit()
    {
        const int w = 4, h = 2;
        var pixels = new byte[] { 0, 32, 64, 96, 128, 160, 192, 224 };
        var bytes = BuildImplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "MONOCHROME2", pixels);

        await using var stream = new MemoryStream(bytes);
        using var reader = DicomReader.Open(stream, ownsStream: false);
        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);
        Assert.True(reader.CanDecodePixels);

        var frame = await EnumerateOne(reader);
        Assert.Equal(pixels, frame.Pixels.Slice(0, w * h).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task Monochrome1IsInvertedRelativeToMonochrome2()
    {
        const int w = 4, h = 1;
        var pixels = new byte[] { 0, 64, 128, 255 };
        var bytes = BuildExplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "MONOCHROME1", pixels);

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        var frame = await EnumerateOne(reader);
        var inverted = new byte[] { 255, 191, 127, 0 };
        Assert.Equal(inverted, frame.Pixels.Slice(0, w).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task Reads16BitGrayscaleLittleEndian()
    {
        const int w = 3, h = 2;
        ushort[] words = [0x0000, 0x1234, 0x7FFF, 0x8000, 0xABCD, 0xFFFF];
        byte[] pixels = new byte[words.Length * 2];
        for (int i = 0; i < words.Length; i++) BinaryPrimitives.WriteUInt16LittleEndian(pixels.AsSpan(i * 2), words[i]);

        var bytes = BuildExplicitVrLeDicom(w, h, bitsAllocated: 16, photometric: "MONOCHROME2", pixels);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(PixelFormat.Gray16, reader.Info.PixelFormat);

        var frame = await EnumerateOne(reader);
        Assert.Equal(pixels, frame.Pixels.Slice(0, pixels.Length).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task Reads24BitRgb_RoundTrips()
    {
        const int w = 2, h = 2;
        byte[] pixels =
        [
            255,   0,   0,   0, 255,   0,
              0,   0, 255, 128, 128, 128,
        ];
        var bytes = BuildExplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "RGB", pixels, samplesPerPixel: 3);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(PixelFormat.Rgb24, reader.Info.PixelFormat);

        var frame = await EnumerateOne(reader);
        Assert.Equal(pixels, frame.Pixels.Slice(0, pixels.Length).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task EncapsulatedTransferSyntaxIsRejectedForPixelDecode()
    {
        // 1.2.840.10008.1.2.4.50 = JPEG Baseline. Mediar's DICOM reader does
        // not yet implement encapsulated pixel-data dispatch.
        var bytes = BuildExplicitVrLeDicom(
            8, 8, bitsAllocated: 8, photometric: "MONOCHROME2",
            pixels: new byte[64], samplesPerPixel: 1,
            transferSyntax: "1.2.840.10008.1.2.4.50");

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.False(reader.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in reader.ReadFramesAsync()) f.Dispose();
        });
    }

    [Fact]
    public async Task ExposesMetadataTagsFromCommonGroups()
    {
        const int w = 2, h = 2;
        var pixels = new byte[4];
        var bytes = BuildExplicitVrLeDicom(w, h, 8, "MONOCHROME2", pixels,
            samplesPerPixel: 1,
            patientName: "DOE^JANE",
            modality: "CT",
            manufacturer: "ACME",
            modelName: "ScannerX",
            studyDate: "20240315");
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);

        Assert.Equal("JANE DOE", reader.Metadata.Author);
        Assert.Equal("ACME", reader.Metadata.CameraMake);
        Assert.Equal("ScannerX", reader.Metadata.CameraModel);
        Assert.Equal("20240315", reader.Metadata.CapturedAtRaw);
        Assert.NotNull(reader.Metadata.CapturedAt);
        Assert.Equal(new DateTimeOffset(2024, 3, 15, 0, 0, 0, TimeSpan.Zero), reader.Metadata.CapturedAt);
        Assert.Equal("CT", reader.Metadata.Tags["DICOM:Modality"]);
        Assert.Equal("1.2.840.10008.1.2.1", reader.Metadata.Tags["DICOM:TransferSyntaxUID"]);

        await Task.CompletedTask;
    }

    [Fact]
    public void Open_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => DicomReader.Open((Stream)null!));
    }

    [Fact]
    public async Task ReadsImplicitVrLittleEndian_Monochrome1_8bit_InvertsPixels()
    {
        const int w = 4, h = 1;
        var pixels = new byte[] { 0, 64, 128, 255 };
        var bytes = BuildImplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "MONOCHROME1", pixels);

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        var frame = await EnumerateOne(reader);
        var inverted = new byte[] { 255, 191, 127, 0 };
        Assert.Equal(inverted, frame.Pixels.Slice(0, w).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task Reads16BitMonochrome1_InvertedRelativeToMonochrome2()
    {
        const int w = 3, h = 1;
        ushort[] words = [0x0000, 0x4000, 0xFFFF];
        byte[] pixels = new byte[words.Length * 2];
        for (int i = 0; i < words.Length; i++)
            BinaryPrimitives.WriteUInt16LittleEndian(pixels.AsSpan(i * 2), words[i]);

        var bytes = BuildExplicitVrLeDicom(w, h, bitsAllocated: 16, photometric: "MONOCHROME1", pixels);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        var frame = await EnumerateOne(reader);
        ushort[] expected = [0xFFFF, 0xBFFF, 0x0000];
        for (int i = 0; i < words.Length; i++)
        {
            ushort actual = BinaryPrimitives.ReadUInt16LittleEndian(frame.Pixels.Span.Slice(i * 2, 2));
            Assert.Equal(expected[i], actual);
        }
        frame.Dispose();
    }

    [Fact]
    public async Task ReadsRgb24_ImplicitVrLittleEndian_RoundTrips()
    {
        const int w = 2, h = 1;
        byte[] pixels = [255, 0, 0, 0, 255, 0];
        var bytes = BuildImplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "RGB",
            pixels, samplesPerPixel: 3);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(PixelFormat.Rgb24, reader.Info.PixelFormat);
        Assert.Equal(3, reader.Info.ChannelCount);
        Assert.Equal(24, reader.Info.BitsPerPixel);

        var frame = await EnumerateOne(reader);
        Assert.Equal(pixels, frame.Pixels.Slice(0, pixels.Length).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task MultiFrame_DecodesEachFrameIndividually()
    {
        const int w = 2, h = 2, frames = 3;
        byte[] frameBytes = new byte[w * h];
        byte[] allPixels = new byte[w * h * frames];
        for (int f = 0; f < frames; f++)
            for (int i = 0; i < w * h; i++)
                allPixels[f * w * h + i] = (byte)((f + 1) * 0x33);

        var bytes = BuildExplicitVrLeDicom(w, h, bitsAllocated: 8, photometric: "MONOCHROME2",
            allPixels, numberOfFrames: frames);

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(frames, reader.Info.FrameCount);
        Assert.True(reader.Info.IsAnimated);

        int frameIndex = 0;
        await foreach (var f in reader.ReadFramesAsync())
        {
            byte expected = (byte)((frameIndex + 1) * 0x33);
            foreach (byte b in f.Pixels.Slice(0, w * h).ToArray())
                Assert.Equal(expected, b);
            f.Dispose();
            frameIndex++;
        }
        Assert.Equal(frames, frameIndex);
    }

    [Fact]
    public async Task TruncatedPixelData_ThrowsImageFormatException()
    {
        // Header declares 4×4 frame but supplies only 8 bytes of pixel data.
        const int declaredW = 4, declaredH = 4;
        var bytes = BuildExplicitVrLeDicom(declaredW, declaredH,
            bitsAllocated: 8, photometric: "MONOCHROME2",
            pixels: new byte[8]);

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        await Assert.ThrowsAsync<ImageFormatException>(async () =>
        {
            await foreach (var f in reader.ReadFramesAsync()) f.Dispose();
        });
    }

    [Fact]
    public async Task NoPreamble_FallsBackToImplicitVrParsing()
    {
        // No 128-byte preamble, no "DICM" magic - parser should still attempt
        // implicit VR LE from offset 0.
        const int w = 2, h = 2;
        var ds = new MemoryStream();
        WriteImplicitUInt16(ds, 0x0028, 0x0002, 1);
        WriteImplicitString(ds, 0x0028, 0x0004, "MONOCHROME2");
        WriteImplicitUInt16(ds, 0x0028, 0x0010, (ushort)h);
        WriteImplicitUInt16(ds, 0x0028, 0x0011, (ushort)w);
        WriteImplicitUInt16(ds, 0x0028, 0x0100, 8);
        WriteImplicitBytes(ds, 0x7FE0, 0x0010, new byte[] { 1, 2, 3, 4 });

        using var reader = DicomReader.Open(new MemoryStream(ds.ToArray()), ownsStream: false);
        Assert.Equal(PixelFormat.Gray8, reader.Info.PixelFormat);

        var frame = await EnumerateOne(reader);
        Assert.Equal(new byte[] { 1, 2, 3, 4 }, frame.Pixels.Slice(0, 4).ToArray());
        frame.Dispose();
    }

    [Fact]
    public async Task UnsupportedSamplesPerPixel_RejectsPixelDecode()
    {
        // 4 samples per pixel is not in the supported PixelFormat switch.
        var bytes = BuildExplicitVrLeDicom(2, 2, bitsAllocated: 8, photometric: "RGB",
            pixels: new byte[2 * 2 * 4], samplesPerPixel: 4);

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(PixelFormat.Unknown, reader.Info.PixelFormat);
        Assert.False(reader.CanDecodePixels);

        var ex = await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in reader.ReadFramesAsync()) f.Dispose();
        });
        Assert.Contains("DICOM photometric/bit configuration", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task EncapsulatedRle_RejectionMessageIncludesTransferSyntax()
    {
        // 1.2.840.10008.1.2.5 = RLE Lossless (encapsulated).
        var bytes = BuildExplicitVrLeDicom(8, 8, bitsAllocated: 8, photometric: "MONOCHROME2",
            pixels: new byte[64], samplesPerPixel: 1, transferSyntax: "1.2.840.10008.1.2.5");

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.False(reader.CanDecodePixels);
        var ex = await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in reader.ReadFramesAsync()) f.Dispose();
        });
        Assert.Contains("1.2.840.10008.1.2.5", ex.Message, StringComparison.Ordinal);
    }

    [Fact]
    public async Task Cancellation_PropagatesOperationCanceled()
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, bitsAllocated: 8, photometric: "MONOCHROME2",
            pixels: new byte[4]);

        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in reader.ReadFramesAsync(cts.Token)) f.Dispose();
        });
    }

    [Fact]
    public async Task DoubleDispose_IsIdempotent()
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, bitsAllocated: 8, photometric: "MONOCHROME2",
            pixels: new byte[4]);

        var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        reader.Dispose();
        reader.Dispose(); // should not throw
        await Task.CompletedTask;
    }

    [Theory]
    [InlineData("JANE^", "JANE")]
    [InlineData("^DOE", "DOE")]
    [InlineData("JustOneName", "JustOneName")]
    [InlineData("A^B^C^D^E", "B A")]
    public void PatientName_NormalisationVariants(string raw, string expected)
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, 8, "MONOCHROME2",
            pixels: new byte[4], samplesPerPixel: 1, patientName: raw);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(expected, reader.Metadata.Author);
    }

    [Fact]
    public void PatientName_WhitespaceOnly_ReturnsNull()
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, 8, "MONOCHROME2",
            pixels: new byte[4], samplesPerPixel: 1, patientName: "   ");
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Null(reader.Metadata.Author);
    }

    [Theory]
    [InlineData("2024030", false)] // too short
    [InlineData("notadate", false)] // wrong length and non-numeric
    [InlineData("00000000", false)] // out of Gregorian range
    [InlineData("20241301", false)] // invalid month
    [InlineData("20240229", true)]  // valid leap day
    [InlineData("20230229", false)] // invalid leap day
    public void StudyDate_ParsesIntoCapturedAtWhenValid(string raw, bool expectParsed)
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, 8, "MONOCHROME2",
            pixels: new byte[4], samplesPerPixel: 1, studyDate: raw);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(raw, reader.Metadata.CapturedAtRaw);
        if (expectParsed)
        {
            Assert.NotNull(reader.Metadata.CapturedAt);
        }
        else
        {
            Assert.Null(reader.Metadata.CapturedAt);
        }
    }

    [Fact]
    public void TransferSyntaxTag_AlwaysPresent_EvenWithoutOtherMetadata()
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, 8, "MONOCHROME2",
            pixels: new byte[4], samplesPerPixel: 1);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal("1.2.840.10008.1.2.1", reader.Metadata.Tags["DICOM:TransferSyntaxUID"]);
        Assert.Null(reader.Metadata.Author);
        Assert.Null(reader.Metadata.CameraMake);
        Assert.Null(reader.Metadata.CapturedAt);
    }

    [Fact]
    public void Format_IsAlwaysDicom_ForOpenedStreams()
    {
        var bytes = BuildExplicitVrLeDicom(2, 2, 8, "MONOCHROME2",
            pixels: new byte[4], samplesPerPixel: 1);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.Equal(ImageFormat.Dicom, reader.Format);
    }

    [Fact]
    public void ZeroDimensions_DisableDecode()
    {
        // width=0 disables canDecode (per OpenBytes: requires width>0 && height>0).
        var bytes = BuildExplicitVrLeDicom(0, 0, bitsAllocated: 8, photometric: "MONOCHROME2",
            pixels: Array.Empty<byte>(), samplesPerPixel: 1);
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.False(reader.CanDecodePixels);
    }

    [Fact]
    public async Task ExplicitVr_JpegLsLossless_IsEncapsulated_RejectsDecode()
    {
        // 1.2.840.10008.1.2.4.80 = JPEG-LS Lossless (encapsulated).
        var bytes = BuildExplicitVrLeDicom(4, 4, 8, "MONOCHROME2", new byte[16],
            samplesPerPixel: 1, transferSyntax: "1.2.840.10008.1.2.4.80");
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.False(reader.CanDecodePixels);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in reader.ReadFramesAsync()) f.Dispose();
        });
    }

    [Fact]
    public async Task ExplicitVrBigEndian_IsTreatedAsNotEncapsulated()
    {
        // 1.2.840.10008.1.2.2 (Explicit VR Big Endian) is NOT encapsulated per
        // IsEncapsulated, but pixel parsing still uses little-endian readers,
        // so the structure is decoded even if pixel byte order is wrong for
        // genuine Big-Endian payloads. This test exercises the
        // non-encapsulated branch of the transfer-syntax switch.
        var bytes = BuildExplicitVrLeDicom(2, 2, 8, "MONOCHROME2",
            pixels: new byte[] { 0x11, 0x22, 0x33, 0x44 },
            samplesPerPixel: 1, transferSyntax: "1.2.840.10008.1.2.2");
        using var reader = DicomReader.Open(new MemoryStream(bytes), ownsStream: false);
        Assert.True(reader.CanDecodePixels);
        var frame = await EnumerateOne(reader);
        Assert.Equal(new byte[] { 0x11, 0x22, 0x33, 0x44 }, frame.Pixels.Slice(0, 4).ToArray());
        frame.Dispose();
    }

    // ── fixture builders ────────────────────────────────────────────────────────

    private static byte[] BuildExplicitVrLeDicom(
        int width, int height, int bitsAllocated, string photometric,
        byte[] pixels, int samplesPerPixel = 1,
        string transferSyntax = "1.2.840.10008.1.2.1",
        string? patientName = null,
        string? modality = null,
        string? manufacturer = null,
        string? modelName = null,
        string? studyDate = null,
        int numberOfFrames = 0)
    {
        var ds = new MemoryStream();

        // Optional File Meta group (Explicit VR LE, group 0002).
        var meta = new MemoryStream();
        WriteExplicitString(meta, 0x0002, 0x0010, "UI", transferSyntax);
        var metaBytes = meta.ToArray();
        // No File Meta length tag for simplicity; the parser falls back to
        // implicit VR when there's no preamble, so we always emit the preamble.

        if (modality is not null) WriteExplicitString(ds, 0x0008, 0x0060, "CS", modality);
        if (studyDate is not null) WriteExplicitString(ds, 0x0008, 0x0020, "DA", studyDate);
        if (manufacturer is not null) WriteExplicitString(ds, 0x0008, 0x0070, "LO", manufacturer);
        if (modelName is not null) WriteExplicitString(ds, 0x0008, 0x1090, "LO", modelName);
        if (patientName is not null) WriteExplicitString(ds, 0x0010, 0x0010, "PN", patientName);

        WriteExplicitUInt16(ds, 0x0028, 0x0002, (ushort)samplesPerPixel);
        WriteExplicitString(ds, 0x0028, 0x0004, "CS", photometric);
        if (numberOfFrames > 0)
        {
            // NumberOfFrames uses VR=IS (Integer String).
            WriteExplicitString(ds, 0x0028, 0x0008, "IS",
                numberOfFrames.ToString(System.Globalization.CultureInfo.InvariantCulture));
        }
        WriteExplicitUInt16(ds, 0x0028, 0x0010, (ushort)height);
        WriteExplicitUInt16(ds, 0x0028, 0x0011, (ushort)width);
        WriteExplicitUInt16(ds, 0x0028, 0x0100, (ushort)bitsAllocated);
        WriteExplicitUInt16(ds, 0x0028, 0x0101, (ushort)bitsAllocated);
        WriteExplicitUInt16(ds, 0x0028, 0x0102, (ushort)(bitsAllocated - 1));
        WriteExplicitUInt16(ds, 0x0028, 0x0103, 0);

        WriteExplicitOb(ds, 0x7FE0, 0x0010, pixels);

        return AssembleFile(metaBytes, ds.ToArray());
    }

    private static byte[] BuildImplicitVrLeDicom(
        int width, int height, int bitsAllocated, string photometric, byte[] pixels,
        int samplesPerPixel = 1)
    {
        // For Implicit VR LE we encode the File Meta as Explicit (always),
        // but the main dataset as Implicit.
        var meta = new MemoryStream();
        WriteExplicitString(meta, 0x0002, 0x0010, "UI", "1.2.840.10008.1.2");

        var ds = new MemoryStream();
        WriteImplicitUInt16(ds, 0x0028, 0x0002, (ushort)samplesPerPixel);
        WriteImplicitString(ds, 0x0028, 0x0004, photometric);
        WriteImplicitUInt16(ds, 0x0028, 0x0010, (ushort)height);
        WriteImplicitUInt16(ds, 0x0028, 0x0011, (ushort)width);
        WriteImplicitUInt16(ds, 0x0028, 0x0100, (ushort)bitsAllocated);
        WriteImplicitBytes(ds, 0x7FE0, 0x0010, pixels);

        return AssembleFile(meta.ToArray(), ds.ToArray());
    }

    private static byte[] AssembleFile(byte[] meta, byte[] dataset)
    {
        var ms = new MemoryStream();
        ms.Write(new byte[128]);
        ms.Write("DICM"u8);
        ms.Write(meta);
        ms.Write(dataset);
        return ms.ToArray();
    }

    private static void WriteExplicitString(Stream s, ushort group, ushort element, string vr, string value)
    {
        Span<byte> body = stackalloc byte[2];
        var asciiBytes = Encoding.ASCII.GetBytes(value);
        if ((asciiBytes.Length & 1) == 1) asciiBytes = [.. asciiBytes, (byte)' '];
        WriteTag(s, group, element);
        s.Write(Encoding.ASCII.GetBytes(vr));
        BinaryPrimitives.WriteUInt16LittleEndian(body, (ushort)asciiBytes.Length);
        s.Write(body);
        s.Write(asciiBytes);
    }

    private static void WriteExplicitUInt16(Stream s, ushort group, ushort element, ushort value)
    {
        WriteTag(s, group, element);
        s.Write("US"u8);
        Span<byte> lenBytes = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(lenBytes, 2);
        s.Write(lenBytes);
        Span<byte> v = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(v, value);
        s.Write(v);
    }

    private static void WriteExplicitOb(Stream s, ushort group, ushort element, byte[] payload)
    {
        WriteTag(s, group, element);
        s.Write("OB"u8);
        Span<byte> reserved = stackalloc byte[2];
        s.Write(reserved);
        Span<byte> len = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(len, (uint)payload.Length);
        s.Write(len);
        s.Write(payload);
    }

    private static void WriteImplicitUInt16(Stream s, ushort group, ushort element, ushort value)
    {
        WriteTag(s, group, element);
        Span<byte> len = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(len, 2);
        s.Write(len);
        Span<byte> v = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(v, value);
        s.Write(v);
    }

    private static void WriteImplicitString(Stream s, ushort group, ushort element, string value)
    {
        var ascii = Encoding.ASCII.GetBytes(value);
        if ((ascii.Length & 1) == 1) ascii = [.. ascii, (byte)' '];
        WriteImplicitBytes(s, group, element, ascii);
    }

    private static void WriteImplicitBytes(Stream s, ushort group, ushort element, byte[] payload)
    {
        WriteTag(s, group, element);
        Span<byte> len = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(len, (uint)payload.Length);
        s.Write(len);
        s.Write(payload);
    }

    private static void WriteTag(Stream s, ushort group, ushort element)
    {
        Span<byte> tag = stackalloc byte[4];
        BinaryPrimitives.WriteUInt16LittleEndian(tag, group);
        BinaryPrimitives.WriteUInt16LittleEndian(tag[2..], element);
        s.Write(tag);
    }

    private static byte[] BuildGradient8(int w, int h)
    {
        var p = new byte[w * h];
        for (int y = 0; y < h; y++)
            for (int x = 0; x < w; x++)
                p[y * w + x] = (byte)((x * 32 + y * 8) & 0xFF);
        return p;
    }

    private static async Task<ImageFrame> EnumerateOne(DicomReader reader)
    {
        ImageFrame? first = null;
        await foreach (var f in reader.ReadFramesAsync())
        {
            if (first is not null) { f.Dispose(); continue; }
            first = f;
        }
        Assert.NotNull(first);
        return first!;
    }
}
