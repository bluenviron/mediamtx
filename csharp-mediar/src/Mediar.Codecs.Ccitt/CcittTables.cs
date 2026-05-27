namespace Mediar.Codecs.Ccitt;

/// <summary>
/// CCITT T.4 / T.6 Modified Huffman tables (white &amp; black run codes,
/// extended makeup codes, two-dimensional codes, EOL). Tables are taken
/// from ITU-T Rec. T.4 (1993) Annex A and T.6 (1988) Annex A.
/// </summary>
/// <remarks>
/// The arrays here store the canonical bit strings ("0011010" etc.) and
/// the static constructor expands them into 13-bit lookup tables for
/// decode and into per-run dictionaries for encode. The lookup approach
/// makes a single peek-of-13-bits sufficient to identify any MH code,
/// which is the hot path for both T.4 and T.6.
/// </remarks>
internal static class CcittTables
{
    /// <summary>Length in bits of the peek window used for MH lookup.</summary>
    public const int MhPeekBits = 13;

    /// <summary>Length in bits of the peek window used for 2-D lookup.</summary>
    public const int TwoDPeekBits = 7;

    /// <summary>EOL = "000000000001" (12 bits). Reported as <c>Run = -2</c>.</summary>
    public const int EolRun = -2;

    /// <summary>White terminating codes: runs 0..63.</summary>
    public static readonly (string Code, int Run)[] WhiteTerminating =
    [
        ("00110101", 0), ("000111", 1), ("0111", 2), ("1000", 3),
        ("1011", 4), ("1100", 5), ("1110", 6), ("1111", 7),
        ("10011", 8), ("10100", 9), ("00111", 10), ("01000", 11),
        ("001000", 12), ("000011", 13), ("110100", 14), ("110101", 15),
        ("101010", 16), ("101011", 17), ("0100111", 18), ("0001100", 19),
        ("0001000", 20), ("0010111", 21), ("0000011", 22), ("0000100", 23),
        ("0101000", 24), ("0101011", 25), ("0010011", 26), ("0100100", 27),
        ("0011000", 28), ("00000010", 29), ("00000011", 30), ("00011010", 31),
        ("00011011", 32), ("00010010", 33), ("00010011", 34), ("00010100", 35),
        ("00010101", 36), ("00010110", 37), ("00010111", 38), ("00101000", 39),
        ("00101001", 40), ("00101010", 41), ("00101011", 42), ("00101100", 43),
        ("00101101", 44), ("00000100", 45), ("00000101", 46), ("00001010", 47),
        ("00001011", 48), ("01010010", 49), ("01010011", 50), ("01010100", 51),
        ("01010101", 52), ("00100100", 53), ("00100101", 54), ("01011000", 55),
        ("01011001", 56), ("01011010", 57), ("01011011", 58), ("01001010", 59),
        ("01001011", 60), ("00110010", 61), ("00110011", 62), ("00110100", 63),
    ];

    /// <summary>White makeup codes: runs 64..1728 step 64.</summary>
    public static readonly (string Code, int Run)[] WhiteMakeup =
    [
        ("11011", 64), ("10010", 128), ("010111", 192), ("0110111", 256),
        ("00110110", 320), ("00110111", 384), ("01100100", 448), ("01100101", 512),
        ("01101000", 576), ("01100111", 640), ("011001100", 704), ("011001101", 768),
        ("011010010", 832), ("011010011", 896), ("011010100", 960), ("011010101", 1024),
        ("011010110", 1088), ("011010111", 1152), ("011011000", 1216), ("011011001", 1280),
        ("011011010", 1344), ("011011011", 1408), ("010011000", 1472), ("010011001", 1536),
        ("010011010", 1600), ("011000", 1664), ("010011011", 1728),
    ];

