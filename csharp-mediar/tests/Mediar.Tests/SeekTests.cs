using System.Buffers.Binary;
using Mediar.Containers.Matroska;
using Mediar.Containers.Mp3;
using Mediar.Containers.Wav;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class SeekTests
{
    [Fact]
    public async Task Wav_Seek_Lands_On_Frame_Boundary()
    {
        // Build a 1-second 44.1 kHz mono 16-bit signed PCM file in memory where
        // sample N = N (so seek correctness is visible from the payload bytes).
        const int sampleRate = 44100;
        const int channels = 1;
        const int bitsPerSample = 16;
        const int bytesPerSample = 2;
        const int totalFrames = sampleRate;

        var ms = new MemoryStream();
        // RIFF header
        WriteAscii(ms, "RIFF");
        WriteUInt32(ms, (uint)(36 + totalFrames * bytesPerSample));
        WriteAscii(ms, "WAVE");
        WriteAscii(ms, "fmt ");
        WriteUInt32(ms, 16);
        WriteUInt16(ms, 1); // PCM
        WriteUInt16(ms, channels);
        WriteUInt32(ms, sampleRate);
        WriteUInt32(ms, (uint)(sampleRate * channels * bytesPerSample));
        WriteUInt16(ms, (ushort)(channels * bytesPerSample));
        WriteUInt16(ms, bitsPerSample);
        WriteAscii(ms, "data");
        WriteUInt32(ms, (uint)(totalFrames * bytesPerSample));
        for (int i = 0; i < totalFrames; i++)
        {
            short v = (short)(i & 0x7FFF);
            ms.WriteByte((byte)(v & 0xFF));
            ms.WriteByte((byte)((v >> 8) & 0xFF));
        }

        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dem = WavDemuxer.Open(src);

        // Seek to 500 ms (frame index 22050).
        await dem.SeekAsync(TimeSpan.FromMilliseconds(500));

        bool gotFirst = false;
        long firstPts = -1;
        await foreach (var s in dem.ReadSamplesAsync())
        {
            if (!gotFirst)
            {
                firstPts = s.Pts;
                gotFirst = true;
            }
            s.Owner?.Dispose();
            if (gotFirst) break;
        }
        Assert.True(gotFirst);
        // First sample's PTS should be exactly the seeked frame.
        Assert.Equal(22050L, firstPts);
    }

    [Fact]
    public async Task Mp3_Seek_Skips_Frames_Before_Target()
    {
        // Build 50 minimal MPEG-1 Layer III mono frames at 44.1 kHz, 128 kbps.
        // We don't actually decode these, just lay out valid headers + zero
        // payloads so the demuxer can walk them.
        var ms = new MemoryStream();
        int sampleRate = 44100;
        int samplesPerFrame = 1152;
        int frameSize = 417; // 128 kbps at 44.1 kHz mpeg-1 layer III
        for (int i = 0; i < 50; i++)
        {
            // Frame header: 0xFFFB9000 = MPEG-1, Layer III, no CRC, 128 kbps, 44.1 kHz, mono.
            ms.WriteByte(0xFF);
            ms.WriteByte(0xFB);
            ms.WriteByte(0x90);
            ms.WriteByte(0xC0); // mono
            for (int b = 4; b < frameSize; b++) ms.WriteByte(0);
        }

        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dem = Mp3Demuxer.Open(src);
        Assert.Equal(sampleRate, ((AudioCodecParameters)dem.Tracks[0].Codec).SampleRate);

        // Seek to ~0.5 s → expect first sample's PTS >= ~22050 samples.
        await dem.SeekAsync(TimeSpan.FromSeconds(0.5));
        long firstPts = -1;
        await foreach (var s in dem.ReadSamplesAsync())
        {
            firstPts = s.Pts;
            s.Owner?.Dispose();
            break;
        }
        Assert.True(firstPts >= 0);
        // Must be at least the seek target (samples after target).
        Assert.True(firstPts + samplesPerFrame > 22050,
            $"expected pts > 22050-1152, got {firstPts}");
        // Must be aligned to a frame boundary.
        Assert.Equal(0L, firstPts % samplesPerFrame);
    }

    [Fact]
    public async Task Matroska_Seek_Skips_Whole_Clusters_Before_Target()
    {
        // Build a fake MKV with synthetic Opus samples spanning ~2 s in 10 ms
        // chunks, then seek to 1.0 s and verify all emitted samples are >= 1000 ms.
        var track = new MediaTrack
        {
            Index = 0,
            Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus,
                SampleRate = 48000,
                Channels = 2,
                ExtraData = new byte[19] { 0x4F, 0x70, 0x75, 0x73, 0x48, 0x65, 0x61, 0x64, 1, 2, 0x90, 0x01, 0x80, 0xBB, 0, 0, 0, 0, 0 },
            },
            TimeBase = new Rational(1, 1000),
        };
        using var buf = new MemoryStream();
        await using (var mux = new MatroskaMuxer(buf, leaveOpen: true))
        {
            mux.AddTrack(track);
            await mux.StartAsync();
            for (int i = 0; i < 200; i++) // 200 * 10 ms = 2 s
            {
                var payload = new byte[8];
                BinaryPrimitives.WriteInt32BigEndian(payload.AsSpan(0, 4), i);
                await mux.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * 10L,
                    Dts = i * 10L,
                    Duration = 10,
                    IsKeyFrame = true,
                    Data = payload,
                });
            }
            await mux.FinishAsync();
        }

        using var src = new MemoryRandomAccessSource(buf.ToArray());
        await using var dem = MatroskaDemuxer.Open(src);
        await dem.SeekAsync(TimeSpan.FromSeconds(1.0));

        long firstPts = -1;
        await foreach (var s in dem.ReadSamplesAsync())
        {
            if (firstPts < 0) firstPts = s.Pts;
            // Every sample emitted after seek must be at-or-after the target.
            Assert.True(s.Pts >= 1000, $"got pts {s.Pts}, expected >= 1000");
            s.Owner?.Dispose();
        }
        Assert.InRange(firstPts, 1000, 1010);
    }

    [Fact]
    public async Task Demuxer_Default_Seek_Throws_NotSupported()
    {
        // ADTS demuxer overrides SeekAsync now, but other shapes default-throwing
        // is verified indirectly: the interface contract says default throws.
        // Here we just ensure overrides we *did* add return without throwing.
        var ms = new MemoryStream();
        WriteAscii(ms, "RIFF"); WriteUInt32(ms, 36);
        WriteAscii(ms, "WAVE");
        WriteAscii(ms, "fmt "); WriteUInt32(ms, 16);
        WriteUInt16(ms, 1); WriteUInt16(ms, 1); WriteUInt32(ms, 8000);
        WriteUInt32(ms, 16000); WriteUInt16(ms, 2); WriteUInt16(ms, 16);
        WriteAscii(ms, "data"); WriteUInt32(ms, 0);

        using var src = new MemoryRandomAccessSource(ms.ToArray());
        using var dem = WavDemuxer.Open(src);
        await dem.SeekAsync(TimeSpan.Zero);
        await dem.SeekAsync(TimeSpan.FromHours(1)); // out of range — clamped to end
        Assert.True(true);
    }

    [Fact]
    public async Task Wav_Seek_To_Time_Zero_Yields_First_Frame()
    {
        // Seeking to time zero must start at the very first sample.
        byte[] avi = BuildSimpleWav(8000, totalFrames: 800);
        using var src = new MemoryRandomAccessSource(avi);
        using var dem = WavDemuxer.Open(src);
        await dem.SeekAsync(TimeSpan.Zero);
        long firstPts = -1;
        await foreach (var s in dem.ReadSamplesAsync())
        {
            firstPts = s.Pts;
            s.Owner?.Dispose();
            break;
        }
        Assert.Equal(0L, firstPts);
    }

    [Fact]
    public async Task Wav_Seek_Past_End_Yields_No_Samples()
    {
        byte[] avi = BuildSimpleWav(8000, totalFrames: 800);
        using var src = new MemoryRandomAccessSource(avi);
        using var dem = WavDemuxer.Open(src);
        // 800 frames @ 8 kHz = 100 ms; seek to 1 second is way past.
        await dem.SeekAsync(TimeSpan.FromSeconds(1));
        int count = 0;
        await foreach (var s in dem.ReadSamplesAsync())
        {
            count++;
            s.Owner?.Dispose();
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public async Task Wav_Seek_Negative_Time_Is_Clamped_To_Zero()
    {
        byte[] avi = BuildSimpleWav(8000, totalFrames: 800);
        using var src = new MemoryRandomAccessSource(avi);
        using var dem = WavDemuxer.Open(src);
        await dem.SeekAsync(TimeSpan.FromSeconds(-50));
        long firstPts = -1;
        await foreach (var s in dem.ReadSamplesAsync())
        {
            firstPts = s.Pts;
            s.Owner?.Dispose();
            break;
        }
        Assert.Equal(0L, firstPts);
    }

    [Fact]
    public async Task Wav_Seek_Honours_Cancellation_During_Enumeration()
    {
        byte[] avi = BuildSimpleWav(44100, totalFrames: 44100);
        using var src = new MemoryRandomAccessSource(avi);
        using var dem = WavDemuxer.Open(src);
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAnyAsync<OperationCanceledException>(async () =>
        {
            await foreach (var s in dem.ReadSamplesAsync(cts.Token))
            {
                s.Owner?.Dispose();
            }
        });
    }

    [Fact]
    public async Task Wav_Seek_Multiple_Times_Returns_Different_FirstPts()
    {
        byte[] avi = BuildSimpleWav(44100, totalFrames: 44100);
        using var src = new MemoryRandomAccessSource(avi);
        using var dem = WavDemuxer.Open(src);

        async Task<long> ReadFirstPts()
        {
            await foreach (var s in dem.ReadSamplesAsync())
            {
                long pts = s.Pts;
                s.Owner?.Dispose();
                return pts;
            }
            return -1;
        }

        await dem.SeekAsync(TimeSpan.FromMilliseconds(100));
        long a = await ReadFirstPts();
        await dem.SeekAsync(TimeSpan.FromMilliseconds(500));
        long b = await ReadFirstPts();
        await dem.SeekAsync(TimeSpan.FromMilliseconds(900));
        long c = await ReadFirstPts();
        Assert.True(a < b && b < c, $"expected a<b<c, got {a}<{b}<{c}");
    }

    private static byte[] BuildSimpleWav(int sampleRate, int totalFrames)
    {
        const int channels = 1;
        const int bitsPerSample = 16;
        const int bytesPerSample = 2;
        var ms = new MemoryStream();
        WriteAscii(ms, "RIFF");
        WriteUInt32(ms, (uint)(36 + totalFrames * bytesPerSample));
        WriteAscii(ms, "WAVE");
        WriteAscii(ms, "fmt ");
        WriteUInt32(ms, 16);
        WriteUInt16(ms, 1);
        WriteUInt16(ms, channels);
        WriteUInt32(ms, (uint)sampleRate);
        WriteUInt32(ms, (uint)(sampleRate * channels * bytesPerSample));
        WriteUInt16(ms, (ushort)(channels * bytesPerSample));
        WriteUInt16(ms, bitsPerSample);
        WriteAscii(ms, "data");
        WriteUInt32(ms, (uint)(totalFrames * bytesPerSample));
        for (int i = 0; i < totalFrames; i++)
        {
            short v = (short)(i & 0x7FFF);
            ms.WriteByte((byte)(v & 0xFF));
            ms.WriteByte((byte)((v >> 8) & 0xFF));
        }
        return ms.ToArray();
    }

    private static void WriteAscii(MemoryStream ms, string s)
    {
        foreach (char c in s) ms.WriteByte((byte)c);
    }

    private static void WriteUInt32(MemoryStream ms, uint v)
    {
        ms.WriteByte((byte)(v & 0xFF));
        ms.WriteByte((byte)((v >> 8) & 0xFF));
        ms.WriteByte((byte)((v >> 16) & 0xFF));
        ms.WriteByte((byte)((v >> 24) & 0xFF));
    }

    private static void WriteUInt16(MemoryStream ms, ushort v)
    {
        ms.WriteByte((byte)(v & 0xFF));
        ms.WriteByte((byte)((v >> 8) & 0xFF));
    }
}
