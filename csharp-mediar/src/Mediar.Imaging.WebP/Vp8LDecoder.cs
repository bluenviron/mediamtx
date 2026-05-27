namespace Mediar.Imaging.WebP;

/// <summary>
/// LSB-first bit reader matching the WebP-Lossless / VP8L bitstream
/// convention (bits are pulled out of bytes starting at bit 0 of byte 0).
/// </summary>
internal ref struct LsbBitReader
{
    private readonly ReadOnlySpan<byte> _data;
    private int _byteIndex;
    private int _bitsInBuffer;
    private ulong _buffer;

    public LsbBitReader(ReadOnlySpan<byte> data)
    {
        _data = data;
        _byteIndex = 0;
        _bitsInBuffer = 0;
        _buffer = 0;
    }

    /// <summary>True once the reader has hit the end of the buffer.</summary>
    public bool EndOfStream => _byteIndex >= _data.Length && _bitsInBuffer == 0;

    /// <summary>Read <paramref name="count"/> bits (1..32) LSB-first.</summary>
    public uint ReadBits(int count)
    {
        while (_bitsInBuffer < count)
        {
            if (_byteIndex >= _data.Length) break;
            _buffer |= (ulong)_data[_byteIndex++] << _bitsInBuffer;
            _bitsInBuffer += 8;
        }
        uint result = (uint)(_buffer & ((1UL << count) - 1));
        _buffer >>= count;
        _bitsInBuffer -= count;
        if (_bitsInBuffer < 0) _bitsInBuffer = 0;
        return result;
    }
}

/// <summary>
/// Lookup-table Huffman decoder used by the VP8L bitstream. Trees are
/// canonical and built from a code-length array per the WebP-Lossless spec.
/// </summary>
internal sealed class HuffmanDecoder
{
    // We use a simple symbol-table search (binary tree) because VP8L's
    // alphabets are at most ~410 symbols and code lengths are bounded by 15.
    private readonly int[] _left;
    private readonly int[] _right;
    private readonly int[] _symbol;
    public int RootNode { get; }
    public int SingleSymbol { get; }

    private HuffmanDecoder(int single)
    {
        _left = Array.Empty<int>();
        _right = Array.Empty<int>();
        _symbol = Array.Empty<int>();
        RootNode = -1;
        SingleSymbol = single;
    }

    private HuffmanDecoder(int[] left, int[] right, int[] symbol)
    {
        _left = left;
        _right = right;
        _symbol = symbol;
        RootNode = 0;
        SingleSymbol = -1;
    }

    public static HuffmanDecoder FromCodeLengths(ReadOnlySpan<int> codeLengths)
    {
        int n = codeLengths.Length;
        int[] count = new int[16];
        int single = -1;
        int nonZero = 0;
        for (int i = 0; i < n; i++)
        {
            int l = codeLengths[i];
            if (l > 15) throw new ImageFormatException("VP8L code length exceeds 15.");
            if (l > 0) { nonZero++; single = i; count[l]++; }
        }
        if (nonZero == 0) return new HuffmanDecoder(0);
        if (nonZero == 1) return new HuffmanDecoder(single);

        // Canonical: assign codes per length
        int[] codes = new int[n];
        int code = 0;
        int[] nextCode = new int[16];
        for (int len = 1; len <= 15; len++)
        {
            code = (code + count[len - 1]) << 1;
            nextCode[len] = code;
        }
        for (int i = 0; i < n; i++)
        {
            int l = codeLengths[i];
            if (l > 0) codes[i] = nextCode[l]++;
        }

        // Build a binary tree we can walk one bit at a time
        int maxNodes = nonZero * 30 + 16;
        var left = new int[maxNodes];
        var right = new int[maxNodes];
        var symbol = new int[maxNodes];
        Array.Fill(left, -1);
        Array.Fill(right, -1);
        Array.Fill(symbol, -1);
        int nextNode = 1;

        for (int i = 0; i < n; i++)
        {
            int len = codeLengths[i];
            if (len == 0) continue;
            int c = codes[i];
            int node = 0;
            for (int bit = len - 1; bit >= 0; bit--)
            {
                int branch = (c >> bit) & 1;
                int next = branch == 0 ? left[node] : right[node];
                if (next == -1)
                {
                    next = nextNode++;
                    if (branch == 0) left[node] = next; else right[node] = next;
                }
                node = next;
            }
            symbol[node] = i;
        }

        return new HuffmanDecoder(left, right, symbol);
    }

