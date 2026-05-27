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
