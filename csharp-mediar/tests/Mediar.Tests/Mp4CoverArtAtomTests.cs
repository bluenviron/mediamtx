using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.IsoBmff;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the iTunes <c>covr</c> cover-art atom parsed by
/// <see cref="Mp4MetadataParser"/>. The atom holds one or more
/// <c>data</c> children whose iTunes dataType identifies the image
/// MIME type (13 = JPEG, 14 = PNG, 27 = BMP) and whose payload is
/// the raw encoded image bytes.
/// </summary>
public sealed class Mp4CoverArtAtomTests
{
    private static readonly byte[] JpegPayload = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46];
    private static readonly byte[] PngPayload = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A];
    private static readonly byte[] BmpPayload = [0x42, 0x4D, 0x36, 0x00, 0x00, 0x00, 0x00, 0x00];

    [Fact]
    public void Covr_Jpeg_Picture_Is_Surfaced_As_Image_Jpeg()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", picture.MimeType);
        Assert.Equal(MediaPictureType.CoverFront, picture.Type);
        Assert.Equal(JpegPayload, picture.Data.ToArray());
    }

    [Fact]
    public void Covr_Png_Picture_Is_Surfaced_As_Image_Png()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 14, PngPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/png", picture.MimeType);
        Assert.Equal(MediaPictureType.CoverFront, picture.Type);
        Assert.Equal(PngPayload, picture.Data.ToArray());
    }

    [Fact]
    public void Covr_Bmp_Picture_Is_Surfaced_As_Image_Bmp()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 27, BmpPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("image/bmp", picture.MimeType);
    }

    [Fact]
    public void Covr_With_Multiple_Data_Children_Surfaces_Multiple_Pictures()
    {
        // iTunes occasionally stores both front + back covers as separate
        // 'data' children inside a single covr atom.
        byte[] ilst = BuildIlst([BuildCovrAtomMulti((13, JpegPayload), (14, PngPayload))]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(2, meta.Pictures.Count);
        Assert.Equal("image/jpeg", meta.Pictures[0].MimeType);
        Assert.Equal("image/png", meta.Pictures[1].MimeType);
    }

    [Fact]
    public void Covr_Coexists_With_Standard_Text_Atoms()
    {
        byte[] ilst = BuildIlst([
            BuildTextAtom("\u00A9nam", "Cover Art Song"),
            BuildCovrAtom(dataType: 13, JpegPayload),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Cover Art Song", meta.Title);
        Assert.Single(meta.Pictures);
    }

    [Fact]
    public void Covr_With_Empty_Payload_Adds_No_Picture()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, [])]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Covr_With_Unknown_DataType_Falls_Back_To_Octet_Stream()
    {
        // dataType=0 is "implicit" in the iTunes spec; an unknown binary
        // payload still surfaces, but with a generic MIME type so callers
        // can sniff it.
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 0, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal("application/octet-stream", picture.MimeType);
    }

    // ----- helpers -----

    private static MediaMetadata ParseIlstAndBuild(byte[] ilstBytes)
    {
        var builder = new MediaMetadataBuilder();
        Mp4MetadataParser.ParseMeta(ilstBytes, builder);
        return builder.Build();
    }

    private static byte[] BuildIlst(byte[][] atoms)
    {
        int totalChildren = 0;
        foreach (var a in atoms) totalChildren += a.Length;
        byte[] ilst = new byte[8 + totalChildren];
        BinaryPrimitives.WriteUInt32BigEndian(ilst.AsSpan(0, 4), (uint)ilst.Length);
        Encoding.ASCII.GetBytes("ilst").CopyTo(ilst.AsSpan(4, 4));
        int p = 8;
        foreach (var a in atoms)
        {
            a.CopyTo(ilst.AsSpan(p));
            p += a.Length;
        }
        return ilst;
    }

    private static byte[] BuildTextAtom(string tag, string value)
    {
        byte[] valueBytes = Encoding.UTF8.GetBytes(value);
        int dataAtomLen = 16 + valueBytes.Length;
        int atomLen = 8 + dataAtomLen;
        byte[] atom = new byte[atomLen];
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(0, 4), (uint)atomLen);
        WriteTag(atom.AsSpan(4, 4), tag);
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(8, 4), (uint)dataAtomLen);
        Encoding.ASCII.GetBytes("data").CopyTo(atom.AsSpan(12, 4));
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(16, 4), 1u);
        valueBytes.CopyTo(atom.AsSpan(24));
        return atom;
    }

    private static byte[] BuildCovrAtom(uint dataType, byte[] payload)
        => BuildCovrAtomMulti((dataType, payload));

    private static byte[] BuildCovrAtomMulti(params (uint dataType, byte[] payload)[] entries)
    {
        int childrenLen = 0;
        foreach (var e in entries) childrenLen += 16 + e.payload.Length;
        int atomLen = 8 + childrenLen;
        byte[] atom = new byte[atomLen];
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(0, 4), (uint)atomLen);
        WriteTag(atom.AsSpan(4, 4), "covr");
        int p = 8;
        foreach (var (dataType, payload) in entries)
        {
            int dataAtomLen = 16 + payload.Length;
            BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(p, 4), (uint)dataAtomLen);
            Encoding.ASCII.GetBytes("data").CopyTo(atom.AsSpan(p + 4, 4));
            BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(p + 8, 4), dataType);
            // locale field is 4 bytes = 0
            payload.CopyTo(atom.AsSpan(p + 16));
            p += dataAtomLen;
        }
        return atom;
    }

    private static void WriteTag(Span<byte> dst, string tag)
    {
        if (tag.Length != 4) throw new ArgumentException("tag must be 4 chars", nameof(tag));
        for (int i = 0; i < 4; i++) dst[i] = (byte)tag[i];
    }
}
