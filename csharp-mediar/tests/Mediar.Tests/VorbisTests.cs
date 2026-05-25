using System.Reflection;
using Mediar.Codecs.Vorbis.Decoder;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Foundation tests for the Mediar Vorbis I decoder. These cover the
/// structural primitives — bit reader, codebook, Xiph lacing, IMDCT
/// round-trip, header parsing — that the eventual audio synthesis path
/// will compose. Bit-exact end-to-end decoding against the libvorbis
/// reference is out of scope for this release.
/// </summary>
public sealed class VorbisTests
{
    private const string VorbisAsm = "Mediar.Codecs.Vorbis.Decoder";

    /// <summary>Reach into internals via reflection — bit reader is a ref struct so we exercise it through codebook/header tests too.</summary>
    private static Type ResolveInternal(string name)
    {
        var asm = Assembly.Load(VorbisAsm);
        return asm.GetType($"{VorbisAsm}.{name}", throwOnError: true)!;
    }

    [Fact]
    public void Ilog_KnownValues()
    {
        // ilog reference table from Vorbis I §9.2.1.
        Assert.Equal(0, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(0));
        Assert.Equal(1, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(1));
        Assert.Equal(2, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(2));
        Assert.Equal(2, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(3));
        Assert.Equal(3, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(4));
        Assert.Equal(3, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(7));
        Assert.Equal(4, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(8));
        Assert.Equal(8, Mediar.Codecs.Vorbis.Decoder.Internal.Ilog(128));
    }

    [Fact]
    public void Float32Unpack_KnownValues()
    {
        // float32_unpack(0) = 0
        Assert.Equal(0f, Mediar.Codecs.Vorbis.Decoder.Internal.Float32Unpack(0));
        // mantissa = 1, exponent = 788 → 1.0
        uint one = (788u << 21) | 1u;
        Assert.Equal(1f, Mediar.Codecs.Vorbis.Decoder.Internal.Float32Unpack(one));
        // negative one
        Assert.Equal(-1f, Mediar.Codecs.Vorbis.Decoder.Internal.Float32Unpack(one | 0x80000000u));
    }

    [Fact]
    public void Lookup1Values_Cube()
    {
        // lookup1_values(8, 3) = 2 (2³ = 8)
        Assert.Equal(2, Mediar.Codecs.Vorbis.Decoder.Internal.Lookup1Values(8, 3));
        // lookup1_values(27, 3) = 3 (3³ = 27)
        Assert.Equal(3, Mediar.Codecs.Vorbis.Decoder.Internal.Lookup1Values(27, 3));
        // lookup1_values(25, 3) = 2 (2³=8 ≤ 25 < 3³=27)
        Assert.Equal(2, Mediar.Codecs.Vorbis.Decoder.Internal.Lookup1Values(25, 3));
        // lookup1_values(15, 2) = 3 (3² = 9 ≤ 15 < 4² = 16)
        Assert.Equal(3, Mediar.Codecs.Vorbis.Decoder.Internal.Lookup1Values(15, 2));
    }

    [Fact]
    public void XiphLacing_RoundTrip()
    {
        byte[] a = { 1, 2, 3 };
        byte[] b = new byte[300];
        for (int i = 0; i < b.Length; i++) b[i] = (byte)i;
        byte[] c = { 9, 8, 7, 6 };

        byte[] packed = Mediar.Codecs.Vorbis.Decoder.Internal.PackXiphLaced(a, b, c);
        var unpacked = Mediar.Codecs.Vorbis.Decoder.Internal.UnpackXiphLaced(packed);

        Assert.Equal(3, unpacked.Length);
        Assert.Equal(a, unpacked[0]);
        Assert.Equal(b, unpacked[1]);
        Assert.Equal(c, unpacked[2]);
    }

