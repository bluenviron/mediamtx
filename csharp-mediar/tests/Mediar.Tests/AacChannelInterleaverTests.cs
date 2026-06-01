using Mediar.Codecs.Aac.Decoder;
using Xunit;

namespace Mediar.Tests;

public class AacChannelInterleaverTests
{
    [Fact]
    public void Interleave_NullChannels_Throws()
    {
        Assert.Throws<ArgumentNullException>(
            () => AacChannelInterleaver.Interleave(
                (IReadOnlyList<AacChannelOutput>)null!, new float[8]));
    }

    [Fact]
    public void Interleave_EmptyChannels_Throws()
    {
        var ex = Assert.Throws<ArgumentException>(
            () => AacChannelInterleaver.Interleave(
                Array.Empty<AacChannelOutput>(), new float[8]));
        Assert.Contains("empty", ex.Message);
    }

    [Fact]
    public void Interleave_NullEntry_Throws()
    {
        var ch = new[]
        {
            BuildChannel([1f, 2f]),
            null!,
        };
        Assert.Throws<ArgumentException>(
            () => AacChannelInterleaver.Interleave(ch, new float[4]));
    }

    [Fact]
    public void Interleave_MismatchedLength_Throws()
    {
        var ch = new[]
        {
            BuildChannel([1f, 2f]),
            BuildChannel([3f, 4f, 5f]),
        };
        var ex = Assert.Throws<ArgumentException>(
            () => AacChannelInterleaver.Interleave(ch, new float[6]));
        Assert.Contains("must be the same length", ex.Message);
    }

    [Fact]
    public void Interleave_DestinationTooSmall_Throws()
    {
        var ch = new[] { BuildChannel([1f, 2f, 3f, 4f]) };
        var ex = Assert.Throws<ArgumentException>(
            () => AacChannelInterleaver.Interleave(ch, new float[3]));
        Assert.Contains("shorter than required", ex.Message);
    }

    [Fact]
    public void Interleave_StereoSpanOverload_ProducesL_R_L_R()
    {
        var ch = new[]
        {
            BuildChannel([1f, 3f, 5f]),  // left
            BuildChannel([2f, 4f, 6f]),  // right
        };
        var dest = new float[6];
        AacChannelInterleaver.Interleave(ch, dest);
        Assert.Equal((float[])[1f, 2f, 3f, 4f, 5f, 6f], dest);
    }

    [Fact]
    public void Interleave_StereoAllocatingOverload_ReturnsCorrectLayout()
    {
        var ch = new[]
        {
            BuildChannel([1f, 3f]),
            BuildChannel([2f, 4f]),
        };
        var result = AacChannelInterleaver.Interleave(ch);
        Assert.Equal(4, result.Length);
        Assert.Equal((float[])[1f, 2f, 3f, 4f], result);
    }

    [Fact]
    public void Interleave_DecodedRawDataBlockOverload_Works()
    {
        var block = new AacDecodedRawDataBlock
        {
            Channels = new[]
            {
                BuildChannel([1f, 3f]),
                BuildChannel([2f, 4f]),
            },
        };
        var result = AacChannelInterleaver.Interleave(block);
        Assert.Equal((float[])[1f, 2f, 3f, 4f], result);
    }

    [Fact]
    public void Interleave_DecodedRawDataBlockSpanOverload_Works()
    {
        var block = new AacDecodedRawDataBlock
        {
            Channels = new[]
            {
                BuildChannel([10f, 30f]),
                BuildChannel([20f, 40f]),
            },
        };
        var dest = new float[4];
        AacChannelInterleaver.Interleave(block, dest);
        Assert.Equal((float[])[10f, 20f, 30f, 40f], dest);
    }

    [Fact]
    public void Interleave_DecodedRawDataBlock_NullBlock_Throws()
    {
        Assert.Throws<ArgumentNullException>(
            () => AacChannelInterleaver.Interleave((AacDecodedRawDataBlock)null!));
    }

