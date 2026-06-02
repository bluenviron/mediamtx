using Mediar.Codecs.PackBits;
using Xunit;

namespace Mediar.Tests;

public sealed class PackBitsCodecTests
{
    [Fact]
    public void Decode_AppleTechNoteExample()
    {
        // Apple TN1023 worked example.
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
        byte[] encoded = [0x80, 0x00, 0xFF];
        Assert.Equal(new byte[] { 0xFF }, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_HonoursExpectedLength()
    {
        byte[] encoded = [0xFE, 0xAA];
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
        Assert.True(encoded.Length < input.Length);
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

    [Fact]
    public void Decode_Empty_Input_Yields_Empty()
    {
        Assert.Equal(Array.Empty<byte>(), PackBitsCodec.Decode(ReadOnlySpan<byte>.Empty));
    }

    [Fact]
    public void Encode_Empty_Input_Yields_Empty()
    {
        Assert.Equal(Array.Empty<byte>(), PackBitsCodec.Encode(ReadOnlySpan<byte>.Empty));
    }

    [Fact]
    public void Encode_Single_Byte_Uses_Literal_Chunk()
    {
        byte[] encoded = PackBitsCodec.Encode(new byte[] { 0x42 });
        // 1-byte literal: control = 0 (n=0 → 1 byte) then byte.
        Assert.Equal(new byte[] { 0x00, 0x42 }, encoded);
    }

    [Fact]
    public void Encode_Two_Identical_Bytes_Stays_Literal()
    {
        // 2-byte run does not justify RLE (control + byte = 2 bytes either way),
        // and encoder requires runLen >= 3 to use the RLE chunk.
        byte[] encoded = PackBitsCodec.Encode(new byte[] { 0x55, 0x55 });
        Assert.Equal(new byte[] { 0x01, 0x55, 0x55 }, encoded);
    }

    [Fact]
    public void Encode_Three_Identical_Bytes_Uses_Rle_Chunk()
    {
        byte[] encoded = PackBitsCodec.Encode(new byte[] { 0xAB, 0xAB, 0xAB });
        // RLE chunk: count = -(3-1) = -2 → 0xFE
        Assert.Equal(new byte[] { 0xFE, 0xAB }, encoded);
    }

    [Fact]
    public void Encode_Max_Literal_Chunk_Is_128_Bytes()
    {
        // 128 distinct bytes followed by 4 of a kind: literal is exactly 128.
        byte[] input = new byte[128];
        for (int i = 0; i < input.Length; i++) input[i] = (byte)i;
        byte[] encoded = PackBitsCodec.Encode(input);
        Assert.Equal(0x7F, encoded[0]); // control byte = 127 → 128 literal bytes
        Assert.Equal(129, encoded.Length);
        Assert.Equal(input, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Encode_Long_Run_Splits_At_128_Bytes()
    {
        // 200 identical bytes → first run = 128 (max), second run = 72.
        byte[] input = new byte[200];
        Array.Fill(input, (byte)0xCC);
        byte[] encoded = PackBitsCodec.Encode(input);
        // First RLE: count -(128-1)=-127 → 0x81; then byte 0xCC.
        // Second RLE: count -(72-1)=-71 → 0xB9; then byte 0xCC.
        Assert.Equal(new byte[] { 0x81, 0xCC, 0xB9, 0xCC }, encoded);
    }

    [Fact]
    public void Decode_Literal_Chunk_Of_128_Bytes()
    {
        // Control byte = 127 → next 128 bytes are literal.
        byte[] encoded = new byte[129];
        encoded[0] = 0x7F;
        for (int i = 0; i < 128; i++) encoded[i + 1] = (byte)i;
        byte[] decoded = PackBitsCodec.Decode(encoded);
        Assert.Equal(128, decoded.Length);
        for (int i = 0; i < 128; i++) Assert.Equal((byte)i, decoded[i]);
    }

    [Fact]
    public void Decode_Rle_Of_128_Bytes()
    {
        // Control byte -127 (0x81) → 128 repeats.
        byte[] encoded = { 0x81, 0xCC };
        byte[] decoded = PackBitsCodec.Decode(encoded);
        Assert.Equal(128, decoded.Length);
        Assert.All(decoded, b => Assert.Equal((byte)0xCC, b));
    }

    [Fact]
    public void Decode_Truncated_Literal_Run_Stops_Cleanly()
    {
        // Control byte says 5 literals but only 2 follow → decoder breaks early.
        byte[] encoded = { 0x04, 0x11, 0x22 };
        byte[] decoded = PackBitsCodec.Decode(encoded);
        Assert.Empty(decoded);
    }

    [Fact]
    public void Decode_Truncated_Rle_Stops_Cleanly()
    {
        // Negative control byte but no run byte follows.
        byte[] encoded = { 0xFE };
        byte[] decoded = PackBitsCodec.Decode(encoded);
        Assert.Empty(decoded);
    }

    [Fact]
    public void Decode_ExpectedLength_Zero_Returns_Empty_Array()
    {
        byte[] encoded = { 0x7F /* says 128 literals */ };
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 0);
        Assert.Empty(decoded);
    }

    [Fact]
    public void Decode_ExpectedLength_Stops_Early_For_Literal()
    {
        // Encoded would produce 5 literal bytes but we cap at 3.
        byte[] encoded = { 0x04, 0xA, 0xB, 0xC, 0xD, 0xE };
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 3);
        Assert.Equal(new byte[] { 0xA, 0xB, 0xC }, decoded);
    }

    [Fact]
    public void Decode_ExpectedLength_Stops_Early_For_Rle()
    {
        // Encoded would produce 10 bytes but we cap at 4.
        byte[] encoded = { 0xF7 /* -9 → 10 */, 0xCC };
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 4);
        Assert.Equal(new byte[] { 0xCC, 0xCC, 0xCC, 0xCC }, decoded);
    }

    [Fact]
    public void Decode_Mix_Of_NoOps_And_Real_Chunks()
    {
        byte[] encoded = { 0x80, 0x80, 0x00, 0xAA, 0x80, 0xFE, 0xBB };
        Assert.Equal(new byte[] { 0xAA, 0xBB, 0xBB, 0xBB }, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Encode_Decode_Mixed_Pattern()
    {
        byte[] input = { 0x01, 0x02, 0x03, 0x44, 0x44, 0x44, 0x44, 0x44, 0x55, 0x66, 0x77 };
        byte[] encoded = PackBitsCodec.Encode(input);
        Assert.Equal(input, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_Worst_Case_Pure_NoOps_Returns_Empty()
    {
        byte[] encoded = new byte[20];
        Array.Fill(encoded, (byte)0x80);
        Assert.Empty(PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_With_Expected_Length_Always_Returns_Exact_Size()
    {
        // Even when the encoded stream is exhausted before we fill the buffer,
        // the array size still matches expectedLength.
        byte[] encoded = { 0x80 }; // single no-op
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 5);
        Assert.Equal(5, decoded.Length);
        Assert.All(decoded, b => Assert.Equal(0, b));
    }

    [Fact]
    public void Encode_All_Zeros_Big_Compresses_Dramatically()
    {
        byte[] input = new byte[4096];
        byte[] encoded = PackBitsCodec.Encode(input);
        // 4096 bytes / 128 = 32 RLE chunks * 2 bytes each = 64 bytes
        Assert.Equal(64, encoded.Length);
    }

    [Fact]
    public void Decode_Negative128_Control_Byte_Is_NoOp()
    {
        // -128 is the no-op marker; it consumes no further input.
        byte[] encoded = { 0x80, 0x80, 0x80, 0x00, 0x42 };
        Assert.Equal(new byte[] { 0x42 }, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_RleOf2_Single_Repeat()
    {
        // count = -(-1)+1 = 2 → 0xFF
        byte[] encoded = { 0xFF, 0x77 };
        Assert.Equal(new byte[] { 0x77, 0x77 }, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Decode_With_ExpectedLength_Beyond_Source_Pads_With_Zeros()
    {
        // Encoded stream produces 3 bytes; expected = 8 → trailing 5 zeros.
        byte[] encoded = { 0xFE, 0xAA }; // 3 × 0xAA
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 8);
        Assert.Equal(8, decoded.Length);
        Assert.Equal(new byte[] { 0xAA, 0xAA, 0xAA, 0x00, 0x00, 0x00, 0x00, 0x00 }, decoded);
    }

    [Fact]
    public void Decode_Literal_Single_Byte()
    {
        // Control 0 → 1 literal byte.
        byte[] encoded = { 0x00, 0x42 };
        Assert.Equal(new byte[] { 0x42 }, PackBitsCodec.Decode(encoded));
    }

    [Fact]
    public void Encode_TwoByte_Run_Followed_By_Literal_Becomes_Literal_Run()
    {
        // 2 same bytes don't form an RLE (encoder requires runLen >= 3);
        // it folds into the literal that follows.
        byte[] input = { 0x55, 0x55, 0x11, 0x22, 0x33 };
        byte[] encoded = PackBitsCodec.Encode(input);
        // All 5 bytes go literal: control = 4, then 5 bytes
        Assert.Equal(new byte[] { 0x04, 0x55, 0x55, 0x11, 0x22, 0x33 }, encoded);
    }

    [Fact]
    public void Encode_LongMixedPattern_RoundTrips()
    {
        // Mix RLE-eligible runs with literal runs of various lengths.
        var input = new byte[1024];
        var rng = new Random(12345);
        for (int i = 0; i < input.Length; i++)
        {
            input[i] = (i % 13 == 0) ? (byte)0x77 : (byte)rng.Next(0, 256);
        }
        byte[] encoded = PackBitsCodec.Encode(input);
        byte[] decoded = PackBitsCodec.Decode(encoded);
        Assert.Equal(input, decoded);
    }

    [Fact]
    public void Decode_ExpectedLength_Stops_Mid_Literal_Chunk()
    {
        // Literal chunk of 4 bytes, but expectedLength = 2 → only first 2 written.
        byte[] encoded = { 0x03, 0xAA, 0xBB, 0xCC, 0xDD };
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 2);
        Assert.Equal(new byte[] { 0xAA, 0xBB }, decoded);
    }

    [Fact]
    public void Decode_ExpectedLength_Stops_Mid_Rle_Chunk()
    {
        // RLE of 5 bytes, but expectedLength = 3.
        byte[] encoded = { 0xFC, 0x99 }; // -4 → 5 copies
        byte[] decoded = PackBitsCodec.Decode(encoded, expectedLength: 3);
        Assert.Equal(new byte[] { 0x99, 0x99, 0x99 }, decoded);
    }

    [Fact]
    public void Encode_127_Identical_Bytes_Single_Rle_Chunk()
    {
        byte[] input = new byte[127];
        Array.Fill(input, (byte)0x33);
        byte[] encoded = PackBitsCodec.Encode(input);
        // RLE: count = -(127-1) = -126 → 0x82
        Assert.Equal(new byte[] { 0x82, 0x33 }, encoded);
    }

    [Fact]
    public void Encode_128_Identical_Bytes_Single_Max_Rle_Chunk()
    {
        byte[] input = new byte[128];
        Array.Fill(input, (byte)0x44);
        byte[] encoded = PackBitsCodec.Encode(input);
        // RLE: count = -(128-1) = -127 → 0x81
        Assert.Equal(new byte[] { 0x81, 0x44 }, encoded);
    }

    [Fact]
    public void Decode_ExpectedLength_Negative_Acts_As_Unspecified()
    {
        // expectedLength < 0 is treated as "unknown" (resizable buffer).
        byte[] encoded = { 0xFE, 0xAA };
        Assert.Equal(new byte[] { 0xAA, 0xAA, 0xAA }, PackBitsCodec.Decode(encoded, expectedLength: -1));
    }

    [Fact]
    public void Encode_Single_NonRepeating_Byte_Sequence_Stays_Literal()
    {
        byte[] input = { 0x01, 0x02, 0x03, 0x04, 0x05 };
        byte[] encoded = PackBitsCodec.Encode(input);
        Assert.Equal(new byte[] { 0x04, 0x01, 0x02, 0x03, 0x04, 0x05 }, encoded);
        Assert.Equal(input, PackBitsCodec.Decode(encoded));
    }
}
