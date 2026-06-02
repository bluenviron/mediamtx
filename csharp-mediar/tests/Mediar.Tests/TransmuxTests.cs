using Mediar.Containers.Caf;
using Mediar.Containers.Ogg;
using Mediar.Containers.Wav;
using Xunit;

namespace Mediar.Tests;

public sealed class TransmuxTests
{
    [Fact]
    public async Task Transmux_Ogg_To_Ogg_Roundtrips_Samples()
    {
        var payloads = BuildPayloads(4, 40);
        var src = await WriteOggOpusAsync(payloads);
        var dst = TempPath(".ogg");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            await using var dem = OggDemuxer.Open(dst);
            Assert.Single(dem.Tracks);
            Assert.Equal(CodecId.Opus, ((AudioCodecParameters)dem.Tracks[0].Codec).Codec);
            int recovered = 0;
            await foreach (var s in dem.ReadSamplesAsync())
            {
                Assert.Equal(payloads[recovered], s.Data.ToArray());
                s.Owner?.Dispose();
                recovered++;
            }
            Assert.Equal(payloads.Length, recovered);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task Transmux_Wav_To_Wav_PreservesSampleData()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2]; // 1s mono S16
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i & 0xFF);
        var src = await WriteWavPcm16Async(sr, 1, pcm);
        var dst = TempPath(".wav");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            await using var dem = WavDemuxer.Open(dst);
            var a = (AudioCodecParameters)dem.Tracks[0].Codec;
            Assert.Equal(CodecId.PcmS16Le, a.Codec);
            long total = 0;
            await foreach (var s in dem.ReadSamplesAsync())
            {
                try { total += s.Data.Length; }
                finally { s.Owner?.Dispose(); }
            }
            Assert.Equal(pcm.Length, total);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task Transmux_Wav_To_Caf_Cross_Format()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr];
        var src = await WriteWavPcm16Async(sr, 1, pcm);
        var dst = TempPath(".caf");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            await using var dem = CafDemuxer.Open(dst);
            Assert.Equal("caf", dem.FormatName);
            Assert.Equal(CodecId.PcmS16Le, ((AudioCodecParameters)dem.Tracks[0].Codec).Codec);
            long total = 0;
            await foreach (var s in dem.ReadSamplesAsync())
            {
                try { total += s.Data.Length; }
                finally { s.Owner?.Dispose(); }
            }
            Assert.Equal(pcm.Length, total);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task Transmux_TrackCount_Preserved()
    {
        var src = await WriteOggOpusAsync(BuildPayloads(2, 16));
        var dst = TempPath(".ogg");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            await using var dem = OggDemuxer.Open(dst);
            Assert.Single(dem.Tracks);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task TransmuxAsync_Null_Source_Throws()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await MediarOperations.TransmuxAsync(null!, "x.wav"));
    }

    [Fact]
    public async Task TransmuxAsync_Null_Destination_Throws()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await MediarOperations.TransmuxAsync("x.wav", null!));
    }

    // ---------- ExtractAudioAsync ----------

    [Fact]
    public async Task ExtractAudio_Ogg_To_M4A()
    {
        var payloads = BuildPayloads(3, 32);
        var src = await WriteOggOpusAsync(payloads);
        var dst = TempPath(".m4a");
        try
        {
            await MediarOperations.ExtractAudioAsync(src, dst);
            Assert.True(File.Exists(dst));
            Assert.True(new FileInfo(dst).Length > 0);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task ExtractAudioAsync_Null_Source_Throws()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await MediarOperations.ExtractAudioAsync(null!, "x.m4a"));
    }

    [Fact]
    public async Task ExtractAudioAsync_Null_Destination_Throws()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await MediarOperations.ExtractAudioAsync("x.wav", null!));
    }

    // ---------- ProbeAsync ----------

    [Fact]
    public async Task Probe_Reports_Format_And_Tracks()
    {
        var src = await WriteWavPcm16Async(8000, 1, new byte[16000]);
        try
        {
            var info = await MediarOperations.ProbeAsync(src);
            Assert.Equal("wav", info.Format);
            Assert.Single(info.Tracks);
            Assert.Equal(StreamKind.Audio, info.Tracks[0].Kind);
            Assert.Equal(CodecId.PcmS16Le, info.Tracks[0].Codec);
        }
        finally { DeleteSafe(src); }
    }

    [Fact]
    public async Task Probe_Reports_Duration()
    {
        var src = await WriteWavPcm16Async(8000, 1, new byte[8000 * 2]); // 1 second
        try
        {
            var info = await MediarOperations.ProbeAsync(src);
            Assert.Equal(TimeSpan.FromSeconds(1), info.Duration);
        }
        finally { DeleteSafe(src); }
    }

    // ---------- Open ----------

    [Fact]
    public async Task Open_Recognizes_Wav()
    {
        var src = await WriteWavPcm16Async(8000, 1, new byte[16]);
        try
        {
            using var dx = MediarOperations.Open(src);
            Assert.Equal("wav", dx.FormatName);
        }
        finally { DeleteSafe(src); }
    }

    [Fact]
    public async Task Open_Recognizes_Ogg()
    {
        var src = await WriteOggOpusAsync(BuildPayloads(1, 16));
        try
        {
            using var dx = MediarOperations.Open(src);
            Assert.Equal("ogg", dx.FormatName);
        }
        finally { DeleteSafe(src); }
    }

    [Fact]
    public void Open_Null_Path_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => MediarOperations.Open(null!));
    }

    [Fact]
    public void Open_Unrecognized_Extension_Throws()
    {
        Assert.Throws<NotSupportedException>(() => MediarOperations.Open("file.xyz"));
    }

    [Theory]
    [InlineData(".wav", typeof(WavMuxer))]
    [InlineData(".caf", typeof(CafMuxer))]
    [InlineData(".ogg", typeof(OggMuxer))]
    [InlineData(".mp3", typeof(Mediar.Containers.Mp3.Mp3Muxer))]
    [InlineData(".mp4", typeof(Mediar.Containers.IsoBmff.Mp4Muxer))]
    [InlineData(".m4a", typeof(Mediar.Containers.IsoBmff.Mp4Muxer))]
    [InlineData(".flac", typeof(Mediar.Containers.Flac.FlacMuxer))]
    [InlineData(".aac", typeof(Mediar.Containers.Adts.AdtsMuxer))]
    [InlineData(".mka", typeof(Mediar.Containers.Matroska.MatroskaMuxer))]
    [InlineData(".webm", typeof(Mediar.Containers.Matroska.MatroskaMuxer))]
    [InlineData(".voc", typeof(Mediar.Containers.Voc.VocMuxer))]
    [InlineData(".gsm", typeof(Mediar.Containers.Gsm.GsmMuxer))]
    [InlineData(".amr", typeof(Mediar.Containers.Amr.AmrMuxer))]
    public void CreateMuxer_Recognizes_Extension(string ext, Type expected)
    {
        using var ms = new MemoryStream();
        using var mux = MediarOperations.CreateMuxer($"x{ext}", ms);
        Assert.IsType(expected, mux);
    }

    [Fact]
    public void CreateMuxer_Null_DestinationPath_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => MediarOperations.CreateMuxer(null!, ms));
    }

    [Fact]
    public void CreateMuxer_Null_Destination_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => MediarOperations.CreateMuxer("x.wav", null!));
    }

    [Fact]
    public void CreateMuxer_Unrecognized_Extension_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<NotSupportedException>(() => MediarOperations.CreateMuxer("x.xyz", ms));
    }

    [Fact]
    public void CreateMuxer_Case_Insensitive_Extension()
    {
        using var ms = new MemoryStream();
        using var mux = MediarOperations.CreateMuxer("X.WAV", ms);
        Assert.IsType<WavMuxer>(mux);
    }

    [Theory]
    [InlineData(".oga", typeof(OggMuxer))]
    [InlineData(".ogv", typeof(OggMuxer))]
    [InlineData(".opus", typeof(OggMuxer))]
    [InlineData(".m4v", typeof(Mediar.Containers.IsoBmff.Mp4Muxer))]
    [InlineData(".mov", typeof(Mediar.Containers.IsoBmff.Mp4Muxer))]
    [InlineData(".3gp", typeof(Mediar.Containers.IsoBmff.Mp4Muxer))]
    [InlineData(".mkv", typeof(Mediar.Containers.Matroska.MatroskaMuxer))]
    [InlineData(".svx", typeof(Mediar.Containers.Iff8Svx.Iff8SvxMuxer))]
    [InlineData(".8svx", typeof(Mediar.Containers.Iff8Svx.Iff8SvxMuxer))]
    [InlineData(".iff", typeof(Mediar.Containers.Iff8Svx.Iff8SvxMuxer))]
    [InlineData(".awb", typeof(Mediar.Containers.Amr.AmrMuxer))]
    public void CreateMuxer_Recognizes_AdditionalExtensions(string ext, Type expected)
    {
        using var ms = new MemoryStream();
        using var mux = MediarOperations.CreateMuxer($"x{ext}", ms);
        Assert.IsType(expected, mux);
    }

    [Fact]
    public void CreateMuxer_Empty_Extension_Throws_NotSupported()
    {
        using var ms = new MemoryStream();
        Assert.Throws<NotSupportedException>(() => MediarOperations.CreateMuxer("file_no_ext", ms));
    }

    [Fact]
    public async Task ProbeAsync_Null_Path_Throws()
    {
        await Assert.ThrowsAsync<ArgumentNullException>(async () =>
            await MediarOperations.ProbeAsync(null!));
    }

    [Fact]
    public async Task Open_MixedCase_Wav_Extension_Recognised()
    {
        var src = await WriteWavPcm16Async(8000, 1, new byte[16]);
        var renamed = src + ".RENAMED.WAV";
        File.Move(src, renamed);
        try
        {
            using var dx = MediarOperations.Open(renamed);
            Assert.Equal("wav", dx.FormatName);
        }
        finally { DeleteSafe(renamed); }
    }

    [Fact]
    public async Task Open_MixedCase_Ogg_Extension_Recognised()
    {
        var src = await WriteOggOpusAsync(BuildPayloads(1, 16));
        var renamed = src + ".RENAMED.OGG";
        File.Move(src, renamed);
        try
        {
            using var dx = MediarOperations.Open(renamed);
            Assert.Equal("ogg", dx.FormatName);
        }
        finally { DeleteSafe(renamed); }
    }

    [Fact]
    public async Task Probe_Ogg_Reports_Opus_Codec()
    {
        var src = await WriteOggOpusAsync(BuildPayloads(1, 16));
        try
        {
            var info = await MediarOperations.ProbeAsync(src);
            Assert.Equal("ogg", info.Format);
            Assert.Single(info.Tracks);
            Assert.Equal(CodecId.Opus, info.Tracks[0].Codec);
            Assert.Equal(StreamKind.Audio, info.Tracks[0].Kind);
        }
        finally { DeleteSafe(src); }
    }

    [Fact]
    public async Task Transmux_Wav_Stereo_To_Wav_Preserves_Channel_Count()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2 * 2]; // 1s stereo S16
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i & 0xFF);
        var src = await WriteWavPcm16Async(sr, ch: 2, pcm);
        var dst = TempPath(".wav");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            await using var dem = WavDemuxer.Open(dst);
            var a = (AudioCodecParameters)dem.Tracks[0].Codec;
            Assert.Equal(2, a.Channels);
            Assert.Equal(sr, a.SampleRate);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task Transmux_Wav_To_Caf_Round_Trips_To_Caf_Sample_Data()
    {
        const int sr = 8000;
        byte[] pcm = new byte[200];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i ^ 0xAA);
        var src = await WriteWavPcm16Async(sr, 1, pcm);
        var dst = TempPath(".caf");
        try
        {
            await MediarOperations.TransmuxAsync(src, dst);
            await using var dem = CafDemuxer.Open(dst);
            long total = 0;
            await foreach (var s in dem.ReadSamplesAsync())
            {
                try { total += s.Data.Length; }
                finally { s.Owner?.Dispose(); }
            }
            Assert.Equal(pcm.Length, total);
        }
        finally { DeleteSafe(src, dst); }
    }

    [Fact]
    public async Task Probe_Wav_With_Mono_Reports_One_Channel()
    {
        var src = await WriteWavPcm16Async(16000, 1, new byte[800]);
        try
        {
            var info = await MediarOperations.ProbeAsync(src);
            Assert.Single(info.Tracks);
            Assert.Equal(StreamKind.Audio, info.Tracks[0].Kind);
        }
        finally { DeleteSafe(src); }
    }

    [Fact]
    public void CreateMuxer_Lowercase_Mka_Returns_NonWebm_Matroska()
    {
        using var ms = new MemoryStream();
        using var mux = MediarOperations.CreateMuxer("x.mka", ms);
        var matroska = Assert.IsType<Mediar.Containers.Matroska.MatroskaMuxer>(mux);
        Assert.NotNull(matroska);
    }

    // ---------- helpers ----------

    private static byte[][] BuildPayloads(int count, int size)
    {
        var arr = new byte[count][];
        for (int i = 0; i < count; i++)
        {
            arr[i] = new byte[size];
            for (int b = 0; b < size; b++) arr[i][b] = (byte)((i * 13 + b) & 0xFF);
        }
        return arr;
    }

    private static async Task<string> WriteOggOpusAsync(byte[][] payloads)
    {
        byte[] opusHead = new byte[19];
        Buffer.BlockCopy("OpusHead"u8.ToArray(), 0, opusHead, 0, 8);
        opusHead[8] = 1; opusHead[9] = 2;
        opusHead[10] = 0x90; opusHead[11] = 0x01;
        opusHead[12] = 0x80; opusHead[13] = 0xBB; opusHead[14] = 0x00; opusHead[15] = 0x00;

        var track = new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus, SampleRate = 48000, Channels = 2, ExtraData = opusHead,
            },
            TimeBase = new Rational(1, 48000),
        };

        var path = TempPath(".ogg");
        await using var fs = File.Create(path);
        await using var mux = new OggMuxer(fs);
        mux.AddTrack(track);
        await mux.StartAsync();
        for (int i = 0; i < payloads.Length; i++)
        {
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = i * 960L, Dts = i * 960L, Duration = 960,
                IsKeyFrame = true, Data = payloads[i],
            });
        }
        await mux.FinishAsync();
        return path;
    }

    private static async Task<string> WriteWavPcm16Async(int sr, int ch, byte[] pcm)
    {
        var path = TempPath(".wav");
        await using var fs = File.Create(path);
        await using var mux = new WavMuxer(fs);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters { Codec = CodecId.PcmS16Le, SampleRate = sr, Channels = ch, BitsPerSample = 16 },
            TimeBase = new Rational(1, sr),
        });
        await mux.StartAsync();
        if (pcm.Length > 0)
        {
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0,
                Duration = pcm.Length / (2 * ch), IsKeyFrame = true, Data = pcm,
            });
        }
        await mux.FinishAsync();
        return path;
    }

    private static string TempPath(string ext)
        => Path.Combine(Path.GetTempPath(), Path.GetRandomFileName() + ext);

    private static void DeleteSafe(params string[] paths)
    {
        foreach (var p in paths)
        {
            try { File.Delete(p); } catch { /* best effort */ }
        }
    }
}
