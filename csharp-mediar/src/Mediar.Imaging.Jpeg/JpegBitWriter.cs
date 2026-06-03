using System.Runtime.CompilerServices;

namespace Mediar.Imaging.Jpeg;

/// <summary>
/// MSB-first bit writer over a backing <see cref="System.IO.Stream"/>
/// that performs JPEG byte-stuffing: every emitted <c>0xFF</c> data
/// byte is followed by an inserted <c>0x00</c>. Used by the entropy
/// coder in <see cref="JpegBaselineEncoder"/>.
/// </summary>
internal struct JpegBitWriter
{
    private readonly Stream _output;
    private uint _buffer;
    private int _bits;

    public JpegBitWriter(Stream output)
    {
        _output = output;
        _buffer = 0;
        _bits = 0;
    }

    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public void WriteBits(int code, int length)
    {
        if (length <= 0) return;
        _buffer = (_buffer << length) | ((uint)code & ((1u << length) - 1u));
        _bits += length;
        while (_bits >= 8)
        {
            _bits -= 8;
            byte b = (byte)((_buffer >> _bits) & 0xFF);
            _output.WriteByte(b);
            if (b == 0xFF) _output.WriteByte(0x00);
        }
    }

    /// <summary>Pad with 1-bits up to the next byte boundary (T.81 §F.1.2.3).</summary>
    public void Flush()
    {
        if (_bits > 0)
        {
            int pad = 8 - _bits;
            WriteBits((1 << pad) - 1, pad);
        }
    }
}

/// <summary>
/// Lookup table for one DC or AC Huffman table used by the encoder:
/// for each 1-byte symbol value <c>v</c>, <c>Codes[v]</c> stores the
/// code word and <c>Sizes[v]</c> stores its length in bits (1..16).
/// Built from the <c>BITS</c> and <c>HUFFVAL</c> lists in T.81 §C.2.
/// </summary>
internal sealed class JpegEncoderHuffmanTable
{
    public int[] Codes { get; } = new int[256];
    public int[] Sizes { get; } = new int[256];
    public byte[] BitsCounts { get; }
    public byte[] Values { get; }

    public JpegEncoderHuffmanTable(byte[] bitsCounts, byte[] values)
    {
        if (bitsCounts.Length != 16)
        {
            throw new ArgumentException("BITS must have 16 entries.", nameof(bitsCounts));
        }
        BitsCounts = bitsCounts;
        Values = values;

        Span<int> size = stackalloc int[256];
        int p = 0;
        for (int l = 1; l <= 16; l++)
        {
            int n = bitsCounts[l - 1];
            for (int j = 0; j < n; j++)
            {
                if (p >= 256) throw new InvalidOperationException("Huffman BITS overflow.");
                size[p++] = l;
            }
        }
        int total = p;

        Span<int> code = stackalloc int[256];
        int c = 0;
        int si = total > 0 ? size[0] : 0;
        int k = 0;
        while (k < total)
        {
            while (k < total && size[k] == si)
            {
                code[k++] = c++;
            }
            if (k >= total) break;
            while (size[k] != si)
            {
                c <<= 1;
                si++;
            }
        }

        if (values.Length != total)
        {
            throw new ArgumentException(
                $"HUFFVAL length {values.Length} does not match BITS sum {total}.", nameof(values));
        }

        for (int i = 0; i < total; i++)
        {
            Codes[values[i]] = code[i];
            Sizes[values[i]] = size[i];
        }
    }
}