    public int Decode(ref LsbBitReader br)
    {
        if (RootNode == -1) return SingleSymbol;
        int node = 0;
        while (true)
        {
            int branch = (int)br.ReadBits(1);
            node = branch == 0 ? _left[node] : _right[node];
            if (node == -1) throw new ImageFormatException("VP8L Huffman decode walked off tree.");
            if (_symbol[node] != -1) return _symbol[node];
        }
    }
}

/// <summary>
/// WebP-Lossless (VP8L) bitstream decoder. Produces a top-down ARGB8888
/// pixel buffer for the dimensions encoded in the bitstream.
/// </summary>
public static class Vp8LDecoder
{
    private static readonly int[] CodeLengthCodeOrder =
    [
        17, 18, 0, 1, 2, 3, 4, 5, 16, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15,
    ];

    private static readonly int[] DistanceMap =
    [
        // Pseudo-distance-to-(x,y) table from spec section "Distance Mapping"
        24,  7, 23,  25, 40,  6, 39,  41,  22, 8, 38,  26, 21,  5, 9, 37,
        56, 20, 36,  4, 54, 55, 19, 53,  58, 57, 27, 28, 10, 11, 12, 13,
        14, 15, 16, 17, 18, 29, 30, 31,  32, 33, 34, 35, 42, 43, 44, 45,
        46, 47, 48, 49, 50, 51, 52, 59,  60, 61, 62, 63, 64, 65, 66, 67,
        68, 69, 70, 71, 72, 73, 74, 75,  76, 77, 78, 79, 80, 81, 82, 83,
        84, 85, 86, 87, 88, 89, 90, 91,  92, 93, 94, 95, 96, 97, 98, 99,
        100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115,
        116, 117, 118, 119, 120,
    ];

    /// <summary>Decode a VP8L bitstream into a flat ARGB pixel array.</summary>
    public static Vp8LImage Decode(ReadOnlySpan<byte> bytes)
    {
        var br = new LsbBitReader(bytes);
        if (br.ReadBits(8) != 0x2F)
            throw new ImageFormatException("VP8L signature mismatch (expected 0x2F).");

        int width = (int)br.ReadBits(14) + 1;
        int height = (int)br.ReadBits(14) + 1;
        bool alphaIsUsed = br.ReadBits(1) == 1;
        int version = (int)br.ReadBits(3);
        if (version != 0)
            throw new ImageFormatException("Unsupported VP8L version " + version);

        var transforms = new List<Vp8LTransform>();
        int curW = width;
        while (br.ReadBits(1) == 1)
        {
            int type = (int)br.ReadBits(2);
            var tr = ReadTransform(ref br, type, curW, height);
            transforms.Add(tr);
            if (type == 3) curW = tr.PackedWidth;
        }

        var pixels = DecodeImage(ref br, curW, height, allowMetaHuffman: true);

        for (int i = transforms.Count - 1; i >= 0; i--)
        {
            pixels = ApplyTransform(transforms[i], pixels, ref curW, height, width);
        }

        return new Vp8LImage(width, height, pixels, alphaIsUsed);
    }

