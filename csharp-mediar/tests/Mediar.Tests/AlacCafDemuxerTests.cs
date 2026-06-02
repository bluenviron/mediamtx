using System.Buffers.Binary;
using Mediar.Codecs.Alac.Decoder;
using Mediar.Containers.Caf;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// CAF demuxer integration for ALAC: verifies the `kuki` chunk is read and
/// attached as the audio track's ExtraData so a downstream
/// <see cref="AlacDecoder"/> can be constructed from the demuxer-provided
/// codec parameters.
/// </summary>
public sealed class AlacCafDemuxerTests
{
    [Fact]
    public void Kuki_Cookie_IsAttachedAsExtraData()
    {
        byte[] cookie = BuildCookie();
        byte[] caf = BuildMinimalAlacCaf(cookie);

        using var src = new MemoryRandomAccessSource(caf);
        using var dx = CafDemuxer.Open(src);

        var t = Assert.Single(dx.Tracks);
        Assert.Equal(CodecId.Alac, t.Codec.Codec);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(cookie.Length, audio.ExtraData.Length);
        Assert.True(audio.ExtraData.Span.SequenceEqual(cookie));

        // Normalised cookie must be the 24-byte body, decoder must accept it.
        var body = AlacExtraData.NormalizeCookie(audio.ExtraData.Span);
        Assert.Equal(24, body.Length);
        var config = AlacSpecificConfig.Parse(body);
        Assert.Equal(2, config.NumChannels);
    }

    [Fact]
    public void MissingKukiChunk_LeavesExtraDataEmpty()
    {
        // ALAC tracks without a kuki chunk leave ExtraData empty; constructing
        // an AlacDecoder will then fail (as expected) — this guards against
        // accidentally synthesising a fake cookie.
        byte[] caf = BuildMinimalAlacCaf(cookie: null);

        using var src = new MemoryRandomAccessSource(caf);
        using var dx = CafDemuxer.Open(src);

        var t = Assert.Single(dx.Tracks);
        var audio = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.True(audio.ExtraData.IsEmpty);
    }

    private static byte[] BuildCookie()
    {
        var b = new byte[24];
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(0, 4), 4096);
        b[4] = 0;
        b[5] = 16;
        b[6] = (byte)AlacSpecificConfig.DefaultPb;
        b[7] = (byte)AlacSpecificConfig.DefaultMb;
        b[8] = (byte)AlacSpecificConfig.DefaultKb;
        b[9] = 2;
        BinaryPrimitives.WriteUInt16BigEndian(b.AsSpan(10, 2), 255);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(12, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(16, 4), 0);
        BinaryPrimitives.WriteUInt32BigEndian(b.AsSpan(20, 4), 44100);
        return b;
    }

    private static byte[] BuildMinimalAlacCaf(byte[]? cookie)
    {
        using var ms = new MemoryStream();
        // 'caff' header
        ms.Write(new byte[] { (byte)'c', (byte)'a', (byte)'f', (byte)'f' });
        WriteBe16(ms, 1); // version
        WriteBe16(ms, 0); // flags

        // desc chunk (32 bytes payload)
        WriteChunkHeader(ms, "desc", 32);
        WriteF64Be(ms, 44100.0);
        Write4cc(ms, "alac");
        WriteBe32(ms, 0); // flags
        WriteBe32(ms, 0); // bytesPerPacket (VBR)
        WriteBe32(ms, 4096); // framesPerPacket
        WriteBe32(ms, 2); // channels
        WriteBe32(ms, 16); // bits per channel

        // kuki chunk (cookie bytes)
        if (cookie is not null)
        {
            WriteChunkHeader(ms, "kuki", cookie.Length);
            ms.Write(cookie);
        }

        // pakt chunk (24 bytes header + zero packet sizes — 1 packet)
        WriteChunkHeader(ms, "pakt", 24 + 1); // 1 byte for varint = 0x00
        WriteBe64(ms, 1); // numPackets
        WriteBe64(ms, 4096); // numValidFrames
        WriteBe32(ms, 0); // primingFrames
        WriteBe32(ms, 0); // remainderFrames
        ms.WriteByte(0); // one varint sample size = 0 (skip)

        // data chunk: 4-byte edit count + 1-byte payload (END element)
        WriteChunkHeader(ms, "data", 4 + 1);
        WriteBe32(ms, 0);
        ms.WriteByte(0xE0); // 3-bit END tag (0b111) in high bits

        return ms.ToArray();
    }

    private static void WriteChunkHeader(MemoryStream ms, string type, long size)
    {
        Write4cc(ms, type);
        WriteBe64(ms, size);
    }
    private static void Write4cc(MemoryStream ms, string s)
    {
        ms.WriteByte((byte)s[0]); ms.WriteByte((byte)s[1]); ms.WriteByte((byte)s[2]); ms.WriteByte((byte)s[3]);
    }
    private static void WriteBe16(MemoryStream ms, ushort v)
    {
        ms.WriteByte((byte)(v >> 8)); ms.WriteByte((byte)v);
    }
    private static void WriteBe32(MemoryStream ms, uint v)
    {
        ms.WriteByte((byte)(v >> 24)); ms.WriteByte((byte)(v >> 16));
        ms.WriteByte((byte)(v >> 8)); ms.WriteByte((byte)v);
    }
    private static void WriteBe64(MemoryStream ms, long v)
    {
        Span<byte> buf = stackalloc byte[8];
        BinaryPrimitives.WriteInt64BigEndian(buf, v);
        ms.Write(buf);
    }
    private static void WriteF64Be(MemoryStream ms, double v)
    {
        Span<byte> buf = stackalloc byte[8];
        BinaryPrimitives.WriteInt64BigEndian(buf, BitConverter.DoubleToInt64Bits(v));
        ms.Write(buf);
    }
}
