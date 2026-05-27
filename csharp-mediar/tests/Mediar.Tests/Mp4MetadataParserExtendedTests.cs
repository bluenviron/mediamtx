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

    private static void WriteTag(Span<byte> dst, string tag)
    {
        if (tag.Length != 4) throw new ArgumentException("tag must be 4 chars", nameof(tag));
        for (int i = 0; i < 4; i++) dst[i] = (byte)tag[i];
    }
}
