using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// AV1 chroma sample position per AV1 Bitstream &amp; Decoding Process Specification §6.4.2.
/// </summary>
public enum Av1ChromaSamplePosition
{
    /// <summary>Unknown sample position.</summary>
    Unknown = 0,
    /// <summary>Vertically co-located, horizontally between two luma samples.</summary>
    Vertical = 1,
    /// <summary>Co-located with the top-left luma sample.</summary>
    Colocated = 2,
    /// <summary>Reserved.</summary>
    Reserved = 3,
}

/// <summary>
/// Typed view of the HEIF / AVIF <c>av1C</c> property (AV1 Codec
/// Configuration Record) per the AV1 ISOBMFF binding specification §2.3.
/// The configOBUs trailing byte array is preserved verbatim so callers
/// that wish to decode the sequence header can do so without re-reading
/// the file.
/// </summary>
public sealed record Av1CodecConfigurationRecord
{
    /// <summary>Spec version (currently always 1).</summary>
    public required byte Version { get; init; }

    /// <summary>Sequence profile (0 = Main, 1 = High, 2 = Professional).</summary>
    public required byte SeqProfile { get; init; }

    /// <summary>Sequence level index for operating point 0 (0 = level 2.0 ... 23 = level 7.3).</summary>
    public required byte SeqLevelIdx0 { get; init; }

    /// <summary>Sequence tier for operating point 0 (0 = Main, 1 = High).</summary>
    public required byte SeqTier0 { get; init; }

    /// <summary>True when the sequence uses 10- or 12-bit samples.</summary>
    public required bool HighBitDepth { get; init; }

    /// <summary>True when the sequence is 12-bit (requires <see cref="HighBitDepth"/>).</summary>
    public required bool TwelveBit { get; init; }

    /// <summary>True when the sequence is monochrome.</summary>
    public required bool Monochrome { get; init; }

    /// <summary>Horizontal chroma subsampling factor (0 = no subsample, 1 = subsampled).</summary>
    public required byte ChromaSubsamplingX { get; init; }

    /// <summary>Vertical chroma subsampling factor (0 = no subsample, 1 = subsampled).</summary>
    public required byte ChromaSubsamplingY { get; init; }

    /// <summary>Chroma sample position (only meaningful when both subsampling factors are 1).</summary>
    public required Av1ChromaSamplePosition ChromaSamplePosition { get; init; }

    /// <summary>Optional initial presentation delay (1..16); null when not signalled.</summary>
    public required byte? InitialPresentationDelay { get; init; }

    /// <summary>Trailing AV1 OBU sequence-header bytes (after the 4-byte fixed header). Empty when not present.</summary>
    public required ImmutableArray<byte> ConfigObus { get; init; }

    /// <summary>Computed bit depth: 8, 10 or 12 bits per sample.</summary>
    public int BitDepth => TwelveBit ? 12 : (HighBitDepth ? 10 : 8);

    /// <summary>
    /// Computed chroma subsampling shorthand: "4:0:0" when monochrome,
    /// "4:2:0" when both X and Y subsampled, "4:2:2" when only X, "4:4:4"
    /// when neither.
    /// </summary>
    public string ChromaFormat => Monochrome
        ? "4:0:0"
        : (ChromaSubsamplingX, ChromaSubsamplingY) switch
        {
            (1, 1) => "4:2:0",
            (1, 0) => "4:2:2",
            (0, 0) => "4:4:4",
            _ => "unknown",
        };

    /// <summary>Decodes a raw <c>av1C</c> payload (4 fixed bytes + optional trailing OBUs).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out Av1CodecConfigurationRecord record)
    {
        record = null!;
        if (data.Length < 4) return false;

        byte b0 = data[0];
        byte b1 = data[1];
        byte b2 = data[2];
        byte b3 = data[3];

        // b0: marker(1) | version(7).
        if ((b0 & 0x80) == 0) return false; // marker bit must be set.
        byte version = (byte)(b0 & 0x7F);

        // b1: seq_profile(3) | seq_level_idx_0(5).
        byte seqProfile = (byte)((b1 >> 5) & 0x7);
        byte seqLevelIdx0 = (byte)(b1 & 0x1F);

        // b2: seq_tier_0(1) | high_bitdepth(1) | twelve_bit(1) | monochrome(1) |
        //     chroma_subsampling_x(1) | chroma_subsampling_y(1) | chroma_sample_position(2).
        byte seqTier0 = (byte)((b2 >> 7) & 0x1);
        bool highBitDepth = ((b2 >> 6) & 0x1) == 1;
        bool twelveBit = ((b2 >> 5) & 0x1) == 1;
        bool monochrome = ((b2 >> 4) & 0x1) == 1;
        byte chromaSubX = (byte)((b2 >> 3) & 0x1);
        byte chromaSubY = (byte)((b2 >> 2) & 0x1);
        byte chromaSamplePos = (byte)(b2 & 0x3);

        // b3: reserved(3) | initial_presentation_delay_present(1) |
        //     initial_presentation_delay_minus_one(4) | reserved(4).
        bool ipdPresent = ((b3 >> 4) & 0x1) == 1;
        byte? initialPresentationDelay = ipdPresent
            ? (byte)((b3 & 0x0F) + 1)
            : null;

        ImmutableArray<byte> configObus = data.Length > 4
            ? ImmutableArray.Create(data[4..])
            : ImmutableArray<byte>.Empty;

        record = new Av1CodecConfigurationRecord
        {
            Version = version,
            SeqProfile = seqProfile,
            SeqLevelIdx0 = seqLevelIdx0,
            SeqTier0 = seqTier0,
            HighBitDepth = highBitDepth,
            TwelveBit = twelveBit,
            Monochrome = monochrome,
            ChromaSubsamplingX = chromaSubX,
            ChromaSubsamplingY = chromaSubY,
            ChromaSamplePosition = (Av1ChromaSamplePosition)chromaSamplePos,
            InitialPresentationDelay = initialPresentationDelay,
            ConfigObus = configObus,
        };
        return true;
    }
}
