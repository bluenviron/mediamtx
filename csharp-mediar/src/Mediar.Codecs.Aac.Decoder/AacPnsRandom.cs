namespace Mediar.Codecs.Aac.Decoder;

/// <summary>
/// Linear-congruential pseudo-random number generator used by the
/// AAC perceptual-noise-substitution (PNS) stage to fabricate
/// noise coefficients for cb = 13 scale-factor bands per
/// ISO/IEC 14496-3 §4.6.12. The recurrence is the standard
/// Numerical-Recipes / Knuth constants:
/// </summary>
/// <remarks>
/// <para>
/// <c>state' = 1664525 * state + 1013904223  (mod 2^32)</c>
/// </para>
/// <para>
/// These constants are widely cited mathematical facts (see
/// L'Ecuyer's tables, Knuth's <i>Art of Computer Programming</i>
/// Vol. 2) and are not copyrightable. The generator itself produces
/// a full-period sequence of 32-bit unsigned values; consumers
/// typically reinterpret them as signed 32-bit integers and divide
/// by <c>2^31</c> to obtain a float in <c>[-1, 1)</c>.
/// </para>
/// <para>
/// PNS uses one PRNG state per <see cref="AacChannelFrame"/>; the
/// state advances strictly forward as PNS bands are emitted. Two
/// frames that select PNS for the same band do so against
/// independent states, so the generator is reseedable (the spec
/// does not pin a particular initial seed; common decoders pick
/// <c>0</c>).
/// </para>
/// </remarks>
public sealed class AacPnsRandom
{
    /// <summary>The recurrence multiplier.</summary>
    public const uint Multiplier = 1664525u;

    /// <summary>The recurrence increment.</summary>
    public const uint Increment = 1013904223u;

    /// <summary>
    /// Scale factor that converts a signed 32-bit sample to a float
    /// in <c>[-1, 1)</c>.
    /// </summary>
    public const float NormalisationScale = 1f / 2147483648f;

    /// <summary>The default seed value used when no explicit seed is supplied.</summary>
    public const uint DefaultSeed = 0u;

    private uint _state;

    /// <summary>Construct a generator seeded with <see cref="DefaultSeed"/>.</summary>
    public AacPnsRandom() : this(DefaultSeed) { }

    /// <summary>Construct a generator seeded with <paramref name="seed"/>.</summary>
    public AacPnsRandom(uint seed)
    {
        _state = seed;
    }

    /// <summary>
    /// Current internal state. Reading does not advance the
    /// generator; writing is permitted via <see cref="Reseed"/>.
    /// </summary>
    public uint State => _state;

    /// <summary>Reseed the generator with a new initial state.</summary>
    public void Reseed(uint seed) => _state = seed;

    /// <summary>
    /// Advance and return the next 32-bit unsigned PRNG value.
    /// </summary>
    public uint Next()
    {
        unchecked
        {
            _state = Multiplier * _state + Increment;
        }
        return _state;
    }

    /// <summary>
    /// Advance the generator and return the next value reinterpreted
    /// as a signed 32-bit integer.
    /// </summary>
    public int NextSigned() => unchecked((int)Next());

    /// <summary>
    /// Advance the generator and return the next value mapped to a
    /// float in the half-open interval <c>[-1, 1)</c>.
    /// </summary>
    public float NextFloat() => NextSigned() * NormalisationScale;

    /// <summary>
    /// Fill <paramref name="destination"/> with normalised noise
    /// samples in <c>[-1, 1)</c>, advancing the generator once per
    /// element.
    /// </summary>
    public void Fill(Span<float> destination)
    {
        for (int i = 0; i < destination.Length; i++)
        {
            destination[i] = NextFloat();
        }
    }
}
