using System.Buffers.Binary;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// One parameter-set array carried inside a <c>hvcC</c> record. Each array
/// groups NAL units of a single <see cref="NalUnitType"/> (VPS = 32,
/// SPS = 33, PPS = 34, prefix SEI = 39, suffix SEI = 40).
/// </summary>
public sealed record HevcParameterSetArray
{
    /// <summary>True when the array carries the complete set of NAL units
    /// of this type for the stream (1-bit <c>array_completeness</c>).</summary>
    public required bool ArrayCompleteness { get; init; }

    /// <summary>6-bit NAL unit type carried by every entry in this array.</summary>
    public required byte NalUnitType { get; init; }

    /// <summary>Raw NAL unit payloads (length-prefixed bytes are stripped).</summary>
    public required ImmutableArray<ImmutableArray<byte>> NalUnits { get; init; }
}

/// <summary>
/// Typed view of the HEIF <c>hvcC</c> property (HEVC Decoder Configuration
/// Record) per ISO/IEC 14496-15 §8.3.3.1.2. The trailing parameter-set
/// arrays (VPS / SPS / PPS / SEI) are preserved verbatim so callers that
/// wish to decode the stream's sequence header can do so without re-reading
/// the file.
/// </summary>
public sealed record HevcCodecConfigurationRecord
{
    /// <summary>Configuration version (currently always 1).</summary>
    public required byte ConfigurationVersion { get; init; }

