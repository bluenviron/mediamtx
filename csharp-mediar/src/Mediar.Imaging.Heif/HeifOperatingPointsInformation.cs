using System.Buffers.Binary;
using System.Collections.Immutable;
using System.Numerics;

namespace Mediar.Imaging.Heif;

/// <summary>
/// One profile-tier-level entry from a HEIF <c>oinf</c> property,
/// mirroring the per-PTL group of the L-HEVC operating-points-
/// information structure in ISO/IEC 14496-15 section 10.4.3.
/// </summary>
public sealed record HeifLhevcProfileTierLevel
{
    /// <summary>2-bit <c>general_profile_space</c>.</summary>
    public required byte ProfileSpace { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c>.</summary>
    public required bool TierFlag { get; init; }

    /// <summary>5-bit <c>general_profile_idc</c>.</summary>
    public required byte ProfileIdc { get; init; }

    /// <summary>32-bit <c>general_profile_compatibility_flags</c>.</summary>
    public required uint ProfileCompatibilityFlags { get; init; }

    /// <summary>48-bit <c>general_constraint_indicator_flags</c>,
    /// right-justified in the low 48 bits of a 64-bit container.</summary>
    public required ulong ConstraintIndicatorFlags { get; init; }

    /// <summary>8-bit <c>general_level_idc</c>.</summary>
    public required byte LevelIdc { get; init; }
}

/// <summary>One layer inside an <see cref="HeifLhevcOperatingPoint"/>.</summary>
public sealed record HeifLhevcOpLayer
{
    /// <summary>Index into <see cref="HeifOperatingPointsInformation.ProfileTierLevels"/>.</summary>
    public required byte PtlIndex { get; init; }

    /// <summary>6-bit layer identifier in the HEVC bitstream.</summary>
    public required byte LayerId { get; init; }

    /// <summary>True when this layer is an output layer of the operating point.</summary>
    public required bool IsOutputLayer { get; init; }

    /// <summary>True when this layer is an alternate output layer.</summary>
    public required bool IsAlternateOutputLayer { get; init; }
}

/// <summary>One operating point entry from a HEIF <c>oinf</c> property.</summary>
public sealed record HeifLhevcOperatingPoint
{
    /// <summary>Index of the output layer set in the bitstream VPS.</summary>
    public required ushort OutputLayerSetIndex { get; init; }

    /// <summary>Maximum temporal sub-layer id present in this operating point.</summary>
    public required byte MaxTemporalId { get; init; }

    /// <summary>Layers participating in this operating point.</summary>
    public required ImmutableArray<HeifLhevcOpLayer> Layers { get; init; }

    /// <summary>Minimum luma picture width (samples).</summary>
    public required ushort MinPicWidth { get; init; }

    /// <summary>Minimum luma picture height (samples).</summary>
    public required ushort MinPicHeight { get; init; }

    /// <summary>Maximum luma picture width (samples).</summary>
    public required ushort MaxPicWidth { get; init; }

    /// <summary>Maximum luma picture height (samples).</summary>
    public required ushort MaxPicHeight { get; init; }

    /// <summary>2-bit chroma format indicator (0=mono, 1=4:2:0, 2=4:2:2, 3=4:4:4).</summary>
    public required byte MaxChromaFormat { get; init; }

    /// <summary>3-bit max bit-depth indicator (0=8 bpc through 7=15 bpc).</summary>
    public required byte MaxBitDepth { get; init; }

    /// <summary>Average frame rate * 256 when frame-rate information is signalled, otherwise null.</summary>
    public required ushort? AvgFrameRate { get; init; }

    /// <summary>2-bit constant-frame-rate indicator when frame-rate information is signalled, otherwise null.</summary>
    public required byte? ConstantFrameRate { get; init; }

    /// <summary>Maximum bit rate (bits per second) when bit-rate information is signalled, otherwise null.</summary>
    public required uint? MaxBitRate { get; init; }

