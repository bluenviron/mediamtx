namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed view over an AVC (H.264) Sequence Parameter Set carried as a
/// length-prefixed NAL unit inside an <c>avcC</c> parameter-set array.
/// Decodes the full SPS RBSP per ITU-T H.264 section 7.3.2.1.1 with the
/// VUI / SPS extension blocks intentionally left undecoded - covers
/// every field a renderer or remuxer typically needs: profile / level /
/// constraint flags, chroma format and bit depths (when a high-profile
/// SPS supplies them), frame number / POC type bookkeeping, picture
/// geometry in macroblocks, and the frame-cropping window.
/// </summary>
public sealed record AvcSequenceParameterSet
{
    /// <summary>SPS_NUT in the H.264 NAL unit type registry (7).</summary>
    public const int SpsNalUnitType = 7;

    /// <summary>8-bit <c>profile_idc</c>.</summary>
    public required byte ProfileIdc { get; init; }

    /// <summary>Packed <c>constraint_set0..5_flag</c> field, MSB-aligned in the low byte.</summary>
    public required byte ConstraintSetFlags { get; init; }

    /// <summary>8-bit <c>level_idc</c>.</summary>
    public required byte LevelIdc { get; init; }

    /// <summary><c>seq_parameter_set_id</c>, exp-Golomb decoded.</summary>
    public required uint SequenceParameterSetId { get; init; }

    /// <summary>4:2:0 (1) by default; only signalled in the bitstream for
    /// high profiles - see ITU-T H.264 section 7.4.2.1.1.</summary>
    public required byte ChromaFormatIdc { get; init; }

    /// <summary>True iff <c>chroma_format_idc</c> is 3 and the SPS sets
    /// <c>separate_colour_plane_flag</c>.</summary>
    public required bool SeparateColourPlaneFlag { get; init; }

    /// <summary>Luma bit depth minus 8; 0 when not signalled.</summary>
    public required byte BitDepthLumaMinus8 { get; init; }

    /// <summary>Chroma bit depth minus 8; 0 when not signalled.</summary>
    public required byte BitDepthChromaMinus8 { get; init; }

    /// <summary><c>log2_max_frame_num_minus4</c>, exp-Golomb decoded.</summary>
    public required uint Log2MaxFrameNumMinus4 { get; init; }

    /// <summary><c>pic_order_cnt_type</c>, exp-Golomb decoded.</summary>
    public required uint PicOrderCntType { get; init; }

    /// <summary><c>max_num_ref_frames</c>, exp-Golomb decoded.</summary>
    public required uint MaxNumRefFrames { get; init; }

    /// <summary><c>gaps_in_frame_num_value_allowed_flag</c>.</summary>
    public required bool GapsInFrameNumValueAllowedFlag { get; init; }

    /// <summary><c>pic_width_in_mbs_minus1</c>, exp-Golomb decoded.</summary>
    public required uint PicWidthInMbsMinus1 { get; init; }

    /// <summary><c>pic_height_in_map_units_minus1</c>, exp-Golomb decoded.</summary>
    public required uint PicHeightInMapUnitsMinus1 { get; init; }

    /// <summary><c>frame_mbs_only_flag</c>; true for progressive streams.</summary>
    public required bool FrameMbsOnlyFlag { get; init; }

    /// <summary><c>mb_adaptive_frame_field_flag</c>; only valid when
    /// <see cref="FrameMbsOnlyFlag"/> is false.</summary>
    public required bool MbAdaptiveFrameFieldFlag { get; init; }

    /// <summary><c>direct_8x8_inference_flag</c>.</summary>
    public required bool Direct8x8InferenceFlag { get; init; }

    /// <summary><c>frame_cropping_flag</c>.</summary>
    public required bool FrameCroppingFlag { get; init; }

    /// <summary><c>frame_crop_left_offset</c> in chroma samples (or luma when
    /// <c>ChromaArrayType</c> is 0). Zero when <see cref="FrameCroppingFlag"/>
    /// is false.</summary>
    public required uint FrameCropLeftOffset { get; init; }

    /// <summary><c>frame_crop_right_offset</c>; same units as left.</summary>
    public required uint FrameCropRightOffset { get; init; }

    /// <summary><c>frame_crop_top_offset</c>; same units as left.</summary>
    public required uint FrameCropTopOffset { get; init; }

