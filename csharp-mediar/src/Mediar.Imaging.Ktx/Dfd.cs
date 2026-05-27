using System.Buffers.Binary;

namespace Mediar.Imaging.Ktx;

/// <summary>
/// Khronos Data Format colour model values (KDF 1.4 § 19.3.1).
/// Identifies which colour-space interpretation applies to the channel
/// samples (RGB vs YUV vs HSL vs the various block-compressed families).
/// </summary>
public enum KhrColorModel : byte
{
    Unspecified = 0,
    Rgbsda = 1,
    Yuvsda = 2,
    Yiqsda = 3,
    Labsda = 4,
    Cmyka = 5,
    Xyzw = 6,
    HsvaAng = 7,
    HslaAng = 8,
    HsvaHex = 9,
    HslaHex = 10,
    YcgcoA = 11,
    Yccbccrc = 12,
    Ictcp = 13,
    CieXyz = 14,
    CieXyy = 15,
    Bc1A = 128,
    Bc2 = 129,
    Bc3 = 130,
    Bc4 = 131,
    Bc5 = 132,
    Bc6H = 133,
    Bc7 = 134,
    Etc1 = 160,
    Etc2 = 161,
    Astc = 162,
    Etc1S = 163,
    Pvrtc = 164,
    Pvrtc2 = 165,
    Uastc = 166,
}

/// <summary>
/// Khronos Data Format colour primaries values (KDF 1.4 § 19.3.2).
/// Identifies the chromaticity coordinates of the colour primaries.
/// </summary>
public enum KhrColorPrimaries : byte
{
    Unspecified = 0,

    /// <summary>BT.709 / sRGB primaries (D65 white point).</summary>
    Bt709 = 1,

    /// <summary>BT.601 PAL/SECAM primaries.</summary>
    Bt601Ebu = 2,

    /// <summary>BT.601 NTSC primaries.</summary>
    Bt601Smpte = 3,

    /// <summary>BT.2020 / UHDTV primaries (HDR).</summary>
    Bt2020 = 4,

    CieXyz = 5,
    Aces = 6,
    Acescc = 7,
    Ntsc1953 = 8,
    Pal525 = 9,
    DisplayP3 = 10,
    AdobeRgb = 11,
}

/// <summary>
/// Khronos Data Format transfer-function values (KDF 1.4 § 19.3.3).
/// Identifies how stored sample values relate to linear scene values.
/// </summary>
public enum KhrTransferFunction : byte
{
    Unspecified = 0,
    Linear = 1,
    SRgb = 2,
    Itu = 3,
    Ntsc = 4,
    Slog = 5,
    Slog2 = 6,
    Bt1886 = 7,
    HlgOetf = 8,
    HlgEotf = 9,
    PqEotf = 10,
    PqOetf = 11,
    DciP3 = 12,
    PalOetf = 13,
    Pal625Eotf = 14,
    St240 = 15,
    AcesCc = 16,
    AcesCct = 17,
    AdobeRgb = 18,
}

/// <summary>Khronos Data Format flags (KDF 1.4 § 19.3.4).</summary>
[Flags]
[System.Diagnostics.CodeAnalysis.SuppressMessage("Naming", "CA1711:Identifiers should not have incorrect suffix",
    Justification = "The [Flags] suffix is the spec-defined name for the KHR_DF flags enum.")]
public enum KhrDfdFlags : byte
{
    None = 0,

    /// <summary>Alpha is premultiplied into the colour channels.</summary>
    AlphaPremultiplied = 1,
}

/// <summary>
/// Single per-sample descriptor within a Khronos Basic Data Format block.
/// Each <see cref="KhrDfdSample"/> describes one channel of one plane
/// (e.g. R / G / B / A for an RGBA8 surface, or the single Y sample in a
/// luminance-only surface).
/// </summary>
public sealed record KhrDfdSample
{
    /// <summary>Bit offset of this sample within the texel block.</summary>
    public required ushort BitOffset { get; init; }

