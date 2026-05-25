namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Internal test surface. Not part of the public API — exists so the
/// Mediar.Tests assembly (granted internals access) can exercise pieces of
/// the decoder without having to mark them public. Stable contract within
/// the test boundary only.
/// </summary>
internal static class Internal
{
    public static int Ilog(int x) => VorbisBitReader.Ilog(x);
    public static float Float32Unpack(uint x) => VorbisBitReader.Float32Unpack(x);
    public static int Lookup1Values(int entries, int dims) => VorbisBitReader.Lookup1Values(entries, dims);

    public static byte[] PackXiphLaced(params ReadOnlySpan<byte[]> packets) => VorbisHeaders.PackXiphLaced(packets);
    public static byte[][] UnpackXiphLaced(ReadOnlySpan<byte> blob) => VorbisHeaders.UnpackXiphLaced(blob);

    public static IdentificationResult ParseIdentification(byte[] packet)
    {
        var h = VorbisHeaders.ParseIdentification(packet);
        return new IdentificationResult(h.VorbisVersion, h.Channels, h.SampleRate, h.Blocksize0, h.Blocksize1);
    }
    public static CommentResult ParseComment(byte[] packet)
    {
        var c = VorbisHeaders.ParseComment(packet);
        return new CommentResult(c.Vendor, c.UserComments);
    }

    public static IInverseMdct CreateMdct(int n)
    {
        var m = new VorbisMdct(n);
        return new MdctAdapter(m);
    }

    /// <summary>Expose <see cref="VorbisWindow.Apply"/> to tests.</summary>
    public static void ApplyWindow(float[] block, int leftWindowLength, int rightWindowLength)
        => VorbisWindow.Apply(block.AsSpan(), leftWindowLength, rightWindowLength);

    /// <summary>Float copy of <see cref="VorbisFloor1.InverseDbTable"/> for tests.</summary>
    public static float[] FloorInverseDbTable() => (float[])VorbisFloor1.InverseDbTable.Clone();

    /// <summary>
    /// Lightweight overlap-add harness for tests: feed pre-windowed blocks
    /// and read back emitted PCM. Each call to <see cref="ILapHarness.Push"/>
    /// returns the per-channel finalized samples for that packet (one fresh
    /// float[] per channel of length <see cref="ILapHarness.LastEmitCount"/>).
    /// </summary>
    public static ILapHarness CreateLapHarness(int channels, int blocksize1)
        => new LapHarness(channels, blocksize1);

    public interface ILapHarness
    {
        int LastEmitCount { get; }
        float[][] Push(float[][] windowedBlock, int currN);
        void Reset();
    }

    private sealed class LapHarness : ILapHarness
    {
        private readonly VorbisLap _lap;
        private readonly int _channels;
        public int LastEmitCount { get; private set; }

        public LapHarness(int channels, int blocksize1)
        {
            _lap = new VorbisLap(channels, blocksize1);
            _channels = channels;
        }

        public float[][] Push(float[][] windowedBlock, int currN)
        {
            for (int ch = 0; ch < _channels; ch++)
                _lap.Accumulate(ch, windowedBlock[ch], currN);
            int emit = _lap.PeekEmit(currN);
            LastEmitCount = emit;
            if (emit == 0)
            {
                _lap.Commit([], currN);
                return [];
            }
            var bufs = new float[_channels][];
            var mems = new Memory<float>[_channels];
            for (int ch = 0; ch < _channels; ch++)
            {
                bufs[ch] = new float[emit];
                mems[ch] = bufs[ch];
            }
            _lap.Commit(mems, currN);
            return bufs;
        }

        public void Reset() => _lap.Reset();
    }

    public sealed record IdentificationResult(uint VorbisVersion, int Channels, int SampleRate, int Blocksize0, int Blocksize1);
    public sealed record CommentResult(string Vendor, IReadOnlyList<string> UserComments);

    public interface IInverseMdct
    {
        void Forward(ReadOnlySpan<float> time, Span<float> freq);
        void Inverse(ReadOnlySpan<float> freq, Span<float> time);
    }

    private sealed class MdctAdapter : IInverseMdct
    {
        private readonly VorbisMdct _m;
        public MdctAdapter(VorbisMdct m) { _m = m; }
        public void Forward(ReadOnlySpan<float> time, Span<float> freq) => _m.Forward(time, freq);
        public void Inverse(ReadOnlySpan<float> freq, Span<float> time) => _m.Inverse(freq, time);
    }
}