    private static Vp8LTransform ReadTransform(ref LsbBitReader br, int type, int width, int height)
    {
        switch (type)
        {
            case 0: // PREDICTOR_TRANSFORM
            case 1: // COLOR_TRANSFORM
                {
                    int bits = (int)br.ReadBits(3) + 2;
                    int blockSize = 1 << bits;
                    int blocksX = (width + blockSize - 1) / blockSize;
                    int blocksY = (height + blockSize - 1) / blockSize;
                    var img = DecodeImage(ref br, blocksX, blocksY, allowMetaHuffman: false);
                    return new Vp8LTransform { Type = type, Bits = bits, BlocksX = blocksX, BlocksY = blocksY, BlockImage = img };
                }
            case 2: // SUBTRACT_GREEN — no parameters
                return new Vp8LTransform { Type = 2 };
            case 3: // COLOR_INDEXING_TRANSFORM
                {
                    int paletteSize = (int)br.ReadBits(8) + 1;
                    var palette = DecodeImage(ref br, paletteSize, 1, allowMetaHuffman: false);
                    // Delta-decode palette entries
                    for (int i = 1; i < paletteSize; i++)
                    {
                        palette[i] = AddArgb(palette[i], palette[i - 1]);
                    }
                    int pixelBundleBits = paletteSize <= 2 ? 3 : paletteSize <= 4 ? 2 : paletteSize <= 16 ? 1 : 0;
                    int packedWidth = pixelBundleBits == 0 ? width : (width + (1 << pixelBundleBits) - 1) >> pixelBundleBits;
                    return new Vp8LTransform
                    {
                        Type = 3,
                        Palette = palette,
                        PixelBundleBits = pixelBundleBits,
                        PackedWidth = packedWidth,
                        OriginalWidth = width,
                    };
                }
            default:
                throw new ImageFormatException("Unknown VP8L transform type " + type);
        }
    }

    private static uint[] DecodeImage(ref LsbBitReader br, int width, int height, bool allowMetaHuffman)
    {
        // Color cache
        int colorCacheBits = 0;
        if (br.ReadBits(1) == 1)
        {
            colorCacheBits = (int)br.ReadBits(4);
            if (colorCacheBits < 1 || colorCacheBits > 11)
                throw new ImageFormatException("Invalid color cache bits " + colorCacheBits);
        }

        // Meta-Huffman (only valid in the top-level image, not inside transform sub-images)
        int huffmanBits = 0;
        int huffmanGroups = 1;
        uint[]? metaImage = null;
        if (allowMetaHuffman && br.ReadBits(1) == 1)
        {
            huffmanBits = (int)br.ReadBits(3) + 2;
            int mhWidth = (width + (1 << huffmanBits) - 1) >> huffmanBits;
            int mhHeight = (height + (1 << huffmanBits) - 1) >> huffmanBits;
            metaImage = DecodeImage(ref br, mhWidth, mhHeight, allowMetaHuffman: false);
            int maxGroup = 0;
            foreach (var p in metaImage)
            {
                int g = (int)(((p >> 8) & 0xFF00) | ((p >> 16) & 0xFF));  // R<<8|G
                if (g > maxGroup) maxGroup = g;
            }
            huffmanGroups = maxGroup + 1;
        }

        // Huffman code groups: 5 per group (G/L, R, B, A, Distance)
        var groups = new HuffmanDecoder[huffmanGroups * 5];
        int colorCacheSize = colorCacheBits == 0 ? 0 : 1 << colorCacheBits;
        int greenAlphabetSize = 256 + 24 + colorCacheSize;
        for (int g = 0; g < huffmanGroups; g++)
        {
            groups[g * 5 + 0] = ReadHuffmanCode(ref br, greenAlphabetSize);
            groups[g * 5 + 1] = ReadHuffmanCode(ref br, 256);
            groups[g * 5 + 2] = ReadHuffmanCode(ref br, 256);
            groups[g * 5 + 3] = ReadHuffmanCode(ref br, 256);
            groups[g * 5 + 4] = ReadHuffmanCode(ref br, 40);
        }

        uint[] cache = colorCacheSize > 0 ? new uint[colorCacheSize] : Array.Empty<uint>();
        var pixels = new uint[width * height];
        int pos = 0;
        int total = width * height;

        while (pos < total)
        {
            int px = pos % width;
            int py = pos / width;

            int groupId = 0;
            if (metaImage is not null)
            {
                int mx = px >> huffmanBits;
                int my = py >> huffmanBits;
                int mhWidth = (width + (1 << huffmanBits) - 1) >> huffmanBits;
                uint mh = metaImage[my * mhWidth + mx];
                groupId = (int)(((mh >> 8) & 0xFF00) | ((mh >> 16) & 0xFF));
            }
            int baseIdx = groupId * 5;

            int code = groups[baseIdx + 0].Decode(ref br);
            if (code < 256)
            {
                int red = groups[baseIdx + 1].Decode(ref br);
                int blue = groups[baseIdx + 2].Decode(ref br);
                int alpha = groups[baseIdx + 3].Decode(ref br);
                uint argb = ((uint)alpha << 24) | ((uint)red << 16) | ((uint)code << 8) | (uint)blue;
                pixels[pos] = argb;
                InsertCache(cache, colorCacheBits, argb);
                pos++;
            }
            else if (code < 256 + 24)
            {
                int lenCode = code - 256;
                int length = LzCodeToValue(ref br, lenCode);
                int distCode = groups[baseIdx + 4].Decode(ref br);
                int distRaw = LzCodeToValue(ref br, distCode);
                int dist = MapDistance(distRaw, width);
                if (dist < 1) dist = 1;
                for (int i = 0; i < length && pos < total; i++)
                {
                    pixels[pos] = pos >= dist ? pixels[pos - dist] : 0;
                    InsertCache(cache, colorCacheBits, pixels[pos]);
                    pos++;
                }
            }
            else
            {
                int idx = code - 256 - 24;
                if (idx < 0 || idx >= cache.Length) throw new ImageFormatException("VP8L color cache index OOB.");
                pixels[pos] = cache[idx];
                pos++;
            }
        }

        return pixels;
    }