    /// <summary>Black terminating codes: runs 0..63.</summary>
    public static readonly (string Code, int Run)[] BlackTerminating =
    [
        ("0000110111", 0), ("010", 1), ("11", 2), ("10", 3),
        ("011", 4), ("0011", 5), ("0010", 6), ("00011", 7),
        ("000101", 8), ("000100", 9), ("0000100", 10), ("0000101", 11),
        ("0000111", 12), ("00000100", 13), ("00000111", 14), ("000011000", 15),
        ("0000010111", 16), ("0000011000", 17), ("0000001000", 18), ("00001100111", 19),
        ("00001101000", 20), ("00001101100", 21), ("00000110111", 22), ("00000101000", 23),
        ("00000010111", 24), ("00000011000", 25), ("000011001010", 26), ("000011001011", 27),
        ("000011001100", 28), ("000011001101", 29), ("000001101000", 30), ("000001101001", 31),
        ("000001101010", 32), ("000001101011", 33), ("000011010010", 34), ("000011010011", 35),
        ("000011010100", 36), ("000011010101", 37), ("000011010110", 38), ("000011010111", 39),
        ("000001101100", 40), ("000001101101", 41), ("000011011010", 42), ("000011011011", 43),
        ("000001010100", 44), ("000001010101", 45), ("000001010110", 46), ("000001010111", 47),
        ("000001100100", 48), ("000001100101", 49), ("000001010010", 50), ("000001010011", 51),
        ("000000100100", 52), ("000000110111", 53), ("000000111000", 54), ("000000100111", 55),
        ("000000101000", 56), ("000001011000", 57), ("000001011001", 58), ("000000101011", 59),
        ("000000101100", 60), ("000001011010", 61), ("000001100110", 62), ("000001100111", 63),
    ];

    /// <summary>Black makeup codes: runs 64..1728 step 64.</summary>
    public static readonly (string Code, int Run)[] BlackMakeup =
    [
        ("0000001111", 64), ("000011001000", 128), ("000011001001", 192), ("000001011011", 256),
        ("000000110011", 320), ("000000110100", 384), ("000000110101", 448), ("0000001101100", 512),
        ("0000001101101", 576), ("0000001001010", 640), ("0000001001011", 704), ("0000001001100", 768),
        ("0000001001101", 832), ("0000001110010", 896), ("0000001110011", 960), ("0000001110100", 1024),
        ("0000001110101", 1088), ("0000001110110", 1152), ("0000001110111", 1216), ("0000001010010", 1280),
        ("0000001010011", 1344), ("0000001010100", 1408), ("0000001010101", 1472), ("0000001011010", 1536),
        ("0000001011011", 1600), ("0000001100100", 1664), ("0000001100101", 1728),
    ];

    /// <summary>Extended makeup codes (1792..2560), shared by white and black.</summary>
    public static readonly (string Code, int Run)[] ExtendedMakeup =
    [
        ("00000001000", 1792), ("00000001100", 1856), ("00000001101", 1920),
        ("000000010010", 1984), ("000000010011", 2048), ("000000010100", 2112),
        ("000000010101", 2176), ("000000010110", 2240), ("000000010111", 2304),
        ("000000011100", 2368), ("000000011101", 2432), ("000000011110", 2496),
        ("000000011111", 2560),
    ];

    /// <summary>2-D mode codes (T.4 2D and T.6 MMR).</summary>
    public static readonly (string Code, TwoDMode Mode)[] TwoDimensional =
    [
        ("0001", TwoDMode.Pass),
        ("001", TwoDMode.Horizontal),
        ("1", TwoDMode.V0),
        ("011", TwoDMode.Vr1),
        ("010", TwoDMode.Vl1),
        ("000011", TwoDMode.Vr2),
        ("000010", TwoDMode.Vl2),
        ("0000011", TwoDMode.Vr3),
        ("0000010", TwoDMode.Vl3),
    ];

    /// <summary>End-of-line: 11 zero bits followed by a 1.</summary>
    public const string EolBits = "000000000001";

    public static readonly MhSym[] WhiteLookup;
    public static readonly MhSym[] BlackLookup;
    public static readonly TwoDSym[] TwoDLookup;

