// [B1] to be unified when B1 merges in.
//
// Minimal Opus range encoder placeholder for Phase B2. This is a clean-room
// port of libopus celt/entenc.c (the inverse of celt/entdec.c, which the
// decoder side already mirrors as OpusRangeDecoder). It is the smallest
// surface area Phase B2 needs to drive coarse-energy + PVQ writes and is
// expected to be replaced wholesale by the B1 sibling session — the public
// API matches what B1 plans to ship so the call-sites can stay put.

namespace Mediar.Codecs.Opus.Encoder;

/// <summary>
/// Opus range encoder (RFC 6716 §4.1, inverse of <c>OpusRangeDecoder</c>).
/// Implemented as a <c>ref struct</c> so the per-packet state lives on the
/// caller's stack — matching the decoder side.
/// </summary>
/// <remarks>
/// <para>
/// Range-coded symbols accumulate at the front of <c>Buffer</c>; raw bits
/// accumulate at the back. After all symbols are written, call
/// <see cref="Finish"/> to flush the carry chain and pack the trailing raw
/// bits, then read <see cref="ByteCount"/> for the encoded packet length.
/// </para>
/// </remarks>
public ref struct OpusRangeEncoder
{
    /// <summary>Bits per coded symbol (always 8).</summary>
    public const int SymbolBits = 8;

    /// <summary>Total bits in the running range.</summary>
    public const int CodeBits = 32;

    /// <summary>EC_CODE_SHIFT = EC_CODE_BITS - EC_SYM_BITS - 1 = 23 — the carry-out shift.</summary>
    public const int CodeShift = CodeBits - SymbolBits - 1;

    /// <summary>EC_CODE_EXTRA = ((CodeBits - 2) % SymbolBits) + 1 = 7.</summary>
    public const int CodeExtra = ((CodeBits - 2) % SymbolBits) + 1;

    /// <summary>Upper boundary of the range (1 &lt;&lt; 31).</summary>
    public const uint CodeTop = 1U << (CodeBits - 1);

    /// <summary>Renormalisation threshold (1 &lt;&lt; 23).</summary>
    public const uint CodeBot = CodeTop >> SymbolBits;

    /// <summary>Threshold above which <see cref="EncodeUint"/> shifts to raw bits.</summary>
    public const int UintBits = 8;

    /// <summary>Bits available in the raw-bit writer window.</summary>
    public const int WindowSize = 32;

    private readonly Span<byte> _buf;
    private uint _low;          // current bottom of the encoding range
    private uint _rng;          // current range size
    private int _rem;           // saved byte being constructed (-1 if none)
    private int _ext;           // pending carry-propagating bytes (0xFF chain)
    private int _offs;          // forward write pointer
    private int _endOffs;       // bytes already written from the END (raw bits)
    private uint _endWindow;    // bit window holding pending raw bits
    private int _nEndBits;      // bits currently buffered in _endWindow
    private int _nBitsTotal;    // total bits consumed (for Tell / TellFrac)

    /// <summary>True if a write overflowed the buffer.</summary>
    public bool HasError { get; private set; }

    /// <summary>Underlying buffer.</summary>
    public readonly Span<byte> Buffer => _buf;

    /// <summary>Range size — for diagnostics and tests.</summary>
    public readonly uint Range => _rng;

    /// <summary>Low value — for diagnostics and tests.</summary>
    public readonly uint Low => _low;

    /// <summary>Initialise an encoder that writes to <paramref name="buffer"/>.</summary>
    public OpusRangeEncoder(Span<byte> buffer)
    {
        _buf = buffer;
        _low = 0;
        _rng = CodeTop;
        _rem = -1;
        _ext = 0;
        _offs = 0;
        _endOffs = 0;
        _endWindow = 0;
        _nEndBits = 0;
        // Mirror of OpusRangeDecoder init: same starting bit count.
        _nBitsTotal = CodeBits + 1 - ((CodeBits - CodeExtra) / SymbolBits) * SymbolBits;
        HasError = false;
    }

    /// <summary>
    /// Number of bytes that must be passed to the decoder for the
    /// produced packet to round-trip. If any raw bits were written, the
    /// raw-bit window sits at the back of the buffer so the full buffer
    /// must be shipped; otherwise only the range-coded prefix is needed.
    /// </summary>
    public readonly int ByteCount => _endOffs > 0 ? _buf.Length : _offs;

    /// <summary>Bits used so far (libopus ec_tell).</summary>
    public readonly int Tell() => _nBitsTotal - EcIlog(_rng);

    /// <summary>Bits used so far in 1/8-bit precision (libopus ec_tell_frac).</summary>
    public readonly int TellFrac()
    {
        // 1/8-bit precision following libopus ec_tell_frac.
        int nbits = _nBitsTotal << 3;
        int l = EcIlog(_rng);
        uint r = _rng >> (l - 16);
        // Refine to 1/8-bit precision via 3 squaring iterations.
        for (int i = 0; i < 3; i++)
        {
            r = (r * r) >> 15;
            int b = (int)(r >> 16);
            l = (l << 1) | b;
            r >>= b;
        }
        return nbits - l;
    }

    /// <summary>Encode a symbol with [fl,fh) of ft (libopus ec_encode).</summary>
    public void Encode(uint fl, uint fh, uint ft)
    {
        uint r = _rng / ft;
        if (fl > 0)
        {
            _low += _rng - r * (ft - fl);
            _rng = r * (fh - fl);
        }
        else
        {
            _rng -= r * (ft - fh);
        }
        Normalize();
    }

    /// <summary>Encode a symbol with [fl,fh) of (1 &lt;&lt; bits) (ec_encode_bin).</summary>
    public void EncodeBin(uint fl, uint fh, int bits)
    {
        uint r = _rng >> bits;
        if (fl > 0)
        {
            _low += _rng - r * ((1U << bits) - fl);
            _rng = r * (fh - fl);
        }
        else
        {
            _rng -= r * ((1U << bits) - fh);
        }
        Normalize();
    }

    /// <summary>Encode a binary symbol with probability 1/2^logp (ec_enc_bit_logp).</summary>
    public void EncodeBitLogP(int value, int logp)
    {
        uint r = _rng;
        uint s = r >> logp;
        if (value != 0)
        {
            _low += r - s;
            _rng = s;
        }
        else
        {
            _rng = r - s;
        }
        Normalize();
    }

    /// <summary>Encode a symbol with the inverse CDF table (ec_enc_icdf).</summary>
    public void EncodeIcdf(int symbol, ReadOnlySpan<byte> icdf, int ftb)
    {
        uint ft = 1U << ftb;
        uint r = _rng >> ftb;
        uint fl = symbol > 0 ? ft - icdf[symbol - 1] : 0;
        uint fh = ft - icdf[symbol];
        if (fl > 0)
        {
            _low += _rng - r * (ft - fl);
            _rng = r * (fh - fl);
        }
        else
        {
            _rng -= r * (ft - fh);
        }
        Normalize();
    }

    /// <summary>Encode an integer in [0, ft) for ft &gt; 1 (ec_enc_uint).</summary>
    public void EncodeUint(uint value, uint ft)
    {
        ArgumentOutOfRangeException.ThrowIfLessThanOrEqual(ft, 1U);
        ArgumentOutOfRangeException.ThrowIfGreaterThanOrEqual(value, ft);
        ft--;
        int ftb = EcIlog(ft);
        if (ftb > UintBits)
        {
            ftb -= UintBits;
            uint hi = (ft >> ftb) + 1;
            Encode(value >> ftb, (value >> ftb) + 1, hi);
            EncodeBitsRaw(value & ((1U << ftb) - 1), ftb);
        }
        else
        {
            Encode(value, value + 1, ft + 1);
        }
    }

    /// <summary>
    /// Encode a signed integer using a discretised Laplace distribution
    /// (libopus <c>ec_laplace_encode</c>, RFC 6716 §4.3.2.1, inverse of
    /// <c>OpusRangeDecoder.DecodeLaplace</c>). Used by the CELT
    /// coarse-energy encoder. Returns the (possibly clamped) value
    /// actually encoded — the encoder may saturate the magnitude to keep
    /// the cumulative range within the [0, 32768) symbol total.
    /// </summary>
    /// <param name="value">Signed integer to encode.</param>
    /// <param name="fs">Initial 15-bit symbol total for the centre bin
    /// (<c>e_prob_model[LM][intra][2i] &lt;&lt; 7</c>).</param>
    /// <param name="decay">Geometric decay factor in Q14
    /// (<c>e_prob_model[LM][intra][2i+1] &lt;&lt; 6</c>).</param>
    public int EncodeLaplace(int value, uint fs, int decay)
    {
        const uint LaplaceMinP = 1;
        const uint LaplaceTwoNMinP = 32;

        uint fl = 0;
        if (value != 0)
        {
            int s = value < 0 ? -1 : 0;
            int absVal = (value + s) ^ s;
            fl = fs;
            uint ft = 32768u - LaplaceTwoNMinP - fs;
            fs = (uint)((int)ft * (16384 - decay) >> 15) + LaplaceMinP;
            for (int i = 1; i < absVal && fs > LaplaceMinP; i++)
            {
                fs *= 2;
                fl += fs;
                fs = (uint)(((int)fs - 2 * (int)LaplaceMinP) * decay >> 15) + LaplaceMinP;
            }
            if (fs <= LaplaceMinP)
            {
                int di = absVal - 1;
                // Saturate the magnitude to what the symbol space allows.
                int maxDi = (int)((32768u - fl - 2 * LaplaceMinP) / (2 * LaplaceMinP));
                if (di > maxDi)
                {
                    di = maxDi;
                    absVal = di + 1;
                }
                fl += (uint)(2 * di) * LaplaceMinP;
            }
            if (s != 0)
            {
                // value was negative — choose the lower half of the centre band.
                value = -absVal;
            }
            else
            {
                fl += fs;
                value = absVal;
            }
        }
        uint fh = fl + fs;
        if (fh > 32768u) fh = 32768u;
        EncodeBin(fl, fh, 15);
        return value;
    }


    public void EncodeBitsRaw(uint value, int bits)
    {
        if ((uint)bits > 25) throw new ArgumentOutOfRangeException(nameof(bits));
        uint window = _endWindow | (value << _nEndBits);
        int nbits = _nEndBits + bits;
        while (nbits >= SymbolBits)
        {
            if (_endOffs >= _buf.Length - _offs)
            {
                HasError = true;
                return;
            }
            _buf[_buf.Length - 1 - _endOffs] = (byte)window;
            _endOffs++;
            window >>= SymbolBits;
            nbits -= SymbolBits;
        }
        _endWindow = window;
        _nEndBits = nbits;
        _nBitsTotal += bits;
    }

    /// <summary>
    /// Reserve <paramref name="bits"/> bits for raw output at the back of the
    /// buffer without actually writing them yet (libopus ec_enc_shrink). Bits
    /// are still counted against the budget.
    /// </summary>
    public void Patch(int bits)
    {
        _nBitsTotal += bits;
    }

    /// <summary>Flush the carry chain and pack the trailing raw bits (ec_enc_done).</summary>
    public void Finish()
    {
        // libopus celt/entenc.c:ec_enc_done — pick an in-range terminator
        // value with the fewest bits, push it through the carry chain,
        // zero-pad the gap, and OR the raw-bit window into the tail.
        int l = CodeBits - EcIlog(_rng);
        uint mask = (CodeTop - 1) >> l;
        uint end = (_low + mask) & ~mask;
        if ((end | mask) >= _low + _rng)
        {
            l++;
            mask >>= 1;
            end = (_low + mask) & ~mask;
        }
        while (l > 0)
        {
            CarryOut((int)(end >> CodeShift));
            end = (end << SymbolBits) & (CodeTop - 1);
            l -= SymbolBits;
        }
        // Flush any saved byte / carry chain that the l-loop left in
        // _rem. libopus relies on the caller's buffer being sized so
        // the gap-clear silently absorbs this; here we commit it
        // explicitly so a tight buffer round-trips.
        if (_rem >= 0 || _ext > 0)
        {
            CarryOut(0);
            _rem = -1;
        }
        // Flush full raw-bit bytes into the back of the buffer.
        uint window = _endWindow;
        int used = _nEndBits;
        while (used >= SymbolBits)
        {
            if (_endOffs >= _buf.Length - _offs)
            {
                HasError = true;
                return;
            }
            _buf[_buf.Length - 1 - _endOffs] = (byte)(window & 0xFFu);
            _endOffs++;
            window >>= SymbolBits;
            used -= SymbolBits;
        }
        if (!HasError)
        {
            // Zero-pad the gap between the forward carry chain (front of
            // the buffer) and the raw-bit window (back). The decoder reads
            // these zeros as harmless filler.
            int gapStart = _offs;
            int gapLen = _buf.Length - _offs - _endOffs;
            if (gapLen > 0)
                _buf.Slice(gapStart, gapLen).Clear();
            // OR the leftover < 8-bit raw window into the last byte
            // already inside the raw-bit region (libopus does the same).
            if (used > 0)
            {
                if (_endOffs >= _buf.Length)
                {
                    HasError = true;
                    return;
                }
                int negL = -l; // bits available in the carry-chain tail byte
                if (_offs + _endOffs >= _buf.Length && negL < used)
                {
                    window &= (1U << negL) - 1;
                    HasError = true;
                }
                _buf[_buf.Length - 1 - _endOffs] |= (byte)window;
                _endOffs++;
            }
        }
        _endWindow = 0;
        _nEndBits = 0;
    }

    private void Normalize()
    {
        while (_rng <= CodeBot)
        {
            CarryOut((int)(_low >> CodeShift));
            _low = (_low << SymbolBits) & (CodeTop - 1);
            _rng <<= SymbolBits;
            _nBitsTotal += SymbolBits;
        }
    }

    private void CarryOut(int c)
    {
        // libopus celt/entenc.c:ec_enc_carry_out — handle the 0xFF carry
        // propagation chain. c is the next byte (0..255 plus carry bit).
        if (c != ((1 << SymbolBits) - 1))
        {
            // Resolve carry into the previous saved byte.
            int carry = c >> SymbolBits;
            if (_rem >= 0)
            {
                WriteForward((byte)(_rem + carry));
            }
            if (_ext > 0)
            {
                byte fill = (byte)(((1 << SymbolBits) - 1) + carry);
                while (_ext-- > 0)
                {
                    WriteForward(fill);
                }
                _ext = 0;
            }
            _rem = c & ((1 << SymbolBits) - 1);
        }
        else
        {
            _ext++;
        }
    }

    private void WriteForward(byte b)
    {
        if (_offs >= _buf.Length - _endOffs)
        {
            HasError = true;
            return;
        }
        _buf[_offs++] = b;
    }

    private static int EcIlog(uint v)
    {
        if (v == 0) return 0;
        int ret = 0;
        while (v > 0)
        {
            ret++;
            v >>= 1;
        }
        return ret;
    }
}