    private static void InsertCache(uint[] cache, int bits, uint argb)
    {
        if (cache.Length == 0) return;
        const uint kHashMul = 0x1E35A7BD;
        int hash = (int)((argb * kHashMul) >> (32 - bits));
        cache[hash] = argb;
    }

    private static int LzCodeToValue(ref LsbBitReader br, int code)
    {
        if (code < 4) return code + 1;
        int extra = (code - 2) >> 1;
        int offset = (2 + (code & 1)) << extra;
        return offset + 1 + (int)br.ReadBits(extra);
    }

    private static int MapDistance(int rawDist, int xsize)
    {
        if (rawDist > 120) return rawDist - 120;
        int idx = rawDist - 1;
        if ((uint)idx >= DistanceMap.Length) return rawDist;
        int packed = DistanceMap[idx];
        int dy = packed >> 4;
        int dx = (packed & 0xF) - 8;
        int dist = dy * xsize + dx;
        return dist < 1 ? 1 : dist;
    }

    private static HuffmanDecoder ReadHuffmanCode(ref LsbBitReader br, int alphabetSize)
    {
        bool isSimple = br.ReadBits(1) == 1;
        if (isSimple)
        {
            int numSymbols = (int)br.ReadBits(1) + 1;
            bool isFirst8Bit = br.ReadBits(1) == 1;
            var lengths = new int[alphabetSize];
            int s1 = isFirst8Bit ? (int)br.ReadBits(8) : (int)br.ReadBits(1);
            if (s1 >= alphabetSize) s1 = alphabetSize - 1;
            if (numSymbols == 1)
            {
                lengths[s1] = 1;
                // Pad something else with length 1 so the canonical tree is well-defined;
                // a single-symbol tree just returns the symbol.
                return HuffmanDecoder.FromCodeLengths(lengths.AsSpan());
            }
            int s2 = (int)br.ReadBits(8);
            if (s2 >= alphabetSize) s2 = alphabetSize - 1;
            if (s1 == s2) { lengths[s1] = 1; return HuffmanDecoder.FromCodeLengths(lengths.AsSpan()); }
            lengths[s1] = 1;
            lengths[s2] = 1;
            return HuffmanDecoder.FromCodeLengths(lengths.AsSpan());
        }
        else
        {
            int numCodeLengths = (int)br.ReadBits(4) + 4;
            var codeLengthCodeLengths = new int[19];
            for (int i = 0; i < numCodeLengths; i++)
            {
                codeLengthCodeLengths[CodeLengthCodeOrder[i]] = (int)br.ReadBits(3);
            }
            var codeLenHuff = HuffmanDecoder.FromCodeLengths(codeLengthCodeLengths.AsSpan());

            int maxSymbol = alphabetSize;
            if (br.ReadBits(1) == 1)
            {
                int lengthBits = (int)br.ReadBits(3) * 2 + 2;
                maxSymbol = (int)br.ReadBits(lengthBits) + 2;
                if (maxSymbol > alphabetSize) maxSymbol = alphabetSize;
            }

            var codeLengths = new int[alphabetSize];
            int symbol = 0;
            int prev = 8;
            int symbolsRead = 0;
            while (symbol < alphabetSize && symbolsRead < maxSymbol)
            {
                int len = codeLenHuff.Decode(ref br);
                if (len < 16)
                {
                    codeLengths[symbol++] = len;
                    if (len != 0) { prev = len; symbolsRead++; }
                }
                else
                {
                    int extraBits, repeat, value;
                    switch (len)
                    {
                        case 16: extraBits = 2; repeat = 3; value = prev; break;
                        case 17: extraBits = 3; repeat = 3; value = 0; break;
                        case 18: extraBits = 7; repeat = 11; value = 0; break;
                        default: throw new ImageFormatException("Invalid VP8L code length symbol " + len);
                    }
                    int actualRepeat = repeat + (int)br.ReadBits(extraBits);
                    for (int i = 0; i < actualRepeat && symbol < alphabetSize; i++)
                    {
                        codeLengths[symbol++] = value;
                        if (value != 0) symbolsRead++;
                    }
                }
            }
            return HuffmanDecoder.FromCodeLengths(codeLengths.AsSpan());
        }
    }

