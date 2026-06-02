using System.Buffers.Binary;
using Mediar.Containers.IsoBmff;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Coverage for the <see cref="OpusHead"/> helper and its wire-up through the
/// MP4 muxer/demuxer (Opus Specific Box, <c>dOps</c>). The helper is the
/// single point of truth for converting between Ogg-form <c>OpusHead</c> and
/// ISOBMFF-form <c>dOps</c>, so we exercise:
/// <list type="bullet">
///   <item><description>Round-tripping each form (family=0 and family=1).</description></item>
///   <item><description>The byte-layout differences (LE vs BE, magic vs no magic, version 1 vs 0).</description></item>
///   <item><description>Cross-form conversion preserves all field values.</description></item>
///   <item><description>Reader rejects malformed input.</description></item>
///   <item><description>End-to-end MP4 mux → demux preserves <c>ExtraData</c>.</description></item>
///   <item><description>The muxer's sample-entry sample rate quirk (always 48000 for Opus).</description></item>
/// </list>
/// </summary>
public sealed class OpusHeadTests
{
    // ----- helper builders -----

    private static OpusHead StereoFamily0() => new()
    {
        ChannelCount = 2,
        PreSkip = 312,
        InputSampleRate = 48000,
        OutputGain = 0,
        ChannelMappingFamily = 0,
    };

    private static OpusHead Surround51Family1() => new()
    {
        ChannelCount = 6,
        PreSkip = 0x1234,
        InputSampleRate = 44100,
        OutputGain = -512,
        ChannelMappingFamily = 1,
        StreamCount = 4,
        CoupledCount = 2,
        // Stream-list indices for the 6 output channels; values are 0..(stream+coupled-1)
        // or 0xFF for a silent channel. Here we include one silent channel to ensure
        // 0xFF survives the round-trip.
        ChannelMapping = new byte[] { 0, 4, 1, 2, 3, 0xFF },
    };

    // ----- size + byte count -----

    [Fact]
    public void OggByteCount_Family0_Is_19()
    {
        Assert.Equal(19, StereoFamily0().OggByteCount);
    }

    [Fact]
    public void OggByteCount_Family1_Includes_Mapping_Table()
    {
        // 19 base + 2 (stream + coupled) + 6 (mapping bytes).
        Assert.Equal(19 + 2 + 6, Surround51Family1().OggByteCount);
    }

    [Fact]
    public void IsobmffByteCount_Family0_Is_11()
    {
        Assert.Equal(11, StereoFamily0().IsobmffByteCount);
    }

    [Fact]
    public void IsobmffByteCount_Family1_Includes_Mapping_Table()
    {
        Assert.Equal(11 + 2 + 6, Surround51Family1().IsobmffByteCount);
    }

    // ----- format-specific version bytes -----

    [Fact]
    public void WriteOgg_Emits_Magic_And_Version_1()
    {
        byte[] bytes = OpusHead.WriteOgg(StereoFamily0());
        Assert.Equal((byte)'O', bytes[0]);
        Assert.True(bytes.AsSpan(0, 8).SequenceEqual("OpusHead"u8));
        Assert.Equal(1, bytes[8]);
    }

    [Fact]
    public void WriteIsobmff_Has_No_Magic_And_Version_0()
    {
        byte[] bytes = OpusHead.WriteIsobmff(StereoFamily0());
        // dOps body begins directly with the 1-byte version; no "OpusHead" magic.
        Assert.Equal(0, bytes[0]);
        Assert.NotEqual((byte)'O', bytes[0]);
    }

    // ----- endianness -----

    [Fact]
    public void WriteOgg_Uses_LittleEndian_For_Multibyte_Fields()
    {
        var head = new OpusHead
        {
            ChannelCount = 1, PreSkip = 0x1234,
            InputSampleRate = 0xAABBCCDD, OutputGain = 0x4321,
            ChannelMappingFamily = 0,
        };
        // family=0 + channels=1 is invalid per spec, so use channels=1 with family=0
        // (channel-count 1 IS valid for family=0).
        byte[] bytes = OpusHead.WriteOgg(head);
        Assert.Equal(0x34, bytes[10]); // pre-skip lo
        Assert.Equal(0x12, bytes[11]); // pre-skip hi
        Assert.Equal(0xDD, bytes[12]); // input-rate byte0
        Assert.Equal(0xAA, bytes[15]); // input-rate byte3
        Assert.Equal(0x21, bytes[16]); // gain lo
        Assert.Equal(0x43, bytes[17]); // gain hi
    }

