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

    [Fact]
    public void Covr_With_Three_Pictures_All_Surface_In_Order()
    {
        byte[] ilst = BuildIlst([BuildCovrAtomMulti(
            (13, JpegPayload),
            (14, PngPayload),
            (27, BmpPayload))]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(3, meta.Pictures.Count);
        Assert.Equal("image/jpeg", meta.Pictures[0].MimeType);
        Assert.Equal("image/png", meta.Pictures[1].MimeType);
        Assert.Equal("image/bmp", meta.Pictures[2].MimeType);
    }

    [Fact]
    public void Covr_Picture_Description_Defaults_To_EmptyString()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(string.Empty, picture.Description);
    }

    [Fact]
    public void Covr_Picture_Has_CoverFront_Type_Even_When_DataType_Implicit()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 0, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var picture = Assert.Single(meta.Pictures);
        Assert.Equal(MediaPictureType.CoverFront, picture.Type);
    }

    [Fact]
    public void Covr_Multiple_Atoms_Accumulate_Pictures()
    {
        byte[] ilst = BuildIlst([
            BuildCovrAtom(dataType: 13, JpegPayload),
            BuildCovrAtom(dataType: 14, PngPayload),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(2, meta.Pictures.Count);
    }

    [Fact]
    public void Covr_With_All_Empty_Children_Adds_No_Pictures()
    {
        // Two data children, both empty payloads — both should be skipped.
        byte[] ilst = BuildIlst([BuildCovrAtomMulti(
            (13, Array.Empty<byte>()),
            (14, Array.Empty<byte>()))]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Covr_With_Mixed_Empty_And_Nonempty_Children_Only_NonEmpty_Survives()
    {
        byte[] ilst = BuildIlst([BuildCovrAtomMulti(
            (13, Array.Empty<byte>()),
            (14, PngPayload),
            (27, Array.Empty<byte>()))]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal("image/png", pic.MimeType);
        Assert.Equal(PngPayload, pic.Data.ToArray());
    }

    [Fact]
    public void Covr_Then_Multiple_Standard_Text_Atoms_Coexist()
    {
        byte[] ilst = BuildIlst([
            BuildCovrAtom(dataType: 14, PngPayload),
            BuildTextAtom("\u00A9nam", "Title"),
            BuildTextAtom("\u00A9ART", "Artist"),
            BuildTextAtom("\u00A9alb", "Album"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Title", meta.Title);
        Assert.Equal("Artist", meta.Artist);
        Assert.Equal("Album", meta.Album);
        Assert.Single(meta.Pictures);
    }

    [Fact]
    public void Covr_Picture_Data_Is_Independent_Of_Source_Buffer()
    {
        // Mutating the original ilst buffer must not bleed into the picture.
        byte[] src = (byte[])JpegPayload.Clone();
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, src)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        var snapshot = pic.Data.ToArray();
        for (int i = 0; i < ilst.Length; i++) ilst[i] = 0xFF;
        Assert.Equal(snapshot, pic.Data.ToArray());
    }

    [Fact]
    public void Covr_Single_Byte_Payload_Is_Surfaced()
    {
        byte[] tiny = [0x42];
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, tiny)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal(tiny, pic.Data.ToArray());
    }

    [Fact]
    public void Covr_DataType_Unknown_Nonzero_Falls_Back_To_OctetStream()
    {
        // 99 isn't 13/14/27 - it should still surface with octet-stream MIME.
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 99, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal("application/octet-stream", pic.MimeType);
    }

    [Fact]
    public void Covr_Multiple_Atoms_With_Mixed_Single_And_Multi_Data_Accumulate_In_Order()
    {
        byte[] ilst = BuildIlst([
            BuildCovrAtom(dataType: 13, JpegPayload),
            BuildCovrAtomMulti((14, PngPayload), (27, BmpPayload)),
            BuildCovrAtom(dataType: 14, PngPayload),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(4, meta.Pictures.Count);
        Assert.Equal("image/jpeg", meta.Pictures[0].MimeType);
        Assert.Equal("image/png", meta.Pictures[1].MimeType);
        Assert.Equal("image/bmp", meta.Pictures[2].MimeType);
        Assert.Equal("image/png", meta.Pictures[3].MimeType);
    }

    [Fact]
    public void Covr_Large_Payload_256_Bytes_Surfaced_Exactly()
    {
        byte[] big = new byte[256];
        for (int i = 0; i < big.Length; i++) big[i] = (byte)(i ^ 0xAA);
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, big)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal(big, pic.Data.ToArray());
    }

    [Fact]
    public void Covr_Picture_Source_Is_Mp4_Cover_Art()
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType: 13, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        // Cover art always surfaces as the front-cover variant regardless of dataType.
        Assert.Equal(MediaPictureType.CoverFront, pic.Type);
    }

    [Fact]
    public void Covr_With_Text_Atoms_Before_And_After_Coexists()
    {
        byte[] ilst = BuildIlst([
            BuildTextAtom("\u00A9nam", "Track Name"),
            BuildCovrAtom(dataType: 13, JpegPayload),
            BuildTextAtom("\u00A9ART", "Track Artist"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Track Name", meta.Title);
        Assert.Equal("Track Artist", meta.Artist);
        Assert.Single(meta.Pictures);
    }

    [Theory]
    [InlineData((uint)13, "image/jpeg")]
    [InlineData((uint)14, "image/png")]
    [InlineData((uint)27, "image/bmp")]
    public void Covr_DataType_To_MimeType_Mapping(uint dataType, string expectedMime)
    {
        byte[] ilst = BuildIlst([BuildCovrAtom(dataType, JpegPayload)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal(expectedMime, pic.MimeType);
    }

    [Fact]
    public void Covr_All_DataTypes_From_Same_Atom_Surface_Distinctly()
    {
        byte[] ilst = BuildIlst([BuildCovrAtomMulti(
            (13, JpegPayload),
            (14, PngPayload),
            (27, BmpPayload),
            (0, JpegPayload))]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(4, meta.Pictures.Count);
        Assert.Equal("image/jpeg", meta.Pictures[0].MimeType);
        Assert.Equal("image/png", meta.Pictures[1].MimeType);
        Assert.Equal("image/bmp", meta.Pictures[2].MimeType);
        Assert.Equal("application/octet-stream", meta.Pictures[3].MimeType);
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
