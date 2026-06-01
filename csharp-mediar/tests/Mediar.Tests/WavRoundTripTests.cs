using System.Buffers.Binary;
using Mediar.Containers.Wav;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class WavRoundTripTests
{
    [Fact]
    public async Task PcmS16Le_RoundTrips_Through_WavMuxer()
    {
        const int sr = 48000;
        const int ch = 2;
        const int frames = sr; // 1 second
        byte[] pcm = SineS16Le(sr, ch, frames);

        byte[] wavBytes = await MuxAsync(pcm, sr, ch, 16, CodecId.PcmS16Le);

        Assert.True(wavBytes.Length > pcm.Length);

        using var source = new MemoryRandomAccessSource(wavBytes);
        using var demuxer = WavDemuxer.Open(source);

        Assert.Equal("wav", demuxer.FormatName);
        Assert.Single(demuxer.Tracks);
        var t = demuxer.Tracks[0];
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        Assert.Equal(sr, audio.SampleRate);
        Assert.Equal(ch, audio.Channels);

        int totalBytes = await SumPayloadAsync(demuxer);
        Assert.Equal(pcm.Length, totalBytes);
    }

    [Fact]
    public async Task PcmS24Le_RoundTrips()
    {
        byte[] pcm = new byte[24 * 3]; // 24 frames of mono 24-bit
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i * 7);
        byte[] wav = await MuxAsync(pcm, 8000, 1, 24, CodecId.PcmS24Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(CodecId.PcmS24Le, audio.Codec);
        Assert.Equal(24, audio.BitsPerSample);
        Assert.Equal(pcm.Length, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task PcmS32Le_RoundTrips()
    {
        byte[] pcm = new byte[8 * 4];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)i;
        byte[] wav = await MuxAsync(pcm, 16000, 1, 32, CodecId.PcmS32Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(CodecId.PcmS32Le, audio.Codec);
    }

    [Fact]
    public async Task PcmF32Le_Uses_IeeeFloat_Format_Tag()
    {
        byte[] pcm = new byte[16 * 4];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i + 1);
        byte[] wav = await MuxAsync(pcm, 8000, 1, 32, CodecId.PcmF32Le);
        // fmt chunk's first word is the format tag, located at offset 20.
        Assert.Equal((ushort)0x0003, BinaryPrimitives.ReadUInt16LittleEndian(wav.AsSpan(20)));
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(CodecId.PcmF32Le, audio.Codec);
    }

    [Fact]
    public async Task PcmS16Le_Uses_Pcm_Format_Tag()
    {
        byte[] wav = await MuxAsync(new byte[4], 48000, 2, 16, CodecId.PcmS16Le);
        Assert.Equal((ushort)0x0001, BinaryPrimitives.ReadUInt16LittleEndian(wav.AsSpan(20)));
    }

    [Fact]
    public async Task Mono_RoundTrips()
    {
        byte[] pcm = SineS16Le(8000, 1, 8000);
        byte[] wav = await MuxAsync(pcm, 8000, 1, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(1, audio.Channels);
    }

    [Fact]
    public async Task Five_Point_One_RoundTrips()
    {
        byte[] pcm = new byte[1000 * 6 * 2]; // 1000 frames * 6 ch * 2 bytes
        byte[] wav = await MuxAsync(pcm, 48000, 6, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(6, audio.Channels);
        Assert.Equal(pcm.Length, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task Duration_Reflects_Data_Length()
    {
        const int sr = 8000;
        byte[] pcm = new byte[sr * 2 /* seconds */ * 2 /* channels */ * 2 /* bytes/sample */];
        byte[] wav = await MuxAsync(pcm, sr, 2, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        Assert.Equal(TimeSpan.FromSeconds(2), demux.Duration);
    }

    [Fact]
    public async Task Padding_Byte_Is_Written_When_Data_Length_Is_Odd()
    {
        // Use 24-bit mono so a single frame is 3 bytes (odd).
        byte[] pcm = new byte[3];
        byte[] wav = await MuxAsync(pcm, 8000, 1, 24, CodecId.PcmS24Le);
        // After the 36-byte header + 8-byte data hdr + 3-byte payload, 1 pad byte → 48 total.
        Assert.Equal(48, wav.Length);
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Null_Stream()
    {
        Assert.Throws<ArgumentNullException>(() => new WavMuxer(null!));
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Non_Writable_Stream()
    {
        using var ms = new MemoryStream(new byte[16], writable: false);
        Assert.Throws<ArgumentException>(() => new WavMuxer(ms));
    }

    [Fact]
    public void Muxer_Constructor_Rejects_Non_Seekable_Stream()
    {
        using var ns = new NonSeekableStream();
        Assert.Throws<ArgumentException>(() => new WavMuxer(ns));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Non_Audio()
    {
        using var ms = new MemoryStream();
        using var mux = new WavMuxer(ms, leaveOpen: true);
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new VideoCodecParameters { Codec = CodecId.H264 },
            TimeBase = new Rational(1, 90000),
        };
        Assert.Throws<ArgumentException>(() => mux.AddTrack(track));
    }

    [Fact]
    public void Muxer_AddTrack_Rejects_Second_Track()
    {
        using var ms = new MemoryStream();
        using var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        Assert.Throws<InvalidOperationException>(() => mux.AddTrack(BuildPcmTrack()));
    }

    [Fact]
    public async Task Muxer_StartAsync_Without_Track_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new WavMuxer(ms, leaveOpen: true);
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_Double_StartAsync_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        await mux.StartAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_WriteSample_Before_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        await Assert.ThrowsAsync<InvalidOperationException>(async () =>
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = 1,
                Data = new byte[4],
            }));
    }

    [Fact]
    public async Task Muxer_Finish_Before_Start_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.FinishAsync());
    }

    [Fact]
    public async Task Muxer_Finish_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        await using var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        await mux.FinishAsync();
    }

    [Fact]
    public async Task Muxer_Unsupported_Codec_Throws()
    {
        using var ms = new MemoryStream();
        await using var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Mp3,
                SampleRate = 44100,
                Channels = 2,
            },
            TimeBase = new Rational(1, 44100),
        });
        await Assert.ThrowsAsync<NotSupportedException>(async () => await mux.StartAsync());
    }

    [Fact]
    public async Task Muxer_Dispose_Finishes_Implicitly()
    {
        var ms = new MemoryStream();
        {
            using var mux = new WavMuxer(ms, leaveOpen: true);
            mux.AddTrack(BuildPcmTrack());
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = 25,
                Data = new byte[100],
            });
        }
        Assert.True(ms.Length > 0);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray()));
        Assert.Equal(100, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task Muxer_DisposeAsync_Finishes_Implicitly()
    {
        var ms = new MemoryStream();
        {
            await using var mux = new WavMuxer(ms, leaveOpen: true);
            mux.AddTrack(BuildPcmTrack());
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = 10,
                Data = new byte[40],
            });
        }
        Assert.True(ms.Length > 0);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray()));
        Assert.Equal(40, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task Muxer_LeaveOpen_True_Keeps_Stream_Open()
    {
        var ms = new MemoryStream();
        var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        mux.Dispose();
        _ = ms.Length;
        ms.Dispose();
    }

    [Fact]
    public async Task Muxer_LeaveOpen_False_Closes_Stream()
    {
        var ms = new MemoryStream();
        var mux = new WavMuxer(ms, leaveOpen: false);
        mux.AddTrack(BuildPcmTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        mux.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = ms.Length);
    }

    [Fact]
    public async Task Muxer_Dispose_Is_Idempotent()
    {
        using var ms = new MemoryStream();
        var mux = new WavMuxer(ms, leaveOpen: true);
        mux.AddTrack(BuildPcmTrack());
        await mux.StartAsync();
        await mux.FinishAsync();
        mux.Dispose();
        mux.Dispose();
    }

    [Fact]
    public void Muxer_FormatName_Is_Wav()
    {
        using var ms = new MemoryStream();
        using var mux = new WavMuxer(ms, leaveOpen: true);
        Assert.Equal("wav", mux.FormatName);
    }

    [Fact]
    public void Demuxer_Open_Null_Source_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => WavDemuxer.Open((IRandomAccessSource)null!));
    }

    [Fact]
    public void Demuxer_Rejects_Too_Small_File()
    {
        Assert.Throws<InvalidDataException>(() =>
            WavDemuxer.Open(new MemoryRandomAccessSource(new byte[] { (byte)'R', (byte)'I', (byte)'F', (byte)'F' })));
    }

    [Fact]
    public void Demuxer_Rejects_Missing_Riff_Marker()
    {
        var bytes = new byte[12];
        bytes[0] = (byte)'X'; bytes[1] = (byte)'X'; bytes[2] = (byte)'X'; bytes[3] = (byte)'X';
        bytes[8] = (byte)'W'; bytes[9] = (byte)'A'; bytes[10] = (byte)'V'; bytes[11] = (byte)'E';
        Assert.Throws<InvalidDataException>(() => WavDemuxer.Open(new MemoryRandomAccessSource(bytes)));
    }

    [Fact]
    public void Demuxer_Rejects_Missing_Wave_Marker()
    {
        var bytes = new byte[12];
        bytes[0] = (byte)'R'; bytes[1] = (byte)'I'; bytes[2] = (byte)'F'; bytes[3] = (byte)'F';
        bytes[8] = (byte)'X'; bytes[9] = (byte)'X'; bytes[10] = (byte)'X'; bytes[11] = (byte)'X';
        Assert.Throws<InvalidDataException>(() => WavDemuxer.Open(new MemoryRandomAccessSource(bytes)));
    }

    [Fact]
    public void Demuxer_Accepts_Rf64_Marker()
    {
        // Build a minimal RF64 file: RF64 header + fmt + data, sentinel sizes + ds64.
        byte[] data = new byte[4];
        byte[] bytes = BuildRf64(channels: 1, sampleRate: 8000, bitsPerSample: 8, data: data);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(bytes));
        Assert.Equal(1, ((AudioCodecParameters)demux.Tracks[0].Codec).Channels);
    }

    [Fact]
    public void Demuxer_Accepts_Bw64_Marker()
    {
        byte[] data = new byte[4];
        byte[] bytes = BuildRf64(channels: 1, sampleRate: 8000, bitsPerSample: 8, data: data, marker: "BW64");
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(bytes));
        Assert.Equal(1, ((AudioCodecParameters)demux.Tracks[0].Codec).Channels);
    }

    [Fact]
    public void Demuxer_Rejects_Missing_Fmt_Chunk()
    {
        // RIFF + WAVE + data chunk only (no fmt).
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'R', (byte)'I', (byte)'F', (byte)'F' });
        WriteUInt32(ms, 12);
        ms.Write(new byte[] { (byte)'W', (byte)'A', (byte)'V', (byte)'E' });
        ms.Write(new byte[] { (byte)'d', (byte)'a', (byte)'t', (byte)'a' });
        WriteUInt32(ms, 0);
        Assert.Throws<InvalidDataException>(() => WavDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray())));
    }

    [Fact]
    public void Demuxer_Rejects_Missing_Data_Chunk()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'R', (byte)'I', (byte)'F', (byte)'F' });
        WriteUInt32(ms, 28);
        ms.Write(new byte[] { (byte)'W', (byte)'A', (byte)'V', (byte)'E' });
        ms.Write(new byte[] { (byte)'f', (byte)'m', (byte)'t', (byte)' ' });
        WriteUInt32(ms, 16);
        WriteFmtBody(ms, formatTag: 1, channels: 1, sr: 8000, bps: 8);
        Assert.Throws<InvalidDataException>(() => WavDemuxer.Open(new MemoryRandomAccessSource(ms.ToArray())));
    }

    [Fact]
    public async Task Demuxer_Seek_Negative_Clamps_To_Zero()
    {
        byte[] pcm = SineS16Le(8000, 1, 8000);
        byte[] wav = await MuxAsync(pcm, 8000, 1, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        await demux.SeekAsync(TimeSpan.FromSeconds(-10));
        Assert.Equal(pcm.Length, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task Demuxer_Seek_Past_End_Clamps_To_End()
    {
        byte[] pcm = SineS16Le(8000, 1, 8000);
        byte[] wav = await MuxAsync(pcm, 8000, 1, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        await demux.SeekAsync(TimeSpan.FromHours(1));
        Assert.Equal(0, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task Demuxer_Seek_Mid_Stream_Skips_Audio()
    {
        byte[] pcm = new byte[16000]; // 1 s mono 8 kHz 16-bit
        byte[] wav = await MuxAsync(pcm, 8000, 1, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        await demux.SeekAsync(TimeSpan.FromMilliseconds(500));
        // Remaining ~500 ms = 4000 frames * 2 bytes = 8000 bytes.
        Assert.Equal(8000, await SumPayloadAsync(demux));
    }

    [Fact]
    public async Task Demuxer_FormatName_Is_Wav()
    {
        byte[] wav = await MuxAsync(new byte[16], 8000, 1, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        Assert.Equal("wav", demux.FormatName);
    }

    [Fact]
    public async Task Demuxer_Open_Path_Works()
    {
        byte[] wav = await MuxAsync(new byte[16], 8000, 1, 16, CodecId.PcmS16Le);
        var path = Path.Combine(Path.GetTempPath(), $"mediar-wav-{Guid.NewGuid():N}.wav");
        File.WriteAllBytes(path, wav);
        try
        {
            using var demux = WavDemuxer.Open(path);
            Assert.Equal("wav", demux.FormatName);
            Assert.Single(demux.Tracks);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Demuxer_Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-wav-missing-{Guid.NewGuid():N}.wav");
        Assert.Throws<FileNotFoundException>(() => WavDemuxer.Open(path));
    }

    [Fact]
    public async Task Demuxer_Dispose_Is_Idempotent()
    {
        byte[] wav = await MuxAsync(new byte[16], 8000, 1, 16, CodecId.PcmS16Le);
        var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        demux.Dispose();
        demux.Dispose();
    }

    [Fact]
    public async Task Demuxer_DisposeAsync_Works()
    {
        byte[] wav = await MuxAsync(new byte[16], 8000, 1, 16, CodecId.PcmS16Le);
        var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        await demux.DisposeAsync();
    }

    [Fact]
    public async Task Demuxer_Metadata_Includes_List_Info_Tags()
    {
        byte[] wav = await MuxAsync(new byte[40], 8000, 1, 16, CodecId.PcmS16Le);
        byte[] withList = AppendListInfo(wav, new (string Id, string Value)[]
        {
            ("INAM", "Title"),
            ("IART", "Artist"),
            ("ICRD", "2024"),
        });
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(withList));
        var m = demux.Metadata;
        Assert.Equal("Title", m.Tags["TITLE"]);
        Assert.Equal("Artist", m.Tags["ARTIST"]);
        Assert.Equal("2024", m.Tags["DATE"]);
    }

    [Fact]
    public async Task Demuxer_Wave_Format_Extensible_Pcm_Subformat_Maps_To_Pcm()
    {
        byte[] wav = BuildExtensiblePcm(channels: 1, sampleRate: 8000, bitsPerSample: 16, isFloat: false);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(CodecId.PcmS16Le, audio.Codec);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Demuxer_Wave_Format_Extensible_Float_Subformat_Maps_To_F32Le()
    {
        byte[] wav = BuildExtensiblePcm(channels: 1, sampleRate: 8000, bitsPerSample: 32, isFloat: true);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var audio = (AudioCodecParameters)demux.Tracks[0].Codec;
        Assert.Equal(CodecId.PcmF32Le, audio.Codec);
        await Task.CompletedTask;
    }

    [Fact]
    public async Task Demuxer_Read_Returns_Roughly_Ten_Millisecond_Packets()
    {
        const int sr = 48000;
        byte[] pcm = new byte[sr * 2 * 2]; // 1 second stereo
        byte[] wav = await MuxAsync(pcm, sr, 2, 16, CodecId.PcmS16Le);
        using var demux = WavDemuxer.Open(new MemoryRandomAccessSource(wav));
        var durations = new List<long>();
        await foreach (var s in demux.ReadSamplesAsync())
        {
            durations.Add(s.Duration);
            s.Owner?.Dispose();
        }
        Assert.True(durations.Count >= 99); // ~100 packets in 1 second
        Assert.Equal(480, durations[0]); // 48000/100
    }

    // -------------------- helpers --------------------

    private static byte[] SineS16Le(int sr, int ch, int frames)
    {
        byte[] pcm = new byte[frames * ch * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)(Math.Sin(2 * Math.PI * 440.0 * i / sr) * 16000);
            for (int c = 0; c < ch; c++)
            {
                int o = (i * ch + c) * 2;
                pcm[o + 0] = (byte)v;
                pcm[o + 1] = (byte)(v >> 8);
            }
        }
        return pcm;
    }

    private static async Task<byte[]> MuxAsync(byte[] pcm, int sr, int ch, int bps, CodecId codec)
    {
        await using var ms = new MemoryStream();
        await using (var mux = new WavMuxer(ms, leaveOpen: true))
        {
            mux.AddTrack(new MediaTrack
            {
                Index = 0,
                Id = 1,
                Codec = new AudioCodecParameters
                {
                    Codec = codec,
                    SampleRate = sr,
                    Channels = ch,
                    BitsPerSample = bps,
                },
                TimeBase = new Rational(1, sr),
            });
            await mux.StartAsync();
            int frameSize = (bps / 8) * ch;
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0,
                Pts = 0,
                Dts = 0,
                Duration = frameSize > 0 ? pcm.Length / frameSize : 0,
                IsKeyFrame = true,
                Data = pcm,
            });
            await mux.FinishAsync();
        }
        return ms.ToArray();
    }

    private static async Task<int> SumPayloadAsync(WavDemuxer demuxer)
    {
        int total = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        return total;
    }

    private static MediaTrack BuildPcmTrack()
    {
        return new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.PcmS16Le,
                SampleRate = 48000,
                Channels = 2,
                BitsPerSample = 16,
            },
            TimeBase = new Rational(1, 48000),
        };
    }

    private static void WriteUInt32(Stream s, uint v)
    {
        Span<byte> b = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(b, v);
        s.Write(b);
    }

    private static void WriteUInt16(Stream s, ushort v)
    {
        Span<byte> b = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(b, v);
        s.Write(b);
    }

    private static void WriteFmtBody(Stream s, ushort formatTag, ushort channels, uint sr, ushort bps)
    {
        ushort blockAlign = (ushort)((bps / 8) * channels);
        uint avgBytesPerSec = sr * blockAlign;
        WriteUInt16(s, formatTag);
        WriteUInt16(s, channels);
        WriteUInt32(s, sr);
        WriteUInt32(s, avgBytesPerSec);
        WriteUInt16(s, blockAlign);
        WriteUInt16(s, bps);
    }

    private static byte[] BuildRf64(int channels, int sampleRate, int bitsPerSample, byte[] data, string marker = "RF64")
    {
        using var ms = new MemoryStream();
        // RIFF/RF64 header
        ms.Write(new byte[] { (byte)marker[0], (byte)marker[1], (byte)marker[2], (byte)marker[3] });
        WriteUInt32(ms, 0xFFFFFFFFu); // sentinel size
        ms.Write(new byte[] { (byte)'W', (byte)'A', (byte)'V', (byte)'E' });

        // ds64 chunk (28 bytes payload)
        ms.Write(new byte[] { (byte)'d', (byte)'s', (byte)'6', (byte)'4' });
        WriteUInt32(ms, 28);
        // riffSize (8), dataSize (8), sampleCount (8), tableLength (4)
        for (int i = 0; i < 8; i++) ms.WriteByte(0); // riffSize
        Span<byte> dataSize = stackalloc byte[8];
        BinaryPrimitives.WriteUInt64LittleEndian(dataSize, (ulong)data.Length);
        ms.Write(dataSize);
        for (int i = 0; i < 8; i++) ms.WriteByte(0); // sampleCount
        for (int i = 0; i < 4; i++) ms.WriteByte(0); // tableLength

        // fmt
        ms.Write(new byte[] { (byte)'f', (byte)'m', (byte)'t', (byte)' ' });
        WriteUInt32(ms, 16);
        WriteFmtBody(ms, 1, (ushort)channels, (uint)sampleRate, (ushort)bitsPerSample);

        // data with sentinel size + actual payload
        ms.Write(new byte[] { (byte)'d', (byte)'a', (byte)'t', (byte)'a' });
        WriteUInt32(ms, 0xFFFFFFFFu);
        ms.Write(data);
        if ((data.Length & 1) == 1) ms.WriteByte(0);
        return ms.ToArray();
    }

    private static byte[] AppendListInfo(byte[] wav, (string Id, string Value)[] entries)
    {
        // Build LIST INFO sub-chunks.
        using var sub = new MemoryStream();
        sub.Write(new byte[] { (byte)'I', (byte)'N', (byte)'F', (byte)'O' });
        foreach (var (id, value) in entries)
        {
            byte[] payload = new byte[value.Length + 1]; // null-terminated
            for (int i = 0; i < value.Length; i++) payload[i] = (byte)value[i];
            sub.Write(new byte[] { (byte)id[0], (byte)id[1], (byte)id[2], (byte)id[3] });
            WriteUInt32(sub, (uint)payload.Length);
            sub.Write(payload);
            if ((payload.Length & 1) == 1) sub.WriteByte(0);
        }
        byte[] subBytes = sub.ToArray();

        // Append LIST chunk to the existing WAV.
        using var ms = new MemoryStream();
        ms.Write(wav);
        ms.Write(new byte[] { (byte)'L', (byte)'I', (byte)'S', (byte)'T' });
        WriteUInt32(ms, (uint)subBytes.Length);
        ms.Write(subBytes);

        // Patch RIFF size at offset 4 to reflect new total.
        byte[] result = ms.ToArray();
        BinaryPrimitives.WriteUInt32LittleEndian(result.AsSpan(4), (uint)(result.Length - 8));
        return result;
    }

    private static byte[] BuildExtensiblePcm(int channels, int sampleRate, int bitsPerSample, bool isFloat)
    {
        ushort blockAlign = (ushort)((bitsPerSample / 8) * channels);
        uint avgBytesPerSec = (uint)sampleRate * blockAlign;
        Guid sub = isFloat
            ? new Guid("00000003-0000-0010-8000-00aa00389b71")
            : new Guid("00000001-0000-0010-8000-00aa00389b71");

        using var fmt = new MemoryStream();
        WriteUInt16(fmt, 0xFFFE); // WAVE_FORMAT_EXTENSIBLE
        WriteUInt16(fmt, (ushort)channels);
        WriteUInt32(fmt, (uint)sampleRate);
        WriteUInt32(fmt, avgBytesPerSec);
        WriteUInt16(fmt, blockAlign);
        WriteUInt16(fmt, (ushort)bitsPerSample);
        WriteUInt16(fmt, 22); // cbSize
        WriteUInt16(fmt, (ushort)bitsPerSample); // validBits
        WriteUInt32(fmt, 0); // channelMask
        fmt.Write(sub.ToByteArray());
        byte[] fmtBytes = fmt.ToArray();

        byte[] data = new byte[8];
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'R', (byte)'I', (byte)'F', (byte)'F' });
        WriteUInt32(ms, 0); // patched below
        ms.Write(new byte[] { (byte)'W', (byte)'A', (byte)'V', (byte)'E' });
        ms.Write(new byte[] { (byte)'f', (byte)'m', (byte)'t', (byte)' ' });
        WriteUInt32(ms, (uint)fmtBytes.Length);
        ms.Write(fmtBytes);
        ms.Write(new byte[] { (byte)'d', (byte)'a', (byte)'t', (byte)'a' });
        WriteUInt32(ms, (uint)data.Length);
        ms.Write(data);
        byte[] result = ms.ToArray();
        BinaryPrimitives.WriteUInt32LittleEndian(result.AsSpan(4), (uint)(result.Length - 8));
        return result;
    }

    private sealed class NonSeekableStream : Stream
    {
        public override bool CanRead => false;
        public override bool CanSeek => false;
        public override bool CanWrite => true;
        public override long Length => throw new NotSupportedException();
        public override long Position { get => throw new NotSupportedException(); set => throw new NotSupportedException(); }
        public override void Flush() { }
        public override int Read(byte[] buffer, int offset, int count) => throw new NotSupportedException();
        public override long Seek(long offset, SeekOrigin origin) => throw new NotSupportedException();
        public override void SetLength(long value) => throw new NotSupportedException();
        public override void Write(byte[] buffer, int offset, int count) { }
    }
}