    [Fact]
    public void WriteIsobmff_Uses_BigEndian_For_Multibyte_Fields()
    {
        var head = new OpusHead
        {
            ChannelCount = 1, PreSkip = 0x1234,
            InputSampleRate = 0xAABBCCDD, OutputGain = 0x4321,
            ChannelMappingFamily = 0,
        };
        byte[] bytes = OpusHead.WriteIsobmff(head);
        Assert.Equal(0x12, bytes[2]); // pre-skip hi
        Assert.Equal(0x34, bytes[3]); // pre-skip lo
        Assert.Equal(0xAA, bytes[4]); // input-rate byte0 (MSB)
        Assert.Equal(0xDD, bytes[7]); // input-rate byte3 (LSB)
        Assert.Equal(0x43, bytes[8]); // gain hi
        Assert.Equal(0x21, bytes[9]); // gain lo
    }

    // ----- round-trips -----

    [Fact]
    public void OggRoundTrip_Family0_Preserves_All_Fields()
    {
        var original = new OpusHead
        {
            ChannelCount = 2, PreSkip = 312,
            InputSampleRate = 48000, OutputGain = -256,
            ChannelMappingFamily = 0,
        };
        byte[] bytes = OpusHead.WriteOgg(original);
        Assert.Equal(19, bytes.Length);
        Assert.True(OpusHead.TryReadOgg(bytes, out var parsed));
        AssertEqualExceptMapping(original, parsed);
        Assert.True(parsed.ChannelMapping.IsEmpty);
    }

    [Fact]
    public void OggRoundTrip_Family1_Preserves_Mapping_Table()
    {
        var original = Surround51Family1();
        byte[] bytes = OpusHead.WriteOgg(original);
        Assert.True(OpusHead.TryReadOgg(bytes, out var parsed));
        AssertEqualExceptMapping(original, parsed);
        Assert.True(parsed.ChannelMapping.Span.SequenceEqual(original.ChannelMapping.Span));
        // Silent-channel marker survives.
        Assert.Equal(0xFF, parsed.ChannelMapping.Span[5]);
    }

    [Fact]
    public void IsobmffRoundTrip_Family0_Preserves_All_Fields()
    {
        var original = new OpusHead
        {
            ChannelCount = 1, PreSkip = 80,
            InputSampleRate = 16000, OutputGain = 256,
            ChannelMappingFamily = 0,
        };
        byte[] bytes = OpusHead.WriteIsobmff(original);
        Assert.Equal(11, bytes.Length);
        Assert.True(OpusHead.TryReadIsobmff(bytes, out var parsed));
        AssertEqualExceptMapping(original, parsed);
    }

    [Fact]
    public void IsobmffRoundTrip_Family1_Preserves_Mapping_Table()
    {
        var original = Surround51Family1();
        byte[] bytes = OpusHead.WriteIsobmff(original);
        Assert.True(OpusHead.TryReadIsobmff(bytes, out var parsed));
        AssertEqualExceptMapping(original, parsed);
        Assert.True(parsed.ChannelMapping.Span.SequenceEqual(original.ChannelMapping.Span));
    }

    [Fact]
    public void CrossFormat_OggToIsobmffToOgg_Roundtrips_ByteForByte()
    {
        // The canonical Ogg form is what AudioCodecParameters.ExtraData holds.
        // Going through dOps and back must reproduce the same bytes.
        byte[] originalOgg = OpusHead.WriteOgg(Surround51Family1());
        Assert.True(OpusHead.TryReadOgg(originalOgg, out var fromOgg));
        byte[] dops = OpusHead.WriteIsobmff(fromOgg);
        Assert.True(OpusHead.TryReadIsobmff(dops, out var fromDops));
        byte[] roundtrippedOgg = OpusHead.WriteOgg(fromDops);
        Assert.Equal(originalOgg, roundtrippedOgg);
    }

    // ----- writer span variant -----

