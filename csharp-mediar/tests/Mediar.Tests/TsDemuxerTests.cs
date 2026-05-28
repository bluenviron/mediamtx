using System.Buffers.Binary;
using Mediar.Containers.MpegTs;
using Mediar.IO;
using Xunit;

namespace Mediar.Tests;

public sealed class TsDemuxerTests
{
    private const ushort PmtPid = 0x100;
    private const ushort H264Pid = 0x101;
    private const ushort AacPid = 0x102;

    [Fact]
    public void TsPacket_TryParse_ValidatesSyncByte()
    {
        Span<byte> bad = stackalloc byte[188];
        bad[0] = 0x46;
        Assert.False(TsPacket.TryParse(bad, out _));
    }

    [Fact]
    public void TsPacket_TryParse_RejectsTransportError()
    {
        Span<byte> p = stackalloc byte[188];
        p[0] = 0x47;
        p[1] = 0x80;
        Assert.False(TsPacket.TryParse(p, out _));
    }

    [Fact]
    public void TsPacket_TryParse_DecodesPidAndFlags()
    {
        Span<byte> p = stackalloc byte[188];
        p[0] = 0x47;
        p[1] = 0x40 | 0x01; // PUSI + high bits of PID 0x100
        p[2] = 0x00;
        p[3] = 0x10 | 0x05; // payload only + CC 5

        Assert.True(TsPacket.TryParse(p, out var info));
        Assert.True(info.PayloadUnitStartIndicator);
        Assert.Equal(0x100, info.Pid);
        Assert.Equal(5, info.ContinuityCounter);
        Assert.True(info.HasPayload);
        Assert.False(info.HasAdaptationField);
        Assert.Equal(4, info.PayloadOffset);
        Assert.Equal(184, info.PayloadLength);
    }

    [Fact]
    public void TsPacket_TryParse_SkipsAdaptationField()
    {
        Span<byte> p = stackalloc byte[188];
        p[0] = 0x47;
        p[1] = 0x00;
        p[2] = 0x21;
        p[3] = 0x30 | 0x00; // AF + payload, CC 0
        p[4] = 7; // adaptation_field_length
        Assert.True(TsPacket.TryParse(p, out var info));
        Assert.Equal(0x21, info.Pid);
        Assert.Equal(4 + 1 + 7, info.PayloadOffset);
        Assert.Equal(188 - (4 + 1 + 7), info.PayloadLength);
    }

    [Fact]
    public void StreamTypes_MapsAdvertisedCodecs()
    {
        Assert.Equal(CodecId.Aac, StreamTypes.ToCodecId(0x0F));
        Assert.Equal(CodecId.Aac, StreamTypes.ToCodecId(0x11));
        Assert.Equal(CodecId.H264, StreamTypes.ToCodecId(0x1B));
        Assert.Equal(CodecId.H265, StreamTypes.ToCodecId(0x24));
        Assert.Equal(CodecId.Mp3, StreamTypes.ToCodecId(0x03));
        Assert.Equal(CodecId.Ac3, StreamTypes.ToCodecId(0x81));
        Assert.Equal(CodecId.EAc3, StreamTypes.ToCodecId(0x87));
        Assert.Equal(CodecId.Unknown, StreamTypes.ToCodecId(0xFE));
    }

    [Fact]
    public void Open_RejectsStreamWithNoSyncByte()
    {
        byte[] noise = new byte[188 * 4];
        for (int i = 0; i < noise.Length; i++) noise[i] = 0x33;
        using var src = new MemoryRandomAccessSource(noise);
        Assert.Throws<InvalidDataException>(() => TsDemuxer.Open(src));
    }

    [Fact]
    public void Open_RejectsStreamShorterThanOnePacket()
    {
        using var src = new MemoryRandomAccessSource(new byte[10]);
        Assert.Throws<InvalidDataException>(() => TsDemuxer.Open(src));
    }