    /// <summary><c>frame_crop_bottom_offset</c>; same units as left.</summary>
    public required uint FrameCropBottomOffset { get; init; }

    /// <summary>Luma bit depth in bits (<see cref="BitDepthLumaMinus8"/> + 8).</summary>
    public byte BitDepthLuma => (byte)(BitDepthLumaMinus8 + 8);

    /// <summary>Chroma bit depth in bits (<see cref="BitDepthChromaMinus8"/> + 8).</summary>
    public byte BitDepthChroma => (byte)(BitDepthChromaMinus8 + 8);

    /// <summary>Coded picture width before cropping = <c>(MbsMinus1 + 1) * 16</c>.</summary>
    public uint PictureWidthInSamples => (PicWidthInMbsMinus1 + 1) * 16;

    /// <summary>Coded picture height before cropping = <c>(MapUnitsMinus1 + 1) * 16 * (FrameMbsOnly ? 1 : 2)</c>.</summary>
    public uint PictureHeightInSamples => (PicHeightInMapUnitsMinus1 + 1) * 16 * (FrameMbsOnlyFlag ? 1u : 2u);

    /// <summary>Decoded picture width after applying the frame-cropping
    /// window with chroma-format-driven CropUnitX.</summary>
    public uint DecodedWidth
    {
        get
        {
            uint cropUnitX = CropUnitX;
            uint crop = cropUnitX * (FrameCropLeftOffset + FrameCropRightOffset);
            return crop >= PictureWidthInSamples ? 0 : PictureWidthInSamples - crop;
        }
    }

    /// <summary>Decoded picture height after applying the frame-cropping
    /// window with chroma-format-driven and interlace-aware CropUnitY.</summary>
    public uint DecodedHeight
    {
        get
        {
            uint cropUnitY = CropUnitY;
            uint crop = cropUnitY * (FrameCropTopOffset + FrameCropBottomOffset);
            return crop >= PictureHeightInSamples ? 0 : PictureHeightInSamples - crop;
        }
    }

    private byte ChromaArrayType => SeparateColourPlaneFlag ? (byte)0 : ChromaFormatIdc;

    private uint CropUnitX => ChromaArrayType switch
    {
        0 => 1,
        1 => 2, // 4:2:0
        2 => 2, // 4:2:2
        3 => 1, // 4:4:4
        _ => 1,
    };

    private uint CropUnitY
    {
        get
        {
            uint subHeightC = ChromaArrayType switch
            {
                1 => 2,
                _ => 1,
            };
            return subHeightC * (FrameMbsOnlyFlag ? 1u : 2u);
        }
    }

