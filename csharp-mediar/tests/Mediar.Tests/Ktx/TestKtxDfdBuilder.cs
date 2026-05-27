using System.Buffers.Binary;
using Mediar.Imaging.Ktx;

namespace Mediar.Tests.Ktx;

/// <summary>
/// Test-only synthesiser for valid KTX 2.x Data Format Descriptor (DFD)
/// byte sequences per Khronos KDF 1.4 § 19. Emits a section consisting
/// of a u32 <c>dfdTotalSize</c> followed by one Khronos Basic Data Format
/// block describing the supplied colour-model / primaries / transfer
/// function / per-channel sample layout.
/// </summary>
internal sealed class TestKtxDfdBuilder
{
    public KhrColorModel ColorModel { get; set; } = KhrColorModel.Rgbsda;
    public KhrColorPrimaries ColorPrimaries { get; set; } = KhrColorPrimaries.Bt709;
    public KhrTransferFunction TransferFunction { get; set; } = KhrTransferFunction.Linear;
    public KhrDfdFlags Flags { get; set; } = KhrDfdFlags.None;
    public byte[] TexelBlockDimensions { get; set; } = new byte[] { 1, 1, 1, 1 };
    public byte[] BytesPlanes { get; set; } = new byte[] { 0, 0, 0, 0, 0, 0, 0, 0 };
    public List<SampleDescriptor> Samples { get; } = new();

    public byte[] Build()
    {
        const int basicHeaderSize = 24;
        const int sampleSize = 16;
        int blockSize = basicHeaderSize + Samples.Count * sampleSize;
        int totalSize = 4 + blockSize;
        var bytes = new byte[totalSize];
        var span = bytes.AsSpan();

        BinaryPrimitives.WriteUInt32LittleEndian(span, (uint)totalSize);
        int cursor = 4;

        // Word 0: vendorId(17) | descriptorType(15)
        // vendorId=0, descriptorType=0 (Khronos Basic).
        BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(cursor), 0u);
        // Word 1: versionNumber(16) | descriptorBlockSize(16)
        BinaryPrimitives.WriteUInt16LittleEndian(span.Slice(cursor + 4), 2);
        BinaryPrimitives.WriteUInt16LittleEndian(span.Slice(cursor + 6), (ushort)blockSize);

        // Word 2: colorModel | colorPrimaries | transferFunction | flags
        span[cursor + 8] = (byte)ColorModel;
        span[cursor + 9] = (byte)ColorPrimaries;
        span[cursor + 10] = (byte)TransferFunction;
        span[cursor + 11] = (byte)Flags;

        // Word 3: texelBlockDimensions [0..3] - stored as (dimension - 1).
        for (int i = 0; i < 4; i++)
        {
            byte declared = i < TexelBlockDimensions.Length ? TexelBlockDimensions[i] : (byte)1;
            span[cursor + 12 + i] = (byte)(declared == 0 ? 0 : declared - 1);
        }

        // Words 4..5: bytesPlanes [0..7]
        for (int i = 0; i < 8; i++)
        {
            span[cursor + 16 + i] = i < BytesPlanes.Length ? BytesPlanes[i] : (byte)0;
        }

        // Samples
        for (int s = 0; s < Samples.Count; s++)
        {
            int o = cursor + basicHeaderSize + s * sampleSize;
            var d = Samples[s];
            BinaryPrimitives.WriteUInt16LittleEndian(span.Slice(o), d.BitOffset);
            span[o + 2] = (byte)(d.BitLength == 0 ? 0 : d.BitLength - 1);
            span[o + 3] = d.ChannelType;
            span[o + 4] = d.PositionX;
            span[o + 5] = d.PositionY;
            span[o + 6] = d.PositionZ;
            span[o + 7] = d.PositionW;
            BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(o + 8), d.SampleLower);
            BinaryPrimitives.WriteUInt32LittleEndian(span.Slice(o + 12), d.SampleUpper);
        }

        return bytes;
    }

    public TestKtxDfdBuilder AddSample(
        ushort bitOffset, byte bitLength, byte channelType,
        uint sampleLower = 0, uint sampleUpper = 0)
    {
        Samples.Add(new SampleDescriptor
        {
            BitOffset = bitOffset,
            BitLength = bitLength,
            ChannelType = channelType,
            SampleLower = sampleLower,
            SampleUpper = sampleUpper,
        });
        return this;
    }

    internal sealed class SampleDescriptor
    {
        public ushort BitOffset { get; set; }
        public byte BitLength { get; set; }
        public byte ChannelType { get; set; }
        public byte PositionX { get; set; }
        public byte PositionY { get; set; }
        public byte PositionZ { get; set; }
        public byte PositionW { get; set; }
        public uint SampleLower { get; set; }
        public uint SampleUpper { get; set; }
    }
}
