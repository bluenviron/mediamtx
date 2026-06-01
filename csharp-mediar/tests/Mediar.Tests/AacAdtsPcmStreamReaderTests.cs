using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacAdtsPcmStreamReaderTests
{
    // ----- constructor guards (forwarded from inner reader) -----

    [Fact]
    public void Ctor_NullStream_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => new AacAdtsPcmStreamReader(
            stream: null!,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullScaleFactorCodebook_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => new AacAdtsPcmStreamReader(
            stream: ms,
            scaleFactorCodebook: null!,
            spectralCodebooks: new AacHuffmanCodebook?[16]));
    }

    [Fact]
    public void Ctor_NullSpectralCodebooks_Throws()
    {
        using var ms = new MemoryStream();
        Assert.Throws<ArgumentNullException>(() => new AacAdtsPcmStreamReader(
            stream: ms,
            scaleFactorCodebook: GetSf(),
            spectralCodebooks: null!));
    }

    // ----- empty / EOF behaviour -----

    [Fact]
    public void ReadNextPcmFrame_EmptyStream_ReturnsNull()
    {
        using var ms = new MemoryStream();
        using var reader = NewReader(ms);
        Assert.Null(reader.ReadNextPcmFrame());
        Assert.Equal(0, reader.FrameCount);
        Assert.Null(reader.CurrentConfig);
    }

    // ----- happy-path single-block frame -----

    [Fact]
    public void ReadNextPcmFrame_SingleSceFrame_ProducesInterleavedMono()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var pcm = reader.ReadNextPcmFrame();
        Assert.NotNull(pcm);
        Assert.Equal(1, pcm!.ChannelCount);
        Assert.Equal(1024, pcm.SamplesPerChannel);
        Assert.Equal(1024, pcm.Samples.Length);
        Assert.Equal(48000, pcm.SampleRate);
        Assert.Single(pcm.Speakers);
        Assert.Equal(AacSpeaker.FrontCentre, pcm.Speakers[0]);

        // Empty SCE produces all-zero spectrum -> all-zero PCM frame.
        Assert.All(pcm.Samples, s => Assert.Equal(0f, s));

        // Second call returns null on clean EOF.
        Assert.Null(reader.ReadNextPcmFrame());
    }

    // ----- multi-block fan-out (one PCM frame per block) -----

    [Fact]
    public void ReadNextPcmFrame_MultiBlockFrame_YieldsOnePcmFramePerBlock()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoMultiSceFrameShared(blockCount: 3);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        var p1 = reader.ReadNextPcmFrame();
        var p2 = reader.ReadNextPcmFrame();
        var p3 = reader.ReadNextPcmFrame();
        var eof = reader.ReadNextPcmFrame();

        Assert.NotNull(p1);
        Assert.NotNull(p2);
        Assert.NotNull(p3);
        Assert.Null(eof);

        Assert.Equal(1024, p1!.Samples.Length);
        Assert.Equal(1024, p2!.Samples.Length);
        Assert.Equal(1024, p3!.Samples.Length);
    }

    // ----- iterator wrapper -----

    [Fact]
    public void ReadPcmFrames_EnumeratesAllBlocksThenStops()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoMultiSceFrameShared(blockCount: 2);
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        int count = 0;
        foreach (var p in reader.ReadPcmFrames())
        {
            Assert.Equal(1, p.ChannelCount);
            count++;
        }
        Assert.Equal(2, count);
    }

    // ----- ResetState -----

    [Fact]
    public void ResetState_AfterRead_ClearsCount()
    {
        byte[] frame = AacAdtsStreamReaderTests.BuildAdtsMonoSceFrameShared();
        using var ms = new MemoryStream(frame);
        using var reader = NewReader(ms);

        _ = reader.ReadNextPcmFrame();
        Assert.Equal(1, reader.FrameCount);

        reader.ResetState();
        Assert.Equal(0, reader.FrameCount);
        Assert.Null(reader.CurrentConfig);
    }

    // ----- Dispose -----

    [Fact]
    public void ReadNextPcmFrame_AfterDispose_Throws()
    {
        using var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => reader.ReadNextPcmFrame());
    }

    [Fact]
    public void Dispose_LeaveOpenFalse_DisposesStream()
    {
        var ms = new MemoryStream();
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: false);
        reader.Dispose();
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Dispose_LeaveOpenTrue_KeepsStreamOpen()
    {
        using var ms = new MemoryStream(new byte[] { 1, 2, 3 });
        var reader = new AacAdtsPcmStreamReader(ms, GetSf(), new AacHuffmanCodebook?[16], leaveOpen: true);
        reader.Dispose();
        Assert.Equal(1, ms.ReadByte());
    }

    // ----- helpers -----

    private static AacHuffmanCodebook GetSf() =>
        AacRawDataBlockTests.GetSharedSyntheticSfCodebook();

    private static AacAdtsPcmStreamReader NewReader(Stream s) =>
        new(s, GetSf(), new AacHuffmanCodebook?[16]);
}