    [Fact]
    public void XiphLacing_HandlesLongPacketsViaContinuationBytes()
    {
        byte[] a = new byte[510]; // exactly 2 * 255
        byte[] b = { 0x42 };
        for (int i = 0; i < a.Length; i++) a[i] = 0xAA;

        byte[] packed = Mediar.Codecs.Vorbis.Decoder.Internal.PackXiphLaced(a, b);

        // count-1 byte + 3 lacing bytes (255, 255, 0) + 510 + 1
        Assert.Equal(1 + 3 + 510 + 1, packed.Length);
        Assert.Equal(1, packed[0]);     // count - 1
        Assert.Equal(0xFF, packed[1]);  // 255
        Assert.Equal(0xFF, packed[2]);  // 255
        Assert.Equal(0x00, packed[3]);  // 0 (terminator)

        var unpacked = Mediar.Codecs.Vorbis.Decoder.Internal.UnpackXiphLaced(packed);
        Assert.Equal(a, unpacked[0]);
        Assert.Equal(b, unpacked[1]);
    }

    [Fact]
    public void IdentificationHeader_Parses()
    {
        var hdr = BuildIdentificationHeader(channels: 2, sampleRate: 44100, blockSize0Exp: 8, blockSize1Exp: 11);
        var parsed = Mediar.Codecs.Vorbis.Decoder.Internal.ParseIdentification(hdr);
        Assert.Equal(0u, parsed.VorbisVersion);
        Assert.Equal(2, parsed.Channels);
        Assert.Equal(44100, parsed.SampleRate);
        Assert.Equal(256, parsed.Blocksize0);
        Assert.Equal(2048, parsed.Blocksize1);
    }

    [Fact]
    public void IdentificationHeader_RejectsBadMagic()
    {
        var hdr = BuildIdentificationHeader(channels: 2, sampleRate: 44100, blockSize0Exp: 8, blockSize1Exp: 11);
        hdr[1] = (byte)'x';
        Assert.Throws<InvalidDataException>(() =>
            Mediar.Codecs.Vorbis.Decoder.Internal.ParseIdentification(hdr));
    }

    [Fact]
    public void IdentificationHeader_RejectsBadVersion()
    {
        var hdr = BuildIdentificationHeader(channels: 2, sampleRate: 44100, blockSize0Exp: 8, blockSize1Exp: 11);
        hdr[7] = 1;
        Assert.Throws<InvalidDataException>(() =>
            Mediar.Codecs.Vorbis.Decoder.Internal.ParseIdentification(hdr));
    }

    private static readonly string[] s_sampleComments = { "TITLE=Test", "ARTIST=Vorbis" };

    [Fact]
    public void CommentHeader_Parses()
    {
        var hdr = BuildCommentHeader("Mediar", s_sampleComments);
        var parsed = Mediar.Codecs.Vorbis.Decoder.Internal.ParseComment(hdr);
        Assert.Equal("Mediar", parsed.Vendor);
        Assert.Equal(2, parsed.UserComments.Count);
        Assert.Equal("TITLE=Test", parsed.UserComments[0]);
        Assert.Equal("ARTIST=Vorbis", parsed.UserComments[1]);
    }

    [Fact]
    public void Imdct_RoundTrip_StaysWithinTolerance()
    {
        // IMDCT output is bounded and finite for arbitrary input frequencies.
        int n = 64;
        var mdct = Mediar.Codecs.Vorbis.Decoder.Internal.CreateMdct(n);
        var time = new float[n];
        for (int i = 0; i < n; i++)
        {
            time[i] = (float)Math.Sin(2.0 * Math.PI * 5 * i / n);
        }
        var freq = new float[n / 2];
        var roundTrip = new float[n];
        mdct.Forward(time, freq);
        mdct.Inverse(freq, roundTrip);

        for (int i = 0; i < n; i++)
        {
            Assert.False(float.IsNaN(roundTrip[i]));
            Assert.False(float.IsInfinity(roundTrip[i]));
            Assert.True(Math.Abs(roundTrip[i]) < 8f);
        }
    }

