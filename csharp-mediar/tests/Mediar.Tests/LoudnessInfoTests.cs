using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the loudness-normalisation parsing on
/// <see cref="MediaMetadataBuilder"/> that aggregates ReplayGain 2.0
/// and Opus R128 tag keys into the typed <see cref="LoudnessInfo"/>
/// record on <see cref="MediaMetadata.Loudness"/>.
/// </summary>
public sealed class LoudnessInfoTests
{
    // ----- TryParseReplayGainDb -----

    [Theory]
    [InlineData("-7.89 dB", -7.89)]
    [InlineData("+0.00 dB", 0.0)]
    [InlineData("-7.89", -7.89)]
    [InlineData("3.5 dB", 3.5)]
    [InlineData("-12.34 DB", -12.34)]
    [InlineData("  -2.5 dB  ", -2.5)]
    public void TryParseReplayGainDb_Accepts_Well_Formed(string input, double expected)
    {
        Assert.True(LoudnessInfo.TryParseReplayGainDb(input, out var db));
        Assert.Equal(expected, db, 4);
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("not a number")]
    [InlineData("100 dB")]
    [InlineData("-1000 dB")]
    [InlineData("NaN dB")]
    public void TryParseReplayGainDb_Rejects_Malformed(string? input)
    {
        Assert.False(LoudnessInfo.TryParseReplayGainDb(input, out var db));
        Assert.Equal(0, db);
    }

    // ----- TryParseReplayGainPeak -----

    [Theory]
    [InlineData("0.987654", 0.987654)]
    [InlineData("0.0", 0.0)]
    [InlineData("1.0", 1.0)]
    [InlineData("1.5", 1.5)]
    public void TryParseReplayGainPeak_Accepts_Well_Formed(string input, double expected)
    {
        Assert.True(LoudnessInfo.TryParseReplayGainPeak(input, out var peak));
        Assert.Equal(expected, peak, 6);
    }

    [Theory]
    [InlineData("-0.5")]
    [InlineData("11.0")]
    [InlineData("not a number")]
    [InlineData("")]
    public void TryParseReplayGainPeak_Rejects_Malformed(string? input)
    {
        Assert.False(LoudnessInfo.TryParseReplayGainPeak(input, out _));
    }

    // ----- TryParseR128Q78 -----

    [Theory]
    [InlineData("-2304", -9.0)]
    [InlineData("0", 0.0)]
    [InlineData("256", 1.0)]
    [InlineData("-256", -1.0)]
    [InlineData("32767", 127.99609375)]
    [InlineData("-32768", -128.0)]
    public void TryParseR128Q78_Accepts_Well_Formed(string input, double expected)
    {
        Assert.True(LoudnessInfo.TryParseR128Q78(input, out var db));
        Assert.Equal(expected, db, 6);
    }

    [Theory]
    [InlineData("32768")]
    [InlineData("-32769")]
    [InlineData("not a number")]
    [InlineData("3.5")]
    [InlineData(null)]
    public void TryParseR128Q78_Rejects_Malformed(string? input)
    {
        Assert.False(LoudnessInfo.TryParseR128Q78(input, out _));
    }

    // ----- Builder integration -----

    [Fact]
    public void Builder_ReplayGain_TrackGain_Populates_Typed_Field()
    {
        var b = new MediaMetadataBuilder();
        b.Set("REPLAYGAIN_TRACK_GAIN", "-7.89 dB");
        var meta = b.Build();
        Assert.NotNull(meta.Loudness);
        Assert.Equal(-7.89, meta.Loudness!.TrackGainDb!.Value, 4);
        Assert.Null(meta.Loudness.AlbumGainDb);
        Assert.Equal("-7.89 dB", meta.Tags["REPLAYGAIN_TRACK_GAIN"]);
    }

