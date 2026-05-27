using Mediar.Codecs.PackBits;
using Xunit;

namespace Mediar.Tests;

public sealed class PackBitsCodecTests
{
    [Fact]
    public void Decode_AppleTechNoteExample()
    {
        // Apple Technical Note TN1023 worked example. The encoded stream
        // FE AA 02 80 00 2A FD AA 03 80 00 2A 22 F7 AA decodes to:
        //   AA AA AA   80 00 2A   AA AA AA AA   80 00 2A 22   AA*10
        byte[] encoded =
        [
            0xFE, 0xAA, 0x02, 0x80, 0x00, 0x2A, 0xFD, 0xAA,
            0x03, 0x80, 0x00, 0x2A, 0x22, 0xF7, 0xAA,
        ];
        byte[] expected =
        [
            0xAA, 0xAA, 0xAA,
            0x80, 0x00, 0x2A,
            0xAA, 0xAA, 0xAA, 0xAA,
            0x80, 0x00, 0x2A, 0x22,
            0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA,
        ];
        Assert.Equal(expected, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_NoOpControlByteIsSkipped()
    {
        byte[] encoded = [0x80, 0x00, 0xFF]; // no-op, then literal of one byte 0xFF
        Assert.Equal(new byte[] { 0xFF }, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_HonoursExpectedLength()
    {
        byte[] encoded = [0xFE, 0xAA]; // would produce 3 bytes
        Assert.Equal(new byte[] { 0xAA, 0xAA }, PackBitsCodec.Decode(encoded, expectedLength: 2));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(1)]
    [InlineData(31)]
    [InlineData(127)]
    [InlineData(128)]
    [InlineData(255)]
    [InlineData(1024)]
    public void RoundTrip_RandomBuffers(int size)
    {
        var rng = new Random(size * 7919);
        var input = new byte[size];
        rng.NextBytes(input);
        byte[] encoded = PackBitsCodec.Encode(input);
        byte[] decoded = PackBitsCodec.Decode(encoded);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void RoundTrip_AllSameByte()
    {
        var input = new byte[513];
        Array.Fill(input, (byte)0x42);
        byte[] encoded = PackBitsCodec.Encode(input);
        Assert.True(encoded.Length < input.Length, "Long runs should compress.");
        Assert.Equal(input, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void RoundTrip_AlternatingBytes()
    {
        var input = new byte[256];
        for (int i = 0; i < input.Length; i++) input[i] = (byte)(i % 2 == 0 ? 0x10 : 0x20);
        byte[] encoded = PackBitsCodec.Encode(input);
        Assert.Equal(input, PackBitsCodec.Decode(encoded));
    }
}
