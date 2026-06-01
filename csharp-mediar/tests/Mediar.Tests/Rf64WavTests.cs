using System.Buffers.Binary;
using Mediar.Containers.Wav;
using Xunit;

namespace Mediar.Tests;

public sealed class Rf64WavTests
{
    // Builds a minimal RF64 stream with ds64 + fmt + data, using 0xFFFFFFFF
    // sentinels so the ds64 64-bit sizes are mandatory.
    private static byte[] BuildRf64(int sampleRate, int channels, int bits, byte[] pcm)
    {
        using var ms = new MemoryStream();
        // RIFF header: "RF64" + 0xFFFFFFFF + "WAVE"
        ms.Write("RF64"u8);
        ms.Write(new byte[] { 0xFF, 0xFF, 0xFF, 0xFF });
        ms.Write("WAVE"u8);

        // ds64 chunk: 28-byte payload.
        ms.Write("ds64"u8);
        Span<byte> dsLen = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(dsLen, 28);
        ms.Write(dsLen);
        // riffSize (i64), dataSize (i64), sampleCount (i64), tableLen (i32)
        Span<byte> i64 = stackalloc byte[8];
        BinaryPrimitives.WriteInt64LittleEndian(i64, 0); ms.Write(i64); // riffSize: ignored
        BinaryPrimitives.WriteInt64LittleEndian(i64, pcm.Length); ms.Write(i64);
        BinaryPrimitives.WriteInt64LittleEndian(i64, pcm.Length / (bits / 8 * channels)); ms.Write(i64);
        Span<byte> tableLen = stackalloc byte[4];
        BinaryPrimitives.WriteInt32LittleEndian(tableLen, 0); ms.Write(tableLen);

        // fmt chunk: 16-byte PCM
        ms.Write("fmt "u8);
        Span<byte> fmtLen = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(fmtLen, 16);
        ms.Write(fmtLen);
        Span<byte> fmt = stackalloc byte[16];
        BinaryPrimitives.WriteUInt16LittleEndian(fmt[..2], 1); // PCM
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.Slice(2, 2), (ushort)channels);
        BinaryPrimitives.WriteUInt32LittleEndian(fmt.Slice(4, 4), (uint)sampleRate);
        BinaryPrimitives.WriteUInt32LittleEndian(fmt.Slice(8, 4), (uint)(sampleRate * channels * bits / 8));
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.Slice(12, 2), (ushort)(channels * bits / 8));
        BinaryPrimitives.WriteUInt16LittleEndian(fmt.Slice(14, 2), (ushort)bits);
        ms.Write(fmt);

        // data chunk with sentinel 0xFFFFFFFF (real size in ds64).
        ms.Write("data"u8);
        ms.Write(new byte[] { 0xFF, 0xFF, 0xFF, 0xFF });
        ms.Write(pcm);
        if ((pcm.Length & 1) != 0) ms.WriteByte(0);

        return ms.ToArray();
    }

    [Fact]
    public async Task Rf64_With_Ds64_Sentinel_Reads_Correct_DataSize()
    {
        const int sr = 8000;
        const int ch = 1;
        const int bits = 16;
        const int frames = 1024;
        byte[] pcm = new byte[frames * 2];
        for (int i = 0; i < frames; i++)
        {
            short v = (short)((i * 13) & 0x7FFF);
            pcm[i * 2] = (byte)v;
            pcm[i * 2 + 1] = (byte)(v >> 8);
        }
        byte[] file = BuildRf64(sr, ch, bits, pcm);

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);

        Assert.Equal("wav", dx.FormatName);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.PcmS16Le, a.Codec);
        Assert.Equal(sr, a.SampleRate);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Bw64_Magic_Also_Accepted()
    {
        const int sr = 16000;
        byte[] pcm = new byte[200];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)i;
        byte[] file = BuildRf64(sr, 1, 8, pcm);
        // Patch magic: RF64 → BW64
        file[0] = (byte)'B'; file[1] = (byte)'W'; file[2] = (byte)'6'; file[3] = (byte)'4';

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Equal("wav", dx.FormatName);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Theory]
    [InlineData(8000, 1, 8)]
    [InlineData(11025, 1, 16)]
    [InlineData(22050, 2, 16)]
    [InlineData(44100, 2, 24)]
    [InlineData(48000, 6, 16)]
    [InlineData(96000, 2, 32)]
    public async Task Rf64_RoundTrips_Various_Formats(int sr, int ch, int bits)
    {
        const int frames = 128;
        byte[] pcm = new byte[frames * ch * bits / 8];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)(i & 0xFF);
        byte[] file = BuildRf64(sr, ch, bits, pcm);

        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        var a = Assert.IsType<AudioCodecParameters>(dx.Tracks[0].Codec);
        Assert.Equal(sr, a.SampleRate);
        Assert.Equal(ch, a.Channels);

        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; }
            finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Rf64_Demuxer_FormatName_Is_Wav()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[2]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Equal("wav", dx.FormatName);
    }

    [Fact]
    public async Task Rf64_With_Empty_Data_Returns_No_Samples()
    {
        byte[] file = BuildRf64(8000, 1, 16, Array.Empty<byte>());
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        int count = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { count++; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(0, count);
    }

    [Fact]
    public async Task Rf64_With_Odd_DataSize_Tolerates_Pad_Byte()
    {
        // 8-bit PCM with odd-length data triggers a pad byte after data.
        byte[] pcm = new byte[151];
        for (int i = 0; i < pcm.Length; i++) pcm[i] = (byte)i;
        byte[] file = BuildRf64(8000, 1, 8, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        long total = 0;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            try { total += s.Data.Length; } finally { s.Owner?.Dispose(); }
        }
        Assert.Equal(pcm.Length, total);
    }

    [Fact]
    public async Task Rf64_Track_Has_Audio_Stream_Kind()
    {
        byte[] file = BuildRf64(48000, 2, 16, new byte[400]);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.Equal(StreamKind.Audio, dx.Tracks[0].Kind);
    }

    [Fact]
    public async Task Rf64_Duration_Reflects_PCM_Frame_Count()
    {
        // 1000 16-bit mono frames at 8 kHz = 125 ms.
        byte[] pcm = new byte[1000 * 2];
        byte[] file = BuildRf64(8000, 1, 16, pcm);
        using var src = new IO.MemoryRandomAccessSource(file);
        using var dx = WavDemuxer.Open(src);
        Assert.InRange((dx.Duration - TimeSpan.FromMilliseconds(125)).TotalMilliseconds, -5, 5);
    }

    [Fact]
    public void Rf64_Wrong_Magic_Throws()
    {
        byte[] file = BuildRf64(8000, 1, 16, new byte[8]);
        file[0] = (byte)'X'; file[1] = (byte)'Y'; file[2] = (byte)'Z'; file[3] = (byte)'!';
        using var src = new IO.MemoryRandomAccessSource(file);
        Assert.Throws<InvalidDataException>(() => WavDemuxer.Open(src));
    }
}