    /// <summary>
    /// Parses an AVC SPS NAL unit. Expects a 1-byte NAL unit header
    /// (nal_unit_type must be 7) followed by the SPS RBSP, optionally
    /// containing emulation prevention bytes which are stripped before
    /// bit decoding. Returns false on any structural violation.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> nalUnit, out AvcSequenceParameterSet? sps)
    {
        sps = null;
        if (nalUnit.Length < 4) return false;

        if ((nalUnit[0] & 0x80) != 0) return false; // forbidden_zero_bit
        int nalUnitType = nalUnit[0] & 0x1F;
        if (nalUnitType != SpsNalUnitType) return false;

        byte[] rbsp = HevcSequenceParameterSet.StripEmulationPreventionBytes(nalUnit.Slice(1));
        var reader = new BitReader(rbsp);

        try
        {
            byte profileIdc = (byte)reader.ReadBits(8);
            byte constraintSet = (byte)reader.ReadBits(8); // includes the 2 reserved bits
            byte levelIdc = (byte)reader.ReadBits(8);
            uint spsId = reader.ReadUe();

            byte chromaFormat = 1;
            bool separatePlane = false;
            byte bdLumaMinus8 = 0;
            byte bdChromaMinus8 = 0;

            if (IsHighProfile(profileIdc))
            {
                uint cf = reader.ReadUe();
                if (cf > 3) return false;
                chromaFormat = (byte)cf;
                if (chromaFormat == 3) separatePlane = reader.ReadBit();

                uint bdLuma = reader.ReadUe();
                uint bdChroma = reader.ReadUe();
                if (bdLuma > 6 || bdChroma > 6) return false;
                bdLumaMinus8 = (byte)bdLuma;
                bdChromaMinus8 = (byte)bdChroma;

                reader.SkipBits(1); // qpprime_y_zero_transform_bypass_flag

                bool seqScalingMatrixPresent = reader.ReadBit();
                if (seqScalingMatrixPresent)
                {
                    int listCount = chromaFormat != 3 ? 8 : 12;
                    for (int i = 0; i < listCount; i++)
                    {
                        if (reader.ReadBit()) SkipScalingList(ref reader, i < 6 ? 16 : 64);
                    }
                }
            }

            uint log2MaxFn = reader.ReadUe();
            uint pocType = reader.ReadUe();
            if (pocType > 2) return false;

            if (pocType == 0)
            {
                reader.ReadUe(); // log2_max_pic_order_cnt_lsb_minus4
            }
            else if (pocType == 1)
            {
                reader.SkipBits(1); // delta_pic_order_always_zero_flag
                ReadSe(ref reader);  // offset_for_non_ref_pic
                ReadSe(ref reader);  // offset_for_top_to_bottom_field
                uint cycle = reader.ReadUe();
                if (cycle > 256) return false;
                for (int i = 0; i < cycle; i++) ReadSe(ref reader);
            }

            uint maxRefFrames = reader.ReadUe();
            bool gaps = reader.ReadBit();
            uint widthMbsMinus1 = reader.ReadUe();
            uint heightMapUnitsMinus1 = reader.ReadUe();
            bool frameMbsOnly = reader.ReadBit();
            bool mbaff = false;
            if (!frameMbsOnly) mbaff = reader.ReadBit();
            bool direct8x8 = reader.ReadBit();

            bool cropFlag = reader.ReadBit();
            uint cl = 0, cr = 0, ct = 0, cb = 0;
            if (cropFlag)
            {
                cl = reader.ReadUe();
                cr = reader.ReadUe();
                ct = reader.ReadUe();
                cb = reader.ReadUe();
            }

            sps = new AvcSequenceParameterSet
            {
                ProfileIdc = profileIdc,
                ConstraintSetFlags = constraintSet,
                LevelIdc = levelIdc,
                SequenceParameterSetId = spsId,
                ChromaFormatIdc = chromaFormat,
                SeparateColourPlaneFlag = separatePlane,
                BitDepthLumaMinus8 = bdLumaMinus8,
                BitDepthChromaMinus8 = bdChromaMinus8,
                Log2MaxFrameNumMinus4 = log2MaxFn,
                PicOrderCntType = pocType,
                MaxNumRefFrames = maxRefFrames,
                GapsInFrameNumValueAllowedFlag = gaps,
                PicWidthInMbsMinus1 = widthMbsMinus1,
                PicHeightInMapUnitsMinus1 = heightMapUnitsMinus1,
                FrameMbsOnlyFlag = frameMbsOnly,
                MbAdaptiveFrameFieldFlag = mbaff,
                Direct8x8InferenceFlag = direct8x8,
                FrameCroppingFlag = cropFlag,
                FrameCropLeftOffset = cl,
                FrameCropRightOffset = cr,
                FrameCropTopOffset = ct,
                FrameCropBottomOffset = cb,
            };
            return true;
        }
        catch (EndOfBitstreamException)
        {
            sps = null;
            return false;
        }
    }

    private static bool IsHighProfile(byte profileIdc) => profileIdc switch
    {
        100 or 110 or 122 or 244 or 44 or 83 or 86 or 118 or 128 or 138 or 139 or 134 or 135 => true,
        _ => false,
    };

    private static int ReadSe(ref BitReader r)
    {
        uint codeNum = r.ReadUe();
        return ((codeNum & 1) != 0)
            ? (int)((codeNum + 1) / 2)
            : -(int)(codeNum / 2);
    }

    private static void SkipScalingList(ref BitReader r, int size)
    {
        int lastScale = 8;
        int nextScale = 8;
        for (int j = 0; j < size; j++)
        {
            if (nextScale != 0)
            {
                int deltaScale = ReadSe(ref r);
                nextScale = (lastScale + deltaScale + 256) % 256;
            }
            lastScale = nextScale == 0 ? lastScale : nextScale;
        }
    }

    private sealed class EndOfBitstreamException : Exception { }

    private struct BitReader
    {
        private readonly byte[] _data;
        private int _bytePos;
        private int _bitPos;

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