    /// <summary>
    /// Number of bits used by this sample (1..256). Stored on disk as
    /// <c>BitLength - 1</c>; this value is the spec-meaningful bit count
    /// (i.e. 8 for an 8-bit channel, never 7).
    /// </summary>
    public required ushort BitLength { get; init; }

    /// <summary>
    /// Low 4 bits identify the channel within the colour model
    /// (RGB: 0=R, 1=G, 2=B, 15=A; YUV: 0=Y, 1=U/Cb, 2=V/Cr, 15=A;
    /// block-compressed: model-specific). High 4 bits are qualifier
    /// flags (KHR_DF_SAMPLE_DATATYPE_LINEAR/EXPONENT/SIGNED/FLOAT).
    /// </summary>
    public required byte ChannelType { get; init; }

    /// <summary>Sample positions (X, Y, Z, W) within the texel block in 1/256 of a texel.</summary>
    public required byte SamplePositionX { get; init; }
    public required byte SamplePositionY { get; init; }
    public required byte SamplePositionZ { get; init; }
    public required byte SamplePositionW { get; init; }

    /// <summary>
    /// Lower bound of the sample's representable value range
    /// (e.g. 0 for UNORM/UINT, INT32_MIN for SINT/SNORM, the
    /// IEEE-754 float bit pattern of the lower clamp for FLOAT).
    /// </summary>
    public required uint SampleLower { get; init; }

    /// <summary>Upper bound of the sample's representable value range.</summary>
    public required uint SampleUpper { get; init; }

    /// <summary>True when the high 4 bits of <see cref="ChannelType"/> mark this sample as a floating-point sample.</summary>
    public bool IsFloat => (ChannelType & 0x80) != 0;

    /// <summary>True when the high 4 bits of <see cref="ChannelType"/> mark this sample as a signed sample.</summary>
    public bool IsSigned => (ChannelType & 0x40) != 0;

    /// <summary>True when the high 4 bits of <see cref="ChannelType"/> mark this sample as an exponent sample (HDR).</summary>
    public bool IsExponent => (ChannelType & 0x20) != 0;

    /// <summary>True when the high 4 bits of <see cref="ChannelType"/> mark this sample as a linear sample (sRGB exempt).</summary>
    public bool IsLinear => (ChannelType & 0x10) != 0;

    /// <summary>Channel ID stripped of the qualifier bits (low 4 bits of <see cref="ChannelType"/>).</summary>
    public byte ChannelId => (byte)(ChannelType & 0x0F);
}

/// <summary>
/// One Data Format Descriptor Block (DFB). A DFD section may contain
/// multiple blocks; the first one is conventionally the Khronos Basic
/// Data Format block (vendorId = 0, descriptorType = 0).
/// </summary>
public sealed record KhrDfdBlock
{
    /// <summary>Vendor identifier (0 = Khronos).</summary>
    public required ushort VendorId { get; init; }

    /// <summary>Descriptor type (0 = Basic Data Format for Khronos vendor).</summary>
    public required ushort DescriptorType { get; init; }

    /// <summary>Descriptor version (1, 2, or 3 for Khronos basic).</summary>
    public required ushort VersionNumber { get; init; }

    /// <summary>Total size of this descriptor block in bytes (including the 4-byte header word).</summary>
    public required ushort DescriptorBlockSize { get; init; }

    /// <summary>Colour model identifier (only meaningful for the Khronos Basic Data Format block).</summary>
    public KhrColorModel ColorModel { get; init; }

    /// <summary>Colour primaries identifier.</summary>
    public KhrColorPrimaries ColorPrimaries { get; init; }

    /// <summary>Transfer function identifier (Linear vs sRGB etc).</summary>
    public KhrTransferFunction TransferFunction { get; init; }

    /// <summary>Flags (KHR_DF_FLAG_ALPHA_PREMULTIPLIED etc).</summary>
    public KhrDfdFlags Flags { get; init; }

    /// <summary>
    /// Texel block dimensions [d0, d1, d2, d3] in texels. Stored on disk
    /// as the dimension minus 1; this value is the spec-meaningful
    /// dimension (i.e. 4 for a BC1 4×4 block, never 3). A value of 1
    /// indicates the dimension does not apply (e.g. d3 for 3D textures).
    /// </summary>
    public IReadOnlyList<byte> TexelBlockDimensions { get; init; } = Array.Empty<byte>();