    [Fact]
    public void WriteOgg_Span_Returns_Byte_Count()
    {
        byte[] buffer = new byte[64];
        int written = OpusHead.WriteOgg(Surround51Family1(), buffer);
        Assert.Equal(Surround51Family1().OggByteCount, written);
    }

    [Fact]
    public void WriteOgg_Span_Throws_When_Destination_Too_Small()
    {
        byte[] buffer = new byte[10];
        Assert.Throws<ArgumentException>(() => OpusHead.WriteOgg(StereoFamily0(), buffer));
    }

    [Fact]
    public void WriteIsobmff_Span_Throws_When_Destination_Too_Small()
    {
        byte[] buffer = new byte[5];
        Assert.Throws<ArgumentException>(() => OpusHead.WriteIsobmff(StereoFamily0(), buffer));
    }

    // ----- reader rejection paths -----

    [Fact]
    public void TryReadOgg_Rejects_Too_Short()
    {
        Assert.False(OpusHead.TryReadOgg(new byte[18], out _));
    }

    [Fact]
    public void TryReadOgg_Rejects_Bad_Magic()
    {
        byte[] bytes = OpusHead.WriteOgg(StereoFamily0());
        bytes[0] = (byte)'X';
        Assert.False(OpusHead.TryReadOgg(bytes, out _));
    }

    [Fact]
    public void TryReadOgg_Rejects_Version_With_NonZero_High_Nibble()
    {
        byte[] bytes = OpusHead.WriteOgg(StereoFamily0());
        bytes[8] = 0x10; // Opus major version 1 → not recognised
        Assert.False(OpusHead.TryReadOgg(bytes, out _));
    }

    [Fact]
    public void TryReadOgg_Accepts_Low_Nibble_Variation_In_Version_Byte()
    {
        // Per RFC 7845, readers MUST accept any minor version (low nibble) as long
        // as the major version (high nibble) is 0.
        byte[] bytes = OpusHead.WriteOgg(StereoFamily0());
        bytes[8] = 0x05;
        Assert.True(OpusHead.TryReadOgg(bytes, out _));
    }

    [Fact]
    public void TryReadOgg_Rejects_Zero_Channels()
    {
        byte[] bytes = OpusHead.WriteOgg(StereoFamily0());
        bytes[9] = 0;
        Assert.False(OpusHead.TryReadOgg(bytes, out _));
    }

    [Fact]
    public void TryReadOgg_Rejects_Family0_With_More_Than_2_Channels()
    {
        var head = new OpusHead { ChannelCount = 2, ChannelMappingFamily = 0, InputSampleRate = 48000 };
        byte[] bytes = OpusHead.WriteOgg(head);
        bytes[9] = 3; // family 0 only allows 1 or 2 channels
        Assert.False(OpusHead.TryReadOgg(bytes, out _));
    }

    [Fact]
    public void TryReadOgg_Rejects_Family1_Missing_Mapping_Table()
    {
        // Take the family=1 6-channel surround head, then truncate the mapping table.
        byte[] bytes = OpusHead.WriteOgg(Surround51Family1());
        byte[] truncated = new byte[bytes.Length - 1];
        bytes.AsSpan(0, truncated.Length).CopyTo(truncated);
        Assert.False(OpusHead.TryReadOgg(truncated, out _));
    }

    [Fact]
    public void TryReadOgg_Rejects_Family1_With_OutOfRange_Mapping_Byte()
    {
        byte[] bytes = OpusHead.WriteOgg(Surround51Family1());
        // Stream+coupled = 4+2 = 6; any value >= 6 (except 0xFF) is invalid.
        bytes[21] = 7;
        Assert.False(OpusHead.TryReadOgg(bytes, out _));
    }

    [Fact]
    public void TryReadIsobmff_Rejects_Too_Short()
    {
        Assert.False(OpusHead.TryReadIsobmff(new byte[10], out _));
    }

    [Fact]
    public void TryReadIsobmff_Rejects_NonZero_Version()
    {
        byte[] bytes = OpusHead.WriteIsobmff(StereoFamily0());
        bytes[0] = 1;
        Assert.False(OpusHead.TryReadIsobmff(bytes, out _));
    }