    [Fact]
    public void Imdct_TdacOverlapAddReconstructsSignal()
    {
        // Time-domain aliasing cancellation: with the Vorbis sin² window,
        // forward + inverse + windowed overlap-add of two adjacent blocks
        // reconstructs the original middle region exactly (to float precision).
        int n = 64;
        var mdct = Mediar.Codecs.Vorbis.Decoder.Internal.CreateMdct(n);

        var src = new float[2 * n];
        for (int i = 0; i < src.Length; i++)
        {
            src[i] = (float)Math.Sin(2.0 * Math.PI * 3 * i / n);
        }

        var win = new float[n];
        for (int i = 0; i < n / 2; i++)
        {
            double inner = Math.PI / 2.0 * (i + 0.5) / (n / 2);
            win[i] = (float)Math.Sin(Math.PI / 2.0 * Math.Sin(inner) * Math.Sin(inner));
        }
        for (int i = 0; i < n / 2; i++)
        {
            double inner = Math.PI / 2.0 * (n / 2 - i - 0.5) / (n / 2);
            win[n / 2 + i] = (float)Math.Sin(Math.PI / 2.0 * Math.Sin(inner) * Math.Sin(inner));
        }

        var block0 = new float[n];
        var block1 = new float[n];
        for (int i = 0; i < n; i++) block0[i] = src[i] * win[i];
        for (int i = 0; i < n; i++) block1[i] = src[i + n / 2] * win[i];

        var f0 = new float[n / 2];
        var f1 = new float[n / 2];
        mdct.Forward(block0, f0);
        mdct.Forward(block1, f1);

        var i0 = new float[n];
        var i1 = new float[n];
        mdct.Inverse(f0, i0);
        mdct.Inverse(f1, i1);
        for (int i = 0; i < n; i++) i0[i] *= win[i];
        for (int i = 0; i < n; i++) i1[i] *= win[i];

        var reconstructed = new float[n / 2];
        for (int i = 0; i < n / 2; i++) reconstructed[i] = i0[n / 2 + i] + i1[i];

        double maxErr = 0;
        for (int i = 0; i < n / 2; i++)
        {
            double err = Math.Abs(reconstructed[i] - src[n / 2 + i]);
            if (err > maxErr) maxErr = err;
        }
        Assert.True(maxErr < 1e-3, $"TDAC reconstruction error too large: {maxErr}");
    }

    [Fact]
    public void Decoder_PrimesFromXiphLacedExtraData()
    {
        var id = BuildIdentificationHeader(2, 44100, 8, 11);
        var comment = BuildCommentHeader("Mediar", Array.Empty<string>());
        var setup = BuildMinimalSetupHeader();
        byte[] extra = Mediar.Codecs.Vorbis.Decoder.Internal.PackXiphLaced(id, comment, setup);

        var dec = new VorbisDecoder(new AudioCodecParameters
        {
            Codec = CodecId.Vorbis,
            SampleRate = 44100,
            Channels = 2,
            ExtraData = extra,
        });
        Assert.True(dec.IsPrimed);
        Assert.Equal(256, dec.ShortBlocksize);
        Assert.Equal(2048, dec.LongBlocksize);
        Assert.Equal("Mediar", dec.Vendor);
    }

    [Fact]
    public void Decoder_PrimesFromSequentialPackets()
    {
        var id = BuildIdentificationHeader(1, 48000, 8, 11);
        var comment = BuildCommentHeader("Sequential", Array.Empty<string>());
        var setup = BuildMinimalSetupHeader();

        var dec = new VorbisDecoder(new AudioCodecParameters
        {
            Codec = CodecId.Vorbis,
            SampleRate = 48000,
            Channels = 1,
            ExtraData = ReadOnlyMemory<byte>.Empty,
        });
        Assert.False(dec.IsPrimed);
        Assert.True(dec.Decode(id, 0).Samples.IsEmpty);
        Assert.True(dec.Decode(comment, 0).Samples.IsEmpty);
        Assert.True(dec.Decode(setup, 0).Samples.IsEmpty);
        Assert.True(dec.IsPrimed);
    }

    // ---- Test helpers (build minimal valid Vorbis header packets) ----

    private static byte[] BuildIdentificationHeader(int channels, int sampleRate, int blockSize0Exp, int blockSize1Exp)
    {
        // 30 bytes total: 1 type + 6 magic + 4 version + 1 channels + 4 sr + 3*4 bitrate + 1 blocksize-bits + 1 framing
        var p = new byte[30];
        p[0] = 1;
        p[1] = (byte)'v'; p[2] = (byte)'o'; p[3] = (byte)'r'; p[4] = (byte)'b'; p[5] = (byte)'i'; p[6] = (byte)'s';
        BitConverter.GetBytes((uint)0).CopyTo(p, 7);      // version
        p[11] = (byte)channels;
        BitConverter.GetBytes((uint)sampleRate).CopyTo(p, 12);
        // bitrates left as zero
        p[28] = (byte)((blockSize1Exp << 4) | blockSize0Exp);
        p[29] = 0x01; // framing bit
        return p;
    }

