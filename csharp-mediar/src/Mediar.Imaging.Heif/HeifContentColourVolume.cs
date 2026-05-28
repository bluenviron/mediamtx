using System.Buffers.Binary;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// HEIF Content Colour Volume property (<c>cclv</c>) per ISO/IEC
/// 23008-12 §6.5.18 (mirrors the HEVC CCV SEI message). Describes the
/// colour volume of the content carried by the associated image item
/// — primaries, peak luminance, average luminance — and is a sibling
/// of <c>mdcv</c> (which describes the volume of the mastering
/// display rather than the content itself).
/// </summary>
public sealed record HeifContentColourVolume
{
    /// <summary>When true, all previously-signalled CCV state for the
    /// associated item is cancelled and no further fields are
    /// transmitted.</summary>
    public required bool CcvCancelFlag { get; init; }

    /// <summary>When true, the CCV state applies to all subsequent
    /// pictures in decoding order until the next CCV SEI message; for
    /// HEIF still items this flag is informational only.</summary>
    public required bool CcvPersistenceFlag { get; init; }

    /// <summary>The CIE 1931 (x, y) chromaticity coordinates of the
    /// content primaries when present. Three entries (R, G, B) of
    /// signed (x, y) pairs in units of 0.00002 (so x = 35400 means
    /// 0.708). Empty when <c>ccv_primaries_present_flag = 0</c>.</summary>
    public required ImmutableArray<int> Primaries { get; init; }

    /// <summary>Minimum luminance value of the content in units of
    /// 0.0001 cd/m². Null when
    /// <c>ccv_min_luminance_value_present_flag = 0</c>.</summary>
    public required uint? MinLuminanceValue { get; init; }

    /// <summary>Maximum luminance value of the content in units of
    /// 0.0001 cd/m². Null when
    /// <c>ccv_max_luminance_value_present_flag = 0</c>.</summary>
    public required uint? MaxLuminanceValue { get; init; }

    /// <summary>Average luminance value of the content in units of
    /// 0.0001 cd/m². Null when
    /// <c>ccv_avg_luminance_value_present_flag = 0</c>.</summary>
    public required uint? AvgLuminanceValue { get; init; }

    /// <summary>Minimum luminance value in cd/m² (nits) when present.</summary>
    public double? MinLuminanceCdM2 => MinLuminanceValue.HasValue ? MinLuminanceValue.Value * 0.0001 : null;

    /// <summary>Maximum luminance value in cd/m² (nits) when present.</summary>
    public double? MaxLuminanceCdM2 => MaxLuminanceValue.HasValue ? MaxLuminanceValue.Value * 0.0001 : null;

    /// <summary>Average luminance value in cd/m² (nits) when present.</summary>
    public double? AvgLuminanceCdM2 => AvgLuminanceValue.HasValue ? AvgLuminanceValue.Value * 0.0001 : null;

    /// <summary>Decodes a raw <c>cclv</c> payload (4-byte FullBox
    /// header + 1 packed flag byte + conditional primaries / luminance
    /// fields based on the flag bits).</summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifContentColourVolume volume)
    {
        volume = null!;
        if (data.Length < 5) return false;
        if (data[0] != 0) return false; // only version 0 is defined

        byte flags = data[4];
        bool cancel = (flags & 0x80) != 0;
        bool persistence = (flags & 0x40) != 0;
        bool primariesPresent = (flags & 0x20) != 0;
        bool minPresent = (flags & 0x10) != 0;
        bool maxPresent = (flags & 0x08) != 0;
        bool avgPresent = (flags & 0x04) != 0;

        ImmutableArray<int> primaries = ImmutableArray<int>.Empty;
        uint? minLum = null;
        uint? maxLum = null;
        uint? avgLum = null;

        int pos = 5;
        if (!cancel)
        {
            if (primariesPresent)
            {
                if (pos + 24 > data.Length) return false;
                var pb = ImmutableArray.CreateBuilder<int>(6);
                for (int i = 0; i < 6; i++)
                {
                    pb.Add(BinaryPrimitives.ReadInt32BigEndian(data.Slice(pos, 4)));
                    pos += 4;
                }
                primaries = pb.ToImmutable();
            }
            if (minPresent)
            {
                if (pos + 4 > data.Length) return false;
                minLum = BinaryPrimitives.ReadUInt32BigEndian(data.Slice(pos, 4));
                pos += 4;
            }
            if (maxPresent)
            {
                if (pos + 4 > data.Length) return false;
                maxLum = BinaryPrimitives.ReadUInt32BigEndian(data.Slice(pos, 4));
                pos += 4;
            }
            if (avgPresent)
            {
                if (pos + 4 > data.Length) return false;
                avgLum = BinaryPrimitives.ReadUInt32BigEndian(data.Slice(pos, 4));
                pos += 4;
            }
        }

        volume = new HeifContentColourVolume
        {
            CcvCancelFlag = cancel,
            CcvPersistenceFlag = persistence,
            Primaries = primaries,
            MinLuminanceValue = minLum,
            MaxLuminanceValue = maxLum,
            AvgLuminanceValue = avgLum,
        };
        return true;
    }
}
