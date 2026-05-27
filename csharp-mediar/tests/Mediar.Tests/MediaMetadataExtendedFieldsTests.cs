using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the extended <see cref="MediaMetadataBuilder"/> field set.
/// Verifies that the additional Vorbis Comment / FLAC / iTunes tag
/// canonical key aliases populate the matching strong-typed properties
/// on the immutable <see cref="MediaMetadata"/> snapshot.
/// </summary>
public sealed class MediaMetadataExtendedFieldsTests
{
    [Theory]
    [InlineData("LYRICIST", "John Doe")]
    [InlineData("CONDUCTOR", "Herbert von Karajan")]
    [InlineData("ARRANGER", "Quincy Jones")]
    [InlineData("ENGINEER", "Geoff Emerick")]
    [InlineData("PRODUCER", "George Martin")]
    [InlineData("MUSICALKEY", "Am")]
    [InlineData("MOOD", "Melancholic")]
    [InlineData("LICENSE", "CC-BY-SA-4.0")]
    [InlineData("CATALOGNUMBER", "TEST-0001")]
    [InlineData("BARCODE", "0123456789012")]
    [InlineData("SUBTITLE", "Live at the Apollo")]
    [InlineData("DISCSUBTITLE", "Bonus Disc")]
    [InlineData("WORK", "Symphony No. 9 in D minor")]
    public void Builder_Maps_New_String_Field(string key, string value)
    {
        var b = new MediaMetadataBuilder();
        b.Set(key, value);
        var m = b.Build();
        // Resolve the corresponding strong-property by reflection (cheap, ok for tests).
        // The canonical key maps to a Pascal-case property name; the key list intentionally
        // uses the canonical (uppercase) Vorbis comment names so this test does the lookup.
        string propName = key switch
        {
            "LYRICIST" => "Lyricist",
            "CONDUCTOR" => "Conductor",
            "ARRANGER" => "Arranger",
            "ENGINEER" => "Engineer",
            "PRODUCER" => "Producer",
            "MUSICALKEY" => "MusicalKey",
            "MOOD" => "Mood",
            "LICENSE" => "License",
            "CATALOGNUMBER" => "CatalogNumber",
            "BARCODE" => "Barcode",
            "SUBTITLE" => "Subtitle",
            "DISCSUBTITLE" => "DiscSubtitle",
            "WORK" => "Work",
            _ => throw new ArgumentException("unknown key", nameof(key)),
        };
        var prop = typeof(MediaMetadata).GetProperty(propName)
            ?? throw new InvalidOperationException("missing property: " + propName);
        Assert.Equal(value, prop.GetValue(m));
    }

    [Fact]
    public void Builder_Maps_Remixer_Aliases()
    {
        var b = new MediaMetadataBuilder();
        b.Set("MIXARTIST", "Tiesto");
        var m = b.Build();
        Assert.Equal("Tiesto", m.Remixer);
    }

    [Fact]
    public void Builder_Maps_Website_Aliases()
    {
        var b = new MediaMetadataBuilder();
        b.Set("CONTACT", "https://example.com");
        var m = b.Build();
        Assert.Equal("https://example.com", m.Website);
    }

    [Fact]
    public void Builder_Maps_Bpm_Integer()
    {
        var b = new MediaMetadataBuilder();
        b.Set("BPM", "128");
        var m = b.Build();
        Assert.Equal(128, m.Bpm);
    }

    [Fact]
    public void Builder_Maps_Bpm_Decimal_Rounds_To_Int()
    {
        var b = new MediaMetadataBuilder();
        b.Set("TEMPO", "127.5");
        var m = b.Build();
        Assert.Equal(128, m.Bpm);
    }

    [Fact]
    public void Builder_Ignores_Invalid_Bpm()
    {
        var b = new MediaMetadataBuilder();
        b.Set("BPM", "not a number");
        Assert.Null(b.Bpm);
    }

    [Theory]
    [InlineData("1", true)]
    [InlineData("0", false)]
    [InlineData("true", true)]
    [InlineData("YES", true)]
    [InlineData("false", false)]
    [InlineData("NO", false)]
    public void Builder_Maps_Compilation_Booleans(string raw, bool expected)
    {
        var b = new MediaMetadataBuilder();
        b.Set("COMPILATION", raw);
        var m = b.Build();
        Assert.Equal(expected, m.Compilation);
    }

