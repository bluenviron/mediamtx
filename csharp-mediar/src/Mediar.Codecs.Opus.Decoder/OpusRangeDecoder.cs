using System.Numerics;

namespace Mediar.Codecs.Opus.Decoder;

/// <summary>
/// Opus range decoder (RFC 6716 §4.1). Drives both the SILK and CELT
/// sub-codecs. Implemented as a <c>ref struct</c> so the per-packet decoder
/// state lives on the caller's stack — there is no heap allocation per
/// packet (which matters because real-time Opus decode invokes this code
/// path millions of times per second).
/// </summary>
/// <remarks>
/// <para>
/// The decoder consumes range-coded symbols sequentially from the start of
/// the buffer and raw bits sequentially from the end of the buffer, exactly
/// matching libopus's <c>ec_dec</c> behaviour. The buffer is supplied by the
/// caller and never copied. Once initialised the struct is mutated in place
/// by every read method.
/// </para>
/// <para>
/// All bit constants follow RFC 6716:
/// <list type="bullet">
///   <item><description><c>EC_CODE_BITS = 32</c></description></item>
///   <item><description><c>EC_SYM_BITS  = 8</c></description></item>
///   <item><description><c>EC_CODE_EXTRA = ((32 - 2) % 8) + 1 = 7</c></description></item>
///   <item><description><c>EC_CODE_TOP  = 1 &lt;&lt; 31</c></description></item>
///   <item><description><c>EC_CODE_BOT  = 1 &lt;&lt; 23</c></description></item>
///   <item><description><c>EC_UINT_BITS = 8</c></description></item>
/// </list>
/// </para>
/// </remarks>
public ref struct OpusRangeDecoder
{
    // ------- spec-named constants -------

    /// <summary>Bits per coded symbol (always 8 — i.e. bytes).</summary>
    public const int SymbolBits = 8;

    /// <summary>Total bits in the running range.</summary>
    public const int CodeBits = 32;

    /// <summary>
    /// EC_CODE_EXTRA = ((CodeBits - 2) % SymbolBits) + 1. Used during init
    /// to shift the first byte into <see cref="_val"/>.
    /// </summary>
    public const int CodeExtra = ((CodeBits - 2) % SymbolBits) + 1; // = 7

    /// <summary>Upper boundary of the range (1 &lt;&lt; 31).</summary>
    public const uint CodeTop = 1U << (CodeBits - 1);

    /// <summary>Renormalisation threshold (1 &lt;&lt; 23).</summary>
    public const uint CodeBot = CodeTop >> SymbolBits;

    /// <summary>Threshold above which <see cref="DecodeUint(uint)"/> shifts to raw bits.</summary>
    public const int UintBits = 8;

    /// <summary>Bits available in the raw-bit reader window.</summary>
    public const int WindowSize = 32;

    // Correction table for the 1/8-bit precision of TellFrac (RFC 6716 §4.1).
    private static readonly ushort[] _correction = { 35733, 38967, 42495, 46340, 50535, 55109, 60097, 65535 };

    // ------- state -------

    private readonly ReadOnlySpan<byte> _buf;
    private int _offs;        // forward read pointer (range-coded bytes)
    private int _endOffs;     // bytes already consumed from the END (raw bits)
    private uint _endWindow;  // bit window holding pending raw bits
    private int _nEndBits;    // bits currently buffered in _endWindow
    private int _nBitsTotal;  // total bits consumed (for Tell / TellFrac)
    private uint _rng;        // current range size
    private uint _val;        // current 'low' value relative to rng
    private int _rem;         // saved remainder byte from the previous read

    /// <summary>True once the decoder has read past the end of the buffer.</summary>
    public bool HasError { get; private set; }

    /// <summary>Current value of the running range (for diagnostics + tests).</summary>
    public uint Range => _rng;

    /// <summary>Current value of the decoder's "value" register (for diagnostics + tests).</summary>
    public uint Value => _val;

    /// <summary>Bytes of range-coded payload consumed from the front of the buffer.</summary>
    public int FrontByteOffset => _offs;

    /// <summary>Bytes of raw-bit payload consumed from the back of the buffer.</summary>
    public int BackByteOffset => _endOffs;

    /// <summary>Length of the underlying buffer.</summary>
    public int BufferLength => _buf.Length;

    /// <summary>
    /// Initialise a new decoder over <paramref name="buffer"/>. The buffer
    /// is captured by reference; the caller must keep it alive for the
    /// lifetime of this struct.
    /// </summary>
    public OpusRangeDecoder(ReadOnlySpan<byte> buffer)
    {
        _buf = buffer;
        _offs = 0;
        _endOffs = 0;
        _endWindow = 0;
        _nEndBits = 0;
        // Spec: nbits_total = CodeBits + 1 - ((CodeBits - CodeExtra) / SymbolBits) * SymbolBits
        //                   = 33 - (25/8)*8 = 33 - 24 = 9.
        _nBitsTotal = CodeBits + 1 - ((CodeBits - CodeExtra) / SymbolBits) * SymbolBits;
        _rng = 1U << CodeExtra; // 128
        _rem = ReadByte();
        // val = rng - 1 - (rem >> (SymbolBits - CodeExtra)) = 127 - (rem >> 1).
        _val = _rng - 1 - (uint)(_rem >> (SymbolBits - CodeExtra));
        HasError = false;
        Normalize();
    }

    // ------- primary range-coded entry points -------

    /// <summary>
    /// Equivalent to libopus <c>ec_decode</c>. Returns the cumulative
    /// frequency <c>fs</c> ∈ [0, ft) of the next symbol; the caller must
    /// then invoke <see cref="Update"/> with the chosen symbol's [fl, fh)
    /// range to consume it.
    /// </summary>
    public uint Decode(uint ft)
    {
        ArgumentOutOfRangeException.ThrowIfZero(ft);
        uint ext = _rng / ft;
        uint s = _val / ext;
        // ft - min(s+1, ft) — using uint subtraction guarded against wrap.
        uint maxFs = s + 1 < ft ? s + 1 : ft;
        return ft - maxFs;
    }

    /// <summary>
    /// Equivalent to libopus <c>ec_decode_bin</c>: shorthand for
    /// <c>Decode(1U &lt;&lt; bits)</c>. Used when the symbol total is an
    /// exact power of two.
    /// </summary>
    public uint DecodeBin(int bits)
    {
        if ((uint)bits > 31) throw new ArgumentOutOfRangeException(nameof(bits));
        uint ext = _rng >> bits;
        if (ext == 0) throw new InvalidOperationException("Range underflow in DecodeBin.");
        uint s = _val / ext;
        uint ft = 1U << bits;
        uint maxFs = s + 1 < ft ? s + 1 : ft;
        return ft - maxFs;
    }

    /// <summary>
    /// Consume the symbol whose CDF range is [<paramref name="fl"/>,
    /// <paramref name="fh"/>) out of <paramref name="ft"/>. Must be called
    /// immediately after <see cref="Decode"/> with the same <paramref name="ft"/>.
    /// </summary>
    public void Update(uint fl, uint fh, uint ft)
    {
        ArgumentOutOfRangeException.ThrowIfZero(ft);
        if (fh > ft || fl > fh) throw new ArgumentException("Invalid symbol range: require 0 <= fl <= fh <= ft.");
        uint ext = _rng / ft;
        uint s = ext * (ft - fh);
        _val -= s;
        _rng = fl > 0 ? ext * (fh - fl) : _rng - s;
        Normalize();
    }

    /// <summary>
    /// Decode a single bit with probability <c>1 / 2^logp</c> of being 1
    /// (RFC 6716 §4.1.3). Used heavily by CELT for low-probability events.
    /// </summary>
    public int DecodeBitLogP(int logp)
    {
        if ((uint)logp > 31) throw new ArgumentOutOfRangeException(nameof(logp));
        uint r = _rng;
        uint d = _val;
        uint s = r >> logp;
        int ret;
        if (d < s)
        {
            ret = 1;
            _rng = s;
        }
        else
        {
            ret = 0;
            _val = d - s;
            _rng = r - s;
        }
        Normalize();
        return ret;
    }

    /// <summary>
    /// Decode a symbol using an inverse CDF table (libopus <c>ec_dec_icdf</c>).
    /// <paramref name="icdf"/> is monotonically decreasing with
    /// <c>icdf[N-1] == 0</c>; the symbol total is <c>1 &lt;&lt; ftb</c>.
    /// </summary>
    public int DecodeIcdf(ReadOnlySpan<byte> icdf, int ftb)
    {
        if ((uint)ftb > 16) throw new ArgumentOutOfRangeException(nameof(ftb));
        if (icdf.IsEmpty) throw new ArgumentException("icdf table must not be empty.", nameof(icdf));
        if (icdf[icdf.Length - 1] != 0)
            throw new ArgumentException("icdf table must end with 0.", nameof(icdf));
        uint s = _rng;
        uint d = _val;
        uint r = s >> ftb;
        int ret = -1;
        uint t;
        do
        {
            t = s;
            ret++;
            s = r * icdf[ret];
        } while (d < s);
        _val = d - s;
        _rng = t - s;
        Normalize();
        return ret;
    }

    /// <summary>
    /// Decode an unsigned integer in [0, ft) where ft &gt; 1
    /// (libopus <c>ec_dec_uint</c>). Values up to 2^8 use the range coder
    /// directly; larger values split into a coarse range-coded part and a
    /// fine raw-bit suffix.
    /// </summary>
    public uint DecodeUint(uint ft)
    {
        if (ft <= 1) throw new ArgumentOutOfRangeException(nameof(ft), "ft must be > 1.");
        ft -= 1;
        int ftb = Ilog(ft);
        if (ftb > UintBits)
        {
            uint t;
            ftb -= UintBits;
            uint ftScaled = (ft >> ftb) + 1;
            uint s = Decode(ftScaled);
            Update(s, s + 1, ftScaled);
            t = (s << ftb) | DecodeBits(ftb);
            if (t <= ft) return t;
            HasError = true;
            return ft;
        }
        else
        {
            ft += 1;
            uint s = Decode(ft);
            Update(s, s + 1, ft);
            return s;
        }
    }

    /// <summary>
    /// Read <paramref name="bits"/> raw bits from the end of the buffer
    /// (libopus <c>ec_dec_bits</c>). Used by both CELT (for spread/skip
    /// signalling) and by <see cref="DecodeUint(uint)"/>.
    /// </summary>
    public uint DecodeBits(int bits)
    {
        if ((uint)bits > 25) throw new ArgumentOutOfRangeException(nameof(bits));
        uint window = _endWindow;
        int available = _nEndBits;
        if (available < bits)
        {
            do
            {
                window |= (uint)ReadByteFromEnd() << available;
                available += SymbolBits;
            } while (available <= WindowSize - SymbolBits);
        }
        uint ret = window & ((1U << bits) - 1U);
        _endWindow = window >> bits;
        _nEndBits = available - bits;
        _nBitsTotal += bits;
        return ret;
    }

    // ------- introspection -------

    /// <summary>
    /// Number of bits the decoder has consumed so far, rounded up to the
    /// next whole bit. Mirrors libopus <c>ec_tell</c>.
    /// </summary>
    public int Tell() => _nBitsTotal - Ilog(_rng);

    /// <summary>
    /// Number of bits the decoder has consumed so far, in 1/8-bit precision
    /// (mirrors libopus <c>ec_tell_frac</c>). SILK uses this for its
    /// per-frame budget tracking.
    /// </summary>
    public uint TellFrac()
    {
        int nbits = _nBitsTotal << 3;
        int l = Ilog(_rng);
        uint r = _rng >> (l - 16);
        int b = (int)(r >> 12) - 8;
        if (r > _correction[b]) b++;
        l = (l << 3) + b;
        return (uint)(nbits - l);
    }

    // ------- internal helpers -------

    private void Normalize()
    {
        while (_rng <= CodeBot)
        {
            _nBitsTotal += SymbolBits;
            _rng <<= SymbolBits;
            int symPrev = _rem;
            _rem = ReadByte();
            // sym = ((prev_rem << 8) | rem) >> (SymBits - CodeExtra) = ... >> 1
            int sym = ((symPrev << SymbolBits) | _rem) >> (SymbolBits - CodeExtra);
            _val = ((_val << SymbolBits) + (uint)(0xFF & ~sym)) & (CodeTop - 1);
        }
    }

    private int ReadByte()
    {
        if (_offs < _buf.Length) return _buf[_offs++];
        HasError = true;
        return 0;
    }

    private int ReadByteFromEnd()
    {
        if (_endOffs < _buf.Length)
        {
            _endOffs++;
            return _buf[_buf.Length - _endOffs];
        }
        HasError = true;
        return 0;
    }

    /// <summary>
    /// Equivalent to libopus <c>EC_ILOG</c>: position of the highest set
    /// bit (1-indexed). 0 ↦ 0; 1 ↦ 1; 2..3 ↦ 2; 4..7 ↦ 3; ...
    /// </summary>
    internal static int Ilog(uint v) => v == 0 ? 0 : BitOperations.Log2(v) + 1;
}
