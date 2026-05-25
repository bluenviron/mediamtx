using BenchmarkDotNet.Attributes;
using Mediar.Containers.IsoBmff;
using Mediar.IO;

namespace Mediar.Bench;

[MemoryDiagnoser]
public class Mp4DemuxBenchmarks
{
    private byte[] _mp4 = null!;

    [GlobalSetup]
    public void Setup()
    {
        using var ms = new MemoryStream();
        using (var muxer = new Mp4Muxer(ms, leaveOpen: true))
        {
            muxer.AddTrack(new MediaTrack
            {
                Index = 0,
                Id = 1,
                Codec = new AudioCodecParameters { Codec = CodecId.Opus, SampleRate = 48000, Channels = 2 },
                TimeBase = new Rational(1, 48000),
            });
            muxer.StartAsync().AsTask().Wait();
            byte[] payload = new byte[256];
            for (int i = 0; i < 500; i++)
            {
                muxer.WriteSampleAsync(new MediaSample
                {
                    TrackIndex = 0,
                    Pts = i * 960,
                    Dts = i * 960,
                    Duration = 960,
                    IsKeyFrame = true,
                    Data = payload,
                }).AsTask().Wait();
            }
            muxer.FinishAsync().AsTask().Wait();
        }
        _mp4 = ms.ToArray();
    }

    [Benchmark]
    public int Parse_Moov_500Samples()
    {
        using var src = new MemoryRandomAccessSource(_mp4);
        using var demuxer = new Mp4Demuxer(src);
        return demuxer.Tracks.Count;
    }

    [Benchmark]
    public async Task<int> Enumerate_500_Samples()
    {
        using var src = new MemoryRandomAccessSource(_mp4);
        using var demuxer = new Mp4Demuxer(src);
        int count = 0;
        await foreach (var s in demuxer.ReadSamplesAsync())
        {
            count++;
            s.Owner?.Dispose();
        }
        return count;
    }
}