    [Fact]
    public void Builder_Maps_Compilation_iTunes_Tag()
    {
        var b = new MediaMetadataBuilder();
        b.Set("ITUNESCOMPILATION", "1");
        var m = b.Build();
        Assert.True(m.Compilation);
    }

    [Fact]
    public void Builder_Ignores_Unknown_Compilation_Value()
    {
        var b = new MediaMetadataBuilder();
        b.Set("COMPILATION", "maybe");
        Assert.Null(b.Compilation);
    }

    [Fact]
    public void Builder_Maps_Version_Aliases()
    {
        var b = new MediaMetadataBuilder();
        b.Set("MIX", "Extended Mix");
        var m = b.Build();
        Assert.Equal("Extended Mix", m.Version);
    }

    [Fact]
    public void Builder_Maps_Work_Aliases()
    {
        var b = new MediaMetadataBuilder();
        b.Set("GROUPING", "Symphonies");
        var m = b.Build();
        Assert.Equal("Symphonies", m.Work);
    }

    [Fact]
    public void Builder_Maps_Lyrics_Aliases()
    {
        var b = new MediaMetadataBuilder();
        b.Set("UNSYNCEDLYRICS", "La la la");
        var m = b.Build();
        Assert.Equal("La la la", m.Lyrics);
    }

    [Fact]
    public void Builder_Maps_License_British_Spelling()
    {
        var b = new MediaMetadataBuilder();
        b.Set("LICENCE", "All Rights Reserved");
        var m = b.Build();
        Assert.Equal("All Rights Reserved", m.License);
    }

    [Fact]
    public void Builder_Maps_Catalog_And_Barcode_Aliases()
    {
        var b = new MediaMetadataBuilder();
        b.Set("CATALOG", "ABC-100");
        b.Set("UPC", "9876543210123");
        var m = b.Build();
        Assert.Equal("ABC-100", m.CatalogNumber);
        Assert.Equal("9876543210123", m.Barcode);
    }

    [Fact]
    public void Builder_IsEmpty_True_Until_Any_Field_Set()
    {
        var b = new MediaMetadataBuilder();
        Assert.True(b.IsEmpty);
        b.Set("PRODUCER", "Quincy");
        Assert.False(b.IsEmpty);
    }

    [Fact]
    public void Builder_Build_Returns_Empty_Singleton_When_No_Fields()
    {
        var b = new MediaMetadataBuilder();
        Assert.Same(MediaMetadata.Empty, b.Build());
    }

    [Fact]
    public void Builder_Build_Preserves_All_New_Fields()
    {
        var b = new MediaMetadataBuilder();
        b.Set("LYRICIST", "Lyricist");
        b.Set("CONDUCTOR", "Conductor");
        b.Set("REMIXER", "Remixer");
        b.Set("ARRANGER", "Arranger");
        b.Set("ENGINEER", "Engineer");
        b.Set("PRODUCER", "Producer");
        b.Set("BPM", "120");
        b.Set("KEY", "Cmaj");
        b.Set("MOOD", "Happy");
        b.Set("COMPILATION", "1");
        b.Set("LICENSE", "License");
        b.Set("WEBSITE", "https://w");
        b.Set("CATALOGNUMBER", "Cat-1");
        b.Set("BARCODE", "00000");
        b.Set("SUBTITLE", "Subtitle");
        b.Set("DISCSUBTITLE", "DiscSub");
        b.Set("WORK", "Work");
        b.Set("VERSION", "Version");
        var m = b.Build();
        Assert.Equal("Lyricist", m.Lyricist);
        Assert.Equal("Conductor", m.Conductor);
        Assert.Equal("Remixer", m.Remixer);
        Assert.Equal("Arranger", m.Arranger);
        Assert.Equal("Engineer", m.Engineer);
        Assert.Equal("Producer", m.Producer);
        Assert.Equal(120, m.Bpm);
        Assert.Equal("Cmaj", m.MusicalKey);
        Assert.Equal("Happy", m.Mood);
        Assert.True(m.Compilation);
        Assert.Equal("License", m.License);
        Assert.Equal("https://w", m.Website);
        Assert.Equal("Cat-1", m.CatalogNumber);
        Assert.Equal("00000", m.Barcode);
        Assert.Equal("Subtitle", m.Subtitle);
        Assert.Equal("DiscSub", m.DiscSubtitle);
        Assert.Equal("Work", m.Work);
        Assert.Equal("Version", m.Version);
        Assert.False(m.IsEmpty);
    }
}