    private static uint[] ApplyTransform(Vp8LTransform tr, uint[] pixels, ref int curW, int height, int finalWidth)
    {
        switch (tr.Type)
        {
            case 0: return ApplyPredictor(tr, pixels, curW, height);
            case 1: return ApplyColor(tr, pixels, curW, height);
            case 2: ApplySubtractGreen(pixels); return pixels;
            case 3:
                {
                    var unpacked = UnpackColorIndex(tr, pixels, curW, height);
                    curW = tr.OriginalWidth;
                    return unpacked;
                }
            default: throw new ImageFormatException("Unknown VP8L transform " + tr.Type);
        }
    }

    private static uint[] ApplyPredictor(Vp8LTransform tr, uint[] pixels, int width, int height)
    {
        int blockSize = 1 << tr.Bits;
        for (int y = 0; y < height; y++)
        {
            for (int x = 0; x < width; x++)
            {
                int idx = y * width + x;
                uint cur = pixels[idx];
                int predictor;
                if (x == 0 && y == 0)
                {
                    predictor = 0;
                    pixels[idx] = AddArgb(cur, 0xFF000000u);
                    continue;
                }
                if (y == 0)
                {
                    pixels[idx] = AddArgb(cur, pixels[idx - 1]);
                    continue;
                }
                if (x == 0)
                {
                    pixels[idx] = AddArgb(cur, pixels[idx - width]);
                    continue;
                }
                int bx = x / blockSize;
                int by = y / blockSize;
                int blocksX = tr.BlocksX;
                uint block = tr.BlockImage![by * blocksX + bx];
                predictor = (int)((block >> 8) & 0xFF);
                uint p = ComputePredictor(predictor, pixels, idx, width);
                pixels[idx] = AddArgb(cur, p);
            }
        }
        return pixels;
    }

