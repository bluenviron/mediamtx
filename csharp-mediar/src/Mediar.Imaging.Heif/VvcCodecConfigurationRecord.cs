using System.Buffers.Binary;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// One parameter-set array carried inside a <c>vvcC</c> record. Each array
/// groups NAL units of a single <see cref="NalUnitType"/>. Note that when
/// the NAL unit type is <c>VVC_OPI_NUT</c> (12) or <c>VVC_DCI_NUT</c> (13)
/// the spec mandates exactly one NAL unit per array (no num_nalus field).
/// Other types (VPS = 14, SPS = 15, PPS = 16, prefix APS = 17, suffix APS
/// = 18, picture header = 19, prefix SEI = 23, suffix SEI = 24) carry a
/// 16-bit count.
/// </summary>
public sealed record VvcParameterSetArray
{
    /// <summary>True when the array carries the complete set of NAL units
    /// of this type for the stream (1-bit <c>array_completeness</c>).</summary>
    public required bool ArrayCompleteness { get; init; }

    /// <summary>5-bit NAL unit type carried by every entry in this array.</summary>
    public required byte NalUnitType { get; init; }

    /// <summary>Raw NAL unit payloads (length-prefixed bytes are stripped).</summary>
    public required ImmutableArray<ImmutableArray<byte>> NalUnits { get; init; }
}

/// <summary>
/// Typed view of the VVC Profile/Tier/Level record carried inside a
/// <c>vvcC</c> when <c>ptl_present_flag = 1</c>, per ISO/IEC 14496-15
/// §11.2.4.2.2. The constraint-info bytes and sublayer-level idcs are
/// preserved verbatim so callers can interpret vendor-specific bits.
/// </summary>
public sealed record VvcProfileTierLevelRecord
{
    /// <summary>6-bit <c>num_bytes_constraint_info</c> (≥ 1).</summary>
    public required byte NumBytesConstraintInfo { get; init; }

    /// <summary>7-bit <c>general_profile_idc</c>.</summary>
    public required byte GeneralProfileIdc { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c> (false = Main, true = High).</summary>
    public required bool GeneralTierFlag { get; init; }

    /// <summary>8-bit <c>general_level_idc</c>.</summary>
    public required byte GeneralLevelIdc { get; init; }

    /// <summary>1-bit <c>ptl_frame_only_constraint_flag</c>.</summary>
    public required bool PtlFrameOnlyConstraintFlag { get; init; }

    /// <summary>1-bit <c>ptl_multi_layer_enabled_flag</c>.</summary>
    public required bool PtlMultiLayerEnabledFlag { get; init; }

    /// <summary>Raw constraint-info bytes (top 2 bits of the first byte
    /// — the frame-only / multi-layer flags — are masked out so the
    /// returned bytes contain only the constraint information itself,
    /// MSB-first).</summary>
    public required ImmutableArray<byte> GeneralConstraintInfo { get; init; }

    /// <summary>Sublayer-level idcs for sublayers whose
    /// <c>ptl_sublayer_level_present_flag</c> was set, in order from
    /// highest sublayer (num_sublayers - 2) down to 0.</summary>
    public required ImmutableArray<byte> SublayerLevelIdcs { get; init; }

    /// <summary>List of 32-bit <c>general_sub_profile_idc</c> values.</summary>
    public required ImmutableArray<uint> GeneralSubProfileIdcs { get; init; }
}

/// <summary>
/// Typed view of the HEIF <c>vvcC</c> property (VVC Decoder Configuration
/// Record) per ISO/IEC 14496-15 §11.2.4.2. Bit-packed with an optional
/// <c>ptl_present_flag</c> block carrying the operating-point index,
/// per-layer Profile/Tier/Level record, and picture dimensions, followed
/// by parameter-set arrays (VPS / SPS / PPS / DCI / OPI / APS / SEI).
/// </summary>
public sealed record VvcCodecConfigurationRecord
{
    /// <summary>Configuration version (currently always 1).</summary>
    public required byte ConfigurationVersion { get; init; }

    /// <summary>6-bit <c>lengthSizeMinusOne</c>; NAL unit length-prefix width = value + 1 bytes.</summary>
    public required byte LengthSizeMinusOne { get; init; }

    /// <summary>1-bit <c>ptl_present_flag</c>. When false the operating-point /
    /// PTL / picture-size fields are absent.</summary>
    public required bool PtlPresentFlag { get; init; }

    /// <summary>13-bit <c>ols_idx</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required ushort? OlsIdx { get; init; }

