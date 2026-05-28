namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over an HEVC Sequence Parameter Set carried as a
/// length-prefixed NAL unit inside an <c>hvcC</c> parameter-set
/// array. Only the leading subset of the SPS RBSP is decoded -
/// profile/tier/level, chroma format, picture geometry (with
/// conformance-window cropping applied), and per-component bit
/// depth - which covers the fields most callers need to render or
/// reformat an HEVC bitstream. The VUI and SPS extension sections
/// are intentionally left undecoded.
/// </summary>
public sealed record HevcSequenceParameterSet
{
    /// <summary>SPS_NUT in the HEVC NAL unit type registry (33).</summary>
    public const int SpsNalUnitType = 33;

    /// <summary>4-bit <c>sps_video_parameter_set_id</c>.</summary>
    public required byte VideoParameterSetId { get; init; }

    /// <summary>3-bit <c>sps_max_sub_layers_minus1</c>.</summary>
    public required byte MaxSubLayersMinus1 { get; init; }

    /// <summary>1-bit <c>sps_temporal_id_nesting_flag</c>.</summary>
    public required bool TemporalIdNestingFlag { get; init; }

    /// <summary>2-bit <c>general_profile_space</c>.</summary>
    public required byte GeneralProfileSpace { get; init; }

    /// <summary>1-bit <c>general_tier_flag</c> (false = Main, true = High).</summary>
    public required bool GeneralTierFlag { get; init; }

    /// <summary>5-bit <c>general_profile_idc</c>.</summary>
    public required byte GeneralProfileIdc { get; init; }

    /// <summary>32-bit <c>general_profile_compatibility_flag</c> bitmap.</summary>
    public required uint GeneralProfileCompatibilityFlags { get; init; }

    /// <summary>48-bit constraint indicator flags right-justified in the low 48 bits.</summary>
    public required ulong GeneralConstraintIndicatorFlags { get; init; }

    /// <summary>8-bit <c>general_level_idc</c>.</summary>
    public required byte GeneralLevelIdc { get; init; }

    /// <summary><c>sps_seq_parameter_set_id</c>, exp-Golomb decoded.</summary>
    public required uint SequenceParameterSetId { get; init; }

    /// <summary><c>chroma_format_idc</c> (0=mono, 1=4:2:0, 2=4:2:2, 3=4:4:4).</summary>
    public required byte ChromaFormatIdc { get; init; }

    /// <summary>True iff <c>chroma_format_idc</c> is 3 and
    /// <c>separate_colour_plane_flag</c> is set.</summary>
    public required bool SeparateColourPlaneFlag { get; init; }

    /// <summary>Coded picture width, before conformance-window cropping.</summary>
    public required uint PictureWidthInLumaSamples { get; init; }

    /// <summary>Coded picture height, before conformance-window cropping.</summary>
    public required uint PictureHeightInLumaSamples { get; init; }

    /// <summary>True when conformance-window offsets are present.</summary>
    public required bool ConformanceWindowFlag { get; init; }

    /// <summary>Conformance-window left offset (chroma samples).</summary>
    public required uint ConformanceWindowLeftOffset { get; init; }

    /// <summary>Conformance-window right offset (chroma samples).</summary>
    public required uint ConformanceWindowRightOffset { get; init; }

    /// <summary>Conformance-window top offset (chroma samples).</summary>
    public required uint ConformanceWindowTopOffset { get; init; }

    /// <summary>Conformance-window bottom offset (chroma samples).</summary>
    public required uint ConformanceWindowBottomOffset { get; init; }

    /// <summary>Luma bit depth minus 8 (0 = 8 bpc, 2 = 10 bpc, 4 = 12 bpc, ...).</summary>
    public required byte BitDepthLumaMinus8 { get; init; }

    /// <summary>Chroma bit depth minus 8.</summary>
    public required byte BitDepthChromaMinus8 { get; init; }

    /// <summary>Luma bit depth in bits (<see cref="BitDepthLumaMinus8"/> + 8).</summary>
    public byte BitDepthLuma => (byte)(BitDepthLumaMinus8 + 8);

    /// <summary>Chroma bit depth in bits (<see cref="BitDepthChromaMinus8"/> + 8).</summary>
    public byte BitDepthChroma => (byte)(BitDepthChromaMinus8 + 8);