    /// <summary>2-bit <c>general_profile_space</c>.</summary>
    public required byte GeneralProfileSpace { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c> (false = Main, true = High).</summary>
    public required bool GeneralTierFlag { get; init; }

    /// <summary>5-bit <c>general_profile_idc</c> (1 = Main, 2 = Main 10, 3 = Main Still Picture, ...).</summary>
    public required byte GeneralProfileIdc { get; init; }

    /// <summary>32-bit <c>general_profile_compatibility_flags</c>.</summary>
    public required uint GeneralProfileCompatibilityFlags { get; init; }

    /// <summary>48-bit <c>general_constraint_indicator_flags</c> (stored in the low 48 bits).</summary>
    public required ulong GeneralConstraintIndicatorFlags { get; init; }

    /// <summary>8-bit <c>general_level_idc</c> (level × 30; e.g. 120 = level 4.0).</summary>
    public required byte GeneralLevelIdc { get; init; }

    /// <summary>12-bit <c>min_spatial_segmentation_idc</c>.</summary>
    public required ushort MinSpatialSegmentationIdc { get; init; }

    /// <summary>2-bit <c>parallelismType</c> (0 = unspecified, 1 = slice-based,
    /// 2 = tile-based, 3 = entropy-coded WPP).</summary>
    public required byte ParallelismType { get; init; }

    /// <summary>2-bit <c>chroma_format_idc</c> (0 = monochrome, 1 = 4:2:0, 2 = 4:2:2, 3 = 4:4:4).</summary>
    public required byte ChromaFormatIdc { get; init; }

    /// <summary>3-bit <c>bit_depth_luma_minus8</c> (0 = 8-bit luma).</summary>
    public required byte BitDepthLumaMinus8 { get; init; }

    /// <summary>3-bit <c>bit_depth_chroma_minus8</c> (0 = 8-bit chroma).</summary>
    public required byte BitDepthChromaMinus8 { get; init; }

    /// <summary>16-bit <c>avgFrameRate</c> in 256ths of a frame per second (0 = unspecified).</summary>
    public required ushort AvgFrameRate { get; init; }

    /// <summary>2-bit <c>constantFrameRate</c> (0 = unspecified or variable, 1 = constant, 2 = each sub-layer constant).</summary>
    public required byte ConstantFrameRate { get; init; }

    /// <summary>3-bit <c>numTemporalLayers</c>.</summary>
    public required byte NumTemporalLayers { get; init; }

    /// <summary>1-bit <c>temporalIdNested</c>.</summary>
    public required bool TemporalIdNested { get; init; }

    /// <summary>2-bit <c>lengthSizeMinusOne</c>; NAL unit length-prefix width = value + 1 bytes.</summary>
    public required byte LengthSizeMinusOne { get; init; }

    /// <summary>Parameter-set arrays (VPS / SPS / PPS / SEI) carried in the record.</summary>
    public required ImmutableArray<HevcParameterSetArray> Arrays { get; init; }

    /// <summary>Effective luma bit depth (8..14).</summary>
    public int BitDepthLuma => 8 + BitDepthLumaMinus8;

    /// <summary>Effective chroma bit depth (8..14).</summary>
    public int BitDepthChroma => 8 + BitDepthChromaMinus8;

    /// <summary>NAL unit length-prefix width in bytes (1, 2, or 4).</summary>
    public int NalUnitLengthBytes => LengthSizeMinusOne + 1;

    /// <summary>Chroma subsampling shorthand: "4:0:0", "4:2:0", "4:2:2", "4:4:4".</summary>
    public string ChromaFormat => ChromaFormatIdc switch
    {
        0 => "4:0:0",
        1 => "4:2:0",
        2 => "4:2:2",
        3 => "4:4:4",
        _ => "unknown",
    };

    /// <summary>Decodes a raw <c>hvcC</c> payload (23-byte fixed header + parameter-set arrays).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HevcCodecConfigurationRecord record)
    {
        record = null!;
        if (data.Length < 23) return false;

        byte version = data[0];
        if (version != 1) return false;

        byte b1 = data[1];
        byte profileSpace = (byte)((b1 >> 6) & 0x3);
        bool tierFlag = ((b1 >> 5) & 0x1) == 1;
        byte profileIdc = (byte)(b1 & 0x1F);

        uint compat = BinaryPrimitives.ReadUInt32BigEndian(data.Slice(2, 4));

        // 48-bit constraint flags: 6 bytes from offset 6.
        ulong constraints = 0;
        for (int i = 0; i < 6; i++)
            constraints = (constraints << 8) | data[6 + i];

        byte levelIdc = data[12];

        // 4 reserved bits + 12-bit min_spatial_segmentation_idc.
        ushort minSeg = (ushort)(BinaryPrimitives.ReadUInt16BigEndian(data.Slice(13, 2)) & 0x0FFF);

        byte parallelism = (byte)(data[15] & 0x3);
        byte chromaFormat = (byte)(data[16] & 0x3);
        byte bdLumaM8 = (byte)(data[17] & 0x7);
        byte bdChromaM8 = (byte)(data[18] & 0x7);

        ushort avgFps = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(19, 2));

        byte b21 = data[21];
        byte constantFps = (byte)((b21 >> 6) & 0x3);
        byte numTemporalLayers = (byte)((b21 >> 3) & 0x7);
        bool temporalIdNested = ((b21 >> 2) & 0x1) == 1;
        byte lengthSizeMinusOne = (byte)(b21 & 0x3);

        byte numArrays = data[22];

        var arraysBuilder = ImmutableArray.CreateBuilder<HevcParameterSetArray>(numArrays);
        int pos = 23;
        for (int a = 0; a < numArrays; a++)
        {
            if (pos + 3 > data.Length) return false;
            byte aByte = data[pos++];
            bool arrayCompleteness = ((aByte >> 7) & 0x1) == 1;
            byte nalUnitType = (byte)(aByte & 0x3F);
            ushort numNalus = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(pos, 2));
            pos += 2;

            var nalus = ImmutableArray.CreateBuilder<ImmutableArray<byte>>(numNalus);
            for (int n = 0; n < numNalus; n++)
            {
                if (pos + 2 > data.Length) return false;
                ushort naluLength = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(pos, 2));
                pos += 2;
                if (pos + naluLength > data.Length) return false;
                nalus.Add(ImmutableArray.Create(data.Slice(pos, naluLength)));
                pos += naluLength;
            }

            arraysBuilder.Add(new HevcParameterSetArray
            {
                ArrayCompleteness = arrayCompleteness,
                NalUnitType = nalUnitType,
                NalUnits = nalus.ToImmutable(),
            });
        }

        record = new HevcCodecConfigurationRecord
        {
            ConfigurationVersion = version,
            GeneralProfileSpace = profileSpace,
            GeneralTierFlag = tierFlag,
            GeneralProfileIdc = profileIdc,
            GeneralProfileCompatibilityFlags = compat,
            GeneralConstraintIndicatorFlags = constraints,
            GeneralLevelIdc = levelIdc,
            MinSpatialSegmentationIdc = minSeg,
            ParallelismType = parallelism,
            ChromaFormatIdc = chromaFormat,
            BitDepthLumaMinus8 = bdLumaM8,
            BitDepthChromaMinus8 = bdChromaM8,
            AvgFrameRate = avgFps,
            ConstantFrameRate = constantFps,
            NumTemporalLayers = numTemporalLayers,
            TemporalIdNested = temporalIdNested,
            LengthSizeMinusOne = lengthSizeMinusOne,
            Arrays = arraysBuilder.ToImmutable(),
        };
        return true;
    }
}
