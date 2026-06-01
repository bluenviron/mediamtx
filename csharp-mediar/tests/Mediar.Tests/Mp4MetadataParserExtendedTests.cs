using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.IsoBmff;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the extended iTunes/MP4 ilst atom -> canonical Vorbis key
/// mappings on <see cref="Mp4MetadataParser"/>.
/// </summary>
public sealed class Mp4MetadataParserExtendedTests
{
    [Theory]
    [InlineData("\u00A9wrk", "Symphony No. 9", "Work")]
    [InlineData("\u00A9grp", "Classical", "Work")]
    [InlineData("\u00A9st3", "Live Mix", "Subtitle")]
    [InlineData("\u00A9con", "Karajan", "Conductor")]
    [InlineData("\u00A9key", "Am", "MusicalKey")]
    [InlineData("\u00A9pub", "Publisher Co", "Publisher")]
    [InlineData("desc", "Short description", "Description")]
    [InlineData("ldes", "Long description text", "Description")]
    public void Ilst_Text_Atom_Maps_To_Strong_Property(string atomTag, string value, string propertyName)
    {
        byte[] ilst = BuildIlst([BuildTextAtom(atomTag, value)]);
        var meta = ParseIlstAndBuild(ilst);
        var prop = typeof(MediaMetadata).GetProperty(propertyName)!;
        Assert.Equal(value, prop.GetValue(meta));
    }