    /// <summary>Decoded picture width after conformance-window cropping.</summary>
    public uint DecodedWidth
    {
        get
        {
            (int subWidthC, _) = SubSamplingFactors;
            uint crop = (uint)subWidthC * (ConformanceWindowLeftOffset + ConformanceWindowRightOffset);
            return crop >= PictureWidthInLumaSamples ? 0 : PictureWidthInLumaSamples - crop;
        }
    }

    /// <summary>Decoded picture height after conformance-window cropping.</summary>
    public uint DecodedHeight
    {
        get
        {
            (_, int subHeightC) = SubSamplingFactors;
            uint crop = (uint)subHeightC * (ConformanceWindowTopOffset + ConformanceWindowBottomOffset);
            return crop >= PictureHeightInLumaSamples ? 0 : PictureHeightInLumaSamples - crop;
        }
    }

    private (int SubWidthC, int SubHeightC) SubSamplingFactors => ChromaFormatIdc switch
    {
        1 => (2, 2),
        2 => (2, 1),
        3 => SeparateColourPlaneFlag ? (1, 1) : (1, 1),
        _ => (1, 1),
    };

    /// <summary>
    /// Parses an HEVC SPS NAL unit. Expects a 2-byte NAL unit
    /// header (nal_unit_type must be 33, SPS_NUT) followed by the
    /// SPS RBSP, optionally containing emulation prevention bytes
    /// (<c>0x00 0x00 0x03</c>) which are stripped before bit
    /// decoding. Returns false on any structural violation.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnit, out HevcSequenceParameterSet? sps)
    {
        sps = null;
        if (nalUnit.Length < 4) return false;

        // forbidden_zero_bit must be 0
        if ((nalUnit[0] & 0x80) != 0) return false;
        int nalUnitType = (nalUnit[0] >> 1) & 0x3F;
        if (nalUnitType != SpsNalUnitType) return false;

        byte[] rbsp = StripEmulationPreventionBytes(nalUnit.Slice(2));
        var reader = new BitReader(rbsp);

        try
        {
            byte vpsId = (byte)reader.ReadBits(4);
            byte maxSubLayersMinus1 = (byte)reader.ReadBits(3);
            bool tempIdNesting = reader.ReadBit();

            // profile_tier_level(profilePresentFlag=1, maxNumSubLayersMinus1)
            byte profileSpace = (byte)reader.ReadBits(2);
            bool tierFlag = reader.ReadBit();
            byte profileIdc = (byte)reader.ReadBits(5);
            uint profileCompat = (uint)reader.ReadBits(32);
            ulong constraint = ((ulong)reader.ReadBits(24) << 24) | (ulong)reader.ReadBits(24);
            byte levelIdc = (byte)reader.ReadBits(8);

            // Sub-layer presence flags and reserved zeros.
            int subLayerProfilePresentMask = 0;
            int subLayerLevelPresentMask = 0;
            for (int i = 0; i < maxSubLayersMinus1; i++)
            {
                if (reader.ReadBit()) subLayerProfilePresentMask |= 1 << i;
                if (reader.ReadBit()) subLayerLevelPresentMask |= 1 << i;
            }
            if (maxSubLayersMinus1 > 0)
            {
                for (int i = maxSubLayersMinus1; i < 8; i++) reader.SkipBits(2);
            }

            for (int i = 0; i < maxSubLayersMinus1; i++)
            {
                if ((subLayerProfilePresentMask & (1 << i)) != 0)
                {
                    // Skip 88 bits per sub-layer profile entry.
                    reader.SkipBits(2 + 1 + 5 + 32 + 48);
                }
                if ((subLayerLevelPresentMask & (1 << i)) != 0)
                {
                    reader.SkipBits(8);
                }
            }

            uint spsId = reader.ReadUe();
            uint chromaFormat = reader.ReadUe();
            if (chromaFormat > 3) return false;
            bool separatePlane = false;
            if (chromaFormat == 3) separatePlane = reader.ReadBit();

            uint picWidth = reader.ReadUe();
            uint picHeight = reader.ReadUe();
            if (picWidth == 0 || picHeight == 0) return false;

            bool confWindow = reader.ReadBit();
            uint left = 0, right = 0, top = 0, bottom = 0;
            if (confWindow)
            {
                left = reader.ReadUe();
                right = reader.ReadUe();
                top = reader.ReadUe();
                bottom = reader.ReadUe();
            }

            uint bdLuma = reader.ReadUe();
            uint bdChroma = reader.ReadUe();
            if (bdLuma > 8 || bdChroma > 8) return false;

            sps = new HevcSequenceParameterSet
            {
                VideoParameterSetId = vpsId,
                MaxSubLayersMinus1 = maxSubLayersMinus1,
                TemporalIdNestingFlag = tempIdNesting,
                GeneralProfileSpace = profileSpace,
                GeneralTierFlag = tierFlag,
                GeneralProfileIdc = profileIdc,
                GeneralProfileCompatibilityFlags = profileCompat,
                GeneralConstraintIndicatorFlags = constraint,
                GeneralLevelIdc = levelIdc,
                SequenceParameterSetId = spsId,
                ChromaFormatIdc = (byte)chromaFormat,
                SeparateColourPlaneFlag = separatePlane,
                PictureWidthInLumaSamples = picWidth,
                PictureHeightInLumaSamples = picHeight,
                ConformanceWindowFlag = confWindow,
                ConformanceWindowLeftOffset = left,
                ConformanceWindowRightOffset = right,
                ConformanceWindowTopOffset = top,
                ConformanceWindowBottomOffset = bottom,
                BitDepthLumaMinus8 = (byte)bdLuma,
                BitDepthChromaMinus8 = (byte)bdChroma,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            sps = null;
            return false;
        }
    }

    /// <summary>
    /// Strips HEVC / AVC emulation prevention bytes (the 0x03 byte
    /// inside a 0x00 0x00 0x03 sequence) from a NAL unit RBSP.
    /// Exposed publicly so callers parsing VPS / PPS / SEI NAL
    /// units alongside this SPS decoder can reuse the same byte
    /// stripper.
    /// </summary>
    public static byte[] StripEmulationPreventionBytes(ReadOnlySpan<byte> src)
    {
        // Strip 0x00 0x00 0x03 sequences where the 0x03 is an emulation prevention byte.
        var result = new byte[src.Length];
        int wi = 0;
        for (int i = 0; i < src.Length; i++)
        {
            if (i + 2 < src.Length && src[i] == 0 && src[i + 1] == 0 && src[i + 2] == 0x03)
            {
                result[wi++] = 0;
                result[wi++] = 0;
                i += 2;
            }
            else
            {
                result[wi++] = src[i];
            }
        }
        if (wi == result.Length) return result;
        var trimmed = new byte[wi];
        Buffer.BlockCopy(result, 0, trimmed, 0, wi);
        return trimmed;
    }

    private sealed class EndOfBitstreamException : Exception { }

    private struct BitReader
    {
        private readonly byte[] _data;
        private int _bytePos;
        private int _bitPos; // 0..7, MSB first

        public BitReader(byte[] data)
        {
            _data = data;
            _bytePos = 0;
            _bitPos = 0;
        }

        public bool ReadBit()
        {
            if (_bytePos >= _data.Length) throw new EndOfBitstreamException();
            int bit = (_data[_bytePos] >> (7 - _bitPos)) & 0x1;
            _bitPos++;
            if (_bitPos == 8) { _bitPos = 0; _bytePos++; }
            return bit != 0;
        }

        public ulong ReadBits(int count)
        {
            if (count is < 0 or > 64) throw new ArgumentOutOfRangeException(nameof(count));
            ulong v = 0;
            for (int i = 0; i < count; i++) v = (v << 1) | (ReadBit() ? 1UL : 0UL);
            return v;
        }

        public void SkipBits(int count)
        {
            ArgumentOutOfRangeException.ThrowIfNegative(count);
            for (int i = 0; i < count; i++)
            {
                if (_bytePos >= _data.Length) throw new EndOfBitstreamException();
                _bitPos++;
                if (_bitPos == 8) { _bitPos = 0; _bytePos++; }
            }
        }

        public uint ReadUe()
        {
            int zeroes = 0;
            while (!ReadBit())
            {
                zeroes++;
                if (zeroes > 31) throw new EndOfBitstreamException();
            }
            if (zeroes == 0) return 0;
            uint tail = (uint)ReadBits(zeroes);
            return (1u << zeroes) - 1 + tail;
        }
    }
}