    /// <summary>3-bit <c>num_sublayers</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required byte? NumSublayers { get; init; }

    /// <summary>2-bit <c>constant_frame_rate</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required byte? ConstantFrameRate { get; init; }

    /// <summary>3-bit <c>chroma_format_idc</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required byte? ChromaFormatIdc { get; init; }

    /// <summary>3-bit <c>bit_depth_minus8</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required byte? BitDepthMinus8 { get; init; }

    /// <summary>Inner Profile/Tier/Level record (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required VvcProfileTierLevelRecord? TrackPtl { get; init; }

    /// <summary>16-bit <c>max_picture_width</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required ushort? MaxPictureWidth { get; init; }

    /// <summary>16-bit <c>max_picture_height</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required ushort? MaxPictureHeight { get; init; }

    /// <summary>16-bit <c>avg_frame_rate</c> (null when <see cref="PtlPresentFlag"/> is false).</summary>
    public required ushort? AvgFrameRate { get; init; }

    /// <summary>Parameter-set arrays (DCI / OPI / VPS / SPS / PPS / APS / SEI) carried in the record.</summary>
    public required ImmutableArray<VvcParameterSetArray> Arrays { get; init; }

    /// <summary>NAL unit length-prefix width in bytes (1, 2, or 4).</summary>
    public int NalUnitLengthBytes => LengthSizeMinusOne + 1;

    /// <summary>Effective bit depth (null when PTL is absent).</summary>
    public int? BitDepth => BitDepthMinus8.HasValue ? 8 + BitDepthMinus8.Value : null;

    /// <summary>Chroma subsampling shorthand: "4:0:0", "4:2:0", "4:2:2", "4:4:4", or null when PTL is absent.</summary>
    public string? ChromaFormat => ChromaFormatIdc switch
    {
        0 => "4:0:0",
        1 => "4:2:0",
        2 => "4:2:2",
        3 => "4:4:4",
        _ => null,
    };