    [Fact]
    public void Builder_Full_ReplayGain_Set_Round_Trip()
    {
        var b = new MediaMetadataBuilder();
        b.Set("REPLAYGAIN_TRACK_GAIN", "-7.89 dB");
        b.Set("REPLAYGAIN_ALBUM_GAIN", "-8.50 dB");
        b.Set("REPLAYGAIN_TRACK_PEAK", "0.987654");
        b.Set("REPLAYGAIN_ALBUM_PEAK", "1.001234");
        b.Set("REPLAYGAIN_TRACK_RANGE", "6.99 dB");
        b.Set("REPLAYGAIN_ALBUM_RANGE", "7.50 dB");
        b.Set("REPLAYGAIN_REFERENCE_LOUDNESS", "-18.0 dB");
        var meta = b.Build();
        var l = meta.Loudness;
        Assert.NotNull(l);
        Assert.Equal(-7.89, l!.TrackGainDb!.Value, 4);
        Assert.Equal(-8.50, l.AlbumGainDb!.Value, 4);
        Assert.Equal(0.987654, l.TrackPeak!.Value, 6);
        Assert.Equal(1.001234, l.AlbumPeak!.Value, 6);
        Assert.Equal(6.99, l.TrackRangeDb!.Value, 4);
        Assert.Equal(7.50, l.AlbumRangeDb!.Value, 4);
        Assert.Equal(-18.0, l.ReferenceLoudnessDb!.Value, 4);
        Assert.Null(l.R128TrackGainDb);
        Assert.Null(l.R128AlbumGainDb);
    }

    [Fact]
    public void Builder_R128_Q78_Decodes_From_Integer()
    {
        var b = new MediaMetadataBuilder();
        b.Set("R128_TRACK_GAIN", "-2304");
        b.Set("R128_ALBUM_GAIN", "-1792");
        var meta = b.Build();
        var l = meta.Loudness;
        Assert.NotNull(l);
        Assert.Equal(-9.0, l!.R128TrackGainDb!.Value, 6);
        Assert.Equal(-7.0, l.R128AlbumGainDb!.Value, 6);
        Assert.Null(l.TrackGainDb);
    }

    [Fact]
    public void Builder_R128_And_ReplayGain_Coexist()
    {
        var b = new MediaMetadataBuilder();
        b.Set("REPLAYGAIN_TRACK_GAIN", "-7.89 dB");
        b.Set("R128_TRACK_GAIN", "-2304");
        var meta = b.Build();
        Assert.Equal(-7.89, meta.Loudness!.TrackGainDb!.Value, 4);
        Assert.Equal(-9.0, meta.Loudness.R128TrackGainDb!.Value, 6);
    }

    [Fact]
    public void Builder_Malformed_ReplayGain_Falls_Back_To_Tag_Only()
    {
        var b = new MediaMetadataBuilder();
        b.Set("REPLAYGAIN_TRACK_GAIN", "abc dB");
        var meta = b.Build();
        Assert.Null(meta.Loudness);
        Assert.Equal("abc dB", meta.Tags["REPLAYGAIN_TRACK_GAIN"]);
    }

    [Fact]
    public void Builder_First_Wins_Semantics_For_Duplicate_TrackGain()
    {
        var b = new MediaMetadataBuilder();
        b.Set("REPLAYGAIN_TRACK_GAIN", "-7.89 dB");
        b.Set("REPLAYGAIN_TRACK_GAIN", "+0.00 dB");
        var meta = b.Build();
        Assert.Equal(-7.89, meta.Loudness!.TrackGainDb!.Value, 4);
    }

    [Fact]
    public void Builder_Without_Loudness_Yields_Null_Loudness()
    {
        var b = new MediaMetadataBuilder();
        b.Set("TITLE", "Test");
        var meta = b.Build();
        Assert.Null(meta.Loudness);
    }

    [Fact]
    public void Builder_Only_Loudness_Causes_Build_To_Return_NonEmpty()
    {
        var b = new MediaMetadataBuilder();
        b.Set("R128_TRACK_GAIN", "-2304");
        var meta = b.Build();
        Assert.False(meta.IsEmpty);
        Assert.NotSame(MediaMetadata.Empty, meta);
        Assert.NotNull(meta.Loudness);
    }

    // ----- LoudnessInfo.IsEmpty -----

    [Fact]
    public void LoudnessInfo_IsEmpty_True_For_All_Null()
    {
        Assert.True(new LoudnessInfo().IsEmpty);
    }

    [Fact]
    public void LoudnessInfo_IsEmpty_False_When_Any_Field_Set()
    {
        Assert.False(new LoudnessInfo { TrackGainDb = -7.89 }.IsEmpty);
        Assert.False(new LoudnessInfo { R128AlbumGainDb = -9.0 }.IsEmpty);
        Assert.False(new LoudnessInfo { TrackPeak = 0.5 }.IsEmpty);
    }
}