    private static uint ComputePredictor(int mode, uint[] pixels, int idx, int width)
    {
        uint L = pixels[idx - 1];
        uint T = pixels[idx - width];
        uint TL = idx - width - 1 >= 0 ? pixels[idx - width - 1] : 0u;
        uint TR = idx - width + 1 < pixels.Length && (idx % width) + 1 < width ? pixels[idx - width + 1] : T;
        return mode switch
        {
            0 => 0xFF000000u,
            1 => L,
            2 => T,
            3 => TR,
            4 => TL,
            5 => Avg2(Avg2(L, TR), T),
            6 => Avg2(L, TL),
            7 => Avg2(L, T),
            8 => Avg2(TL, T),
            9 => Avg2(T, TR),
            10 => Avg2(Avg2(L, TL), Avg2(T, TR)),
            11 => Select(L, T, TL),
            12 => ClampAddSubFull(L, T, TL),
            13 => ClampAddSubHalf(Avg2(L, T), TL),
            _ => 0,
        };
    }

    private static uint[] ApplyColor(Vp8LTransform tr, uint[] pixels, int width, int height)
    {
        int blockSize = 1 << tr.Bits;
        int blocksX = tr.BlocksX;
        for (int y = 0; y < height; y++)
        {
            for (int x = 0; x < width; x++)
            {
                int idx = y * width + x;
                uint c = pixels[idx];
                int bx = x / blockSize;
                int by = y / blockSize;
                uint t = tr.BlockImage![by * blocksX + bx];
                sbyte gToR = (sbyte)(t & 0xFF);
                sbyte gToB = (sbyte)((t >> 8) & 0xFF);
                sbyte rToB = (sbyte)((t >> 16) & 0xFF);
                byte cg = (byte)((c >> 8) & 0xFF);
                byte cr = (byte)((c >> 16) & 0xFF);
                byte cb = (byte)(c & 0xFF);
                cr = (byte)(cr + (sbyte)((gToR * (sbyte)cg) >> 5));
                cb = (byte)(cb + (sbyte)((gToB * (sbyte)cg) >> 5) + (sbyte)((rToB * (sbyte)cr) >> 5));
                pixels[idx] = (c & 0xFF00FF00u) | ((uint)cr << 16) | cb;
            }
        }
        return pixels;
    }

    private static void ApplySubtractGreen(uint[] pixels)
    {
        for (int i = 0; i < pixels.Length; i++)
        {
            uint c = pixels[i];
            byte g = (byte)((c >> 8) & 0xFF);
            byte r = (byte)(((c >> 16) + g) & 0xFF);
            byte b = (byte)((c + g) & 0xFF);
            pixels[i] = (c & 0xFF00FF00u) | ((uint)r << 16) | b;
        }
    }

    private static uint[] UnpackColorIndex(Vp8LTransform tr, uint[] pixels, int packedWidth, int height)
    {
        int width = tr.OriginalWidth;
        int bundleBits = tr.PixelBundleBits;
        var palette = tr.Palette!;
        int paletteMax = palette.Length;
        var output = new uint[width * height];
        if (bundleBits == 0)
        {
            for (int i = 0; i < pixels.Length && i < output.Length; i++)
            {
                int idx = (int)((pixels[i] >> 8) & 0xFF);
                if ((uint)idx >= (uint)paletteMax) idx = paletteMax - 1;
                output[i] = palette[idx];
            }
            return output;
        }

        int pixelsPerByte = 1 << bundleBits;
        int mask = pixelsPerByte - 1;
        int bitsPerEntry = 8 >> bundleBits;
        int entryMask = (1 << bitsPerEntry) - 1;
        for (int y = 0; y < height; y++)
        {
            for (int bx = 0; bx < packedWidth; bx++)
            {
                uint packed = pixels[y * packedWidth + bx];
                byte g = (byte)((packed >> 8) & 0xFF);
                for (int sub = 0; sub < pixelsPerByte; sub++)
                {
                    int outX = (bx << bundleBits) + sub;
                    if (outX >= width) break;
                    int paletteIdx = (g >> (sub * bitsPerEntry)) & entryMask;
                    if (paletteIdx >= paletteMax) paletteIdx = paletteMax - 1;
                    output[y * width + outX] = palette[paletteIdx];
                }
            }
        }
        return output;
    }

