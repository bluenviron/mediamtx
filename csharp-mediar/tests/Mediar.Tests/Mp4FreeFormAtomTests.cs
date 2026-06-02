using System.Buffers.Binary;
using System.Text;
using Mediar.Containers.IsoBmff;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for iTunes-style "----" freeform atom parsing on
/// <see cref="Mp4MetadataParser"/>. These atoms carry the bulk of
/// extended audio metadata written by MusicBrainz Picard, Mp3tag,
/// Beets, Roon and similar tools (BARCODE, CATALOGNUMBER, MOOD,
/// LICENSE, REPLAYGAIN_*, MUSICBRAINZ_* identifiers, sort variants,
/// etc.) that have no dedicated single-purpose iTunes 4CC.
/// </summary>
public sealed class Mp4FreeFormAtomTests
{
    [Fact]
    public void FreeForm_Barcode_Maps_To_Barcode()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "BARCODE", "0888072462533")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("0888072462533", meta.Barcode);
    }

    [Fact]
    public void FreeForm_CatalogNumber_Maps_To_CatalogNumber()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "CATALOGNUMBER", "DG-477-6543")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("DG-477-6543", meta.CatalogNumber);
    }

    [Fact]
    public void FreeForm_License_Maps_To_License()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "LICENSE", "CC BY-SA 4.0")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("CC BY-SA 4.0", meta.License);
    }

    [Fact]
    public void FreeForm_Mood_Maps_To_Mood()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "MOOD", "Energetic")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Energetic", meta.Mood);
    }

    [Fact]
    public void FreeForm_InitialKey_Aliases_To_MusicalKey()
    {
        // Picard / Mp3tag / Beets write the key as "initialkey" or
        // "INITIALKEY"; both alias to the canonical MusicalKey field.
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "initialkey", "Am")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Am", meta.MusicalKey);
    }

    [Fact]
    public void FreeForm_Website_Maps_To_Website()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "WEBSITE", "https://example.org")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("https://example.org", meta.Website);
    }

    [Fact]
    public void FreeForm_Unknown_Key_Flows_To_Tags()
    {
        // Unknown freeform names should still be readable via the Tags
        // dictionary (upper-cased) so callers aren't blocked.
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "CustomKey", "Some Value")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Some Value", meta.Tags["CUSTOMKEY"]);
    }

    [Fact]
    public void FreeForm_ReplayGain_Track_Gain_Flows_To_Tags()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "REPLAYGAIN_TRACK_GAIN", "-6.42 dB")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("-6.42 dB", meta.Tags["REPLAYGAIN_TRACK_GAIN"]);
    }

    [Fact]
    public void FreeForm_MusicBrainz_TrackId_Normalises_To_UnderscoreForm()
    {
        // Picard writes the key with spaces ("MusicBrainz Track Id"); the
        // canonical Vorbis-style key uses underscores
        // ("MUSICBRAINZ_TRACKID"). Both must reach the same dictionary
        // entry.
        byte[] guid = Encoding.UTF8.GetBytes("d39c1a4f-7d9f-4d6e-9e1b-1a2b3c4d5e6f");
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "MusicBrainz Track Id", "d39c1a4f-7d9f-4d6e-9e1b-1a2b3c4d5e6f")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("d39c1a4f-7d9f-4d6e-9e1b-1a2b3c4d5e6f", meta.Tags["MUSICBRAINZ_TRACKID"]);
    }

    [Fact]
    public void FreeForm_AcoustId_Fingerprint_Folds_Onto_Canonical_Key()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "Acoustid Fingerprint", "AQAAA...")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("AQAAA...", meta.Tags["ACOUSTID_FINGERPRINT"]);
    }

    [Fact]
    public void FreeForm_Unknown_Mean_Namespace_Is_Ignored()
    {
        // Non-Apple namespaces use the same wire layout but may carry
        // conflicting keys (e.g. Sony's "----:com.sony.xxx" arrays). The
        // parser must skip them to avoid polluting the canonical
        // metadata dictionary.
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.sony.preview", "BARCODE", "0000000000000")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Null(meta.Barcode);
        Assert.False(meta.Tags.ContainsKey("BARCODE"));
    }

    [Fact]
    public void FreeForm_QuickTime_Namespace_Is_Accepted()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.QuickTime", "LICENSE", "Public Domain")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Public Domain", meta.License);
    }

    [Fact]
    public void FreeForm_Empty_Name_Is_Ignored()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "", "Some Value")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.True(meta.Tags.Count == 0);
    }

    [Fact]
    public void FreeForm_Mixed_With_Standard_Atom_Both_Populate()
    {
        // Real-world M4A files have both ©nam (title) and ---- freeform
        // atoms in the same ilst container; both must coexist.
        byte[] ilst = BuildIlst([
            BuildTextAtom("\u00A9nam", "Symphony No. 9"),
            BuildFreeFormAtom("com.apple.iTunes", "BARCODE", "0888072462533"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Symphony No. 9", meta.Title);
        Assert.Equal("0888072462533", meta.Barcode);
    }

    [Fact]
    public void FreeForm_Mood_LowercaseKey_AliasesToCanonical()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "mood", "Reflective")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Reflective", meta.Mood);
    }

    [Theory]
    [InlineData("REPLAYGAIN_TRACK_GAIN", "REPLAYGAIN_TRACK_GAIN")]
    [InlineData("REPLAYGAIN_TRACK_PEAK", "REPLAYGAIN_TRACK_PEAK")]
    [InlineData("REPLAYGAIN_ALBUM_GAIN", "REPLAYGAIN_ALBUM_GAIN")]
    [InlineData("REPLAYGAIN_ALBUM_PEAK", "REPLAYGAIN_ALBUM_PEAK")]
    public void FreeForm_AllReplayGainKeys_FlowToTags(string key, string canonicalKey)
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", key, "0.000000 dB")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("0.000000 dB", meta.Tags[canonicalKey]);
    }

    [Theory]
    [InlineData("MusicBrainz Album Id", "MUSICBRAINZ_ALBUMID")]
    [InlineData("MusicBrainz Artist Id", "MUSICBRAINZ_ARTISTID")]
    [InlineData("MusicBrainz Album Artist Id", "MUSICBRAINZ_ALBUMARTISTID")]
    [InlineData("MusicBrainz Release Track Id", "MUSICBRAINZ_RELEASETRACKID")]
    [InlineData("MusicBrainz Release Group Id", "MUSICBRAINZ_RELEASEGROUPID")]
    [InlineData("Acoustid Id", "ACOUSTID_ID")]
    public void FreeForm_MusicBrainz_AndAcoustId_KeyNormalisation(string source, string canonical)
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", source, "guid-value")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("guid-value", meta.Tags[canonical]);
    }

    [Fact]
    public void FreeForm_MissingMean_Atom_StillAccepted()
    {
        // If the mean sub-atom is empty/missing, the parser treats the
        // namespace as the default (Apple) and accepts the value.
        byte[] ilst = BuildIlst([BuildFreeFormAtom("", "MOOD", "Quiet")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Quiet", meta.Mood);
    }

    [Fact]
    public void FreeForm_LongValue_NotTruncated()
    {
        string longValue = new string('x', 4096);
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "CUSTOMKEY", longValue)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(longValue, meta.Tags["CUSTOMKEY"]);
        Assert.Equal(4096, meta.Tags["CUSTOMKEY"].Length);
    }

    [Fact]
    public void FreeForm_TwoFreeFormAtomsForSameKey_FirstValueWins()
    {
        byte[] ilst = BuildIlst([
            BuildFreeFormAtom("com.apple.iTunes", "BARCODE", "1111111111111"),
            BuildFreeFormAtom("com.apple.iTunes", "BARCODE", "2222222222222"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        // MediaMetadataBuilder.Set is first-write-wins, so the first BARCODE persists.
        Assert.Equal("1111111111111", meta.Barcode);
    }

    [Fact]
    public void FreeForm_QuickTime_Namespace_CaseInsensitive()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("COM.APPLE.QUICKTIME", "MOOD", "Calm")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("Calm", meta.Mood);
    }

    [Fact]
    public void FreeForm_EmptyValue_DoesNotProduceTag()
    {
        // Empty payloads are ignored: the atom parser drops zero-length data.
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "CUSTOMKEY", "")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.False(meta.Tags.ContainsKey("CUSTOMKEY"));
    }

    [Fact]
    public void FreeForm_MultipleDistinctKeys_AllFlow_To_Tags()
    {
        byte[] ilst = BuildIlst([
            BuildFreeFormAtom("com.apple.iTunes", "CUSTOM_A", "AAA"),
            BuildFreeFormAtom("com.apple.iTunes", "CUSTOM_B", "BBB"),
            BuildFreeFormAtom("com.apple.iTunes", "CUSTOM_C", "CCC"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("AAA", meta.Tags["CUSTOM_A"]);
        Assert.Equal("BBB", meta.Tags["CUSTOM_B"]);
        Assert.Equal("CCC", meta.Tags["CUSTOM_C"]);
    }

    [Theory]
    [InlineData("BARCODE")]
    [InlineData("barcode")]
    [InlineData("BarCode")]
    public void FreeForm_BarcodeKey_CaseInsensitive_MapsToCanonical(string keyCasing)
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", keyCasing, "1234567890123")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("1234567890123", meta.Barcode);
    }

    [Fact]
    public void FreeForm_Mood_UppercaseKey_Wins_Over_LowercaseDuplicate()
    {
        // First-write-wins semantics: the first MOOD entry persists.
        byte[] ilst = BuildIlst([
            BuildFreeFormAtom("com.apple.iTunes", "MOOD", "First"),
            BuildFreeFormAtom("com.apple.iTunes", "mood", "Second"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("First", meta.Mood);
    }

    [Theory]
    [InlineData("REPLAYGAIN_TRACK_GAIN")]
    [InlineData("REPLAYGAIN_TRACK_PEAK")]
    [InlineData("REPLAYGAIN_ALBUM_GAIN")]
    [InlineData("REPLAYGAIN_ALBUM_PEAK")]
    [InlineData("REPLAYGAIN_REFERENCE_LOUDNESS")]
    public void FreeForm_AnyReplayGainKey_FlowsToTags(string key)
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", key, "-3.21 dB")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("-3.21 dB", meta.Tags[key]);
    }

    [Fact]
    public void FreeForm_Unicode_NonAscii_ValueRoundTripsUTF8()
    {
        // Mediar metadata is UTF-8 across the wire; ensure non-ASCII content
        // (Japanese + emoji) survives parsing intact.
        string original = "ジャズ 🎷";
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "GENRE_HINT", original)]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal(original, meta.Tags["GENRE_HINT"]);
    }

    [Fact]
    public void FreeForm_Sony_Namespace_Mood_Ignored()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.sony.preview", "MOOD", "Calm")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Null(meta.Mood);
    }

    [Fact]
    public void FreeForm_WhitespaceOnlyValue_FlowsToTags()
    {
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", "CUSTOMKEY", "   ")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("   ", meta.Tags["CUSTOMKEY"]);
    }

    [Fact]
    public void FreeForm_LongFreeformKey_FlowsThroughVerbatim()
    {
        // Long custom keys not specifically normalised end up under the
        // raw key in Tags.
        const string key = "ARTIST_SORT_KEY_LONG";
        byte[] ilst = BuildIlst([BuildFreeFormAtom("com.apple.iTunes", key, "ZZZ")]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("ZZZ", meta.Tags[key]);
    }

    [Fact]
    public void FreeForm_MixedNamespaces_ItunesAndSonyKeys_OnlyItunesParsed()
    {
        byte[] ilst = BuildIlst([
            BuildFreeFormAtom("com.apple.iTunes", "BARCODE", "0001"),
            BuildFreeFormAtom("com.sony.preview", "BARCODE", "FFFF"),
        ]);
        var meta = ParseIlstAndBuild(ilst);
        Assert.Equal("0001", meta.Barcode);
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
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(16, 4), 1u); // dataType = UTF-8
        valueBytes.CopyTo(atom.AsSpan(24));
        return atom;
    }

    /// <summary>
    /// Build a "----" freeform atom carrying three child sub-atoms:
    /// <c>mean</c> (FullBox + namespace string), <c>name</c> (FullBox +
    /// key name), and <c>data</c> (typeFlags + locale + UTF-8 value).
    /// </summary>
    private static byte[] BuildFreeFormAtom(string mean, string name, string value)
    {
        byte[] meanBytes = Encoding.UTF8.GetBytes(mean);
        byte[] nameBytes = Encoding.UTF8.GetBytes(name);
        byte[] valueBytes = Encoding.UTF8.GetBytes(value);

        int meanAtomLen = 8 + 4 + meanBytes.Length; // size+type+ver/flags+payload
        int nameAtomLen = 8 + 4 + nameBytes.Length;
        int dataAtomLen = 8 + 4 + 4 + valueBytes.Length; // size+type+typeFlags+locale+payload

        int atomLen = 8 + meanAtomLen + nameAtomLen + dataAtomLen;
        byte[] atom = new byte[atomLen];
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(0, 4), (uint)atomLen);
        WriteTag(atom.AsSpan(4, 4), "----");
        int p = 8;

        // mean sub-atom
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(p, 4), (uint)meanAtomLen);
        Encoding.ASCII.GetBytes("mean").CopyTo(atom.AsSpan(p + 4, 4));
        // 4 bytes version + flags = 0
        meanBytes.CopyTo(atom.AsSpan(p + 12));
        p += meanAtomLen;

        // name sub-atom
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(p, 4), (uint)nameAtomLen);
        Encoding.ASCII.GetBytes("name").CopyTo(atom.AsSpan(p + 4, 4));
        nameBytes.CopyTo(atom.AsSpan(p + 12));
        p += nameAtomLen;

        // data sub-atom
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(p, 4), (uint)dataAtomLen);
        Encoding.ASCII.GetBytes("data").CopyTo(atom.AsSpan(p + 4, 4));
        BinaryPrimitives.WriteUInt32BigEndian(atom.AsSpan(p + 8, 4), 1u); // dataType = UTF-8
        // 4 bytes locale = 0
        valueBytes.CopyTo(atom.AsSpan(p + 16));

        return atom;
    }

    private static void WriteTag(Span<byte> dst, string tag)
    {
        if (tag.Length != 4) throw new ArgumentException("tag must be 4 chars", nameof(tag));
        for (int i = 0; i < 4; i++) dst[i] = (byte)tag[i];
    }
}