    /// <summary>Decodes a raw <c>vvcC</c> payload.</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out VvcCodecConfigurationRecord record)
    {
        record = null!;
        if (data.Length < 3) return false;

        byte version = data[0];
        if (version != 1) return false;

        byte lengthSizeMinusOne = (byte)(data[1] & 0x3F);

        byte b2 = data[2];
        bool ptlPresent = ((b2 >> 5) & 0x1) == 1;

        int pos;
        ushort? olsIdx = null;
        byte? numSublayers = null;
        byte? constantFrameRate = null;
        byte? chromaFormatIdc = null;
        byte? bitDepthMinus8 = null;
        VvcProfileTierLevelRecord? trackPtl = null;
        ushort? maxPicWidth = null;
        ushort? maxPicHeight = null;
        ushort? avgFps = null;

        if (ptlPresent)
        {
            if (data.Length < 6) return false;
            olsIdx = (ushort)(((b2 & 0x1F) << 8) | data[3]);

            byte b4 = data[4];
            numSublayers = (byte)((b4 >> 5) & 0x7);
            constantFrameRate = (byte)((b4 >> 3) & 0x3);
            chromaFormatIdc = (byte)(b4 & 0x7);

            bitDepthMinus8 = (byte)((data[5] >> 5) & 0x7);

            pos = 6;
            if (!TryParsePtl(data, ref pos, numSublayers.Value, out trackPtl)) return false;

            if (pos + 6 > data.Length) return false;
            maxPicWidth = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(pos, 2)); pos += 2;
            maxPicHeight = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(pos, 2)); pos += 2;
            avgFps = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(pos, 2)); pos += 2;
        }
        else
        {
            pos = 3;
        }

        if (pos >= data.Length) return false;
        byte numArrays = data[pos++];

        var arraysBuilder = ImmutableArray.CreateBuilder<VvcParameterSetArray>(numArrays);
        for (int a = 0; a < numArrays; a++)
        {
            if (pos + 1 > data.Length) return false;
            byte aByte = data[pos++];
            bool arrayCompleteness = ((aByte >> 7) & 0x1) == 1;
            byte nalUnitType = (byte)(aByte & 0x1F);

            int numNalus;
            // VVC_OPI_NUT (12) and VVC_DCI_NUT (13) carry exactly one NAL unit
            // and omit the count field per the spec.
            if (nalUnitType == 12 || nalUnitType == 13)
            {
                numNalus = 1;
            }
            else
            {
                if (pos + 2 > data.Length) return false;
                numNalus = BinaryPrimitives.ReadUInt16BigEndian(data.Slice(pos, 2));
                pos += 2;
            }

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

            arraysBuilder.Add(new VvcParameterSetArray
            {
                ArrayCompleteness = arrayCompleteness,
                NalUnitType = nalUnitType,
                NalUnits = nalus.ToImmutable(),
            });
        }

        record = new VvcCodecConfigurationRecord
        {
            ConfigurationVersion = version,
            LengthSizeMinusOne = lengthSizeMinusOne,
            PtlPresentFlag = ptlPresent,
            OlsIdx = olsIdx,
            NumSublayers = numSublayers,
            ConstantFrameRate = constantFrameRate,
            ChromaFormatIdc = chromaFormatIdc,
            BitDepthMinus8 = bitDepthMinus8,
            TrackPtl = trackPtl,
            MaxPictureWidth = maxPicWidth,
            MaxPictureHeight = maxPicHeight,
            AvgFrameRate = avgFps,
            Arrays = arraysBuilder.ToImmutable(),
        };
        return true;
    }

    private static bool TryParsePtl(
        ReadOnlySpan<byte> data,
        ref int pos,
        byte numSublayers,
        out VvcProfileTierLevelRecord ptl)
    {
        ptl = null!;
        if (pos + 3 > data.Length) return false;
        byte b0 = data[pos++];
        byte numBytesConstraintInfo = (byte)(b0 & 0x3F);
        if (numBytesConstraintInfo < 1) return false;

        byte b1 = data[pos++];
        byte generalProfileIdc = (byte)((b1 >> 1) & 0x7F);
        bool generalTierFlag = (b1 & 0x1) == 1;
        byte generalLevelIdc = data[pos++];

        if (pos + numBytesConstraintInfo > data.Length) return false;
        var ciBlock = data.Slice(pos, numBytesConstraintInfo);
        pos += numBytesConstraintInfo;

        bool frameOnly = ((ciBlock[0] >> 7) & 0x1) == 1;
        bool multiLayer = ((ciBlock[0] >> 6) & 0x1) == 1;

        var constraintInfo = ImmutableArray.CreateBuilder<byte>(numBytesConstraintInfo);
        constraintInfo.Add((byte)(ciBlock[0] & 0x3F));
        for (int i = 1; i < numBytesConstraintInfo; i++) constraintInfo.Add(ciBlock[i]);

        // Sublayer level present flags: one bit per (num_sublayers - 1) sublayers
        // when num_sublayers > 1, packed MSB first, padded to a byte boundary.
        bool[] sublayerPresent = new bool[numSublayers];
        if (numSublayers > 1)
        {
            if (pos >= data.Length) return false;
            byte sb = data[pos++];
            for (int i = numSublayers - 2; i >= 0; i--)
            {
                int bitFromTop = (numSublayers - 2 - i);
                bool flag = ((sb >> (7 - bitFromTop)) & 0x1) == 1;
                sublayerPresent[i] = flag;
            }
        }

        var sublayerLevelIdcs = ImmutableArray.CreateBuilder<byte>();
        if (numSublayers > 1)
        {
            for (int i = numSublayers - 2; i >= 0; i--)
            {
                if (sublayerPresent[i])
                {
                    if (pos >= data.Length) return false;
                    sublayerLevelIdcs.Add(data[pos++]);
                }
            }
        }

        if (pos >= data.Length) return false;
        byte numSubProfiles = data[pos++];

        if (pos + 4 * numSubProfiles > data.Length) return false;
        var subProfileIdcs = ImmutableArray.CreateBuilder<uint>(numSubProfiles);
        for (int i = 0; i < numSubProfiles; i++)
        {
            subProfileIdcs.Add(BinaryPrimitives.ReadUInt32BigEndian(data.Slice(pos, 4)));
            pos += 4;
        }

        ptl = new VvcProfileTierLevelRecord
        {
            NumBytesConstraintInfo = numBytesConstraintInfo,
            GeneralProfileIdc = generalProfileIdc,
            GeneralTierFlag = generalTierFlag,
            GeneralLevelIdc = generalLevelIdc,
            PtlFrameOnlyConstraintFlag = frameOnly,
            PtlMultiLayerEnabledFlag = multiLayer,
            GeneralConstraintInfo = constraintInfo.ToImmutable(),
            SublayerLevelIdcs = sublayerLevelIdcs.ToImmutable(),
            GeneralSubProfileIdcs = subProfileIdcs.ToImmutable(),
        };
        return true;
    }
}
