using Mediar.Codecs.Flac.Decoder;
using Xunit;

namespace Mediar.Tests;

public sealed class FlacDecoderTests
{
    /// <summary>
    /// Hand-crafted minimal FLAC frame: 1-channel, 44.1 kHz, 16-bit, 192-sample
    /// block, CONSTANT subframe with value 1000. Exercises frame header
    /// parsing, the CONSTANT subframe path and CRC-8 / CRC-16 verification.
    /// </summary>
    [Fact]
    public void Decodes_Constant_Subframe_Frame()
    {
        byte[] frame = BuildConstantFrame(constantValue: 1000);

        var pars = new AudioCodecParameters
        {
            Codec = CodecId.Flac,
            SampleRate = 44100,
            Channels = 1,
            BitsPerSample = 16,
            ExtraData = BuildStreamInfo(44100, 1, 16, maxBlock: 192),
        };

        using var dec = new FlacDecoder(pars);
        using var decoded = dec.Decode(frame, pts: 0);

        Assert.Equal(192, decoded.SamplesPerChannel);
        Assert.Equal(1, decoded.Channels);
        Assert.Equal(44100, decoded.SampleRate);
        float expected = 1000f / 32768f;
        var samples = decoded.Samples.Span;
        for (int i = 0; i < samples.Length; i++)
        {
            Assert.Equal(expected, samples[i], precision: 5);
        }
    }

    [Fact]
    public void Mismatched_Crc8_Throws()
    {
        byte[] frame = BuildConstantFrame(constantValue: 0);
        frame[5] ^= 0xFF; // corrupt CRC-8

        var pars = new AudioCodecParameters
        {
            Codec = CodecId.Flac, SampleRate = 44100, Channels = 1, BitsPerSample = 16,
            ExtraData = BuildStreamInfo(44100, 1, 16, 192),
        };
        using var dec = new FlacDecoder(pars);
        Assert.Throws<InvalidDataException>(() => dec.Decode(frame, 0));
    }

    [Fact]
    public void Mismatched_Crc16_Throws()
    {
        byte[] frame = BuildConstantFrame(constantValue: 0);
        frame[^1] ^= 0xFF; // corrupt CRC-16

        var pars = new AudioCodecParameters
        {
            Codec = CodecId.Flac, SampleRate = 44100, Channels = 1, BitsPerSample = 16,
            ExtraData = BuildStreamInfo(44100, 1, 16, 192),
        };
        using var dec = new FlacDecoder(pars);
        Assert.Throws<InvalidDataException>(() => dec.Decode(frame, 0));
    }

    private static byte[] BuildConstantFrame(int constantValue)
    {
        // Header:
        // 14 bits sync (1111 1111 1111 10) + 1 reserved 0 + 1 blocking 0    -> 0xFF 0xF8
        // 4 bits blocksize (0001=192) + 4 bits sample rate (1001=44.1kHz)   -> 0x19
        // 4 bits channels (0000=1) + 3 bits sample size (100=16) + 1 res 0  -> 0x08
        // UTF-8 frame number 0                                              -> 0x00
        // CRC-8 over the 5 bytes above
        byte[] header = new byte[6];
        header[0] = 0xFF;
        header[1] = 0xF8;
        header[2] = 0x19;
        header[3] = 0x08;
        header[4] = 0x00;
        header[5] = FlacCrc.Crc8(header.AsSpan(0, 5));

        // Subframe (CONSTANT):
        //   1 bit reserved 0 | 6 bits type 000000 | 1 bit wasted 0 | 16-bit signed sample
        // Total = 24 bits = 3 bytes; byte-aligned.
        byte[] sub = new byte[3];
        ushort v = (ushort)(constantValue & 0xFFFF);
        sub[0] = 0x00;             // 0 0000000 — type + wasted + first 0 bit of sample
        sub[1] = (byte)(v >> 8);
        sub[2] = (byte)v;

        byte[] payload = new byte[header.Length + sub.Length];
        Buffer.BlockCopy(header, 0, payload, 0, header.Length);
        Buffer.BlockCopy(sub, 0, payload, header.Length, sub.Length);

        ushort crc16 = FlacCrc.Crc16(payload);
        byte[] full = new byte[payload.Length + 2];
        Buffer.BlockCopy(payload, 0, full, 0, payload.Length);
        full[^2] = (byte)(crc16 >> 8);
        full[^1] = (byte)crc16;
        return full;
    }

    private static byte[] BuildStreamInfo(int sampleRate, int channels, int bps, int maxBlock)
    {
        byte[] si = new byte[34];
        // min/max block size (16+16 bits)
        si[0] = 0; si[1] = 16;
        si[2] = (byte)(maxBlock >> 8); si[3] = (byte)maxBlock;
        // min/max frame size left zero.
        // 20 bits sample rate + 3 bits channels-1 + 5 bits bps-1 + 36 bits total samples.
        long packed = ((long)sampleRate << 28) | ((long)(channels - 1) << 25) | ((long)(bps - 1) << 20);
        si[10] = (byte)(packed >> 56);
        si[11] = (byte)(packed >> 48);
        si[12] = (byte)(packed >> 40);
        si[13] = (byte)(packed >> 32);
        si[14] = (byte)(packed >> 24);
        si[15] = (byte)(packed >> 16);
        si[16] = (byte)(packed >> 8);
        si[17] = (byte)packed;
        return si;
    }
}