    [Fact]
    public void Open_RejectsStreamWithoutPmt()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        for (int i = 0; i < 4; i++) builder.AddNullPacket();
        using var src = new MemoryRandomAccessSource(builder.Build());
        Assert.Throws<InvalidDataException>(() => TsDemuxer.Open(src));
    }

    [Fact]
    public void Open_RejectsPmtWithOnlyUnknownStreamTypes()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: 0x1FFF,
            new TsBuilder.PmtEntry(StreamType: 0xFE, ElementaryPid: 0x200));
        using var src = new MemoryRandomAccessSource(builder.Build());
        Assert.Throws<InvalidDataException>(() => TsDemuxer.Open(src));
    }

    [Fact]
    public void Open_DiscoversMultipleTracks()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid),
            new TsBuilder.PmtEntry(StreamType: 0x0F, ElementaryPid: AacPid));

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        Assert.Equal("mpegts", dx.FormatName);
        Assert.Equal(2, dx.Tracks.Count);
        Assert.Equal(CodecId.H264, dx.Tracks[0].Codec.Codec);
        Assert.Equal(StreamKind.Video, dx.Tracks[0].Kind);
        Assert.Equal(H264Pid, dx.Tracks[0].Id);
        Assert.Equal(CodecId.Aac, dx.Tracks[1].Codec.Codec);
        Assert.Equal(StreamKind.Audio, dx.Tracks[1].Kind);
        Assert.Equal(AacPid, dx.Tracks[1].Id);
        Assert.Equal(new Rational(1, TsDemuxer.SystemClockHz), dx.Tracks[0].TimeBase);
    }

    [Fact]
    public async Task ReadSamplesAsync_EmitsSinglePesWithPts()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));

        byte[] payload = Enumerable.Range(0, 100).Select(i => (byte)i).ToArray();
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 90_000, dts: null, payload);

        // A second PUSI starts a new PES, which marks the first complete.
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 180_000, dts: null, new byte[10]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        var samples = new List<MediaSample>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            samples.Add(s);
            if (samples.Count >= 2) break;
        }

        Assert.True(samples.Count >= 1);
        var first = samples[0];
        try
        {
            Assert.Equal(0, first.TrackIndex);
            Assert.Equal(90_000, first.Pts);
            Assert.Equal(90_000, first.Dts);
            Assert.Equal(100, first.Data.Length);
            for (int i = 0; i < 100; i++) Assert.Equal((byte)i, first.Data.Span[i]);
        }
        finally
        {
            foreach (var s in samples) s.Owner?.Dispose();
        }
    }

    [Fact]
    public async Task ReadSamplesAsync_DecodesPtsDtsPair()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 200_000, dts: 150_000, new byte[32]);
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 210_000, dts: 160_000, new byte[8]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        var samples = new List<MediaSample>();
        await foreach (var s in dx.ReadSamplesAsync())
        {
            samples.Add(s);
            if (samples.Count >= 1) break;
        }
        try
        {
            Assert.Single(samples);
            Assert.Equal(200_000, samples[0].Pts);
            Assert.Equal(150_000, samples[0].Dts);
        }
        finally
        {
            foreach (var s in samples) s.Owner?.Dispose();
        }
    }

    [Fact]
    public async Task ReadSamplesAsync_HandlesMultiPacketPes()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));

        byte[] big = new byte[600];
        for (int i = 0; i < big.Length; i++) big[i] = (byte)((i * 7) & 0xFF);
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 1234, dts: null, big);

        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 1245, dts: null, new byte[1]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        MediaSample? sample = null;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            sample = s;
            break;
        }
        Assert.NotNull(sample);
        try
        {
            Assert.Equal(big.Length, sample!.Data.Length);
            Assert.True(big.AsSpan().SequenceEqual(sample.Data.Span));
        }
        finally
        {
            sample.Owner?.Dispose();
        }
    }

    [Fact]
    public async Task ReadSamplesAsync_InterleavesTwoPids()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid),
            new TsBuilder.PmtEntry(StreamType: 0x0F, ElementaryPid: AacPid));

        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 1000, dts: null, new byte[64]);
        builder.AddPesStream(AacPid, streamId: 0xC0, pts: 1001, dts: null, new byte[48]);
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 1100, dts: null, new byte[64]);
        builder.AddPesStream(AacPid, streamId: 0xC0, pts: 1101, dts: null, new byte[48]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        var perTrack = new Dictionary<int, List<long>>();
        var samples = new List<MediaSample>();
        try
        {
            await foreach (var s in dx.ReadSamplesAsync())
            {
                samples.Add(s);
                if (!perTrack.TryGetValue(s.TrackIndex, out var list))
                {
                    list = [];
                    perTrack[s.TrackIndex] = list;
                }
                list.Add(s.Pts);
                if (samples.Count >= 4) break;
            }
        }
        finally
        {
            foreach (var s in samples) s.Owner?.Dispose();
        }

        Assert.True(perTrack.ContainsKey(0));
        Assert.True(perTrack.ContainsKey(1));
    }

    [Fact]
    public async Task ReadSamplesAsync_DropsPesOnContinuityGap()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));

        // First PES is corrupted by a CC skip - should be dropped.
        builder.AddPesStreamWithCcSkip(H264Pid, streamId: 0xE0, pts: 9000, payload: new byte[400]);
        // Second PES is intact - should be emitted.
        byte[] good = new byte[64];
        for (int i = 0; i < good.Length; i++) good[i] = (byte)(0xA0 + (i & 0xF));
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 9500, dts: null, good);
        // Third PES caps the second so the assembler completes it.
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 9999, dts: null, new byte[1]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        var samples = new List<MediaSample>();
        try
        {
            await foreach (var s in dx.ReadSamplesAsync())
            {
                samples.Add(s);
                if (samples.Count >= 1) break;
            }
            Assert.Single(samples);
            Assert.Equal(9500, samples[0].Pts);
            Assert.True(good.AsSpan().SequenceEqual(samples[0].Data.Span));
        }
        finally
        {
            foreach (var s in samples) s.Owner?.Dispose();
        }
    }

    [Fact]
    public async Task ReadSamplesAsync_IgnoresNullPidPackets()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 42, dts: null, new byte[50]);
        for (int i = 0; i < 3; i++) builder.AddNullPacket();
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 84, dts: null, new byte[10]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        MediaSample? sample = null;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            sample = s;
            break;
        }
        Assert.NotNull(sample);
        try { Assert.Equal(42, sample!.Pts); }
        finally { sample.Owner?.Dispose(); }
    }

    [Fact]
    public async Task ReadSamplesAsync_FlushesLastPesAtEof()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 555, dts: null, new byte[20]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        MediaSample? sample = null;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            sample = s;
            break;
        }
        Assert.NotNull(sample);
        try
        {
            Assert.Equal(555, sample!.Pts);
            Assert.Equal(20, sample.Data.Length);
        }
        finally { sample.Owner?.Dispose(); }
    }

    [Fact]
    public async Task ReadSamplesAsync_SkipsScrambledPackets()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));
        builder.AddScrambledPesPacket(H264Pid, streamId: 0xE0, pts: 1, payload: new byte[100]);
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 77, dts: null, new byte[40]);
        builder.AddPesStream(H264Pid, streamId: 0xE0, pts: 88, dts: null, new byte[1]);

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        MediaSample? sample = null;
        await foreach (var s in dx.ReadSamplesAsync())
        {
            sample = s;
            break;
        }
        Assert.NotNull(sample);
        try { Assert.Equal(77, sample!.Pts); }
        finally { sample.Owner?.Dispose(); }
    }

    [Fact]
    public void Duration_IsZeroForStreamingSource()
    {
        var builder = new TsBuilder();
        builder.AddPatPacket(programNumber: 1, pmtPid: PmtPid);
        builder.AddPmtPacket(PmtPid, programNumber: 1, pcrPid: H264Pid,
            new TsBuilder.PmtEntry(StreamType: 0x1B, ElementaryPid: H264Pid));

        using var src = new MemoryRandomAccessSource(builder.Build());
        using var dx = TsDemuxer.Open(src);

        Assert.Equal(TimeSpan.Zero, dx.Duration);
    }

    /// <summary>
    /// Builds a minimal MPEG-TS byte stream out of 188-byte packets carrying
    /// hand-crafted PAT, PMT, PES, and null payloads. Internal helper used
    /// only by tests.
    /// </summary>
    internal sealed class TsBuilder
    {
        public sealed record PmtEntry(byte StreamType, ushort ElementaryPid);

        private readonly List<byte> _bytes = [];
        private readonly Dictionary<ushort, int> _cc = [];

        public byte[] Build() => _bytes.ToArray();

        public void AddNullPacket()
        {
            byte[] pkt = new byte[188];
            pkt[0] = 0x47;
            pkt[1] = (byte)((0x1FFF >> 8) & 0x1F);
            pkt[2] = (byte)(0x1FFF & 0xFF);
            pkt[3] = 0x10;
            for (int i = 4; i < 188; i++) pkt[i] = 0xFF;
            _bytes.AddRange(pkt);
        }

        public void AddPatPacket(ushort programNumber, ushort pmtPid)
        {
            byte[] section = BuildPatSection(programNumber, pmtPid);
            WritePsiPacket(0x0000, section);
        }

        public void AddPmtPacket(ushort pmtPid, ushort programNumber, ushort pcrPid, params PmtEntry[] entries)
        {
            byte[] section = BuildPmtSection(programNumber, pcrPid, entries);
            WritePsiPacket(pmtPid, section);
        }

        public void AddPesStream(ushort pid, byte streamId, long pts, long? dts, byte[] payload)
        {
            byte[] pes = BuildPesPacket(streamId, pts, dts, payload);
            WriteAsTsPackets(pid, pes, corruptSecondCc: false, scramble: false);
        }

        public void AddPesStreamWithCcSkip(ushort pid, byte streamId, long pts, byte[] payload)
        {
            byte[] pes = BuildPesPacket(streamId, pts, null, payload);
            WriteAsTsPackets(pid, pes, corruptSecondCc: true, scramble: false);
        }

        public void AddScrambledPesPacket(ushort pid, byte streamId, long pts, byte[] payload)
        {
            byte[] pes = BuildPesPacket(streamId, pts, null, payload);
            WriteAsTsPackets(pid, pes, corruptSecondCc: false, scramble: true);
        }

        private byte NextCc(ushort pid)
        {
            int next = _cc.TryGetValue(pid, out int v) ? (v + 1) & 0xF : 0;
            _cc[pid] = next;
            return (byte)next;
        }

        private void WritePsiPacket(ushort pid, byte[] section)
        {
            if (section.Length > 183) throw new ArgumentException("PSI section too large for single packet.", nameof(section));
            byte[] pkt = new byte[188];
            pkt[0] = 0x47;
            pkt[1] = (byte)(0x40 | ((pid >> 8) & 0x1F));
            pkt[2] = (byte)(pid & 0xFF);
            pkt[3] = (byte)(0x10 | NextCc(pid));
            pkt[4] = 0x00; // pointer_field
            Array.Copy(section, 0, pkt, 5, section.Length);
            for (int i = 5 + section.Length; i < 188; i++) pkt[i] = 0xFF;
            _bytes.AddRange(pkt);
        }

        private void WriteAsTsPackets(ushort pid, byte[] pes, bool corruptSecondCc, bool scramble)
        {
            int offset = 0;
            bool firstChunk = true;
            int packetIndex = 0;
            while (offset < pes.Length)
            {
                int payloadRoom = 184;
                int chunk = Math.Min(payloadRoom, pes.Length - offset);
                byte[] pkt = new byte[188];
                pkt[0] = 0x47;
                pkt[1] = (byte)(((firstChunk ? 0x40 : 0x00)) | ((pid >> 8) & 0x1F));
                pkt[2] = (byte)(pid & 0xFF);

                byte cc = NextCc(pid);
                if (corruptSecondCc && packetIndex == 1)
                {
                    cc = (byte)((cc + 4) & 0xF);
                    _cc[pid] = cc;
                }
                byte tsc = (byte)(scramble ? 0x40 : 0x00);
                pkt[3] = (byte)(tsc | 0x10 | cc);

                int headerEnd = 4;
                if (chunk < payloadRoom)
                {
                    int stuff = payloadRoom - chunk;
                    pkt[3] |= 0x20;
                    pkt[4] = (byte)(stuff - 1);
                    if (stuff >= 2) pkt[5] = 0x00;
                    for (int i = 6; i < 4 + stuff; i++) pkt[i] = 0xFF;
                    headerEnd = 4 + stuff;
                }
                Array.Copy(pes, offset, pkt, headerEnd, chunk);
                _bytes.AddRange(pkt);
                offset += chunk;
                firstChunk = false;
                packetIndex++;
            }
        }

        private static byte[] BuildPatSection(ushort programNumber, ushort pmtPid)
        {
            // table_id + flags + length + tsId + version/cur + sec_no + last + N*4 entries + CRC
            int entriesBytes = 4;
            int sectionLength = 5 + entriesBytes + 4;
            byte[] section = new byte[3 + sectionLength];
            section[0] = 0x00; // table_id = PAT
            section[1] = (byte)(0xB0 | ((sectionLength >> 8) & 0x0F));
            section[2] = (byte)(sectionLength & 0xFF);
            BinaryPrimitives.WriteUInt16BigEndian(section.AsSpan(3, 2), 0x0001); // tsId
            section[5] = 0xC1; // version=0, current=1
            section[6] = 0x00; // section_number
            section[7] = 0x00; // last_section_number
            BinaryPrimitives.WriteUInt16BigEndian(section.AsSpan(8, 2), programNumber);
            section[10] = (byte)(0xE0 | ((pmtPid >> 8) & 0x1F));
            section[11] = (byte)(pmtPid & 0xFF);
            // CRC zeros are tolerated by the parser (it does not verify CRC).
            return section;
        }

        private static byte[] BuildPmtSection(ushort programNumber, ushort pcrPid, PmtEntry[] entries)
        {
            int streamBytes = 0;
            foreach (var _ in entries) streamBytes += 5;
            int sectionLength = 9 + streamBytes + 4;
            byte[] section = new byte[3 + sectionLength];
            section[0] = 0x02; // table_id = PMT
            section[1] = (byte)(0xB0 | ((sectionLength >> 8) & 0x0F));
            section[2] = (byte)(sectionLength & 0xFF);
            BinaryPrimitives.WriteUInt16BigEndian(section.AsSpan(3, 2), programNumber);
            section[5] = 0xC1;
            section[6] = 0x00;
            section[7] = 0x00;
            section[8] = (byte)(0xE0 | ((pcrPid >> 8) & 0x1F));
            section[9] = (byte)(pcrPid & 0xFF);
            section[10] = 0xF0;
            section[11] = 0x00; // program_info_length = 0
            int o = 12;
            foreach (var e in entries)
            {
                section[o++] = e.StreamType;
                section[o++] = (byte)(0xE0 | ((e.ElementaryPid >> 8) & 0x1F));
                section[o++] = (byte)(e.ElementaryPid & 0xFF);
                section[o++] = 0xF0;
                section[o++] = 0x00;
            }
            return section;
        }

        private static byte[] BuildPesPacket(byte streamId, long pts, long? dts, byte[] payload)
        {
            byte ptsDtsFlags = dts.HasValue ? (byte)0xC0 : (byte)0x80;
            int ptsDtsLen = dts.HasValue ? 10 : 5;
            int headerDataLength = ptsDtsLen;
            int total = 6 + 3 + headerDataLength + payload.Length;
            byte[] pes = new byte[total];
            pes[0] = 0x00; pes[1] = 0x00; pes[2] = 0x01;
            pes[3] = streamId;
            int packetLengthField = total - 6;
            pes[4] = (byte)((packetLengthField >> 8) & 0xFF);
            pes[5] = (byte)(packetLengthField & 0xFF);
            pes[6] = 0x80; // marker bits
            pes[7] = ptsDtsFlags;
            pes[8] = (byte)headerDataLength;

            EncodeTimestamp(pes.AsSpan(9, 5), pts, dts.HasValue ? 0b0011 : 0b0010);
            if (dts.HasValue)
                EncodeTimestamp(pes.AsSpan(14, 5), dts.Value, 0b0001);

            Array.Copy(payload, 0, pes, 9 + headerDataLength, payload.Length);
            return pes;
        }

        private static void EncodeTimestamp(Span<byte> dst, long ts, int prefix)
        {
            long high = (ts >> 30) & 0x7;
            long mid = (ts >> 15) & 0x7FFF;
            long low = ts & 0x7FFF;
            dst[0] = (byte)(((prefix & 0xF) << 4) | (int)((high & 0x7) << 1) | 0x1);
            dst[1] = (byte)((mid >> 7) & 0xFF);
            dst[2] = (byte)(((mid & 0x7F) << 1) | 0x1);
            dst[3] = (byte)((low >> 7) & 0xFF);
            dst[4] = (byte)(((low & 0x7F) << 1) | 0x1);
        }
    }
}