    /// <summary>Average bit rate (bits per second) when bit-rate information is signalled, otherwise null.</summary>
    public required uint? AvgBitRate { get; init; }
}

/// <summary>
/// One entry in the layer-dependency graph carried by a HEIF
/// <c>oinf</c> property. Each dependent layer declares the set of
/// layers it depends on plus the dimension identifiers indexed by
/// the <see cref="HeifOperatingPointsInformation.ScalabilityMask"/>.
/// </summary>
public sealed record HeifLhevcLayerDependency
{
    /// <summary>Layer identifier of the layer whose dependencies are declared.</summary>
    public required byte DependentLayerId { get; init; }

    /// <summary>Layer identifiers this layer depends on.</summary>
    public required ImmutableArray<byte> DependsOnLayerIds { get; init; }

    /// <summary>Dimension identifiers, one per set bit in
    /// <see cref="HeifOperatingPointsInformation.ScalabilityMask"/>.</summary>
    public required ImmutableArray<byte> DimensionIdentifiers { get; init; }
}

/// <summary>
/// Typed view over a HEIF <c>oinf</c> (Operating Points Information)
/// property per ISO/IEC 14496-15 section 10.4.3. The property is
/// associated with a layered HEVC item and describes the available
/// operating points, their per-layer composition, and the layer
/// dependency graph. Pairs with <see cref="HeifTargetOutputLayerSet"/>
/// (<c>tols</c>) which selects which operating point is rendered.
/// </summary>
public sealed record HeifOperatingPointsInformation
{
    /// <summary>16-bit scalability mask. Set bits identify the
    /// scalability dimensions declared per layer in the dependency
    /// graph.</summary>
    public required ushort ScalabilityMask { get; init; }

    /// <summary>Profile-tier-level records referenced by operating-point layers.</summary>
    public required ImmutableArray<HeifLhevcProfileTierLevel> ProfileTierLevels { get; init; }

    /// <summary>Operating points declared by the bitstream.</summary>
    public required ImmutableArray<HeifLhevcOperatingPoint> OperatingPoints { get; init; }

    /// <summary>Layer dependency graph.</summary>
    public required ImmutableArray<HeifLhevcLayerDependency> LayerDependencies { get; init; }

    /// <summary>Parses a raw <c>oinf</c> payload. Returns false on any
    /// FullBox-version mismatch or length / structure inconsistency.</summary>
    public static bool TryParse(ReadOnlySpan<byte> payload, out HeifOperatingPointsInformation? result)
    {
        result = null;
        if (payload.Length < 10) return false;
        if (payload[0] != 0) return false; // FullBox version must be 0

        int pos = 4;
        ushort scalabilityMask = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2));
        pos += 2;

        if (pos >= payload.Length) return false;
        int numPtl = payload[pos++] & 0x3F; // upper 2 bits reserved

        var ptls = ImmutableArray.CreateBuilder<HeifLhevcProfileTierLevel>(numPtl);
        for (int i = 0; i < numPtl; i++)
        {
            if (pos + 12 > payload.Length) return false;
            byte ptlByte = payload[pos++];
            byte profileSpace = (byte)((ptlByte >> 6) & 0x3);
            bool tierFlag = ((ptlByte >> 5) & 0x1) != 0;
            byte profileIdc = (byte)(ptlByte & 0x1F);

            uint compat = BinaryPrimitives.ReadUInt32BigEndian(payload.Slice(pos, 4));
            pos += 4;

            ulong constraint = 0;
            for (int k = 0; k < 6; k++) constraint = (constraint << 8) | payload[pos++];

            byte levelIdc = payload[pos++];

            ptls.Add(new HeifLhevcProfileTierLevel
            {
                ProfileSpace = profileSpace,
                TierFlag = tierFlag,
                ProfileIdc = profileIdc,
                ProfileCompatibilityFlags = compat,
                ConstraintIndicatorFlags = constraint,
                LevelIdc = levelIdc,
            });
        }

