using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Dds;
using Xunit;

namespace Mediar.Tests.Dds;

/// <summary>
/// Tests for DXGI 32-bit integer formats - <c>R32_UINT</c> (42),
/// <c>R32_SINT</c> (43), <c>R32G32_UINT</c> (17), <c>R32G32_SINT</c> (18),
/// <c>R32G32B32_UINT</c> (7), <c>R32G32B32_SINT</c> (8),
/// <c>R32G32B32A32_UINT</c> (1), <c>R32G32B32A32_SINT</c> (3) decoded
/// via the new integer PixelFormat family.
/// </summary>
public sealed class DdsDxgiIntegerFormatsTests
{
    private const uint DDPF_FOURCC = 0x4;

    private static byte[] BuildDx10Dds(int width, int height, uint dxgiFormat)
    {
        var hdr = new byte[128];
        hdr[0] = (byte)'D'; hdr[1] = (byte)'D'; hdr[2] = (byte)'S'; hdr[3] = (byte)' ';
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4), 124);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(8), 0x1007);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(12), (uint)height);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(16), (uint)width);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(76), 32);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(80), DDPF_FOURCC);
        var dx10 = Encoding.ASCII.GetBytes("DX10");
        hdr[84] = dx10[0]; hdr[85] = dx10[1]; hdr[86] = dx10[2]; hdr[87] = dx10[3];
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(108), 0x1000);

        var tail = new byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(0, 4), dxgiFormat);
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(4, 4), 3);
        BinaryPrimitives.WriteUInt32LittleEndian(tail.AsSpan(12, 4), 1);

        var combined = new byte[hdr.Length + tail.Length];
        Buffer.BlockCopy(hdr, 0, combined, 0, hdr.Length);
        Buffer.BlockCopy(tail, 0, combined, hdr.Length, tail.Length);
        return combined;
    }

    private static byte[] Concat(byte[] a, byte[] b)
    {
        var r = new byte[a.Length + b.Length];
        Buffer.BlockCopy(a, 0, r, 0, a.Length);
        Buffer.BlockCopy(b, 0, r, a.Length, b.Length);
        return r;
    }

    [Theory]
    [InlineData(42u, PixelFormat.Gray32UInt, 32, 1, false)]
    [InlineData(43u, PixelFormat.Gray32SInt, 32, 1, false)]
    [InlineData(17u, PixelFormat.Rg64UInt, 64, 2, false)]
    [InlineData(18u, PixelFormat.Rg64SInt, 64, 2, false)]
    [InlineData(7u, PixelFormat.Rgb96UInt, 96, 3, false)]
    [InlineData(8u, PixelFormat.Rgb96SInt, 96, 3, false)]
    [InlineData(1u, PixelFormat.Rgba128UInt, 128, 4, true)]
    [InlineData(3u, PixelFormat.Rgba128SInt, 128, 4, true)]
    public void Integer_Format_Maps_Correctly(uint dxgi, PixelFormat expected, int bpp, int channels, bool hasAlpha)
    {
        int bytesPerPixel = bpp / 8;
        var file = Concat(BuildDx10Dds(1, 1, dxgi), new byte[bytesPerPixel]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Equal(expected, reader.Info.PixelFormat);
        Assert.Equal(bpp, reader.Info.BitsPerPixel);
        Assert.Equal(channels, reader.Info.ChannelCount);
        Assert.Equal(hasAlpha, reader.Info.HasAlpha);
        Assert.Null(reader.Info.ColorSpace);
    }

    [Fact]
    public async Task R32_UINT_Round_Trips_Bytes()
    {
        var payload = new byte[8]; // 2 pixels of u32
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(0, 4), 0xDEADBEEF);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(4, 4), 0x12345678);
        var file = Concat(BuildDx10Dds(2, 1, 42), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Gray32UInt, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public async Task R32G32B32A32_SINT_Round_Trips_Bytes()
    {
        var payload = new byte[16]; // 1 pixel of i32x4
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(0, 4), -1);
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(4, 4), int.MinValue);
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(8, 4), int.MaxValue);
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(12, 4), 42);
        var file = Concat(BuildDx10Dds(1, 1, 3), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgba128SInt, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public void Integer_Format_Extension_Methods()
    {
        Assert.Equal(32, PixelFormat.Gray32UInt.BitsPerPixel());
        Assert.Equal(32, PixelFormat.Gray32SInt.BitsPerPixel());
        Assert.Equal(64, PixelFormat.Rg64UInt.BitsPerPixel());
        Assert.Equal(64, PixelFormat.Rg64SInt.BitsPerPixel());
        Assert.Equal(96, PixelFormat.Rgb96UInt.BitsPerPixel());
        Assert.Equal(96, PixelFormat.Rgb96SInt.BitsPerPixel());
        Assert.Equal(128, PixelFormat.Rgba128UInt.BitsPerPixel());
        Assert.Equal(128, PixelFormat.Rgba128SInt.BitsPerPixel());

        Assert.Equal(1, PixelFormat.Gray32UInt.ChannelCount());
        Assert.Equal(2, PixelFormat.Rg64UInt.ChannelCount());
        Assert.Equal(3, PixelFormat.Rgb96UInt.ChannelCount());
        Assert.Equal(4, PixelFormat.Rgba128UInt.ChannelCount());

        Assert.False(PixelFormat.Gray32UInt.HasAlpha());
        Assert.False(PixelFormat.Rg64SInt.HasAlpha());
        Assert.False(PixelFormat.Rgb96UInt.HasAlpha());
        // Note: HasAlpha for Rgba128UInt/SInt is determined at the DDS layer
        // (it sets the flag explicitly), not via the PixelFormat extension.
    }

    [Fact]
    public async Task R32_SINT_Round_Trips_Negative_Bytes()
    {
        var payload = new byte[8];
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(0, 4), -1);
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(4, 4), int.MinValue);
        var file = Concat(BuildDx10Dds(2, 1, 43), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Gray32SInt, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public async Task R32G32_UINT_Round_Trips_Bytes()
    {
        var payload = new byte[8];
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(0, 4), 0xFFFFFFFFu);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(4, 4), 0x80000000u);
        var file = Concat(BuildDx10Dds(1, 1, 17), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rg64UInt, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public async Task R32G32B32_SINT_Round_Trips_Negative_Bytes()
    {
        var payload = new byte[12];
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(0, 4), int.MinValue);
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(4, 4), -1);
        BinaryPrimitives.WriteInt32LittleEndian(payload.AsSpan(8, 4), int.MaxValue);
        var file = Concat(BuildDx10Dds(1, 1, 8), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgb96SInt, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public async Task R32G32B32A32_UINT_Round_Trips_Bytes()
    {
        var payload = new byte[16];
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(0, 4), 0xDEADBEEFu);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(4, 4), 0xCAFEBABEu);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(8, 4), 0x12345678u);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(12, 4), 0xFFFFFFFFu);
        var file = Concat(BuildDx10Dds(1, 1, 1), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(PixelFormat.Rgba128UInt, frame!.PixelFormat);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public async Task R32_UINT_MultiRow_Stride_And_Bytes_Preserved()
    {
        const int w = 2, h = 3;
        var payload = new byte[w * h * 4];
        for (int i = 0; i < w * h; i++)
        {
            BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(i * 4, 4), (uint)(i * 0x01020304));
        }
        var file = Concat(BuildDx10Dds(w, h, 42), payload);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        ImageFrame? frame = null;
        await foreach (var f in reader.ReadFramesAsync()) { frame = f; }
        Assert.NotNull(frame);
        Assert.Equal(w * 4, frame!.Stride);
        Assert.Equal(h, frame!.Height);
        Assert.True(frame.Pixels.Span.SequenceEqual(payload));
    }

    [Fact]
    public void Integer_Format_ColorSpace_Is_Null_For_R32G32B32_UINT()
    {
        var file = Concat(BuildDx10Dds(1, 1, 7), new byte[12]);
        using var reader = DdsReader.Open(new MemoryStream(file, writable: false));
        Assert.Null(reader.Info.ColorSpace);
        Assert.Equal(PixelFormat.Rgb96UInt, reader.Info.PixelFormat);
    }

    [Fact]
    public void Integer_Format_Empty_Stream_Throws_ImageFormatException()
    {
        using var ms = new MemoryStream(Array.Empty<byte>(), writable: false);
        Assert.Throws<ImageFormatException>(() => DdsReader.Open(ms));
    }
}