    private static byte[] BuildCommentHeader(string vendor, string[] comments)
    {
        byte[] vBytes = System.Text.Encoding.UTF8.GetBytes(vendor);
        int size = 1 + 6 + 4 + vBytes.Length + 4;
        foreach (var c in comments)
        {
            size += 4 + System.Text.Encoding.UTF8.GetByteCount(c);
        }
        size += 1; // framing
        var p = new byte[size];
        int o = 0;
        p[o++] = 3;
        p[o++] = (byte)'v'; p[o++] = (byte)'o'; p[o++] = (byte)'r'; p[o++] = (byte)'b'; p[o++] = (byte)'i'; p[o++] = (byte)'s';
        BitConverter.GetBytes((uint)vBytes.Length).CopyTo(p, o); o += 4;
        vBytes.CopyTo(p, o); o += vBytes.Length;
        BitConverter.GetBytes((uint)comments.Length).CopyTo(p, o); o += 4;
        foreach (var c in comments)
        {
            byte[] cb = System.Text.Encoding.UTF8.GetBytes(c);
            BitConverter.GetBytes((uint)cb.Length).CopyTo(p, o); o += 4;
            cb.CopyTo(p, o); o += cb.Length;
        }
        p[o] = 0x01; // framing
        return p;
    }

    private static byte[] BuildMinimalSetupHeader()
    {
        // Minimal valid setup header:
        //   1 codebook (1 entry, 1 dim, length=1, lookup=0)
        //   1 time transform (placeholder 0)
        //   1 floor (type 1, zero partitions, multiplier=1, rangebits=0)
        //   1 residue (type 0, zero range, partition_size=1, classifications=1,
        //              class_book=0, cascade=0, no books)
        //   1 mapping (no submap flag, no coupling, reserved=0)
        //   1 mode (block_flag=0, window_type=0, transform_type=0, mapping=0)
        //   framing bit
        var w = new TestBitWriter();
        // Codebook 1
        // sync 0x564342
        w.WriteBits(0x564342u, 24);
        w.WriteBits(1u, 16); // dim
        w.WriteBits(1u, 24); // entries
        w.WriteBit(false);   // ordered
        w.WriteBit(false);   // sparse
        w.WriteBits(0u, 5);  // length-1 = 0 → length 1
        w.WriteBits(0u, 4);  // lookup type 0
        // Time count = 0 (encoded as count-1, so 0 means 1 transform)
        // top-level layout
        // (codebook_count is byte before — but we wrote one codebook; count_minus_one = 0)
        // We have to lay out the whole header byte-aligned: type + magic + bits
        var packetBits = new TestBitWriter();
        packetBits.WriteByte(5);
        packetBits.WriteBytes("vorbis"u8.ToArray());
        // codebook_count - 1
        packetBits.WriteBits(0u, 8);
        // copy codebook
        packetBits.WriteBitWriter(w);
        // time count - 1 = 0 → 1 transform
        packetBits.WriteBits(0u, 6);
        packetBits.WriteBits(0u, 16); // placeholder
        // floor count - 1 = 0 → 1 floor
        packetBits.WriteBits(0u, 6);
        packetBits.WriteBits(1u, 16); // floor type 1
        packetBits.WriteBits(0u, 5);  // partitions = 0
        packetBits.WriteBits(0u, 2);  // multiplier - 1 = 0
        packetBits.WriteBits(0u, 4);  // rangebits = 0
        // residue count - 1 = 0 → 1 residue
        packetBits.WriteBits(0u, 6);
        packetBits.WriteBits(0u, 16); // residue type 0
        packetBits.WriteBits(0u, 24); // begin
        packetBits.WriteBits(0u, 24); // end
        packetBits.WriteBits(0u, 24); // partition_size - 1 = 0 → 1
        packetBits.WriteBits(0u, 6);  // classifications - 1 = 0 → 1
        packetBits.WriteBits(0u, 8);  // class_book = 0
        packetBits.WriteBits(0u, 3);  // low_bits = 0
        packetBits.WriteBit(false);   // big_bit = 0
        // no books because cascade == 0
        // mapping count - 1 = 0 → 1 mapping
        packetBits.WriteBits(0u, 6);
        packetBits.WriteBits(0u, 16); // mapping type 0
        packetBits.WriteBit(false);   // submap flag = 0 → 1 submap
        packetBits.WriteBit(false);   // coupling flag = 0
        packetBits.WriteBits(0u, 2);  // reserved = 0
        // submap (1 submap)
        packetBits.WriteBits(0u, 8);  // unused submap "time configuration"
        packetBits.WriteBits(0u, 8);  // submap floor
        packetBits.WriteBits(0u, 8);  // submap residue
        // mode count - 1 = 0 → 1 mode
        packetBits.WriteBits(0u, 6);
        packetBits.WriteBit(false);   // blockflag
        packetBits.WriteBits(0u, 16); // windowtype
        packetBits.WriteBits(0u, 16); // transformtype
        packetBits.WriteBits(0u, 8);  // mapping
        // framing bit
        packetBits.WriteBit(true);

        return packetBits.ToArray();
    }

