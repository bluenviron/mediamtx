namespace Mediar;

/// <summary>
/// A timestamp expressed in arbitrary-base ticks (numerator) together with the time-base
/// of the track that produced it (<see cref="TimeBase"/>). One tick equals
/// <c>TimeBase.Numerator / TimeBase.Denominator</c> seconds.
/// </summary>
public readonly record struct MediaTimestamp(long Ticks, Rational TimeBase)
{
    /// <summary>An unset / unknown timestamp marker.</summary>
    public static readonly MediaTimestamp Unset = new(long.MinValue, Rational.One);

    /// <summary>True if this timestamp is not the <see cref="Unset"/> sentinel.</summary>
    public bool HasValue => Ticks != long.MinValue;

    /// <summary>Convert to a <see cref="TimeSpan"/>.</summary>
    public TimeSpan ToTimeSpan()
    {
        if (!HasValue) return TimeSpan.Zero;
        // ticks_in_TimeSpan_units = Ticks * TimeBase.Num / TimeBase.Den * TimeSpan.TicksPerSecond
        Int128 num = (Int128)Ticks * TimeBase.Numerator * TimeSpan.TicksPerSecond;
        Int128 den = TimeBase.Denominator;
        return new TimeSpan((long)(num / den));
    }

    /// <summary>Rescale to a new time-base.</summary>
    public MediaTimestamp Rescale(Rational target)
    {
        if (!HasValue) return Unset;
        return new MediaTimestamp(TimeBase.Rescale(Ticks, target), target);
    }

    /// <inheritdoc/>
    public override string ToString() =>
        HasValue ? $"{Ticks} @ {TimeBase}" : "unset";
}
