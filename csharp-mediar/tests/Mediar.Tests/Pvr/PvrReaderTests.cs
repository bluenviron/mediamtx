using Mediar.Codecs.Bcn;
using Mediar.Codecs.Etc;
using Mediar.Imaging;
using Mediar.Imaging.Pvr;
using Xunit;

namespace Mediar.Tests.Pvr;

/// <summary>
/// Tests for <see cref="PvrReader"/> (PVR v3), covering version-word
/// validation, endianness handling, pre-known format-id mapping to BCn /
/// ETC, channel-descriptor mapping to uncompressed PixelFormat, mip
/// pyramid enumeration, metadata-block parsing, and pixel-decode
/// round-trips.
/// </summary>
public sealed class PvrReaderTests
{
    [Fact]
    public void Rejects_Truncated_File()
    {
        var tiny = new byte[16];
        using var ms = new MemoryStream(tiny, writable: false);
        Assert.Throws<ImageFormatException>(() => PvrReader.Open(ms));
    }

    [Fact]
    public void Rejects_Invalid_Version_Word()
    {
        var bytes = new byte[64];
        bytes[0] = 0xDE; bytes[1] = 0xAD; bytes[2] = 0xBE; bytes[3] = 0xEF;
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => PvrReader.Open(ms));
    }

    [Fact]
    public void Parses_Uncompressed_Rgba8_Channel_Descriptor()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 4, Height = 4,
        };
        var payload = new byte[4 * 4 * 4];
        for (int i = 0; i < payload.Length; i += 4)
        {
            payload[i + 0] = 0x10; payload[i + 1] = 0x20;
            payload[i + 2] = 0x30; payload[i + 3] = 0xFF;
        }
        b.Payloads.Add(payload);
        var bytes = b.Build();

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(4, reader.Info.Width);
        Assert.Equal(4, reader.Info.Height);
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
        Assert.Single(reader.Levels);
        Assert.Equal(BcnFormat.None, reader.Pvr.Bcn);
        Assert.Equal(EtcFormat.None, reader.Pvr.Etc);
        Assert.Equal(PvrFormatId.None, reader.Pvr.FormatId);
    }

    [Fact]
    public async Task ReadFrames_Uncompressed_Rgba8_Round_Trips_Pixels()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 2, Height = 2,
        };
        var payload = new byte[]
        {
            0x10, 0x20, 0x30, 0xFF,
            0x40, 0x50, 0x60, 0xFF,
            0x70, 0x80, 0x90, 0xFF,
            0xA0, 0xB0, 0xC0, 0xFF,
        };
        b.Payloads.Add(payload);
        var bytes = b.Build();

        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        await foreach (var f in reader.ReadFramesAsync())
        {
            Assert.Equal(2, f.Width);
            Assert.Equal(2, f.Height);
            Assert.Equal(PixelFormat.Rgba32, f.PixelFormat);
            var pixels = f.Pixels.Span;
            Assert.Equal(0x10, pixels[0]);
            Assert.Equal(0xC0, pixels[14]);
            Assert.Equal(0xFF, pixels[15]);
            f.Dispose();
            return;
        }
    }

    [Fact]
    public void Detects_Bc1_From_FormatId()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Bc1,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[8]); // 4x4 BC1 block = 8 bytes
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(BcnFormat.Bc1, reader.Pvr.Bcn);
        Assert.Equal(PvrFormatId.Bc1, reader.Pvr.FormatId);
        Assert.Equal(PixelFormat.Bgra32, reader.Info.PixelFormat);
    }

    [Fact]
    public void Detects_Dxt1_Aliases_To_Bc1()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Dxt1,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.Equal(BcnFormat.Bc1, reader.Pvr.Bcn);
    }

    [Fact]
    public void Detects_Etc1_From_FormatId()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Etc1,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[8]); // 4x4 ETC1 block = 8 bytes
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.True(reader.CanDecodePixels);
        Assert.Equal(EtcFormat.Etc1Rgb, reader.Pvr.Etc);
        Assert.Equal(PixelFormat.Rgba32, reader.Info.PixelFormat);
    }

    [Fact]
    public void Detects_Etc2_Rgba_From_FormatId()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Etc2Rgba,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[16]); // 4x4 ETC2 RGBA8 block = 16 bytes
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.Equal(EtcFormat.Etc2Rgba8, reader.Pvr.Etc);
    }

    [Fact]
    public async Task ReadFrames_Etc1_Yields_Decoded_Rgba32()
    {
        // All-zero ETC1 block: ETC1 individual mode with base color 0,0,0
        // and modifier table index 0 row 0 = +2 -> pixels all (2,2,2,255).
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Etc1,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        await foreach (var f in reader.ReadFramesAsync())
        {
            Assert.Equal(PixelFormat.Rgba32, f.PixelFormat);
            Assert.Equal(4 * 4 * 4, f.Pixels.Length);
            var p = f.Pixels.Span;
            Assert.Equal(2, p[0]); Assert.Equal(2, p[1]);
            Assert.Equal(2, p[2]); Assert.Equal(0xFF, p[3]);
            f.Dispose();
            return;
        }
    }

    [Fact]
    public void Walks_Multiple_Mip_Levels()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 4, Height = 4,
            NumMipMaps = 3,
        };
        b.Payloads.Add(new byte[4 * 4 * 4]); // 4x4
        b.Payloads.Add(new byte[2 * 2 * 4]); // 2x2
        b.Payloads.Add(new byte[1 * 1 * 4]); // 1x1
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.Equal(3, reader.Levels.Count);
        Assert.Equal(4, reader.Levels[0].Width);
        Assert.Equal(2, reader.Levels[1].Width);
        Assert.Equal(1, reader.Levels[2].Width);
    }

    [Fact]
    public void Cubemap_Has_Six_Faces_Per_Level()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 2, Height = 2,
            NumFaces = 6,
        };
        for (int f = 0; f < 6; f++) b.Payloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.Equal(6, reader.Levels.Count);
        for (int i = 0; i < 6; i++) Assert.Equal(i, reader.Levels[i].Face);
    }

    [Fact]
    public void Metadata_Block_Is_Parsed()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 2, Height = 2,
        };
        // PVR\x03 namespace, key=4 (orientation), 3 bytes of payload.
        b.MetaEntries.Add((0x03525650u, 4u, new byte[] { 1, 0, 0 }));
        b.Payloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.Single(reader.Pvr.MetaEntries);
        Assert.Equal(0x03525650u, reader.Pvr.MetaEntries[0].FourCc);
        Assert.Equal(4u, reader.Pvr.MetaEntries[0].Key);
        Assert.Equal(3, reader.Pvr.MetaEntries[0].Data.Length);
        Assert.True(reader.Pvr.MetaByFourCcKey.ContainsKey(((ulong)0x03525650u << 32) | 4u));
    }

    [Fact]
    public void Pvrtc_Surface_Is_Surfaced_As_Undecodable()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Pvrtc4BppRgba,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[8]); // 4x4 PVRTC 4bpp = 8 bytes
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.False(reader.CanDecodePixels);
        Assert.Equal(PvrFormatId.Pvrtc4BppRgba, reader.Pvr.FormatId);
    }

    [Fact]
    public async Task ReadFramesAsync_Throws_When_Surface_Is_Undecodable()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Astc4x4,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[16]); // 4x4 ASTC = 16 bytes
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        await Assert.ThrowsAsync<NotSupportedException>(async () =>
        {
            await foreach (var _ in reader.ReadFramesAsync()) { }
        });
    }

    [Fact]
    public void Big_Endian_File_Is_Recognised()
    {
        var b = new TestPvrBuilder
        {
            LittleEndian = false,
            PixelFormatWord = (ulong)PvrFormatId.Bc1,
            Width = 4, Height = 4,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        using var ms = new MemoryStream(bytes, writable: false);
        using var reader = PvrReader.Open(ms);
        Assert.False(reader.Pvr.LittleEndian);
        Assert.Equal(BcnFormat.Bc1, reader.Pvr.Bcn);
        Assert.Equal(4, reader.Info.Width);
    }

    [Fact]
    public void Detector_Recognises_Pvr_Magic()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = (ulong)PvrFormatId.Bc1,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        var fmt = ImageFormatDetector.Detect(bytes);
        Assert.Equal(ImageFormat.Pvr, fmt);
    }

    [Fact]
    public void Detector_Recognises_BigEndian_Pvr_Magic()
    {
        var b = new TestPvrBuilder
        {
            LittleEndian = false,
            PixelFormatWord = (ulong)PvrFormatId.Bc1,
        };
        b.Payloads.Add(new byte[8]);
        var bytes = b.Build();
        var fmt = ImageFormatDetector.Detect(bytes);
        Assert.Equal(ImageFormat.Pvr, fmt);
    }

    [Fact]
    public void Rejects_Out_Of_Bounds_Metadata()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 2, Height = 2,
        };
        b.Payloads.Add(new byte[2 * 2 * 4]);
        var bytes = b.Build();
        // Patch metaDataSize at offset 48 to huge value.
        bytes[48] = 0xFF; bytes[49] = 0xFF; bytes[50] = 0xFF; bytes[51] = 0xFF;
        using var ms = new MemoryStream(bytes, writable: false);
        Assert.Throws<ImageFormatException>(() => PvrReader.Open(ms));
    }

    [Fact]
    public void Open_Null_Stream_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => PvrReader.Open((Stream)null!));
    }

    [Fact]
    public void Open_Null_Path_Throws_ArgumentNullException()
    {
        Assert.Throws<ArgumentNullException>(() => PvrReader.Open((string)null!));
    }

    [Fact]
    public void Open_With_OwnsStream_True_Disposes_Underlying_Stream()
    {
        byte[] bytes = MinimalRgbaPvr();
        var ms = new MemoryStream(bytes, writable: false);
        using (var r = PvrReader.Open(ms, ownsStream: true))
        {
            Assert.Equal(ImageFormat.Pvr, r.Format);
        }
        Assert.Throws<ObjectDisposedException>(() => ms.ReadByte());
    }

    [Fact]
    public void Open_With_OwnsStream_False_Leaves_Stream_Open()
    {
        byte[] bytes = MinimalRgbaPvr();
        using var ms = new MemoryStream(bytes, writable: false);
        using (var r = PvrReader.Open(ms))
        {
            Assert.Equal(ImageFormat.Pvr, r.Format);
        }
        ms.Position = 0;
        Assert.Equal((byte)'P', (byte)ms.ReadByte());
    }

    [Fact]
    public void Double_Dispose_Is_Idempotent()
    {
        byte[] bytes = MinimalRgbaPvr();
        var r = PvrReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        r.Dispose();
        r.Dispose();
    }

    [Fact]
    public void Info_Format_Equals_Pvr()
    {
        byte[] bytes = MinimalRgbaPvr();
        using var r = PvrReader.Open(new MemoryStream(bytes, writable: false));
        Assert.Equal(ImageFormat.Pvr, r.Info.Format);
    }

    private static byte[] MinimalRgbaPvr()
    {
        var b = new TestPvrBuilder
        {
            PixelFormatWord = TestPvrBuilder.PackChannelDescriptor("rgba", new byte[] { 8, 8, 8, 8 }),
            Width = 2, Height = 2,
        };
        b.Payloads.Add(new byte[2 * 2 * 4]);
        return b.Build();
    }

    [Fact]
    public async Task ReadFramesAsync_Honors_PreCancelled_Token()
    {
        byte[] bytes = MinimalRgbaPvr();
        using var r = PvrReader.Open(new MemoryStream(bytes, writable: false), ownsStream: true);
        if (!r.CanDecodePixels) return;
        using var cts = new CancellationTokenSource();
        cts.Cancel();
        await Assert.ThrowsAsync<OperationCanceledException>(async () =>
        {
            await foreach (var f in r.ReadFramesAsync(cts.Token))
            {
                f.Dispose();
            }
        });
    }
}
