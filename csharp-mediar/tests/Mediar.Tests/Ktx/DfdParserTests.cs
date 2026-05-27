using Mediar.Imaging.Ktx;
using Xunit;

namespace Mediar.Tests.Ktx;

public class DfdParserTests
{
    [Fact]
    public void Parse_Returns_Null_For_Empty_Section()
    {
        var bytes = new byte[64];
        var dfd = DfdParser.Parse(bytes, 0, 0);
        Assert.Null(dfd);
    }

    [Fact]
    public void Parse_Returns_Null_For_Length_Smaller_Than_Header()
    {
        var bytes = new byte[64];
        var dfd = DfdParser.Parse(bytes, 0, 3);
        Assert.Null(dfd);
    }

    [Fact]
    public void Parse_Returns_Null_When_Section_Exceeds_Buffer()
    {
        var bytes = new byte[10];
        var dfd = DfdParser.Parse(bytes, 5, 32);
        Assert.Null(dfd);
    }

    [Fact]
    public void Parse_Rgba8_Srgb_Decodes_All_Basic_Fields()
    {
        // R8G8B8A8_SRGB layout: 4 channels, 8 bits each, byte-packed, sRGB transfer.
        const byte ChannelR = 0;
        const byte ChannelG = 1;
        const byte ChannelB = 2;
        const byte ChannelA = 15;

        var builder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.SRgb,
            Flags = KhrDfdFlags.None,
            TexelBlockDimensions = new byte[] { 1, 1, 1, 1 },
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        // Alpha is forced linear (high bit 0x10) even when transfer is sRGB,
        // per KDF 1.4. Other channels inherit the block's transfer function.
        builder.AddSample(0, 8, ChannelR, sampleLower: 0, sampleUpper: 255);
        builder.AddSample(8, 8, ChannelG, sampleLower: 0, sampleUpper: 255);
        builder.AddSample(16, 8, ChannelB, sampleLower: 0, sampleUpper: 255);
        builder.AddSample(24, 8, (byte)(ChannelA | 0x10), sampleLower: 0, sampleUpper: 255);

        var bytes = builder.Build();
        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);

        Assert.NotNull(parsed);
        Assert.Equal((uint)bytes.Length, parsed!.TotalSize);
        Assert.Single(parsed.Blocks);

        var basic = parsed.Basic;
        Assert.NotNull(basic);
        Assert.True(basic!.IsKhronosBasic);
        Assert.Equal(KhrColorModel.Rgbsda, basic.ColorModel);
        Assert.Equal(KhrColorPrimaries.Bt709, basic.ColorPrimaries);
        Assert.Equal(KhrTransferFunction.SRgb, basic.TransferFunction);
        Assert.Equal(KhrDfdFlags.None, basic.Flags);
        Assert.Equal(4, basic.TexelBlockDimensions.Count);
        Assert.All(basic.TexelBlockDimensions, d => Assert.Equal(1, d));
        Assert.Equal(8, basic.BytesPlanes.Count);
        Assert.Equal(4, basic.BytesPlanes[0]);
        Assert.Equal(4, basic.Samples.Count);

        for (int i = 0; i < 4; i++)
        {
            Assert.Equal((ushort)(i * 8), basic.Samples[i].BitOffset);
            Assert.Equal(8, basic.Samples[i].BitLength);
            Assert.Equal(0u, basic.Samples[i].SampleLower);
            Assert.Equal(255u, basic.Samples[i].SampleUpper);
        }