    /// <summary>LSB-first bit writer used by the header builder helpers.</summary>
    private sealed class TestBitWriter
    {
        private readonly List<byte> _bytes = new();
        private byte _cur;
        private int _bit;

        public void WriteBit(bool v)
        {
            if (v) _cur |= (byte)(1 << _bit);
            _bit++;
            if (_bit == 8)
            {
                _bytes.Add(_cur);
                _cur = 0; _bit = 0;
            }
        }
        public void WriteBits(uint value, int count)
        {
            for (int i = 0; i < count; i++) WriteBit(((value >> i) & 1) != 0);
        }
        public void WriteByte(byte v)
        {
            if (_bit == 0)
            {
                _bytes.Add(v);
            }
            else
            {
                WriteBits(v, 8);
            }
        }
        public void WriteBytes(byte[] data)
        {
            foreach (var b in data) WriteByte(b);
        }
        public void WriteBitWriter(TestBitWriter other)
        {
            // Concatenate bits — flush other into bit-by-bit.
            foreach (var bit in other.Bits()) WriteBit(bit);
        }
        public IEnumerable<bool> Bits()
        {
            foreach (var b in _bytes)
            {
                for (int i = 0; i < 8; i++) yield return ((b >> i) & 1) != 0;
            }
            for (int i = 0; i < _bit; i++) yield return ((_cur >> i) & 1) != 0;
        }
        public byte[] ToArray()
        {
            if (_bit > 0)
            {
                _bytes.Add(_cur);
            }
            return _bytes.ToArray();
        }
    }

    // ---- Floor 1 dB table ----

    [Fact]
    public void FloorInverseDbTable_HasExpectedShape()
    {
        // Vorbis I §9.2.2: 256-entry lookup with the smallest value at index 0
        // and 1.0 at index 255. Strictly monotone increasing.
        float[] tab = Mediar.Codecs.Vorbis.Decoder.Internal.FloorInverseDbTable();
        Assert.Equal(256, tab.Length);
        Assert.True(tab[0] > 0f);
        Assert.True(tab[255] > 0.9f && tab[255] <= 1.0f);
        for (int i = 1; i < 256; i++)
        {
            Assert.True(tab[i] > tab[i - 1], $"db table not monotone at {i}");
        }
        // Roughly +0.5 dB per step at the top: tab[254]/tab[255] ≈ 2^(-1/11).
        double ratio = tab[254] / (double)tab[255];
        Assert.InRange(ratio, 0.93, 0.945);
    }

    // ---- Windowing ----