    [Fact]
    public void Interleave_MonoOverwritesExtraneousBytes_LeavesOtherUnchanged()
    {
        var ch = new[] { BuildChannel([7f, 8f]) };
        var dest = new[] { 0f, 0f, 99f, 99f };
        AacChannelInterleaver.Interleave(ch, dest);
        Assert.Equal(7f, dest[0]);
        Assert.Equal(8f, dest[1]);
        // dest[2..] left untouched
        Assert.Equal(99f, dest[2]);
        Assert.Equal(99f, dest[3]);
    }

    [Fact]
    public void Interleave_FiveOne_LayoutMatchesChannelOrder()
    {
        // 5.1: FC, FL, FR, SL, SR, LFE
        var ch = new[]
        {
            BuildChannel([1f]),
            BuildChannel([2f]),
            BuildChannel([3f]),
            BuildChannel([4f]),
            BuildChannel([5f]),
            BuildChannel([6f]),
        };
        var result = AacChannelInterleaver.Interleave(ch);
        Assert.Equal((float[])[1f, 2f, 3f, 4f, 5f, 6f], result);
    }

    // ---- PCE variants ----

    [Fact]
    public void Interleave_Pce_StereoSpanOverload_ProducesInterleaved()
    {
        var ch = new[]
        {
            BuildPceChannel([1f, 3f], pairIndex: 0),
            BuildPceChannel([2f, 4f], pairIndex: 1),
        };
        var dest = new float[4];
        AacChannelInterleaver.Interleave(ch, dest);
        Assert.Equal((float[])[1f, 2f, 3f, 4f], dest);
    }

    [Fact]
    public void Interleave_Pce_AllocatingOverload_ReturnsCorrectLayout()
    {
        var ch = new[]
        {
            BuildPceChannel([1f], pairIndex: 0),
            BuildPceChannel([2f], pairIndex: 1),
        };
        Assert.Equal((float[])[1f, 2f], AacChannelInterleaver.Interleave(ch));
    }

    [Fact]
    public void Interleave_Pce_DecodedBlockOverload_Works()
    {
        var block = new AacPceDecodedRawDataBlock
        {
            Channels = new[]
            {
                BuildPceChannel([5f, 7f], pairIndex: 0),
                BuildPceChannel([6f, 8f], pairIndex: 1),
            },
        };
        var dest = new float[4];
        AacChannelInterleaver.Interleave(block, dest);
        Assert.Equal((float[])[5f, 6f, 7f, 8f], dest);
    }

    [Fact]
    public void Interleave_Pce_DecodedBlockAllocOverload_Works()
    {
        var block = new AacPceDecodedRawDataBlock
        {
            Channels = new[]
            {
                BuildPceChannel([9f], pairIndex: null),
            },
        };
        Assert.Equal((float[])[9f], AacChannelInterleaver.Interleave(block));
    }

    [Fact]
    public void Interleave_Pce_NullBlock_Throws()
    {
        Assert.Throws<ArgumentNullException>(
            () => AacChannelInterleaver.Interleave((AacPceDecodedRawDataBlock)null!));
    }

    [Fact]
    public void Interleave_Pce_MismatchedLength_Throws()
    {
        var ch = new[]
        {
            BuildPceChannel([1f, 2f], pairIndex: 0),
            BuildPceChannel([3f], pairIndex: 1),
        };
        Assert.Throws<ArgumentException>(
            () => AacChannelInterleaver.Interleave(ch, new float[4]));
    }

    [Fact]
    public void Interleave_Pce_DestinationTooSmall_Throws()
    {
        var ch = new[]
        {
            BuildPceChannel([1f, 2f], pairIndex: 0),
            BuildPceChannel([3f, 4f], pairIndex: 1),
        };
        var ex = Assert.Throws<ArgumentException>(
            () => AacChannelInterleaver.Interleave(ch, new float[3]));
        Assert.Contains("shorter than required", ex.Message);
    }

    // ---- helpers ----

    private static AacChannelOutput BuildChannel(float[] samples) => new()
    {
        Speaker = AacSpeaker.FrontCentre,
        Samples = samples,
    };

    private static AacPceChannelOutput BuildPceChannel(float[] samples, int? pairIndex) => new()
    {
        Region = AacPceChannelRegion.Front,
        RegionIndex = 0,
        PairIndex = pairIndex,
        Samples = samples,
    };
}
