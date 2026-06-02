using Mediar.IO;

namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// MPEG-1/2/2.5 Audio Layer III side-information layout, per ISO 11172-3
/// §2.4.1.7 and ISO 13818-3 §2.4.1.7.
/// </summary>
/// <remarks>
/// <para>
/// Side-information lives immediately after the (optional) 16-bit CRC and
/// before the main-data area. Its size is fixed by (MPEG version, channel
/// count): 32, 17, 17, or 9 bytes.
/// </para>
/// </remarks>
internal sealed class Mp3SideInfo
{
    public int MainDataBegin;     // back-pointer (bytes) into the reservoir
    public int PrivateBits;
    public readonly int[,] Scfsi = new int[2, 4]; // [channel, sfb_group] — MPEG-1 only

    /// <summary>Per-granule, per-channel side info. <c>Granules[0..granuleCount-1, channel]</c>.</summary>
    public readonly Mp3GranuleInfo[,] Granules;

    public readonly int GranuleCount;
    public readonly int ChannelCount;

    public Mp3SideInfo(int granuleCount, int channelCount)
    {
        GranuleCount = granuleCount;
        ChannelCount = channelCount;
        Granules = new Mp3GranuleInfo[granuleCount, channelCount];
        for (int g = 0; g < granuleCount; g++)
            for (int c = 0; c < channelCount; c++)
                Granules[g, c] = new Mp3GranuleInfo();
    }

    /// <summary>Bytes consumed by the side info for the given configuration.</summary>
    public static int SideInfoBytes(int mpegVersion, int channelCount)
    {
        if (mpegVersion == 1) return channelCount == 1 ? 17 : 32;
        return channelCount == 1 ? 9 : 17; // MPEG-2 LSF and MPEG-2.5
    }

    /// <summary>
    /// Parse the side-info bytes for a Layer III frame. <paramref name="bytes"/>
    /// must be exactly <see cref="SideInfoBytes"/> long.
    /// </summary>
    public static Mp3SideInfo Parse(ReadOnlySpan<byte> bytes, int mpegVersion, int channelCount)
    {
        int expected = SideInfoBytes(mpegVersion, channelCount);
        if (bytes.Length != expected)
            throw new ArgumentException($"Side-info expected {expected} bytes, got {bytes.Length}.", nameof(bytes));

        int granuleCount = mpegVersion == 1 ? 2 : 1;
        var si = new Mp3SideInfo(granuleCount, channelCount);
        var br = new BitReader(bytes);

        if (mpegVersion == 1)
        {
            si.MainDataBegin = (int)br.ReadBits(9);
            si.PrivateBits = (int)br.ReadBits(channelCount == 1 ? 5 : 3);
            for (int c = 0; c < channelCount; c++)
                for (int s = 0; s < 4; s++)
                    si.Scfsi[c, s] = (int)br.ReadBits(1);
        }
        else
        {
            si.MainDataBegin = (int)br.ReadBits(8);
            si.PrivateBits = (int)br.ReadBits(channelCount == 1 ? 1 : 2);
            // No SCFSI for MPEG-2 LSF / 2.5
        }

        for (int g = 0; g < granuleCount; g++)
        {
            for (int c = 0; c < channelCount; c++)
            {
                var gr = si.Granules[g, c];
                gr.Part2_3_Length = (int)br.ReadBits(12);
                gr.BigValues = (int)br.ReadBits(9);
                gr.GlobalGain = (int)br.ReadBits(8);
                gr.ScalefacCompress = (int)br.ReadBits(mpegVersion == 1 ? 4 : 9);
                gr.WindowSwitchingFlag = br.ReadBit();
                if (gr.WindowSwitchingFlag)
                {
                    gr.BlockType = (int)br.ReadBits(2);
                    gr.MixedBlockFlag = br.ReadBit();
                    gr.TableSelect[0] = (int)br.ReadBits(5);
                    gr.TableSelect[1] = (int)br.ReadBits(5);
                    gr.TableSelect[2] = 0;
                    gr.SubblockGain[0] = (int)br.ReadBits(3);
                    gr.SubblockGain[1] = (int)br.ReadBits(3);
                    gr.SubblockGain[2] = (int)br.ReadBits(3);
                    // Region splits implied by block_type for window-switched granules.
                    if (gr.BlockType == 2 && !gr.MixedBlockFlag)
                    {
                        gr.Region0Count = 8;
                        gr.Region1Count = 36;
                    }
                    else
                    {
                        gr.Region0Count = 7;
                        gr.Region1Count = 36;
                    }
                }
                else
                {
                    gr.BlockType = 0;
                    gr.MixedBlockFlag = false;
                    gr.TableSelect[0] = (int)br.ReadBits(5);
                    gr.TableSelect[1] = (int)br.ReadBits(5);
                    gr.TableSelect[2] = (int)br.ReadBits(5);
                    gr.Region0Count = (int)br.ReadBits(4);
                    gr.Region1Count = (int)br.ReadBits(3);
                    gr.SubblockGain[0] = 0;
                    gr.SubblockGain[1] = 0;
                    gr.SubblockGain[2] = 0;
                }

                if (mpegVersion == 1)
                {
                    gr.PreFlag = br.ReadBit();
                    gr.ScalefacScale = br.ReadBit() ? 1 : 0;
                    gr.Count1TableSelect = (int)br.ReadBits(1);
                }
                else
                {
                    // MPEG-2 LSF: preflag is derived from scalefactor decoding.
                    gr.PreFlag = false;
                    gr.ScalefacScale = br.ReadBit() ? 1 : 0;
                    gr.Count1TableSelect = (int)br.ReadBits(1);
                }
            }
        }

        return si;
    }
}

/// <summary>Per-granule, per-channel decoded side-information for one Layer III frame.</summary>
internal sealed class Mp3GranuleInfo
{
    public int Part2_3_Length;
    public int BigValues;
    public int GlobalGain;
    public int ScalefacCompress;
    public bool WindowSwitchingFlag;
    public int BlockType;          // 0=normal long, 1=start, 2=short, 3=stop
    public bool MixedBlockFlag;
    public readonly int[] TableSelect = new int[3];
    public readonly int[] SubblockGain = new int[3];
    public int Region0Count;
    public int Region1Count;
    public bool PreFlag;
    public int ScalefacScale;
    public int Count1TableSelect;
}
