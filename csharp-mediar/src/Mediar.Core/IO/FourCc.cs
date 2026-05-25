using System.Runtime.CompilerServices;

namespace Mediar.IO;

/// <summary>
/// A four-character code (e.g. <c>moov</c>, <c>RIFF</c>) packed into a <see cref="uint"/>
/// in big-endian byte order: <c>(byte0 &lt;&lt; 24) | (byte1 &lt;&lt; 16) | (byte2 &lt;&lt; 8) | byte3</c>.
/// </summary>
public readonly record struct FourCc(uint Value)
{
    /// <summary>Construct from four ASCII characters.</summary>
    public FourCc(char a, char b, char c, char d) : this(
        ((uint)(byte)a << 24) | ((uint)(byte)b << 16) | ((uint)(byte)c << 8) | (byte)d)
    {
    }

    /// <summary>Construct from a 4-character string.</summary>
    public FourCc(string ascii) : this(FromString(ascii))
    {
    }

    private static uint FromString(string ascii)
    {
        ArgumentNullException.ThrowIfNull(ascii);
        if (ascii.Length != 4) throw new ArgumentException("FourCC must be exactly 4 characters.", nameof(ascii));
        return ((uint)(byte)ascii[0] << 24) | ((uint)(byte)ascii[1] << 16) | ((uint)(byte)ascii[2] << 8) | (byte)ascii[3];
    }

    /// <summary>Equality by case-sensitive ASCII match.</summary>
    [MethodImpl(MethodImplOptions.AggressiveInlining)]
    public bool Equals(string ascii) =>
        ascii is { Length: 4 } &&
        (byte)ascii[0] == (byte)(Value >> 24) &&
        (byte)ascii[1] == (byte)(Value >> 16) &&
        (byte)ascii[2] == (byte)(Value >> 8) &&
        (byte)ascii[3] == (byte)Value;

    /// <inheritdoc/>
    public override string ToString()
    {
        Span<char> chars = stackalloc char[4];
        chars[0] = (char)(byte)(Value >> 24);
        chars[1] = (char)(byte)(Value >> 16);
        chars[2] = (char)(byte)(Value >> 8);
        chars[3] = (char)(byte)Value;
        // Replace non-printable with '.'
        for (int i = 0; i < 4; i++)
        {
            if (chars[i] < 32 || chars[i] > 126) chars[i] = '.';
        }
        return new string(chars);
    }

    /// <summary>Implicit conversion from <see cref="uint"/>.</summary>
    public static implicit operator FourCc(uint v) => new(v);

    /// <summary>Implicit conversion to <see cref="uint"/>.</summary>
    public static implicit operator uint(FourCc fc) => fc.Value;
}