    /// <summary>
    /// Bytes-per-plane for each of the 8 planes (0..7). For tightly
    /// packed formats only <c>BytesPlanes[0]</c> is non-zero and gives
    /// the size of the texel block in bytes.
    /// </summary>
    public IReadOnlyList<byte> BytesPlanes { get; init; } = Array.Empty<byte>();

    /// <summary>Per-sample descriptors (one entry per channel-of-plane).</summary>
    public IReadOnlyList<KhrDfdSample> Samples { get; init; } = Array.Empty<KhrDfdSample>();

    /// <summary>True when this block is the Khronos Basic Data Format block (vendor 0, type 0).</summary>
    public bool IsKhronosBasic => VendorId == 0 && DescriptorType == 0;
}

/// <summary>
/// Parsed KTX 2.x Data Format Descriptor (DFD) section. Holds the
/// total size word plus zero or more <see cref="KhrDfdBlock"/> entries.
/// </summary>
public sealed record KtxDfd
{
    /// <summary>Total size of the DFD section in bytes (header u32 + all blocks).</summary>
    public required uint TotalSize { get; init; }

    /// <summary>The parsed Data Format Descriptor Blocks.</summary>
    public IReadOnlyList<KhrDfdBlock> Blocks { get; init; } = Array.Empty<KhrDfdBlock>();

    /// <summary>
    /// Convenience accessor for the first Khronos Basic Data Format block,
    /// or <c>null</c> when the DFD does not contain one.
    /// </summary>
    public KhrDfdBlock? Basic
    {
        get
        {
            for (int i = 0; i < Blocks.Count; i++)
            {
                if (Blocks[i].IsKhronosBasic) return Blocks[i];
            }
            return null;
        }
    }
}

/// <summary>
/// Parser for the KTX 2.x Data Format Descriptor section. Walks the
/// <c>dfdTotalSize + DFB[]</c> layout per Khronos KDF 1.4 § 19 and
/// surfaces the strongly-typed <see cref="KtxDfd"/> structure.
/// </summary>
/// <remarks>
/// The parser is intentionally tolerant of malformed inputs: any structural
/// problem (truncated section, mismatched total size, block size larger
/// than the containing section, malformed Basic Data Format block) causes
/// the method to return <c>null</c> rather than throwing, so a broken DFD
/// never prevents an otherwise-decodable texture from being opened.
/// </remarks>
public static class DfdParser
{
    private const int HeaderSize = 4;
    private const int BasicBlockHeaderSize = 24;
    private const int BasicBlockSampleSize = 16;

    /// <summary>
    /// Parses the DFD section starting at <paramref name="offset"/> in
    /// <paramref name="bytes"/> consuming <paramref name="length"/> bytes.
    /// Returns <c>null</c> for an absent (<c>length == 0</c>) or
    /// structurally invalid section.
    /// </summary>
    public static KtxDfd? Parse(ReadOnlySpan<byte> bytes, int offset, int length)
    {
        if (length == 0) return null;
        if (offset < 0 || length < HeaderSize) return null;
        if ((long)offset + length > bytes.Length) return null;

        var section = bytes.Slice(offset, length);
        uint totalSize = BinaryPrimitives.ReadUInt32LittleEndian(section);
        if (totalSize > section.Length || totalSize < HeaderSize) return null;

        var blocks = new List<KhrDfdBlock>(1);
        int cursor = HeaderSize;
        // Use the declared totalSize when smaller than the supplied length, so
        // trailing padding bytes do not get parsed as a phantom block.
        int endExclusive = (int)totalSize;
        while (cursor + 4 <= endExclusive)
        {
            uint word0 = BinaryPrimitives.ReadUInt32LittleEndian(section.Slice(cursor));
            ushort vendorId = (ushort)(word0 & 0x1FFFFu);
            ushort descriptorType = (ushort)((word0 >> 17) & 0x7FFFu);

            if (cursor + 8 > endExclusive) return null;
            ushort versionNumber = BinaryPrimitives.ReadUInt16LittleEndian(section.Slice(cursor + 4));
            ushort blockSize = BinaryPrimitives.ReadUInt16LittleEndian(section.Slice(cursor + 6));
            if (blockSize < 8) return null;
            if (cursor + blockSize > endExclusive) return null;

            var blockSpan = section.Slice(cursor, blockSize);
            KhrDfdBlock parsed;
            if (vendorId == 0 && descriptorType == 0)
            {
                var maybeBasic = ParseBasicBlock(blockSpan, vendorId, descriptorType, versionNumber, blockSize);
                if (maybeBasic is null) return null;
                parsed = maybeBasic;
            }
            else
            {
                // Unknown vendor/type: record the header without claiming to decode payload.
                parsed = new KhrDfdBlock
                {
                    VendorId = vendorId,
                    DescriptorType = descriptorType,
                    VersionNumber = versionNumber,
                    DescriptorBlockSize = blockSize,
                };
            }

            blocks.Add(parsed);
            cursor += blockSize;
        }

        return new KtxDfd
        {
            TotalSize = totalSize,
            Blocks = blocks,
        };
    }