    [Fact]
    public void Ilst_Tmpo_Maps_To_Bpm()
    {
        // tmpo is uint16 BE in dataType 21 (signed integer).
        byte[] tmpoData = [0x00, 128];
        byte[] ilst = BuildIlst([BuildIntegerAtom("tmpo", tmpoData, dataType: 21)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(128, meta.Bpm);
    }

    [Fact]
    public void Ilst_Cpil_True_Maps_To_Compilation()
    {
        byte[] ilst = BuildIlst([BuildIntegerAtom("cpil", [0x01], dataType: 21)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.True(meta.Compilation);
    }

    [Fact]
    public void Ilst_Cpil_False_Maps_To_Compilation()
    {
        byte[] ilst = BuildIlst([BuildIntegerAtom("cpil", [0x00], dataType: 21)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.False(meta.Compilation);
    }

    [Fact]
    public void Ilst_aART_Maps_To_AlbumArtist()
    {
        byte[] ilst = BuildIlst([BuildTextAtom("aART", "Various Artists")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Various Artists", meta.AlbumArtist);
    }

    [Fact]
    public void Ilst_Cprt_Maps_To_Copyright()
    {
        byte[] ilst = BuildIlst([BuildTextAtom("cprt", "(C) 2024")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("(C) 2024", meta.Copyright);
    }

    [Fact]
    public void Ilst_Multiple_Atoms_Populate_Independently()
    {
        byte[] ilst = BuildIlst(
        [
            BuildTextAtom("\u00A9nam", "Track Title"),
            BuildTextAtom("\u00A9con", "Bernstein"),
            BuildTextAtom("\u00A9wrk", "Concerto"),
            BuildIntegerAtom("tmpo", [0x00, 0x78], dataType: 21),
            BuildIntegerAtom("cpil", [0x01], dataType: 21),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Track Title", meta.Title);
        Assert.Equal("Bernstein", meta.Conductor);
        Assert.Equal("Concerto", meta.Work);
        Assert.Equal(120, meta.Bpm);
        Assert.True(meta.Compilation);
    }

    [Fact]
    public void Ilst_Unsigned_Integer_DataType_22_Is_Decoded()
    {
        // tmpo with unsigned int (dataType 22) 2-byte value 0x00A0 = 160.
        byte[] ilst = BuildIlst([BuildIntegerAtom("tmpo", [0x00, 0xA0], dataType: 22)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(160, meta.Bpm);
    }

    [Fact]
    public void Ilst_Trkn_Parses_Number_And_Total()
    {
        // trkn payload starts with 2-byte pad, then 2-byte number, 2-byte total.
        byte[] data = [0, 0, 0, 5, 0, 12];
        byte[] ilst = BuildIlst([BuildIntegerAtom("trkn", data, dataType: 0)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(5, meta.TrackNumber);
        Assert.Equal(12, meta.TrackCount);
    }

    [Fact]
    public void Ilst_Trkn_Number_Only_When_Total_Zero()
    {
        byte[] data = [0, 0, 0, 7, 0, 0];
        byte[] ilst = BuildIlst([BuildIntegerAtom("trkn", data, dataType: 0)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(7, meta.TrackNumber);
        Assert.Null(meta.TrackCount);
    }

    [Fact]
    public void Ilst_Disk_Parses_Number_And_Total()
    {
        byte[] data = [0, 0, 0, 2, 0, 3];
        byte[] ilst = BuildIlst([BuildIntegerAtom("disk", data, dataType: 0)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(2, meta.DiscNumber);
        Assert.Equal(3, meta.DiscCount);
    }

    [Fact]
    public void Ilst_Covr_Jpeg_Adds_Picture()
    {
        // dataType 13 -> JPEG.
        byte[] jpegBytes = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10];
        byte[] ilst = BuildIlst([BuildIntegerAtom("covr", jpegBytes, dataType: 13)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", pic.MimeType);
        Assert.Equal(MediaPictureType.CoverFront, pic.Type);
        Assert.Equal(jpegBytes, pic.Data.ToArray());
    }

    [Fact]
    public void Ilst_Covr_Png_Adds_Picture()
    {
        byte[] pngBytes = [0x89, 0x50, 0x4E, 0x47];
        byte[] ilst = BuildIlst([BuildIntegerAtom("covr", pngBytes, dataType: 14)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal("image/png", pic.MimeType);
    }

    [Fact]
    public void Ilst_Covr_Bmp_Adds_Picture()
    {
        byte[] bmpBytes = [0x42, 0x4D, 0x00, 0x00];
        byte[] ilst = BuildIlst([BuildIntegerAtom("covr", bmpBytes, dataType: 27)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal("image/bmp", pic.MimeType);
    }

    [Fact]
    public void Ilst_Covr_UnknownType_Falls_Back_To_OctetStream()
    {
        byte[] blob = [0xAB, 0xCD];
        byte[] ilst = BuildIlst([BuildIntegerAtom("covr", blob, dataType: 99)]);
        var meta = ParseIlstAndBuild(ilst);
        var pic = Assert.Single(meta.Pictures);
        Assert.Equal("application/octet-stream", pic.MimeType);
    }

    [Fact]
    public void Ilst_Covr_Empty_Data_Skipped()
    {
        // dataType 13 but 0 bytes of payload — should be skipped.
        byte[] ilst = BuildIlst([BuildIntegerAtom("covr", Array.Empty<byte>(), dataType: 13)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Empty(meta.Pictures);
    }

    [Fact]
    public void Ilst_Unknown_Atom_Tag_Ignored()
    {
        byte[] ilst = BuildIlst([BuildTextAtom("XXXX", "ignored")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Null(meta.Title);
        Assert.Null(meta.Artist);
    }

    [Fact]
    public void Ilst_DataType_Zero_Is_Read_As_String()
    {
        // dataType 0 with non-empty value should also map to text.
        byte[] payload = Encoding.UTF8.GetBytes("test-text");
        byte[] ilst = BuildIlst([BuildIntegerAtom("\u00A9nam", payload, dataType: 0)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("test-text", meta.Title);
    }

    [Fact]
    public void Ilst_FreeForm_Apple_iTunes_Namespace_Populates_Tag()
    {
        byte[] freeform = BuildFreeFormAtom("com.apple.iTunes", "BARCODE", Encoding.UTF8.GetBytes("0123456789012"), dataType: 1);
        byte[] ilst = BuildIlst([freeform]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("0123456789012", meta.Barcode);
    }

    [Fact]
    public void Ilst_FreeForm_Apple_QuickTime_Namespace_Populates_Tag()
    {
        byte[] freeform = BuildFreeFormAtom("com.apple.QuickTime", "MOOD", Encoding.UTF8.GetBytes("Energetic"), dataType: 1);
        byte[] ilst = BuildIlst([freeform]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Energetic", meta.Mood);
    }

    [Fact]
    public void Ilst_FreeForm_OtherVendor_Namespace_Is_Rejected()
    {
        byte[] freeform = BuildFreeFormAtom("com.sony.xxx", "BARCODE", Encoding.UTF8.GetBytes("nope"), dataType: 1);
        byte[] ilst = BuildIlst([freeform]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Null(meta.Barcode);
    }

    [Fact]
    public void Ilst_FreeForm_InitialKey_Aliases_To_MusicalKey()
    {
        byte[] freeform = BuildFreeFormAtom("com.apple.iTunes", "INITIALKEY", Encoding.UTF8.GetBytes("Cm"), dataType: 1);
        byte[] ilst = BuildIlst([freeform]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Cm", meta.MusicalKey);
    }

    [Fact]
    public void Ilst_FreeForm_Without_Name_Box_Ignored()
    {
        // Construct a '----' container with mean + data but no 'name' child.
        var mean = BuildSubBoxWithFullHeader("mean", Encoding.UTF8.GetBytes("com.apple.iTunes"));
        var data = BuildDataChild(Encoding.UTF8.GetBytes("value"), dataType: 1);
        byte[] freeform = BuildContainerAtom("----", new[] { mean, data });
        byte[] ilst = BuildIlst([freeform]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Null(meta.Barcode);
        Assert.Null(meta.Mood);
    }

    [Fact]
    public void Ilst_FreeForm_Without_Data_Box_Ignored()
    {
        var mean = BuildSubBoxWithFullHeader("mean", Encoding.UTF8.GetBytes("com.apple.iTunes"));
        var name = BuildSubBoxWithFullHeader("name", Encoding.UTF8.GetBytes("BARCODE"));
        byte[] freeform = BuildContainerAtom("----", new[] { mean, name });
        byte[] ilst = BuildIlst([freeform]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Null(meta.Barcode);
    }

    [Fact]
    public void Ilst_TmpoSignedTwoByte_Negative_Sign_Extends()
    {
        // dataType 21 = signed 16-bit BE: 0xFFFF = -1
        byte[] ilst = BuildIlst([BuildIntegerAtom("tmpo", [0xFF, 0xFF], dataType: 21)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(-1, meta.Bpm);
    }

    [Fact]
    public void Ilst_Integer_OneByte_Cpil_True_Across_DataType_21()
    {
        // Single-byte signed integer (1).
        byte[] ilst = BuildIlst([BuildIntegerAtom("cpil", [0x01], dataType: 21)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.True(meta.Compilation);
    }

    // ----- helpers -----

    private static MediaMetadata ParseIlstAndBuild(byte[] ilstBytes)
    {
        var builder = new MediaMetadataBuilder();
        // ParseMeta expects its input to already be a sequence of child boxes
        // (the FullBox version/flags header is stripped by ParseUdta).
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
        // Atom: size(4) type(4) [data sub-atom: size(4) "data"(4) typeFlags(4) locale(4) UTF-8 value]
        byte[] valueBytes = Encoding.UTF8.GetBytes(value);
        return BuildAtomWithData(tag, valueBytes, dataType: 1);
    }

    private static byte[] BuildIntegerAtom(string tag, byte[] valueBytes, uint dataType)
    {
        return BuildAtomWithData(tag, valueBytes, dataType);
    }

    private static byte[] BuildAtomWithData(string tag, byte[] valueBytes, uint dataType)
    {
        int dataAtomLen = 16 + valueBytes.Length;
        int atomLen = 8 + dataAtomLen;
        byte[] atom = new byte[atomLen];
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(0, 4), (uint)atomLen);
        WriteTag(atom.AsSpan(4, 4), tag);
        // data sub-atom
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(8, 4), (uint)dataAtomLen);
        Encoding.ASCII.GetBytes("data").CopyTo(atom.AsSpan(12, 4));
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(16, 4), dataType);
        // locale (4 bytes) = 0
        valueBytes.CopyTo(atom.AsSpan(24));
        return atom;
    }

    private static byte[] BuildSubBoxWithFullHeader(string tag, byte[] valueBytes)
    {
        // [size(4)][type(4)][version+flags(4)][value]
        int boxLen = 12 + valueBytes.Length;
        byte[] box = new byte[boxLen];
        BinaryPrimitives.WriteUInt32BigEndian(box.AsSpan(0, 4), (uint)boxLen);
        WriteTag(box.AsSpan(4, 4), tag);
        valueBytes.CopyTo(box.AsSpan(12));
        return box;
    }

    private static byte[] BuildDataChild(byte[] valueBytes, uint dataType)
    {
        // [size(4)][type=data(4)][typeFlags(4)][locale(4)][value]
        int boxLen = 16 + valueBytes.Length;
        byte[] box = new byte[boxLen];
        BinaryPrimitives.WriteUInt32BigEndian(box.AsSpan(0, 4), (uint)boxLen);
        Encoding.ASCII.GetBytes("data").CopyTo(box.AsSpan(4, 4));
        BinaryPrimitives.WriteUInt32BigEndian(box.AsSpan(8, 4), dataType);
        valueBytes.CopyTo(box.AsSpan(16));
        return box;
    }

    private static byte[] BuildContainerAtom(string tag, byte[][] children)
    {
        int total = 8;
        foreach (var c in children) total += c.Length;
        byte[] box = new byte[total];
        BinaryPrimitives.WriteUInt32BigEndian(box.AsSpan(0, 4), (uint)total);
        WriteTag(box.AsSpan(4, 4), tag);
        int p = 8;
        foreach (var c in children) { c.CopyTo(box.AsSpan(p)); p += c.Length; }
        return box;
    }

    private static byte[] BuildFreeFormAtom(string mean, string name, byte[] valueBytes, uint dataType)
    {
        var meanBox = BuildSubBoxWithFullHeader("mean", Encoding.UTF8.GetBytes(mean));
        var nameBox = BuildSubBoxWithFullHeader("name", Encoding.UTF8.GetBytes(name));
        var dataBox = BuildDataChild(valueBytes, dataType);
        return BuildContainerAtom("----", new[] { meanBox, nameBox, dataBox });
    }

    private static void WriteTag(Span<byte> dst, string tag)
    {
        if (tag.Length != 4) throw new ArgumentException("tag must be 4 chars", nameof(tag));
        for (int i = 0; i < 4; i++) dst[i] = (byte)tag[i];
    }
}