    [Fact]
    public void TryReadIsobmff_Rejects_Zero_Channels()
    {
        byte[] bytes = OpusHead.WriteIsobmff(StereoFamily0());
        bytes[1] = 0;
        Assert.False(OpusHead.TryReadIsobmff(bytes, out _));
    }

    [Fact]
    public void TryReadIsobmff_Rejects_Family0_With_3_Channels()
    {
        byte[] bytes = OpusHead.WriteIsobmff(StereoFamily0());
        bytes[1] = 3;
        Assert.False(OpusHead.TryReadIsobmff(bytes, out _));
    }

    // ----- MP4 end-to-end wiring -----

    [Fact]
    public async Task Mp4Muxer_Throws_When_Opus_ExtraData_Is_Empty()
    {
        // The MP4 muxer cannot produce a conformant Opus track without an OpusHead;
        // surface a clear error rather than emit a non-conformant file. The sample
        // table (stsd, where dOps lives) is built during FinishAsync, so that is
        // where the throw surfaces.
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus, SampleRate = 48000, Channels = 2,
                ExtraData = ReadOnlyMemory<byte>.Empty,
            },
            TimeBase = new Rational(1, 48000),
        });
        await mux.StartAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.FinishAsync());
    }

    [Fact]
    public async Task Mp4Muxer_Throws_When_Opus_ExtraData_Is_Malformed()
    {
        await using var ms = new MemoryStream();
        await using var mux = new Mp4Muxer(ms, leaveOpen: true);
        mux.AddTrack(new MediaTrack
        {
            Index = 0, Id = 1,
            Codec = new AudioCodecParameters
            {
                Codec = CodecId.Opus, SampleRate = 48000, Channels = 2,
                // Wrong magic so TryReadOgg returns false.
                ExtraData = new byte[19],
            },
            TimeBase = new Rational(1, 48000),
        });
        await mux.StartAsync();
        await Assert.ThrowsAsync<InvalidOperationException>(async () => await mux.FinishAsync());
    }

    [Fact]
    public async Task Mp4_OpusRoundTrip_Preserves_ExtraData_ByteForByte()
    {
        var head = new OpusHead
        {
            ChannelCount = 2, PreSkip = 312,
            InputSampleRate = 44100,   // intentionally non-48000 to exercise the override
            OutputGain = -128,
            ChannelMappingFamily = 0,
        };
        byte[] originalExtra = OpusHead.WriteOgg(head);

        byte[] bytes;
        await using (var ms = new MemoryStream())
        await using (var mux = new Mp4Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.Opus, SampleRate = 48000, Channels = 2,
                    ExtraData = originalExtra,
                },
                TimeBase = new Rational(1, 48000),
            });
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960,
                IsKeyFrame = true, Data = new byte[] { 0xFC, 0xDE, 0xAD },
            });
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        using var src = new MemoryRandomAccessSource(bytes);
        using var dx = new Mp4Demuxer(src);
        var t = Assert.Single(dx.Tracks);
        var a = Assert.IsType<AudioCodecParameters>(t.Codec);
        Assert.Equal(CodecId.Opus, a.Codec);
        Assert.Equal(originalExtra, a.ExtraData.ToArray());
    }

    [Fact]
    public async Task Mp4Muxer_Sets_SampleEntry_SampleRate_To_48000_For_Opus()
    {
        // Per the Opus-in-ISOBMFF spec, the AudioSampleEntry samplerate MUST be
        // 48000<<16 regardless of OpusHead's InputSampleRate. Build a track with
        // a non-48000 InputSampleRate, mux it, and verify the on-disk bytes.
        var head = new OpusHead
        {
            ChannelCount = 1, PreSkip = 0,
            InputSampleRate = 24000, OutputGain = 0,
            ChannelMappingFamily = 0,
        };

        byte[] bytes;
        await using (var ms = new MemoryStream())
        await using (var mux = new Mp4Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.Opus, SampleRate = 24000, Channels = 1,
                    ExtraData = OpusHead.WriteOgg(head),
                },
                TimeBase = new Rational(1, 48000),
            });
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960,
                IsKeyFrame = true, Data = new byte[] { 0xFC },
            });
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        // Locate the "Opus" sample-entry FourCC and inspect its samplerate field.
        // AudioSampleEntry layout (after the 8-byte box header):
        //   6 bytes reserved + 2 bytes data_reference_index   (8)
        //   8 bytes reserved                                  (16)
        //   2 bytes channels + 2 bytes sample size            (20)
        //   2 bytes predefined + 2 bytes reserved             (24)
        //   4 bytes samplerate (16.16 fixed point)            (28)
        int sampleEntryStart = IndexOfFourCc(bytes, "Opus");
        Assert.True(sampleEntryStart >= 0, "Opus sample entry not found in muxer output.");
        // sampleEntryStart points at the FourCC; the box body starts immediately after.
        int sampleRateOffset = sampleEntryStart + 4 /* FourCc */ + 24;
        uint sampleRateField = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(sampleRateOffset, 4));
        Assert.Equal((uint)(48000u << 16), sampleRateField);
    }

    [Fact]
    public async Task Mp4Muxer_Emits_Dops_Child_Box_Inside_Opus_Sample_Entry()
    {
        // Confirm the dOps body the muxer emits is what we'd produce by writing
        // the OpusHead struct in ISOBMFF form.
        var head = new OpusHead
        {
            ChannelCount = 2, PreSkip = 312,
            InputSampleRate = 48000, OutputGain = 0,
            ChannelMappingFamily = 0,
        };
        byte[] expectedDops = OpusHead.WriteIsobmff(head);

        byte[] bytes;
        await using (var ms = new MemoryStream())
        await using (var mux = new Mp4Muxer(ms, leaveOpen: true))
        {
            mux.AddTrack(new MediaTrack
            {
                Index = 0, Id = 1,
                Codec = new AudioCodecParameters
                {
                    Codec = CodecId.Opus, SampleRate = 48000, Channels = 2,
                    ExtraData = OpusHead.WriteOgg(head),
                },
                TimeBase = new Rational(1, 48000),
            });
            await mux.StartAsync();
            await mux.WriteSampleAsync(new MediaSample
            {
                TrackIndex = 0, Pts = 0, Dts = 0, Duration = 960,
                IsKeyFrame = true, Data = new byte[] { 0x00, 0x01 },
            });
            await mux.FinishAsync();
            bytes = ms.ToArray();
        }

        int dopsStart = IndexOfFourCc(bytes, "dOps");
        Assert.True(dopsStart >= 0, "dOps box not found in muxer output.");
        // dOps is a regular Box: 4 bytes size + 4 bytes type, then the body.
        int boxStart = dopsStart - 4; // back up to the size field
        uint boxSize = BinaryPrimitives.ReadUInt32BigEndian(bytes.AsSpan(boxStart, 4));
        int bodyStart = dopsStart + 4;
        int bodyLength = (int)boxSize - 8;
        Assert.Equal(expectedDops.Length, bodyLength);
        Assert.Equal(expectedDops, bytes.AsSpan(bodyStart, bodyLength).ToArray());
    }

    // ----- helpers -----

    private static void AssertEqualExceptMapping(OpusHead expected, OpusHead actual)
    {
        Assert.Equal(expected.ChannelCount, actual.ChannelCount);
        Assert.Equal(expected.PreSkip, actual.PreSkip);
        Assert.Equal(expected.InputSampleRate, actual.InputSampleRate);
        Assert.Equal(expected.OutputGain, actual.OutputGain);
        Assert.Equal(expected.ChannelMappingFamily, actual.ChannelMappingFamily);
        Assert.Equal(expected.StreamCount, actual.StreamCount);
        Assert.Equal(expected.CoupledCount, actual.CoupledCount);
    }

    /// <summary>
    /// Find the byte offset of a 4-character ASCII box-type tag in <paramref name="haystack"/>.
    /// Returns -1 when not found.
    /// </summary>
    private static int IndexOfFourCc(ReadOnlySpan<byte> haystack, string fourCc)
    {
        Span<byte> needle = stackalloc byte[4];
        for (int i = 0; i < 4; i++) needle[i] = (byte)fourCc[i];
        for (int i = 0; i <= haystack.Length - 4; i++)
        {
            if (haystack.Slice(i, 4).SequenceEqual(needle)) return i;
        }
        return -1;
    }
}