    public static readonly IReadOnlyDictionary<int, (uint Bits, int Length)> WhiteEncode;
    public static readonly IReadOnlyDictionary<int, (uint Bits, int Length)> BlackEncode;

#pragma warning disable CA1810 // initialize static fields inline — table build is non-trivial
    static CcittTables()
#pragma warning restore CA1810
    {
        WhiteLookup = new MhSym[1 << MhPeekBits];
        BlackLookup = new MhSym[1 << MhPeekBits];
        TwoDLookup = new TwoDSym[1 << TwoDPeekBits];

        for (int i = 0; i < WhiteLookup.Length; i++)
        {
            WhiteLookup[i] = new MhSym(-1, 0);
            BlackLookup[i] = new MhSym(-1, 0);
        }
        for (int i = 0; i < TwoDLookup.Length; i++)
        {
            TwoDLookup[i] = new TwoDSym(TwoDMode.Invalid, 0);
        }

        var (eolBits, eolLen) = ParseCode(EolBits);
        PlantMh(WhiteLookup, eolBits, eolLen, new MhSym(EolRun, (byte)eolLen));
        PlantMh(BlackLookup, eolBits, eolLen, new MhSym(EolRun, (byte)eolLen));

        foreach (var (code, run) in WhiteTerminating.Concat(WhiteMakeup).Concat(ExtendedMakeup))
        {
            var (bits, len) = ParseCode(code);
            PlantMh(WhiteLookup, bits, len, new MhSym((short)run, (byte)len));
        }
        foreach (var (code, run) in BlackTerminating.Concat(BlackMakeup).Concat(ExtendedMakeup))
        {
            var (bits, len) = ParseCode(code);
            PlantMh(BlackLookup, bits, len, new MhSym((short)run, (byte)len));
        }
        foreach (var (code, mode) in TwoDimensional)
        {
            var (bits, len) = ParseCode(code);
            PlantTwoD(TwoDLookup, bits, len, new TwoDSym(mode, (byte)len));
        }

        var whiteEnc = new Dictionary<int, (uint Bits, int Length)>();
        var blackEnc = new Dictionary<int, (uint Bits, int Length)>();
        foreach (var (code, run) in WhiteTerminating.Concat(WhiteMakeup).Concat(ExtendedMakeup))
        {
            var (bits, len) = ParseCode(code);
            whiteEnc[run] = (bits, len);
        }
        foreach (var (code, run) in BlackTerminating.Concat(BlackMakeup).Concat(ExtendedMakeup))
        {
            var (bits, len) = ParseCode(code);
            blackEnc[run] = (bits, len);
        }
        WhiteEncode = whiteEnc;
        BlackEncode = blackEnc;
    }

    private static (uint Bits, int Length) ParseCode(string code)
    {
        uint bits = 0;
        for (int i = 0; i < code.Length; i++)
        {
            bits = (bits << 1) | (uint)(code[i] - '0');
        }
        return (bits, code.Length);
    }

    private static void PlantMh(MhSym[] table, uint bits, int len, MhSym sym)
    {
        int shift = MhPeekBits - len;
        int baseIdx = (int)(bits << shift);
        int count = 1 << shift;
        for (int i = 0; i < count; i++)
        {
            table[baseIdx + i] = sym;
        }
    }

    private static void PlantTwoD(TwoDSym[] table, uint bits, int len, TwoDSym sym)
    {
        int shift = TwoDPeekBits - len;
        int baseIdx = (int)(bits << shift);
        int count = 1 << shift;
        for (int i = 0; i < count; i++)
        {
            table[baseIdx + i] = sym;
        }
    }
}

/// <summary>Single MH lookup entry. <see cref="Run"/> = -1 means "invalid", -2 means EOL.</summary>
internal readonly record struct MhSym(short Run, byte Length);

/// <summary>2-D code mode.</summary>
internal enum TwoDMode : byte
{
    Invalid = 0,
    Pass,
    Horizontal,
    V0,
    Vr1,
    Vr2,
    Vr3,
    Vl1,
    Vl2,
    Vl3,
}

/// <summary>Single 2-D lookup entry.</summary>
internal readonly record struct TwoDSym(TwoDMode Mode, byte Length);