        if (pos + 2 > payload.Length) return false;
        ushort numOps = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2));
        pos += 2;

        var ops = ImmutableArray.CreateBuilder<HeifLhevcOperatingPoint>(numOps);
        for (int i = 0; i < numOps; i++)
        {
            if (pos + 4 > payload.Length) return false;
            ushort olsIdx = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2));
            pos += 2;
            byte maxTid = payload[pos++];
            byte layerCount = payload[pos++];

            var layers = ImmutableArray.CreateBuilder<HeifLhevcOpLayer>(layerCount);
            for (int j = 0; j < layerCount; j++)
            {
                if (pos + 2 > payload.Length) return false;
                byte ptlIdx = payload[pos++];
                byte layerByte = payload[pos++];
                byte layerId = (byte)((layerByte >> 2) & 0x3F);
                bool isOut = ((layerByte >> 1) & 0x1) != 0;
                bool isAltOut = (layerByte & 0x1) != 0;
                layers.Add(new HeifLhevcOpLayer
                {
                    PtlIndex = ptlIdx,
                    LayerId = layerId,
                    IsOutputLayer = isOut,
                    IsAlternateOutputLayer = isAltOut,
                });
            }

            if (pos + 9 > payload.Length) return false;
            ushort minW = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2)); pos += 2;
            ushort minH = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2)); pos += 2;
            ushort maxW = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2)); pos += 2;
            ushort maxH = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2)); pos += 2;

            byte packed = payload[pos++];
            byte maxChroma = (byte)((packed >> 6) & 0x3);
            byte maxBd = (byte)((packed >> 3) & 0x7);
            // bit 2 reserved
            bool frameRateFlag = ((packed >> 1) & 0x1) != 0;
            bool bitRateFlag = (packed & 0x1) != 0;

            ushort? avgFr = null;
            byte? constFr = null;
            if (frameRateFlag)
            {
                if (pos + 3 > payload.Length) return false;
                avgFr = BinaryPrimitives.ReadUInt16BigEndian(payload.Slice(pos, 2)); pos += 2;
                constFr = (byte)(payload[pos++] & 0x3);
            }

            uint? maxBr = null, avgBr = null;
            if (bitRateFlag)
            {
                if (pos + 8 > payload.Length) return false;
                maxBr = BinaryPrimitives.ReadUInt32BigEndian(payload.Slice(pos, 4)); pos += 4;
                avgBr = BinaryPrimitives.ReadUInt32BigEndian(payload.Slice(pos, 4)); pos += 4;
            }

            ops.Add(new HeifLhevcOperatingPoint
            {
                OutputLayerSetIndex = olsIdx,
                MaxTemporalId = maxTid,
                Layers = layers.ToImmutable(),
                MinPicWidth = minW,
                MinPicHeight = minH,
                MaxPicWidth = maxW,
                MaxPicHeight = maxH,
                MaxChromaFormat = maxChroma,
                MaxBitDepth = maxBd,
                AvgFrameRate = avgFr,
                ConstantFrameRate = constFr,
                MaxBitRate = maxBr,
                AvgBitRate = avgBr,
            });
        }

        if (pos >= payload.Length) return false;
        int maxLayerCount = payload[pos++];
        int dimCount = BitOperations.PopCount((uint)scalabilityMask);

        var layerDeps = ImmutableArray.CreateBuilder<HeifLhevcLayerDependency>(maxLayerCount);
        for (int i = 0; i < maxLayerCount; i++)
        {
            if (pos + 2 > payload.Length) return false;
            byte depId = payload[pos++];
            int numDeps = payload[pos++];

            if (pos + numDeps > payload.Length) return false;
            var deps = ImmutableArray.CreateBuilder<byte>(numDeps);
            for (int j = 0; j < numDeps; j++) deps.Add(payload[pos++]);

            if (pos + dimCount > payload.Length) return false;
            var dims = ImmutableArray.CreateBuilder<byte>(dimCount);
            for (int j = 0; j < dimCount; j++) dims.Add(payload[pos++]);

            layerDeps.Add(new HeifLhevcLayerDependency
            {
                DependentLayerId = depId,
                DependsOnLayerIds = deps.ToImmutable(),
                DimensionIdentifiers = dims.ToImmutable(),
            });
        }

        result = new HeifOperatingPointsInformation
        {
            ScalabilityMask = scalabilityMask,
            ProfileTierLevels = ptls.ToImmutable(),
            OperatingPoints = ops.ToImmutable(),
            LayerDependencies = layerDeps.ToImmutable(),
        };
        return true;
    }
}