        Assert.Equal(ChannelR, basic.Samples[0].ChannelId);
        Assert.Equal(ChannelG, basic.Samples[1].ChannelId);
        Assert.Equal(ChannelB, basic.Samples[2].ChannelId);
        Assert.Equal(ChannelA, basic.Samples[3].ChannelId);
        Assert.True(basic.Samples[3].IsLinear);
        Assert.False(basic.Samples[0].IsLinear);
    }

    [Fact]
    public void Parse_Bc1_Decodes_Block_Compressed_Layout()
    {
        // BC1 4x4 block, 64 bits = 8 bytes per block.
        var builder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Bc1A,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.SRgb,
            TexelBlockDimensions = new byte[] { 4, 4, 1, 1 },
            BytesPlanes = new byte[] { 8, 0, 0, 0, 0, 0, 0, 0 },
        };
        // BC1: 2 samples - one for colour bits (32) and one for index bits (32).
        builder.AddSample(0, 64, 0, sampleLower: 0, sampleUpper: 0xFFFFFFFFu);

        var bytes = builder.Build();
        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);

        Assert.NotNull(parsed);
        var basic = parsed!.Basic;
        Assert.NotNull(basic);
        Assert.Equal(KhrColorModel.Bc1A, basic!.ColorModel);
        Assert.Equal(4, basic.TexelBlockDimensions[0]);
        Assert.Equal(4, basic.TexelBlockDimensions[1]);
        Assert.Equal(8, basic.BytesPlanes[0]);
        Assert.Single(basic.Samples);
        Assert.Equal(64, basic.Samples[0].BitLength);
        Assert.Equal(0xFFFFFFFFu, basic.Samples[0].SampleUpper);
    }

    [Fact]
    public void Parse_Records_AlphaPremultiplied_Flag()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.Linear,
            Flags = KhrDfdFlags.AlphaPremultiplied,
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        builder.AddSample(0, 8, 0);
        builder.AddSample(8, 8, 1);
        builder.AddSample(16, 8, 2);
        builder.AddSample(24, 8, 15);

        var bytes = builder.Build();
        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);

        Assert.NotNull(parsed);
        Assert.Equal(KhrDfdFlags.AlphaPremultiplied, parsed!.Basic!.Flags);
    }

    [Fact]
    public void Parse_Returns_Null_For_Truncated_Block()
    {
        // Header claims a block extending past the section.
        var bytes = new byte[16];
        // dfdTotalSize = 16 (header + claimed block)
        bytes[0] = 16;
        // block header at offset 4: vendorId=0, type=0, version=2, size=100
        // -> 100 > remaining 12 bytes -> reject.
        bytes[4] = bytes[5] = bytes[6] = bytes[7] = 0;
        bytes[8] = 2; bytes[9] = 0;     // version
        bytes[10] = 100; bytes[11] = 0; // descriptorBlockSize

        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Null(parsed);
    }

    [Fact]
    public void Parse_Returns_Null_For_BlockSize_Below_Header()
    {
        // Block size 4 < 8 (vendor word + version word) -> reject.
        var bytes = new byte[16];
        bytes[0] = 16;
        bytes[4] = bytes[5] = bytes[6] = bytes[7] = 0;
        bytes[8] = 2; bytes[9] = 0;
        bytes[10] = 4; bytes[11] = 0;

        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Null(parsed);
    }

    [Fact]
    public void Parse_Returns_Null_For_Sample_Bytes_Not_Multiple_Of_16()
    {
        // Basic block size = 24 (header) + 8 (partial sample) -> reject.
        var bytes = new byte[36];
        bytes[0] = 36;
        bytes[4] = bytes[5] = bytes[6] = bytes[7] = 0;
        bytes[8] = 2; bytes[9] = 0;
        bytes[10] = 32; bytes[11] = 0;
        // ... rest of header padded with zeros is fine

        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Null(parsed);
    }

    [Fact]
    public void Parse_Supports_Multiple_Blocks_Including_Unknown_Vendor()
    {
        // First block: Khronos basic, no samples (size 24).
        // Second block: unknown vendor (vendorId=1), size 16, payload ignored.
        var basicBuilder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        byte[] basic = basicBuilder.Build();

        const int extraSize = 16;
        // Layout: combined dfdTotalSize header + basic block (sans its 4-byte header word) + extra.
        // basicBuilder.Build() already encodes its own dfdTotalSize at byte 0, so we strip
        // the first 4 bytes (total-size word) and rewrite our combined size.
        byte[] basicBody = basic.AsSpan(4).ToArray();
        int totalSize = 4 + basicBody.Length + extraSize;
        byte[] combined = new byte[totalSize];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(combined, (uint)totalSize);
        Buffer.BlockCopy(basicBody, 0, combined, 4, basicBody.Length);

        // Append unknown-vendor block: vendorId=1, type=2, version=1, size=16.
        int extraOffset = 4 + basicBody.Length;
        // word0: vendorId in low 17 bits, descriptorType in bits 17..31.
        uint word0 = 1u | (2u << 17);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(combined.AsSpan(extraOffset), word0);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(
            combined.AsSpan(extraOffset + 4), 1);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(
            combined.AsSpan(extraOffset + 6), (ushort)extraSize);

        var parsed = DfdParser.Parse(combined, 0, combined.Length);

        Assert.NotNull(parsed);
        Assert.Equal(2, parsed!.Blocks.Count);
        Assert.True(parsed.Blocks[0].IsKhronosBasic);
        Assert.False(parsed.Blocks[1].IsKhronosBasic);
        Assert.Equal((ushort)1, parsed.Blocks[1].VendorId);
        Assert.Equal((ushort)2, parsed.Blocks[1].DescriptorType);
        Assert.Equal((ushort)extraSize, parsed.Blocks[1].DescriptorBlockSize);
        Assert.Empty(parsed.Blocks[1].Samples);
    }

    [Fact]
    public void Parse_Returns_Null_When_TotalSize_Larger_Than_Section()
    {
        var bytes = new byte[8];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(bytes, 99);
        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);
        Assert.Null(parsed);
    }

    [Fact]
    public void Parse_Float_Channel_Detects_Float_Qualifier()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            TransferFunction = KhrTransferFunction.Linear,
            BytesPlanes = new byte[] { 12, 0, 0, 0, 0, 0, 0, 0 },
        };
        builder.AddSample(0, 32, (byte)(0 | 0x80)); // R, float qualifier
        builder.AddSample(32, 32, (byte)(1 | 0x80));
        builder.AddSample(64, 32, (byte)(2 | 0x80));

        var bytes = builder.Build();
        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);

        Assert.NotNull(parsed);
        var basic = parsed!.Basic!;
        Assert.True(basic.Samples[0].IsFloat);
        Assert.Equal(0, basic.Samples[0].ChannelId);
    }

    [Fact]
    public void Parse_Captures_Vendor_Block_RawBytes_For_Inspection()
    {
        var basicBuilder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.Linear,
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        byte[] basic = basicBuilder.Build();

        // Vendor block: vendorId=42, descriptorType=3, size=16. Payload (8 bytes)
        // carries a distinctive sentinel sequence we can verify round-trips.
        byte[] payload = new byte[] { 0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE };
        const int extraSize = 16;
        byte[] basicBody = basic.AsSpan(4).ToArray();
        int totalSize = 4 + basicBody.Length + extraSize;
        byte[] combined = new byte[totalSize];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(combined, (uint)totalSize);
        Buffer.BlockCopy(basicBody, 0, combined, 4, basicBody.Length);

        int extraOffset = 4 + basicBody.Length;
        uint word0 = 42u | (3u << 17);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(combined.AsSpan(extraOffset), word0);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(
            combined.AsSpan(extraOffset + 4), 1);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(
            combined.AsSpan(extraOffset + 6), (ushort)extraSize);
        Buffer.BlockCopy(payload, 0, combined, extraOffset + 8, payload.Length);

        var parsed = DfdParser.Parse(combined, 0, combined.Length);

        Assert.NotNull(parsed);
        Assert.Equal(2, parsed!.Blocks.Count);
        var vendor = parsed.Blocks[1];
        Assert.False(vendor.IsKhronosBasic);
        Assert.True(vendor.IsVendorExtension);
        Assert.Equal((ushort)42, vendor.VendorId);
        Assert.Equal((ushort)3, vendor.DescriptorType);
        Assert.Equal(payload.Length, vendor.RawBytes.Length);
        Assert.Equal(payload, vendor.RawBytes.ToArray());
    }

    [Fact]
    public void Parse_Captures_Basic_Block_RawBytes_For_Roundtrip()
    {
        var builder = new TestKtxDfdBuilder
        {
            ColorModel = KhrColorModel.Rgbsda,
            ColorPrimaries = KhrColorPrimaries.Bt709,
            TransferFunction = KhrTransferFunction.SRgb,
            BytesPlanes = new byte[] { 4, 0, 0, 0, 0, 0, 0, 0 },
        };
        builder.AddSample(0, 8, 0, 0, 255);
        builder.AddSample(8, 8, 1, 0, 255);
        builder.AddSample(16, 8, 2, 0, 255);
        builder.AddSample(24, 8, 15, 0, 255);

        var bytes = builder.Build();
        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);

        Assert.NotNull(parsed);
        var basic = parsed!.Basic!;
        Assert.True(basic.IsKhronosBasic);
        Assert.False(basic.IsVendorExtension);
        // Basic block payload = blockSize - 8 (the 4+4 byte header words).
        Assert.Equal(basic.DescriptorBlockSize - 8, basic.RawBytes.Length);
        Assert.True(basic.RawBytes.Length > 0);
    }

    [Fact]
    public void Parse_Vendor_Block_Without_Payload_Has_Empty_RawBytes()
    {
        // Minimum-size vendor block: header only (8 bytes).
        const int blockSize = 8;
        const int totalSize = 4 + blockSize;
        byte[] bytes = new byte[totalSize];
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(bytes, totalSize);
        uint word0 = 7u | (5u << 17);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(4), word0);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(bytes.AsSpan(8), 1);
        System.Buffers.Binary.BinaryPrimitives.WriteUInt16LittleEndian(bytes.AsSpan(10), blockSize);

        var parsed = DfdParser.Parse(bytes, 0, bytes.Length);

        Assert.NotNull(parsed);
        Assert.Single(parsed!.Blocks);
        var vendor = parsed.Blocks[0];
        Assert.True(vendor.IsVendorExtension);
        Assert.Equal(0, vendor.RawBytes.Length);
    }
}
