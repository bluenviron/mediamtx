using System.Buffers.Binary;
using System.Text;
using Mediar.Imaging;
using Mediar.Imaging.Heif;
using Xunit;

namespace Mediar.Tests;

public class HeifOperatingPointsInformationTests
{
    [Fact]
    public void TryParse_Accepts_Minimal_Empty_Payload()
    {
        // version=0, flags=0 | scalability_mask=0 | num_ptl=0 | num_ops=0 | max_layer_count=0
        byte[] payload = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0];
        Assert.True(HeifOperatingPointsInformation.TryParse(payload, out var oinf));
        Assert.NotNull(oinf);
        Assert.Equal((ushort)0, oinf!.ScalabilityMask);
        Assert.Empty(oinf.ProfileTierLevels);
        Assert.Empty(oinf.OperatingPoints);
        Assert.Empty(oinf.LayerDependencies);
    }

    [Fact]
    public void TryParse_Accepts_Single_Ptl_Entry()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); // scalability_mask
        w.WriteByte(0x01); // num_ptl=1 (upper 2 bits reserved)
        // PTL: space=1, tier=1, idc=5; compat=0xAABBCCDD; constraint=0x010203040506; level=120
        w.WriteByte((byte)((1 << 6) | (1 << 5) | 5));
        w.WriteUInt32(0xAABBCCDDu);
        w.WriteBytes([0x01, 0x02, 0x03, 0x04, 0x05, 0x06]);
        w.WriteByte(120);
        w.WriteUInt16(0); // num_ops=0
        w.WriteByte(0); // max_layer_count=0

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Single(oinf!.ProfileTierLevels);
        var ptl = oinf.ProfileTierLevels[0];
        Assert.Equal((byte)1, ptl.ProfileSpace);
        Assert.True(ptl.TierFlag);
        Assert.Equal((byte)5, ptl.ProfileIdc);
        Assert.Equal(0xAABBCCDDu, ptl.ProfileCompatibilityFlags);
        Assert.Equal(0x010203040506UL, ptl.ConstraintIndicatorFlags);
        Assert.Equal((byte)120, ptl.LevelIdc);
    }

    [Fact]
    public void TryParse_Accepts_Operating_Point_Without_Optional_Rates()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); // mask
        w.WriteByte(0); // num_ptl
        w.WriteUInt16(1); // num_ops=1
        // OP: ols=2, tid=6, layer_count=1, [ptl_idx=0, layer_id=42, out=1, alt=0],
        //     minW=320, minH=240, maxW=1920, maxH=1080,
        //     chroma=1 bd=2 reserved=0 frFlag=0 brFlag=0
        w.WriteUInt16(2);
        w.WriteByte(6);
        w.WriteByte(1);
        w.WriteByte(0);
        w.WriteByte((byte)((42 << 2) | (1 << 1) | 0));
        w.WriteUInt16(320);
        w.WriteUInt16(240);
        w.WriteUInt16(1920);
        w.WriteUInt16(1080);
        w.WriteByte((byte)((1 << 6) | (2 << 3) | 0));
        w.WriteByte(0); // max_layer_count

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        var op = Assert.Single(oinf!.OperatingPoints);
        Assert.Equal((ushort)2, op.OutputLayerSetIndex);
        Assert.Equal((byte)6, op.MaxTemporalId);
        var layer = Assert.Single(op.Layers);
        Assert.Equal((byte)42, layer.LayerId);
        Assert.True(layer.IsOutputLayer);
        Assert.False(layer.IsAlternateOutputLayer);
        Assert.Equal((ushort)320, op.MinPicWidth);
        Assert.Equal((ushort)1080, op.MaxPicHeight);
        Assert.Equal((byte)1, op.MaxChromaFormat);
        Assert.Equal((byte)2, op.MaxBitDepth);
        Assert.Null(op.AvgFrameRate);
        Assert.Null(op.MaxBitRate);
    }

    [Fact]
    public void TryParse_Captures_Frame_Rate_And_Bit_Rate_When_Flags_Set()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0);
        w.WriteByte(0);
        w.WriteUInt16(1);
        w.WriteUInt16(0);
        w.WriteByte(0);
        w.WriteByte(0); // layer_count=0
        w.WriteUInt16(640); w.WriteUInt16(480);
        w.WriteUInt16(640); w.WriteUInt16(480);
        // chroma=1 bd=0 reserved=0 frFlag=1 brFlag=1
        w.WriteByte((byte)((1 << 6) | (0 << 3) | (1 << 1) | 1));
        // frame-rate block: avgFr=7680 (30fps * 256), constFr=2
        w.WriteUInt16(7680);
        w.WriteByte(0x02);
        // bit-rate block: maxBr=2_000_000, avgBr=1_500_000
        w.WriteUInt32(2_000_000u);
        w.WriteUInt32(1_500_000u);
        w.WriteByte(0); // max_layer_count

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        var op = Assert.Single(oinf!.OperatingPoints);
        Assert.Equal((ushort)7680, op.AvgFrameRate);
        Assert.Equal((byte)2, op.ConstantFrameRate);
        Assert.Equal(2_000_000u, op.MaxBitRate);
        Assert.Equal(1_500_000u, op.AvgBitRate);
    }

    [Fact]
    public void TryParse_Reads_Dimension_Identifiers_Per_Set_Scalability_Bit()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0x000B); // bits 0,1,3 set => popcount=3
        w.WriteByte(0);
        w.WriteUInt16(0);
        w.WriteByte(1); // max_layer_count=1
        w.WriteByte(0xAA); // dependent_layerID
        w.WriteByte(2);    // num_layers_dependent_on
        w.WriteByte(0xB1); // dep0
        w.WriteByte(0xB2); // dep1
        w.WriteByte(0xC1); // dim0
        w.WriteByte(0xC2); // dim1
        w.WriteByte(0xC3); // dim2

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Equal((ushort)0x000B, oinf!.ScalabilityMask);
        var dep = Assert.Single(oinf.LayerDependencies);
        Assert.Equal((byte)0xAA, dep.DependentLayerId);
        byte[] expectedDeps = [0xB1, 0xB2];
        Assert.Equal(expectedDeps, dep.DependsOnLayerIds);
        byte[] expectedDims = [0xC1, 0xC2, 0xC3];
        Assert.Equal(expectedDims, dep.DimensionIdentifiers);
    }

    [Fact]
    public void TryParse_Rejects_Wrong_Version()
    {
        byte[] payload = [1, 0, 0, 0, 0, 0, 0, 0, 0, 0];
        Assert.False(HeifOperatingPointsInformation.TryParse(payload, out var oinf));
        Assert.Null(oinf);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Header()
    {
        byte[] payload = [0, 0, 0, 0, 0, 0, 0, 0, 0]; // 9 bytes
        Assert.False(HeifOperatingPointsInformation.TryParse(payload, out var oinf));
        Assert.Null(oinf);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Ptl_Entry()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0);
        w.WriteByte(1); // num_ptl=1
        // missing 12 PTL bytes
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Null(oinf);
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Frame_Rate_Block()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(0);
        w.WriteUInt16(0); w.WriteUInt16(0); w.WriteUInt16(0); w.WriteUInt16(0);
        w.WriteByte((byte)((1 << 1))); // frFlag=1 set
        w.WriteByte(0); w.WriteByte(0); // only 2 bytes of the required 3
        // missing max_layer_count entirely
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Null(oinf);
    }

    [Fact]
    public void HeifReader_TryGetOperatingPointsInformation_Roundtrips()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); // mask=0
        w.WriteByte(1);    // num_ptl=1
        w.WriteByte(0); w.WriteUInt32(0); w.WriteBytes([0, 0, 0, 0, 0, 0]); w.WriteByte(60);
        w.WriteUInt16(0); w.WriteByte(0); // num_ops=0, max_layer_count=0

        var bytes = BuildHeifWithProperty("oinf", w.ToArray());
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);

        Assert.True(r.TryGetOperatingPointsInformation(1, out var oinf));
        Assert.Single(oinf.ProfileTierLevels);
        Assert.Equal((byte)60, oinf.ProfileTierLevels[0].LevelIdc);
    }

    [Fact]
    public void HeifReader_TryGetOperatingPointsInformation_Returns_False_When_Missing()
    {
        var bytes = BuildHeifWithProperty(null, null);
        using var r = HeifReader.Open(new MemoryStream(bytes), ImageFormat.Heif, ownsStream: true);
        Assert.False(r.TryGetOperatingPointsInformation(1, out var oinf));
        Assert.Null(oinf);
    }

    [Fact]
    public void TryParse_Accepts_Multiple_Ptl_Entries()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0);
        w.WriteByte(3); // num_ptl=3
        // PTL 0: space=0, tier=0, idc=1
        w.WriteByte(0x01); w.WriteUInt32(0x11111111u);
        w.WriteBytes([0, 0, 0, 0, 0, 0]); w.WriteByte(30);
        // PTL 1: space=2, tier=1, idc=2
        w.WriteByte((byte)((2 << 6) | (1 << 5) | 2));
        w.WriteUInt32(0x22222222u);
        w.WriteBytes([1, 2, 3, 4, 5, 6]); w.WriteByte(60);
        // PTL 2: space=3, tier=0, idc=7
        w.WriteByte((byte)((3 << 6) | 7));
        w.WriteUInt32(0x33333333u);
        w.WriteBytes([0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA]); w.WriteByte(180);
        w.WriteUInt16(0); // num_ops
        w.WriteByte(0); // max_layer_count

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Equal(3, oinf!.ProfileTierLevels.Length);
        Assert.Equal((byte)0, oinf.ProfileTierLevels[0].ProfileSpace);
        Assert.False(oinf.ProfileTierLevels[0].TierFlag);
        Assert.Equal((byte)1, oinf.ProfileTierLevels[0].ProfileIdc);
        Assert.Equal((byte)2, oinf.ProfileTierLevels[1].ProfileSpace);
        Assert.True(oinf.ProfileTierLevels[1].TierFlag);
        Assert.Equal((byte)2, oinf.ProfileTierLevels[1].ProfileIdc);
        Assert.Equal((byte)3, oinf.ProfileTierLevels[2].ProfileSpace);
        Assert.False(oinf.ProfileTierLevels[2].TierFlag);
        Assert.Equal(0xFFFEFDFCFBFAUL, oinf.ProfileTierLevels[2].ConstraintIndicatorFlags);
    }

    [Fact]
    public void TryParse_Ignores_Upper_Two_Reserved_Bits_Of_NumPtl()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0);
        // num_ptl byte with upper 2 reserved bits set; lower 6 bits = 1
        w.WriteByte(0xC1);
        w.WriteByte(0); w.WriteUInt32(0); w.WriteBytes([0, 0, 0, 0, 0, 0]); w.WriteByte(60);
        w.WriteUInt16(0);
        w.WriteByte(0);

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Single(oinf!.ProfileTierLevels);
    }

    [Fact]
    public void TryParse_Accepts_Operating_Point_With_BitRate_Only()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(0);
        w.WriteUInt16(1); w.WriteUInt16(2); w.WriteUInt16(3); w.WriteUInt16(4);
        // chroma=0 bd=0 reserved=0 frFlag=0 brFlag=1
        w.WriteByte(0x01);
        w.WriteUInt32(99u); w.WriteUInt32(55u);
        w.WriteByte(0);

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        var op = Assert.Single(oinf!.OperatingPoints);
        Assert.Null(op.AvgFrameRate);
        Assert.Null(op.ConstantFrameRate);
        Assert.Equal(99u, op.MaxBitRate);
        Assert.Equal(55u, op.AvgBitRate);
    }

    [Fact]
    public void TryParse_Accepts_Operating_Point_With_FrameRate_Only()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(0);
        w.WriteUInt16(1); w.WriteUInt16(2); w.WriteUInt16(3); w.WriteUInt16(4);
        // chroma=3 bd=7 reserved=0 frFlag=1 brFlag=0
        w.WriteByte((byte)((3 << 6) | (7 << 3) | (1 << 1)));
        w.WriteUInt16(0x1234); w.WriteByte(0xFF); // constFr should mask to lower 2 bits = 0x3
        w.WriteByte(0);

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        var op = Assert.Single(oinf!.OperatingPoints);
        Assert.Equal((ushort)0x1234, op.AvgFrameRate);
        Assert.Equal((byte)0x3, op.ConstantFrameRate); // 0xFF & 0x3 = 0x3
        Assert.Null(op.MaxBitRate);
        Assert.Null(op.AvgBitRate);
        Assert.Equal((byte)3, op.MaxChromaFormat);
        Assert.Equal((byte)7, op.MaxBitDepth);
    }

    [Fact]
    public void TryParse_Accepts_Multiple_Operating_Points()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0);
        w.WriteUInt16(2); // num_ops=2

        // OP 0: layer_count=0
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(0);
        w.WriteUInt16(100); w.WriteUInt16(101); w.WriteUInt16(200); w.WriteUInt16(201);
        w.WriteByte(0);

        // OP 1: layer_count=2
        w.WriteUInt16(7); w.WriteByte(3); w.WriteByte(2);
        w.WriteByte(0); w.WriteByte((byte)((10 << 2) | (1 << 1)));
        w.WriteByte(0); w.WriteByte((byte)((20 << 2) | 1));
        w.WriteUInt16(50); w.WriteUInt16(51); w.WriteUInt16(60); w.WriteUInt16(61);
        w.WriteByte(0);
        w.WriteByte(0);

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Equal(2, oinf!.OperatingPoints.Length);
        Assert.Empty(oinf.OperatingPoints[0].Layers);
        Assert.Equal(2, oinf.OperatingPoints[1].Layers.Length);
        Assert.Equal((byte)10, oinf.OperatingPoints[1].Layers[0].LayerId);
        Assert.True(oinf.OperatingPoints[1].Layers[0].IsOutputLayer);
        Assert.False(oinf.OperatingPoints[1].Layers[0].IsAlternateOutputLayer);
        Assert.Equal((byte)20, oinf.OperatingPoints[1].Layers[1].LayerId);
        Assert.False(oinf.OperatingPoints[1].Layers[1].IsOutputLayer);
        Assert.True(oinf.OperatingPoints[1].Layers[1].IsAlternateOutputLayer);
    }

    [Fact]
    public void TryParse_Accepts_Multiple_Layer_Dependencies()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0x0001); // popcount=1
        w.WriteByte(0); w.WriteUInt16(0);
        w.WriteByte(2); // max_layer_count=2
        // dep 0
        w.WriteByte(0x10); w.WriteByte(1); w.WriteByte(0x05); w.WriteByte(0xAA);
        // dep 1
        w.WriteByte(0x20); w.WriteByte(0); w.WriteByte(0xBB);

        Assert.True(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out var oinf));
        Assert.Equal(2, oinf!.LayerDependencies.Length);
        Assert.Equal((byte)0x10, oinf.LayerDependencies[0].DependentLayerId);
        Assert.Equal(new byte[] { 0x05 }, oinf.LayerDependencies[0].DependsOnLayerIds.ToArray());
        Assert.Equal(new byte[] { 0xAA }, oinf.LayerDependencies[0].DimensionIdentifiers.ToArray());
        Assert.Equal((byte)0x20, oinf.LayerDependencies[1].DependentLayerId);
        Assert.Empty(oinf.LayerDependencies[1].DependsOnLayerIds);
        Assert.Equal(new byte[] { 0xBB }, oinf.LayerDependencies[1].DimensionIdentifiers.ToArray());
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Before_Numptl_Byte()
    {
        // header (4) + scalabilityMask (2) = 6 bytes, no num_ptl byte
        byte[] payload = [0, 0, 0, 0, 0, 0];
        Assert.False(HeifOperatingPointsInformation.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Before_NumOps()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0);
        w.WriteByte(0); // num_ptl=0
        // missing num_ops (2 bytes)
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_OperatingPoint_Header()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        // only 3 bytes of the required 4 OP header bytes
        w.WriteUInt16(0); w.WriteByte(0);
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Layer_Entry()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(1); // layer_count=1
        w.WriteByte(0); // only ptlIdx, no layer byte
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_OperatingPoint_Dimensions()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(0); // layer_count=0
        w.WriteUInt16(0); w.WriteUInt16(0); // only 4 of the required 9 bytes
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_BitRate_Block()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(1);
        w.WriteUInt16(0); w.WriteByte(0); w.WriteByte(0);
        w.WriteUInt16(0); w.WriteUInt16(0); w.WriteUInt16(0); w.WriteUInt16(0);
        w.WriteByte(0x01); // brFlag=1
        // only 4 of the required 8 bit-rate bytes
        w.WriteUInt32(0u);
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_LayerDep_Header()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(0);
        w.WriteByte(1); // max_layer_count=1
        w.WriteByte(0); // only depId, no numDeps byte
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_LayerDep_DepsList()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0); w.WriteByte(0); w.WriteUInt16(0);
        w.WriteByte(1); // max_layer_count=1
        w.WriteByte(0); w.WriteByte(2); // depId, numDeps=2
        w.WriteByte(0); // only 1 of the required 2 dep bytes
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_LayerDep_Dimensions()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0x0007); // popcount=3
        w.WriteByte(0); w.WriteUInt16(0);
        w.WriteByte(1);
        w.WriteByte(0); w.WriteByte(0); // depId, numDeps=0
        w.WriteByte(0); w.WriteByte(0); // only 2 of the required 3 dimension bytes
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Fact]
    public void TryParse_Rejects_Truncated_Before_MaxLayerCount()
    {
        var w = new PayloadWriter();
        w.WriteFullBoxHeader();
        w.WriteUInt16(0);
        w.WriteByte(0); // num_ptl=0
        w.WriteUInt16(0); // num_ops=0
        // missing max_layer_count byte
        Assert.False(HeifOperatingPointsInformation.TryParse(w.ToSpan(), out _));
    }

    [Theory]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(0xFF)]
    public void TryParse_Rejects_Nonzero_FullBox_Version(byte version)
    {
        byte[] payload = [version, 0, 0, 0, 0, 0, 0, 0, 0, 0];
        Assert.False(HeifOperatingPointsInformation.TryParse(payload, out _));
    }

    [Fact]
    public void TryParse_Flag_Bytes_Are_Ignored()
    {
        // FullBox flags should not affect parsing.
        byte[] payload = [0, 0xAB, 0xCD, 0xEF, 0, 0, 0, 0, 0, 0];
        Assert.True(HeifOperatingPointsInformation.TryParse(payload, out var oinf));
        Assert.NotNull(oinf);
    }

    [Fact]
    public void Record_Equality_And_With_Expression()
    {
        byte[] payload = [0, 0, 0, 0, 0, 0, 0, 0, 0, 0];
        Assert.True(HeifOperatingPointsInformation.TryParse(payload, out var a));
        Assert.True(HeifOperatingPointsInformation.TryParse(payload, out var b));
        Assert.Equal(a, b);
        Assert.Equal(a!.GetHashCode(), b!.GetHashCode());

        var c = a with { ScalabilityMask = 0xBEEF };
        Assert.NotEqual(a, c);
        Assert.Equal((ushort)0xBEEF, c.ScalabilityMask);
    }

    [Fact]
    public void HeifLhevcProfileTierLevel_Record_Equality()
    {
        var a = new HeifLhevcProfileTierLevel
        {
            ProfileSpace = 1, TierFlag = true, ProfileIdc = 2,
            ProfileCompatibilityFlags = 0xDEAD, ConstraintIndicatorFlags = 0xBEEF, LevelIdc = 30,
        };
        var b = a with { LevelIdc = 60 };
        Assert.NotEqual(a, b);
        Assert.Equal((byte)60, b.LevelIdc);
        Assert.Equal((byte)30, a.LevelIdc);
    }

    [Fact]
    public void HeifLhevcOpLayer_Record_Equality()
    {
        var a = new HeifLhevcOpLayer
        {
            PtlIndex = 0, LayerId = 5, IsOutputLayer = true, IsAlternateOutputLayer = false,
        };
        var b = a with { IsAlternateOutputLayer = true };
        Assert.NotEqual(a, b);
        Assert.True(b.IsAlternateOutputLayer);
        Assert.False(a.IsAlternateOutputLayer);
    }

    [Fact]
    public void HeifLhevcLayerDependency_Record_Equality()
    {
        var a = new HeifLhevcLayerDependency
        {
            DependentLayerId = 1,
            DependsOnLayerIds = System.Collections.Immutable.ImmutableArray.Create<byte>(0),
            DimensionIdentifiers = System.Collections.Immutable.ImmutableArray<byte>.Empty,
        };
        var b = a with { DependentLayerId = 2 };
        Assert.NotEqual(a, b);
        Assert.Equal((byte)2, b.DependentLayerId);
    }

    // ---------- helpers ----------

    private sealed class PayloadWriter : IDisposable
    {
        private readonly MemoryStream _ms = new();

        public void WriteFullBoxHeader()
        {
            Span<byte> b = stackalloc byte[4];
            _ms.Write(b);
        }

        public void WriteByte(byte b) => _ms.WriteByte(b);

        public void WriteBytes(ReadOnlySpan<byte> bytes) => _ms.Write(bytes);

        public void WriteUInt16(ushort v)
        {
            Span<byte> b = stackalloc byte[2];
            BinaryPrimitives.WriteUInt16BigEndian(b, v);
            _ms.Write(b);
        }

        public void WriteUInt32(uint v)
        {
            Span<byte> b = stackalloc byte[4];
            BinaryPrimitives.WriteUInt32BigEndian(b, v);
            _ms.Write(b);
        }

        public ReadOnlySpan<byte> ToSpan() => _ms.ToArray();
        public byte[] ToArray() => _ms.ToArray();

        public void Dispose() => _ms.Dispose();
    }

    private static byte[] BuildHeifWithProperty(string? propertyType, byte[]? propertyPayload)
    {
        using var ms = new MemoryStream();
        WriteBox(ms, "ftyp", w =>
        {
            w.Write(Encoding.ASCII.GetBytes("heic"));
            Span<byte> minor = stackalloc byte[4];
            w.Write(minor);
            w.Write(Encoding.ASCII.GetBytes("mif1"));
            w.Write(Encoding.ASCII.GetBytes("heic"));
        });
        WriteBox(ms, "meta", meta =>
        {
            Span<byte> vf = stackalloc byte[4];
            meta.Write(vf);
            WriteBox(meta, "hdlr", h =>
            {
                Span<byte> b = stackalloc byte[25];
                Encoding.ASCII.GetBytes("pict").CopyTo(b.Slice(8));
                h.Write(b);
            });
            WriteBox(meta, "pitm", h =>
            {
                Span<byte> b = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(b.Slice(4, 2), 1);
                h.Write(b);
            });
            WriteBox(meta, "iinf", h =>
            {
                Span<byte> hdr = stackalloc byte[6];
                BinaryPrimitives.WriteUInt16BigEndian(hdr.Slice(4, 2), 1);
                h.Write(hdr);
                WriteBox(h, "infe", inf =>
                {
                    Span<byte> data = stackalloc byte[15];
                    data[0] = 2;
                    BinaryPrimitives.WriteUInt16BigEndian(data.Slice(4, 2), 1);
                    Encoding.ASCII.GetBytes("hvc1").CopyTo(data.Slice(8));
                    inf.Write(data);
                });
            });
            WriteBox(meta, "iprp", iprp =>
            {
                WriteBox(iprp, "ipco", ipco =>
                {
                    WriteBox(ipco, "ispe", isp =>
                    {
                        Span<byte> data = stackalloc byte[12];
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(4, 4), 64);
                        BinaryPrimitives.WriteUInt32BigEndian(data.Slice(8, 4), 64);
                        isp.Write(data);
                    });
                    if (propertyType is not null && propertyPayload is not null)
                    {
                        WriteBox(ipco, propertyType, p => p.Write(propertyPayload));
                    }
                });
                WriteBox(iprp, "ipma", ipma =>
                {
                    Span<byte> hdr = stackalloc byte[8];
                    BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(4, 4), 1);
                    ipma.Write(hdr);
                    int assocCount = propertyType is null ? 1 : 2;
                    Span<byte> entry = stackalloc byte[3 + assocCount];
                    BinaryPrimitives.WriteUInt16BigEndian(entry.Slice(0, 2), 1);
                    entry[2] = (byte)assocCount;
                    for (int i = 0; i < assocCount; i++) entry[3 + i] = (byte)(i + 1);
                    ipma.Write(entry);
                });
            });
        });
        return ms.ToArray();
    }

    private static void WriteBox(Stream s, string type, Action<MemoryStream> writePayload)
    {
        using var inner = new MemoryStream();
        writePayload(inner);
        var payload = inner.ToArray();
        int total = payload.Length + 8;
        Span<byte> hdr = stackalloc byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(0, 4), (uint)total);
        Encoding.ASCII.GetBytes(type).CopyTo(hdr.Slice(4, 4));
        s.Write(hdr);
        s.Write(payload);
    }
}