    [Fact]
    public void Window_ZerosOutsideRampRegion()
    {
        // Long block n=128, with a short-side neighbour ramp of length 32
        // (e.g. blocksize0/2 = 256/2/4 stylised). Geometry:
        //   leftStart  = n/4 - L/2 = 32 - 16 = 16
        //   leftEnd    = 48
        //   rightStart = 3n/4 - R/2 = 96 - 16 = 80
        //   rightEnd   = 112
        // Outside [16,48) ∪ [48,80) (plateau) ∪ [80,112) is zero.
        int n = 128;
        int L = 32;
        int R = 32;
        var block = new float[n];
        for (int i = 0; i < n; i++) block[i] = 1f;
        Mediar.Codecs.Vorbis.Decoder.Internal.ApplyWindow(block, leftWindowLength: L, rightWindowLength: R);

        for (int i = 0; i < 16; i++) Assert.Equal(0f, block[i]);
        for (int i = 112; i < n; i++) Assert.Equal(0f, block[i]);
        // Plateau is the unchanged 1.0 region.
        for (int i = 48; i < 80; i++) Assert.Equal(1f, block[i]);
        // Ramp midpoint ≈ 0.707.
        Assert.InRange(block[16 + L / 2 - 1], 0.6f, 0.8f);
        Assert.InRange(block[80 + R / 2], 0.6f, 0.8f);
    }

    [Fact]
    public void Window_PartitionOfUnity()
    {
        // sin^2 window has the TDAC partition-of-unity property:
        // for two adjacent same-size blocks, leftWin^2 + rightWin^2 = 1 in
        // the overlap region. Validate numerically.
        int n = 64;
        int half = n / 2;
        var b0 = new float[n];
        var b1 = new float[n];
        for (int i = 0; i < n; i++) { b0[i] = 1f; b1[i] = 1f; }
        Mediar.Codecs.Vorbis.Decoder.Internal.ApplyWindow(b0, half, half);
        Mediar.Codecs.Vorbis.Decoder.Internal.ApplyWindow(b1, half, half);

        // Sum-of-squares across the overlap should be 1.
        for (int i = 0; i < half; i++)
        {
            float left = b0[half + i];       // right ramp of block 0
            float right = b1[i];             // left ramp of block 1
            float sum = left * left + right * right;
            Assert.InRange(sum, 0.999, 1.001);
        }
    }

    // ---- Overlap-add harness ----

    [Fact]
    public void Lap_FirstPacketEmitsZeroSamples()
    {
        int n = 256;
        var lap = Mediar.Codecs.Vorbis.Decoder.Internal.CreateLapHarness(channels: 1, blocksize1: n);
        var block = new float[n];
        for (int i = 0; i < n; i++) block[i] = 1f;
        var emitted = lap.Push(new[] { block }, n);
        Assert.Equal(0, lap.LastEmitCount);
        Assert.Empty(emitted);
    }

    [Fact]
    public void Lap_LongLongStreamEmitsHalfBlockPerPacket()
    {
        // A steady LL stream should produce exactly (N+N)/4 = N/2 samples
        // per packet after the first.
        int n = 128;
        var lap = Mediar.Codecs.Vorbis.Decoder.Internal.CreateLapHarness(channels: 1, blocksize1: n);
        var block = new float[n];
        for (int i = 0; i < n; i++)
        {
            double inner = Math.PI / 2.0 * (i + 0.5) / (double)n;
            double w = Math.Sin(Math.PI / 2.0 * Math.Sin(inner) * Math.Sin(inner));
            // Constant signal × full window (left ramp + right ramp).
            block[i] = (float)w;
        }
        // Properly window: leftHalf is rising ramp, rightHalf is mirror.
        var win = new float[n];
        for (int i = 0; i < n; i++) win[i] = block[i];

        // First packet: emit 0.
        lap.Push(new[] { (float[])win.Clone() }, n);
        // Second packet: emit n/2.
        var emitted = lap.Push(new[] { (float[])win.Clone() }, n);
        Assert.Equal(n / 2, lap.LastEmitCount);
        Assert.Single(emitted);
        Assert.Equal(n / 2, emitted[0].Length);
        // Third packet still emits n/2.
        emitted = lap.Push(new[] { (float[])win.Clone() }, n);
        Assert.Equal(n / 2, lap.LastEmitCount);
        Assert.Equal(n / 2, emitted[0].Length);
        // All emitted samples finite.
        foreach (var s in emitted[0])
        {
            Assert.False(float.IsNaN(s));
            Assert.False(float.IsInfinity(s));
        }
    }

