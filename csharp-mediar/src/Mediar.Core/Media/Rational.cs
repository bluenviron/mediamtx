namespace Mediar;

/// <summary>
/// A non-zero rational number used for time bases and frame rates.
/// Stored without simplifying; comparisons treat equivalent fractions as equal.
/// </summary>
public readonly record struct Rational(int Numerator, int Denominator)
{
    /// <summary>The rational 1/1.</summary>
    public static Rational One => new(1, 1);

    /// <summary>The rational 0/1.</summary>
    public static Rational Zero => new(0, 1);

    /// <summary>Returns true when the denominator is non-zero.</summary>
    public bool IsValid => Denominator != 0;

    /// <summary>Returns the rational evaluated as a double.</summary>
    public double ToDouble() => (double)Numerator / Denominator;

    /// <summary>
    /// Converts a number of units in this rational time-base to ticks of another base.
    /// </summary>
    public long Rescale(long value, Rational target)
    {
        if (Denominator == 0 || target.Denominator == 0)
        {
            throw new InvalidOperationException("Zero denominator.");
        }

        // value * (Numerator / Denominator) * (target.Denominator / target.Numerator)
        // Use 128-bit-ish multiplication via Int128 to avoid overflow.
        Int128 num = (Int128)value * Numerator * target.Denominator;
        Int128 den = (Int128)Denominator * target.Numerator;
        return (long)(num / den);
    }

    /// <inheritdoc/>
    public override string ToString() => $"{Numerator}/{Denominator}";
}