    private static KhrDfdBlock? ParseBasicBlock(
        ReadOnlySpan<byte> block,
        ushort vendorId,
        ushort descriptorType,
        ushort versionNumber,
        ushort blockSize)
    {
        if (blockSize < BasicBlockHeaderSize) return null;
        int sampleBytes = blockSize - BasicBlockHeaderSize;
        if (sampleBytes % BasicBlockSampleSize != 0) return null;
        int sampleCount = sampleBytes / BasicBlockSampleSize;

        byte colorModel = block[8];
        byte colorPrimaries = block[9];
        byte transferFunction = block[10];
        byte flags = block[11];

        var texelDims = new byte[4];
        for (int i = 0; i < 4; i++)
        {
            // Stored as (dimension - 1) per spec; the spec-meaningful
            // dimension is therefore stored + 1.
            texelDims[i] = (byte)(block[12 + i] + 1);
        }

        var bytesPlanes = new byte[8];
        for (int i = 0; i < 8; i++) bytesPlanes[i] = block[16 + i];

        var samples = new KhrDfdSample[sampleCount];
        for (int s = 0; s < sampleCount; s++)
        {
            int o = BasicBlockHeaderSize + s * BasicBlockSampleSize;
            ushort bitOffset = BinaryPrimitives.ReadUInt16LittleEndian(block.Slice(o));
            byte bitLengthMinus1 = block[o + 2];
            byte channelType = block[o + 3];
            byte sx = block[o + 4];
            byte sy = block[o + 5];
            byte sz = block[o + 6];
            byte sw = block[o + 7];
            uint sampleLower = BinaryPrimitives.ReadUInt32LittleEndian(block.Slice(o + 8));
            uint sampleUpper = BinaryPrimitives.ReadUInt32LittleEndian(block.Slice(o + 12));

            samples[s] = new KhrDfdSample
            {
                BitOffset = bitOffset,
                BitLength = (ushort)(bitLengthMinus1 + 1),
                ChannelType = channelType,
                SamplePositionX = sx,
                SamplePositionY = sy,
                SamplePositionZ = sz,
                SamplePositionW = sw,
                SampleLower = sampleLower,
                SampleUpper = sampleUpper,
            };
        }

        return new KhrDfdBlock
        {
            VendorId = vendorId,
            DescriptorType = descriptorType,
            VersionNumber = versionNumber,
            DescriptorBlockSize = blockSize,
            ColorModel = (KhrColorModel)colorModel,
            ColorPrimaries = (KhrColorPrimaries)colorPrimaries,
            TransferFunction = (KhrTransferFunction)transferFunction,
            Flags = (KhrDfdFlags)flags,
            TexelBlockDimensions = texelDims,
            BytesPlanes = bytesPlanes,
            Samples = samples,
        };
    }
}
