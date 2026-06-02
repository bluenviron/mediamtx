using Mediar.Imaging;
using Mediar.Imaging.Flif;
using Xunit;

namespace Mediar.Tests;

public class FlifReaderTests
{
    [Fact]
    public void Parses_Flif_Header()
    {
        byte[] file = BuildFlif(width: 100, height: 80, channels: 4, bitDepth8: true, frames: 1);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);

        Assert.Equal(ImageFormat.Flif, r.Format);
        Assert.Equal(100, r.Info.Width);
        Assert.Equal(80, r.Info.Height);
        Assert.Equal(4, r.Channels);
        Assert.True(r.Info.HasAlpha);
        Assert.Equal(1, r.NumFrames);
    }

    [Fact]
    public void Parses_Animated_Flif()
    {
        byte[] file = BuildFlif(64, 48, 3, true, 16);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);
        Assert.Equal(16, r.NumFrames);
        Assert.True(r.Info.IsAnimated);
    }

    [Fact]
    public void Rejects_Non_Flif_Bytes()
    {
        Assert.Throws<ImageFormatException>(() =>
            FlifReader.Open(new MemoryStream(new byte[] { 0, 1, 2, 3, 4, 5 }), ownsStream: true));
    }

    [Fact]
    public async Task ReadFramesAsync_Throws()
    {
        byte[] file = BuildFlif(8, 8, 3, true, 1);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
    }

    [Fact]
    public async Task ReadFramesAsync_Message_Mentions_Flif()
    {
        byte[] file = BuildFlif(8, 8, 3, true, 1);
        using var r = FlifReader.Open(new MemoryStream(file), ownsStream: true);
        var ex = await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync()) { f.Dispose(); }
        });
        Assert.Contains("FLIF", ex.Message);
    }

    [Fact]
    public void Open_Stream_Null_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => FlifReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Path_Works()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-flif-{Guid.NewGuid():N}.flif");
        File.WriteAllBytes(path, BuildFlif(40, 30, 3, true, 1));
        try
        {
            using var r = FlifReader.Open(path);
            Assert.Equal(40, r.Info.Width);
            Assert.Equal(30, r.Info.Height);
        }
        finally { File.Delete(path); }
    }

    [Fact]
    public void Open_Path_Missing_Throws()
    {
        var path = Path.Combine(Path.GetTempPath(), $"mediar-flif-missing-{Guid.NewGuid():N}.flif");
        Assert.Throws<FileNotFoundException>(() => FlifReader.Open(path));
    }

    [Fact]
    public void Rejects_Stream_Too_Short()
    {
        Assert.Throws<ImageFormatException>(() =>
            FlifReader.Open(new MemoryStream(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F', 0 }), ownsStream: true));
    }

    [Theory]
    [InlineData((byte)'X', (byte)'L', (byte)'I', (byte)'F')]
    [InlineData((byte)'F', (byte)'X', (byte)'I', (byte)'F')]
    [InlineData((byte)'F', (byte)'L', (byte)'X', (byte)'F')]
    [InlineData((byte)'F', (byte)'L', (byte)'I', (byte)'X')]
    public void Rejects_Wrong_Signature(byte b0, byte b1, byte b2, byte b3)
    {
        byte[] bad = new byte[] { b0, b1, b2, b3, 0x40, (byte)'1' };
        Assert.Throws<ImageFormatException>(() => FlifReader.Open(new MemoryStream(bad), ownsStream: true));
    }

    [Fact]
    public void Format_Is_Flif()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(8, 8, 3, true, 1)), ownsStream: true);
        Assert.Equal(ImageFormat.Flif, r.Format);
        Assert.Equal(ImageFormat.Flif, r.Info.Format);
    }

    [Fact]
    public void Cannot_Decode_Pixels()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(8, 8, 3, true, 1)), ownsStream: true);
        Assert.False(r.CanDecodePixels);
    }

    [Fact]
    public void Metadata_Is_Empty()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(8, 8, 3, true, 1)), ownsStream: true);
        Assert.Same(ImageMetadata.Empty, r.Metadata);
    }

    [Theory]
    [InlineData(1, false)] // grayscale
    [InlineData(2, true)]  // gray + alpha
    [InlineData(3, false)] // RGB
    [InlineData(4, true)]  // RGBA
    public void Channels_Drive_HasAlpha(int channels, bool expectedAlpha)
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, channels, true, 1)), ownsStream: true);
        Assert.Equal(channels, r.Channels);
        Assert.Equal(expectedAlpha, r.Info.HasAlpha);
        Assert.Equal(channels, r.Info.ChannelCount);
    }

    [Fact]
    public void Channels_Zero_Falls_Back_To_Three()
    {
        // Flag byte 0x40 (interlaced, channels=0) should fall back to 3 channels.
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' });
        ms.WriteByte(0x40);
        ms.WriteByte((byte)'1');
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 0);
        using var r = FlifReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(3, r.Channels);
    }

    [Fact]
    public void BitDepth_Code_8Bit_Produces_8_Bits_Per_Channel()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, 3, true, 1)), ownsStream: true);
        Assert.Equal(1, r.BitDepthCode);
        Assert.Equal(24, r.Info.BitsPerPixel); // 8 * 3
    }

    [Fact]
    public void BitDepth_Code_16Bit_Produces_16_Bits_Per_Channel()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, 4, false, 1)), ownsStream: true);
        Assert.Equal(2, r.BitDepthCode);
        Assert.Equal(64, r.Info.BitsPerPixel); // 16 * 4
    }

    [Fact]
    public void BitDepth_Code_Unknown_Zero_Acts_As_8Bit_Effective()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' });
        ms.WriteByte(0x44);          // interlaced, channels=4
        ms.WriteByte((byte)'0');     // unknown bit depth
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 0);
        using var r = FlifReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(0, r.BitDepthCode);
        Assert.Equal(32, r.Info.BitsPerPixel); // bdCode!=2 → 8 bits * 4 channels
    }

    [Fact]
    public void BitDepth_Code_Unrecognised_Defaults_To_One()
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' });
        ms.WriteByte(0x43);
        ms.WriteByte((byte)'9');     // bogus
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 0);
        using var r = FlifReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.Equal(1, r.BitDepthCode);
    }

    [Fact]
    public void Interlaced_Flag_Detected_When_Bit5_Is_Zero()
    {
        // 0x40 has bit 5 (0x20) clear → interlaced
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, 3, true, 1)), ownsStream: true);
        Assert.True(r.IsInterlaced);
    }

    [Fact]
    public void Non_Interlaced_Flag_Detected_When_Bit5_Is_Set()
    {
        // Build with flags = 0x60 (interlaced cleared, channels=0 → 3)
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' });
        ms.WriteByte(0x63);          // 0x60 | 3
        ms.WriteByte((byte)'1');
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 9);
        WriteVarInt(ms, 0);
        using var r = FlifReader.Open(new MemoryStream(ms.ToArray()), ownsStream: true);
        Assert.False(r.IsInterlaced);
    }

    [Fact]
    public void Frame_Count_Of_One_Is_Static()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, 3, true, 1)), ownsStream: true);
        Assert.Equal(1, r.NumFrames);
        Assert.False(r.Info.IsAnimated);
        Assert.Equal(1, r.Info.FrameCount);
    }

    [Fact]
    public void Frame_Count_Above_One_Is_Animated()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, 3, true, 5)), ownsStream: true);
        Assert.True(r.Info.IsAnimated);
        Assert.Equal(5, r.Info.FrameCount);
    }

    [Fact]
    public void Large_Dimensions_Use_Multi_Byte_VarInt()
    {
        using var r = FlifReader.Open(new MemoryStream(BuildFlif(4096, 8192, 3, true, 1)), ownsStream: true);
        Assert.Equal(4096, r.Info.Width);
        Assert.Equal(8192, r.Info.Height);
    }

    [Fact]
    public void Dispose_Is_Idempotent()
    {
        var r = FlifReader.Open(new MemoryStream(BuildFlif(16, 16, 3, true, 1)), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Dispose_With_OwnsStream_True_Closes_Underlying()
    {
        var s = new MemoryStream(BuildFlif(16, 16, 3, true, 1));
        var r = FlifReader.Open(s, ownsStream: true);
        r.Dispose();
        Assert.Throws<ObjectDisposedException>(() => _ = s.Length);
    }

    [Fact]
    public void Dispose_With_OwnsStream_False_Leaves_Open()
    {
        var s = new MemoryStream(BuildFlif(16, 16, 3, true, 1));
        var r = FlifReader.Open(s, ownsStream: false);
        r.Dispose();
        _ = s.Length;
        s.Dispose();
    }

    // -------------------- helpers --------------------

    private static byte[] BuildFlif(int width, int height, int channels, bool bitDepth8, int frames)
    {
        using var ms = new MemoryStream();
        ms.Write(new byte[] { (byte)'F', (byte)'L', (byte)'I', (byte)'F' });
        byte flags = (byte)(0x40 | (channels & 0xF));
        ms.WriteByte(flags);
        ms.WriteByte((byte)(bitDepth8 ? '1' : '2'));
        WriteVarInt(ms, (uint)(width - 1));
        WriteVarInt(ms, (uint)(height - 1));
        WriteVarInt(ms, (uint)(frames - 1));
        return ms.ToArray();
    }

    private static void WriteVarInt(Stream s, uint v)
    {
        Span<byte> stack = stackalloc byte[5];
        int n = 0;
        do
        {
            stack[n++] = (byte)(v & 0x7F);
            v >>= 7;
        } while (v != 0);
        for (int i = n - 1; i >= 0; i--)
        {
            byte by = stack[i];
            if (i > 0) by |= 0x80;
            s.WriteByte(by);
        }
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => FlifReader.Open((string)null!));
    }
}