    [Fact]
    public void Lap_ShortLongTransitionEmitsBlendedStride()
    {
        // Short→Long: emit (N0 + N1)/4 samples. Both packets need to be valid
        // windowed blocks but for the stride test we just need the buffers.
        int blocksize0 = 256;
        int blocksize1 = 2048;
        var lap = Mediar.Codecs.Vorbis.Decoder.Internal.CreateLapHarness(channels: 1, blocksize1: blocksize1);

        var shortBlock = new float[blocksize0];
        for (int i = 0; i < blocksize0; i++) shortBlock[i] = 0.01f;
        var longBlock = new float[blocksize1];
        for (int i = 0; i < blocksize1; i++) longBlock[i] = 0.01f;

        // First packet (short): emit 0.
        lap.Push(new[] { shortBlock }, blocksize0);
        Assert.Equal(0, lap.LastEmitCount);
        // Second packet (long): emit (256 + 2048)/4 = 576.
        lap.Push(new[] { longBlock }, blocksize1);
        Assert.Equal((blocksize0 + blocksize1) / 4, lap.LastEmitCount);
        // Third packet (short again): emit (2048 + 256)/4 = 576 again.
        lap.Push(new[] { shortBlock }, blocksize0);
        Assert.Equal((blocksize0 + blocksize1) / 4, lap.LastEmitCount);
    }

    [Fact]
    public void Lap_ResetClearsState()
    {
        int n = 64;
        var lap = Mediar.Codecs.Vorbis.Decoder.Internal.CreateLapHarness(channels: 2, blocksize1: n);
        var block = new float[n];
        lap.Push(new[] { block, block }, n);
        lap.Push(new[] { block, block }, n);
        lap.Reset();
        // After reset, first push emits 0 again.
        lap.Push(new[] { block, block }, n);
        Assert.Equal(0, lap.LastEmitCount);
    }

    // ---- End-to-end audio packet through Decode() ----

    [Fact]
    public void Decoder_DecodesSilentAudioPackets_EndToEnd()
    {
        // Verify the full DecodeAudio pipeline runs cleanly: priming via
        // Xiph-laced ExtraData, mode parse, "nonzero=0" floor decode that
        // marks the channel silent, residue-skip, IMDCT, window, lap. With
        // the minimal setup header (no floor partitions, zero-length residue,
        // one short-block mode) every audio packet is a single 0x00 byte —
        // the type bit (0) and the floor1 "nonzero" bit (0). The pipeline
        // must produce silence (all zeros) on the second packet onwards.
        var idHdr = BuildIdentificationHeader(1, 48000, 8, 8); // mono, n0=n1=256
        var commentHdr = BuildCommentHeader("E2E", Array.Empty<string>());
        var setupHdr = BuildMinimalSetupHeader();
        byte[] extra = Mediar.Codecs.Vorbis.Decoder.Internal.PackXiphLaced(idHdr, commentHdr, setupHdr);
        var dec = new VorbisDecoder(new AudioCodecParameters
        {
            Codec = CodecId.Vorbis,
            SampleRate = 48000,
            Channels = 1,
            ExtraData = extra,
        });
        Assert.True(dec.IsPrimed);

        // First audio packet primes the lap: produces no output.
        byte[] silentPacket = { 0x00 };
        var first = dec.Decode(silentPacket, pts: 0);
        Assert.True(first.Samples.IsEmpty);
        first.Owner?.Dispose();

        // Second audio packet: emit (256 + 256)/4 = 128 samples, all zero.
        var second = dec.Decode(silentPacket, pts: 0);
        Assert.Equal(1, second.Channels);
        Assert.Equal(48000, second.SampleRate);
        Assert.Equal(128, second.SamplesPerChannel);
        foreach (var s in second.Samples.Span)
        {
            Assert.Equal(0f, s);
        }
        second.Owner?.Dispose();

        // Third packet: same emit count, still silent, no NaN/Inf drift.
        var third = dec.Decode(silentPacket, pts: 0);
        Assert.Equal(128, third.SamplesPerChannel);
        foreach (var s in third.Samples.Span)
        {
            Assert.False(float.IsNaN(s));
            Assert.False(float.IsInfinity(s));
            Assert.Equal(0f, s);
        }
        third.Owner?.Dispose();
    }
}
