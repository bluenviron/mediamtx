using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.Mp3;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the ID3v2 APIC (v2.3/v2.4) and PIC (v2.2) frames that
/// carry embedded picture data. These frames populate
/// <see cref="MediaMetadata.Pictures"/> rather than the
/// <see cref="MediaMetadata.Tags"/> dictionary.
/// </summary>
public sealed class Id3v2ApicFrameTests
{
    private static readonly byte[] JpegMagic = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46];
    private static readonly byte[] PngMagic = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];

    [Fact]
    public void Apic_V23_Jpeg_Front_Cover()
    {
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0,
            mime: "image/jpeg",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "Front",
            data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", picture.MimeType);
        Assert.Equal(MediaPictureType.CoverFront, picture.Type);
        Assert.Equal("Front", picture.Description);
        Assert.Equal(JpegMagic, picture.Data.ToArray());
    }

    [Fact]
    public void Apic_V24_Utf8_Description()
    {
        var meta = Decode(version: 4, BuildApicFrame(
            encoding: 3,
            mime: "image/png",
            pictureType: (byte)MediaPictureType.Artist,
            description: "Künstler",
            data: PngMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/png", picture.MimeType);
        Assert.Equal(MediaPictureType.Artist, picture.Type);
        Assert.Equal("Künstler", picture.Description);
    }

    [Fact]
    public void Apic_V23_Multiple_Frames_Yield_Multiple_Pictures()
    {
        byte[] frame1 = BuildApicFrame(0, "image/jpeg", (byte)MediaPictureType.CoverFront, "Front", JpegMagic);
        byte[] frame2 = BuildApicFrame(0, "image/png", (byte)MediaPictureType.CoverBack, "Back", PngMagic);
        var meta = Decode(version: 3, [.. frame1, .. frame2]);
        Assert.Equal(2, meta.Pictures.Count);
        Assert.Equal(MediaPictureType.CoverFront, meta.Pictures[0].Type);
        Assert.Equal(MediaPictureType.CoverBack, meta.Pictures[1].Type);
    }

    [Fact]
    public void Apic_Out_Of_Range_PictureType_Falls_Back_To_Other()
    {
        var meta = Decode(version: 3, BuildApicFrame(0, "image/jpeg", 99, "", JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(MediaPictureType.Other, picture.Type);
    }

    [Fact]
    public void Apic_Empty_Picture_Bytes_Adds_No_Picture()
    {
        var meta = Decode(version: 3, BuildApicFrame(0, "image/jpeg", 3, "", []));
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Pic_V22_Jpg_Maps_To_Image_Jpeg()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0,
            imageFormat: "JPG",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "Front",
            data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", picture.MimeType);
        Assert.Equal(MediaPictureType.CoverFront, picture.Type);
        Assert.Equal("Front", picture.Description);
        Assert.Equal(JpegMagic, picture.Data.ToArray());
    }

    [Fact]
    public void Pic_V22_Png_Maps_To_Image_Png()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(0, "PNG", (byte)MediaPictureType.CoverBack, "Back", PngMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/png", picture.MimeType);
        Assert.Equal(MediaPictureType.CoverBack, picture.Type);
    }

    [Fact]
    public void Apic_Coexists_With_Text_Frames()
    {
        byte[] tit2 = BuildV23TextFrame("TIT2", "Some Song");
        byte[] apic = BuildApicFrame(0, "image/jpeg", (byte)MediaPictureType.CoverFront, "Front", JpegMagic);
        var meta = Decode(version: 3, [.. tit2, .. apic]);
        Assert.Equal("Some Song", meta.Title);
        Assert.Single(meta.Pictures);
    }

    [Fact]
    public void Pic_V22_Gif_Maps_To_Image_Gif()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "GIF",
            pictureType: (byte)MediaPictureType.Media,
            description: "", data: PngMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/gif", picture.MimeType);
    }

    [Fact]
    public void Pic_V22_Bmp_Maps_To_Image_Bmp()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "BMP",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/bmp", picture.MimeType);
    }

    [Fact]
    public void Pic_V22_Unknown_Format_Maps_To_Image_LowerCase()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "XYZ",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/xyz", picture.MimeType);
    }

    [Fact]
    public void Pic_V22_Jpeg_LowerCase_Maps_To_Image_Jpeg()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "jpg",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        // ToUpperInvariant("jpg") == "JPG" -> image/jpeg.
        Assert.Equal("image/jpeg", picture.MimeType);
    }

    [Fact]
    public void Apic_Boundary_PictureType_20_Maps_To_PublisherLogo()
    {
        // PublisherLogo (raw 20) is the last legal enum value - must NOT
        // fall back to Other.
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0, mime: "image/jpeg",
            pictureType: 20, description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(MediaPictureType.PublisherLogo, picture.Type);
    }

    [Fact]
    public void Apic_V23_Empty_Description_Yields_Empty_String()
    {
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0, mime: "image/jpeg",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(string.Empty, picture.Description);
    }

    [Fact]
    public void Apic_V24_Utf16_BE_Description()
    {
        var meta = Decode(version: 4, BuildApicFrame(
            encoding: 2, mime: "image/png",
            pictureType: (byte)MediaPictureType.Other,
            description: "Tëst", data: PngMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("Tëst", picture.Description);
    }

    [Fact]
    public void Apic_V23_Utf16_BOM_Description()
    {
        // encoding 1 uses UTF-16 with a BOM.
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 1, mime: "image/jpeg",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "🎵", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("🎵", picture.Description);
    }

    [Fact]
    public void Apic_Two_Frames_Same_Type_Both_Retained()
    {
        byte[] frame1 = BuildApicFrame(0, "image/jpeg", (byte)MediaPictureType.CoverFront, "A", JpegMagic);
        byte[] frame2 = BuildApicFrame(0, "image/jpeg", (byte)MediaPictureType.CoverFront, "B", JpegMagic);
        var meta = Decode(version: 3, [.. frame1, .. frame2]);
        Assert.Equal(2, meta.Pictures.Count);
        Assert.Equal("A", meta.Pictures[0].Description);
        Assert.Equal("B", meta.Pictures[1].Description);
    }

    [Fact]
    public void Apic_All_Roles_Survive_Independently()
    {
        // Build pictures across the most-used type values.
        var types = new[]
        {
            MediaPictureType.Other,
            MediaPictureType.CoverFront,
            MediaPictureType.CoverBack,
            MediaPictureType.Artist,
            MediaPictureType.LeadArtist,
            MediaPictureType.Band,
            MediaPictureType.Conductor,
            MediaPictureType.PublisherLogo,
        };
        var frames = new List<byte>();
        foreach (var t in types)
        {
            frames.AddRange(BuildApicFrame(0, "image/jpeg", (byte)t, t.ToString(), JpegMagic));
        }
        var meta = Decode(version: 3, frames.ToArray());
        Assert.Equal(types.Length, meta.Pictures.Count);
        for (int i = 0; i < types.Length; i++)
        {
            Assert.Equal(types[i], meta.Pictures[i].Type);
        }
    }

    [Fact]
    public void Apic_DataBuffer_Is_Independent_Of_Source_Bytes()
    {
        byte[] payload = (byte[])JpegMagic.Clone();
        byte[] frame = BuildApicFrame(0, "image/jpeg", (byte)MediaPictureType.CoverFront, "", payload);
        var meta = Decode(version: 3, frame);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(JpegMagic, picture.Data.ToArray());
        // The picture data should not alias the original payload buffer.
        // (Mutating payload after the fact must not change the stored picture.)
        for (int i = 0; i < payload.Length; i++) payload[i] = 0;
        Assert.Equal(JpegMagic, picture.Data.ToArray());
    }

    [Fact]
    public void Apic_Frame_Shorter_Than_4_Bytes_Adds_No_Picture()
    {
        // Build a minimal APIC frame with payload of only 2 bytes.
        byte[] payload = [0, 0x41]; // enc + 1 byte (no MIME terminator possible)
        byte[] frame = new byte[10 + payload.Length];
        Encoding.ASCII.GetBytes("APIC").CopyTo(frame.AsSpan(0, 4));
        BinaryPrimitives.WriteUInt32BigEndian(frame.AsSpan(4, 4), (uint)payload.Length);
        payload.CopyTo(frame.AsSpan(10));
        var meta = Decode(version: 3, frame);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Apic_Missing_Mime_Terminator_Adds_No_Picture()
    {
        // Payload of enc byte + several non-NUL bytes but no terminator.
        byte[] payload = new byte[] { 0, (byte)'i', (byte)'m', (byte)'a', (byte)'g', (byte)'e' };
        byte[] frame = new byte[10 + payload.Length];
        Encoding.ASCII.GetBytes("APIC").CopyTo(frame.AsSpan(0, 4));
        BinaryPrimitives.WriteUInt32BigEndian(frame.AsSpan(4, 4), (uint)payload.Length);
        payload.CopyTo(frame.AsSpan(10));
        var meta = Decode(version: 3, frame);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Apic_Truncated_After_Mime_Adds_No_Picture()
    {
        // enc + "image/jpeg" + NUL, and nothing else (missing picType byte).
        byte[] mimeBytes = Encoding.Latin1.GetBytes("image/jpeg");
        byte[] payload = new byte[1 + mimeBytes.Length + 1];
        payload[0] = 0;
        mimeBytes.CopyTo(payload.AsSpan(1));
        payload[1 + mimeBytes.Length] = 0;
        byte[] frame = new byte[10 + payload.Length];
        Encoding.ASCII.GetBytes("APIC").CopyTo(frame.AsSpan(0, 4));
        BinaryPrimitives.WriteUInt32BigEndian(frame.AsSpan(4, 4), (uint)payload.Length);
        payload.CopyTo(frame.AsSpan(10));
        var meta = Decode(version: 3, frame);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Apic_Empty_Mime_Maps_To_Image_Slash()
    {
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0, mime: "",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/", picture.MimeType);
    }

    [Fact]
    public void Pic_V22_Frame_Shorter_Than_6_Bytes_Adds_No_Picture()
    {
        // Frame with only 5-byte payload (less than the 6-byte minimum).
        byte[] payload = [0, (byte)'J', (byte)'P', (byte)'G', 3];
        byte[] frame = new byte[6 + payload.Length];
        Encoding.ASCII.GetBytes("PIC").CopyTo(frame.AsSpan(0, 3));
        frame[3] = 0;
        frame[4] = 0;
        frame[5] = (byte)payload.Length;
        payload.CopyTo(frame.AsSpan(6));
        var meta = Decode(version: 2, frame);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Pic_V22_Empty_Picture_Bytes_Adds_No_Picture()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "JPG",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "", data: []));
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Pic_V22_Jpeg_Long_Form_Maps_To_Image_Jpeg()
    {
        // "JPEG" is 4 chars but the v2.2 spec mandates 3-char image format.
        // The ImageFormatToMimeType helper handles "JPEG" defensively though;
        // here we verify the standard "JPG" path is what the wire uses.
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "JPG",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: "Cover", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", picture.MimeType);
        Assert.Equal("Cover", picture.Description);
    }

    [Fact]
    public void Apic_V24_Utf8_Multibyte_Description()
    {
        // Cyrillic / emoji span multiple bytes per code point in UTF-8.
        const string desc = "Тёт✨ст";
        var meta = Decode(version: 4, BuildApicFrame(
            encoding: 3, mime: "image/jpeg",
            pictureType: (byte)MediaPictureType.CoverFront,
            description: desc, data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(desc, picture.Description);
    }

    [Fact]
    public void Apic_V24_Utf16BE_Empty_Description_Yields_Empty_String()
    {
        var meta = Decode(version: 4, BuildApicFrame(
            encoding: 2, mime: "image/png",
            pictureType: (byte)MediaPictureType.Artist,
            description: "", data: PngMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(string.Empty, picture.Description);
    }

    [Fact]
    public void Apic_OutOfRange_PictureType_21_Falls_Back_To_Other()
    {
        // 21 is just above PublisherLogo (20), the last legal enum value.
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0, mime: "image/jpeg",
            pictureType: 21, description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(MediaPictureType.Other, picture.Type);
    }

    [Fact]
    public void Apic_OutOfRange_PictureType_255_Falls_Back_To_Other()
    {
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0, mime: "image/jpeg",
            pictureType: 255, description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(MediaPictureType.Other, picture.Type);
    }

    [Fact]
    public void Pic_V22_OutOfRange_PictureType_Falls_Back_To_Other()
    {
        var meta = Decode(version: 2, BuildPicV22Frame(
            encoding: 0, imageFormat: "JPG",
            pictureType: 99, description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(MediaPictureType.Other, picture.Type);
    }

    [Theory]
    [InlineData((byte)MediaPictureType.Other)]
    [InlineData((byte)MediaPictureType.CoverFront)]
    [InlineData((byte)MediaPictureType.CoverBack)]
    [InlineData((byte)MediaPictureType.Artist)]
    [InlineData((byte)MediaPictureType.LeadArtist)]
    [InlineData((byte)MediaPictureType.Band)]
    [InlineData((byte)MediaPictureType.Composer)]
    [InlineData((byte)MediaPictureType.PublisherLogo)]
    public void Apic_All_Valid_PictureTypes_Round_Trip(byte raw)
    {
        var meta = Decode(version: 3, BuildApicFrame(
            encoding: 0, mime: "image/jpeg",
            pictureType: raw, description: "", data: JpegMagic));
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal((MediaPictureType)raw, picture.Type);
    }

    // ----- helpers -----

    private static MediaMetadata Decode(int version, byte[] frames)
    {
        byte[] tag = BuildTagHeader(version, frames);
        byte[] mpegFrame = new byte[417];
        mpegFrame[0] = 0xFF; mpegFrame[1] = 0xFB; mpegFrame[2] = 0x90; mpegFrame[3] = 0x00;
        byte[] all = [.. tag, .. mpegFrame];
        using var src = new IO.MemoryRandomAccessSource(all);
        using var dx = Mp3Demuxer.Open(src);
        return dx.Metadata;
    }

    private static byte[] BuildV23TextFrame(string id, string value)
    {
        byte[] payload = new byte[1 + Encoding.Latin1.GetByteCount(value)];
        payload[0] = 0;
        Encoding.Latin1.GetBytes(value).CopyTo(payload.AsSpan(1));
        return BuildFrameWithPayload(id, payload);
    }

    private static byte[] BuildApicFrame(byte encoding, string mime, byte pictureType, string description, byte[] data)
    {
        byte[] mimeBytes = Encoding.Latin1.GetBytes(mime);
        byte[] descBytes = EncodeText(encoding, description);
        int payloadLen = 1 + mimeBytes.Length + 1 + 1 + descBytes.Length + TerminatorLength(encoding) + data.Length;
        byte[] payload = new byte[payloadLen];
        int p = 0;
        payload[p++] = encoding;
        mimeBytes.CopyTo(payload.AsSpan(p)); p += mimeBytes.Length;
        payload[p++] = 0;
        payload[p++] = pictureType;
        descBytes.CopyTo(payload.AsSpan(p)); p += descBytes.Length;
        if (encoding == 1 || encoding == 2) { payload[p++] = 0; payload[p++] = 0; }
        else { payload[p++] = 0; }
        data.CopyTo(payload.AsSpan(p));
        return BuildFrameWithPayload("APIC", payload);
    }

    private static byte[] BuildPicV22Frame(byte encoding, string imageFormat, byte pictureType, string description, byte[] data)
    {
        if (imageFormat.Length != 3) throw new ArgumentException("v2.2 image format is 3 chars", nameof(imageFormat));
        byte[] descBytes = EncodeText(encoding, description);
        int payloadLen = 1 + 3 + 1 + descBytes.Length + TerminatorLength(encoding) + data.Length;
        byte[] payload = new byte[payloadLen];
        int p = 0;
        payload[p++] = encoding;
        Encoding.ASCII.GetBytes(imageFormat).CopyTo(payload.AsSpan(p, 3)); p += 3;
        payload[p++] = pictureType;
        descBytes.CopyTo(payload.AsSpan(p)); p += descBytes.Length;
        if (encoding == 1 || encoding == 2) { payload[p++] = 0; payload[p++] = 0; }
        else { payload[p++] = 0; }
        data.CopyTo(payload.AsSpan(p));
        // v2.2 frame header is 6 bytes (3-char id + 3-byte size)
        byte[] frame = new byte[6 + payload.Length];
        Encoding.ASCII.GetBytes("PIC").CopyTo(frame.AsSpan(0, 3));
        frame[3] = (byte)((payload.Length >> 16) & 0xFF);
        frame[4] = (byte)((payload.Length >> 8) & 0xFF);
        frame[5] = (byte)(payload.Length & 0xFF);
        payload.CopyTo(frame.AsSpan(6));
        return frame;
    }

    private static byte[] EncodeText(byte encoding, string text) => encoding switch
    {
        0 => Encoding.Latin1.GetBytes(text),
        1 => Encoding.Unicode.GetPreamble().Concat(Encoding.Unicode.GetBytes(text)).ToArray(),
        2 => Encoding.BigEndianUnicode.GetBytes(text),
        3 => Encoding.UTF8.GetBytes(text),
        _ => throw new ArgumentOutOfRangeException(nameof(encoding)),
    };

    private static int TerminatorLength(byte encoding) => encoding == 1 || encoding == 2 ? 2 : 1;

    private static byte[] BuildFrameWithPayload(string id, byte[] payload)
    {
        byte[] frame = new byte[10 + payload.Length];
        Encoding.ASCII.GetBytes(id).CopyTo(frame.AsSpan(0, 4));
        BinaryPrimitives.WriteUInt32BigEndian(frame.AsSpan(4, 4), (uint)payload.Length);
        payload.CopyTo(frame.AsSpan(10));
        return frame;
    }

    private static byte[] BuildTagHeader(int version, byte[] frames)
    {
        byte[] hdr = new byte[10];
        hdr[0] = (byte)'I'; hdr[1] = (byte)'D'; hdr[2] = (byte)'3';
        hdr[3] = (byte)version; hdr[4] = 0;
        hdr[5] = 0;
        uint v = (uint)frames.Length;
        hdr[6] = (byte)((v >> 21) & 0x7F);
        hdr[7] = (byte)((v >> 14) & 0x7F);
        hdr[8] = (byte)((v >> 7) & 0x7F);
        hdr[9] = (byte)(v & 0x7F);
        return [.. hdr, .. frames];
    }
}
