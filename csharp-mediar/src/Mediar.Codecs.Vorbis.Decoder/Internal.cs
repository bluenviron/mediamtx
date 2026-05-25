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

    public static byte[] PackXiphLaced(params byte[][] packets) => VorbisHeaders.PackXiphLaced(packets);
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