    private static uint AddArgb(uint a, uint b)
    {
        byte aA = (byte)((a >> 24) + (b >> 24));
        byte rA = (byte)(((a >> 16) & 0xFF) + ((b >> 16) & 0xFF));
        byte gA = (byte)(((a >> 8) & 0xFF) + ((b >> 8) & 0xFF));
        byte bA = (byte)((a & 0xFF) + (b & 0xFF));
        return ((uint)aA << 24) | ((uint)rA << 16) | ((uint)gA << 8) | bA;
    }

    private static uint Avg2(uint a, uint b)
    {
        byte aa = (byte)(((a >> 24) + (b >> 24)) >> 1);
        byte ar = (byte)((((a >> 16) & 0xFF) + ((b >> 16) & 0xFF)) >> 1);
        byte ag = (byte)((((a >> 8) & 0xFF) + ((b >> 8) & 0xFF)) >> 1);
        byte ab = (byte)(((a & 0xFF) + (b & 0xFF)) >> 1);
        return ((uint)aa << 24) | ((uint)ar << 16) | ((uint)ag << 8) | ab;
    }

    private static uint Select(uint L, uint T, uint TL)
    {
        int pa = SumAbsDiff(L, TL);
        int pb = SumAbsDiff(T, TL);
        return pa < pb ? L : T;
    }

    private static int SumAbsDiff(uint a, uint b)
    {
        int s = Math.Abs(((int)(a >> 24)) - ((int)(b >> 24)));
        s += Math.Abs(((int)(a >> 16) & 0xFF) - ((int)(b >> 16) & 0xFF));
        s += Math.Abs(((int)(a >> 8) & 0xFF) - ((int)(b >> 8) & 0xFF));
        s += Math.Abs(((int)a & 0xFF) - ((int)b & 0xFF));
        return s;
    }

    private static uint ClampAddSubFull(uint a, uint b, uint c)
    {
        byte aa = (byte)Math.Clamp(((int)(a >> 24)) + ((int)(b >> 24)) - ((int)(c >> 24)), 0, 255);
        byte ar = (byte)Math.Clamp(((int)(a >> 16) & 0xFF) + ((int)(b >> 16) & 0xFF) - ((int)(c >> 16) & 0xFF), 0, 255);
        byte ag = (byte)Math.Clamp(((int)(a >> 8) & 0xFF) + ((int)(b >> 8) & 0xFF) - ((int)(c >> 8) & 0xFF), 0, 255);
        byte ab = (byte)Math.Clamp(((int)a & 0xFF) + ((int)b & 0xFF) - ((int)c & 0xFF), 0, 255);
        return ((uint)aa << 24) | ((uint)ar << 16) | ((uint)ag << 8) | ab;
    }

    private static uint ClampAddSubHalf(uint a, uint b)
    {
        byte aa = (byte)Math.Clamp(((int)(a >> 24)) + ((((int)(a >> 24)) - ((int)(b >> 24))) >> 1), 0, 255);
        byte ar = (byte)Math.Clamp(((int)(a >> 16) & 0xFF) + ((((int)(a >> 16) & 0xFF) - ((int)(b >> 16) & 0xFF)) >> 1), 0, 255);
        byte ag = (byte)Math.Clamp(((int)(a >> 8) & 0xFF) + ((((int)(a >> 8) & 0xFF) - ((int)(b >> 8) & 0xFF)) >> 1), 0, 255);
        byte ab = (byte)Math.Clamp(((int)a & 0xFF) + ((((int)a & 0xFF) - ((int)b & 0xFF)) >> 1), 0, 255);
        return ((uint)aa << 24) | ((uint)ar << 16) | ((uint)ag << 8) | ab;
    }
}

internal sealed class Vp8LTransform
{
    public int Type { get; init; }
    public int Bits { get; init; }
    public int BlocksX { get; init; }
    public int BlocksY { get; init; }
    public uint[]? BlockImage { get; init; }
    public uint[]? Palette { get; init; }
    public int PixelBundleBits { get; init; }
    public int PackedWidth { get; init; }
    public int OriginalWidth { get; init; }
}

/// <summary>Output of <see cref="Vp8LDecoder.Decode"/>.</summary>
public readonly record struct Vp8LImage(int Width, int Height, uint[] PixelsArgb, bool AlphaUsed);
