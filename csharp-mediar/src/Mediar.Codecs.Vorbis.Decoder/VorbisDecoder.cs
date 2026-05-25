using System.Buffers;

namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Vorbis I audio decoder. Patent-free per Xiph.org and shipped under
/// Mediar's MIT license.
///
/// Decodes the three header packets (identification, comment, setup) and
/// subsequent audio packets:
/// <list type="number">
///   <item>parse mode + window flags</item>
///   <item>floor 1 decode per channel</item>
///   <item>residue decode per submap (with channel coupling)</item>
///   <item>inverse channel coupling</item>
///   <item>floor curve × residue → MDCT bins</item>
///   <item>IMDCT → time-domain block</item>
///   <item>sin² window</item>
///   <item>per-channel overlap-add → emit <c>(n_prev + n_curr)/4</c> samples</item>
/// </list>
///
/// Accepts the standard Matroska/WebM-style Xiph-laced
/// <see cref="CodecParameters.ExtraData"/> blob (count-1, then per-packet
/// lengths, then the three Vorbis priming packets concatenated). When
/// <c>ExtraData</c> is empty the decoder treats the first three
/// <see cref="Decode"/> calls as the priming sequence.
/// </summary>
public sealed class VorbisDecoder : IAudioDecoder
{
    private VorbisIdentificationHeader? _id;
    private VorbisCommentHeader? _comment;
    private VorbisSetup? _setup;
    private int _primed;
    private long _samplesProduced;

    private VorbisMdct? _mdctShort;
    private VorbisMdct? _mdctLong;
    private VorbisLap? _lap;

    /// <inheritdoc/>
    public CodecId Codec => CodecId.Vorbis;

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>Vendor string from the comment header, or empty until parsed.</summary>
    public string Vendor => _comment?.Vendor ?? string.Empty;

    /// <summary>User comments from the comment header.</summary>
    public IReadOnlyList<string> UserComments => _comment?.UserComments ?? [];

    /// <summary>Short-block length (samples), 0 until headers parsed.</summary>
    public int ShortBlocksize => _id?.Blocksize0 ?? 0;

    /// <summary>Long-block length (samples), 0 until headers parsed.</summary>
    public int LongBlocksize => _id?.Blocksize1 ?? 0;

    /// <summary>True once all three Vorbis priming packets have been processed.</summary>
    public bool IsPrimed => _primed >= 3;

    /// <summary>Create a Vorbis decoder.</summary>
    public VorbisDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec != CodecId.Vorbis)
            throw new ArgumentException("VorbisDecoder requires Codec=Vorbis.", nameof(parameters));
        Parameters = parameters;

        if (!parameters.ExtraData.IsEmpty)
        {
            var packets = VorbisHeaders.UnpackXiphLaced(parameters.ExtraData.Span);
            if (packets.Length != 3)
                throw new InvalidDataException("Vorbis ExtraData must contain exactly 3 xiph-laced packets.");
            foreach (var pkt in packets) ConsumeHeader(pkt);
        }
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;

        if (_primed < 3)
        {
            ConsumeHeader(encoded);
            return default;
        }

        return DecodeAudio(encoded, pts);
    }

    private DecodedAudioFrame DecodeAudio(ReadOnlySpan<byte> packet, long pts)
    {
        var id = _id!;
        var setup = _setup!;
        var lap = _lap!;
        int channels = id.Channels;
        int sampleRate = id.SampleRate;

        var r = new VorbisBitReader(packet);
        // §4.3.1 — packet type bit (must be 0 for audio).
        if (r.ReadBit())
            throw new InvalidDataException("Audio packet with non-zero type bit.");

        // §4.3.2 — mode number.
        int modeBits = VorbisBitReader.Ilog(setup.Modes.Length - 1);
        int modeNumber = (int)r.ReadBits(modeBits);
        if ((uint)modeNumber >= (uint)setup.Modes.Length)
            throw new InvalidDataException("Mode number out of range.");
        var mode = setup.Modes[modeNumber];
        bool isLong = mode.BlockFlag;
        int n = isLong ? id.Blocksize1 : id.Blocksize0;
        int n2 = n / 2;

        // §4.3.2 — window flags for long blocks; short blocks always use
        // the short-side window on both edges.
        int leftWinLen;
        int rightWinLen;
        if (isLong)
        {
            bool prevLong = r.ReadBit();
            bool nextLong = r.ReadBit();
            leftWinLen = (prevLong ? id.Blocksize1 : id.Blocksize0) / 2;
            rightWinLen = (nextLong ? id.Blocksize1 : id.Blocksize0) / 2;
        }
        else
        {
            leftWinLen = id.Blocksize0 / 2;
            rightWinLen = id.Blocksize0 / 2;
        }

        var mapping = setup.Mappings[mode.Mapping];

        // §4.3.3 — floor decode per channel.
        var floorYs = new int[channels][];
        Span<bool> noResidue = stackalloc bool[channels];
        for (int ch = 0; ch < channels; ch++)
        {
            int submap = mapping.ChannelMux[ch];
            var floor = setup.Floors[mapping.SubmapFloor[submap]];
            if (floor.Type == 1)
            {
                var y = VorbisFloor1.Decode(ref r, floor, setup.Codebooks);
                floorYs[ch] = y!;
                noResidue[ch] = y is null;
            }
            else
            {
                // Floor 0 unsupported — emit silence for this channel.
                floorYs[ch] = null!;
                noResidue[ch] = true;
            }
        }

        // §4.3.4 — apply the coupling no_residue rule.
        foreach (var step in mapping.CouplingSteps)
        {
            if (!noResidue[step.MagnitudeChannel] || !noResidue[step.AngleChannel])
            {
                noResidue[step.MagnitudeChannel] = false;
                noResidue[step.AngleChannel] = false;
            }
        }

        // Allocate per-channel residue/spectrum buffers.
        float[][] residues = new float[channels][];
        for (int ch = 0; ch < channels; ch++) residues[ch] = new float[n2];

        // §4.3.5 — residue decode, grouped by submap.
        int submapCount = mapping.SubmapResidue.Length;
        for (int submapIdx = 0; submapIdx < submapCount; submapIdx++)
        {
            int subChannels = 0;
            for (int ch = 0; ch < channels; ch++) if (mapping.ChannelMux[ch] == submapIdx) subChannels++;
            if (subChannels == 0) continue;

            var channelIdx = new int[subChannels];
            var subMask = new bool[subChannels];
            var subVectors = new float[subChannels][];
            int sci = 0;
            for (int ch = 0; ch < channels; ch++)
            {
                if (mapping.ChannelMux[ch] != submapIdx) continue;
                channelIdx[sci] = ch;
                subMask[sci] = noResidue[ch];
                subVectors[sci] = residues[ch];
                sci++;
            }

            var residue = setup.Residues[mapping.SubmapResidue[submapIdx]];
            VorbisResidue.Decode(ref r, residue, setup.Codebooks, n2, subMask, subVectors);
        }

        // §4.3.4 — inverse coupling (M/A → L/R).
        for (int s = mapping.CouplingSteps.Length - 1; s >= 0; s--)
        {
            var step = mapping.CouplingSteps[s];
            var m = residues[step.MagnitudeChannel];
            var a = residues[step.AngleChannel];
            for (int i = 0; i < n2; i++)
            {
                float mag = m[i];
                float ang = a[i];
                float newM;
                float newA;
                if (mag > 0)
                {
                    if (ang > 0) { newM = mag; newA = mag - ang; }
                    else { newA = mag; newM = mag + ang; }
                }
                else
                {
                    if (ang > 0) { newM = mag; newA = mag + ang; }
                    else { newA = mag; newM = mag - ang; }
                }
                m[i] = newM;
                a[i] = newA;
            }
        }

        // Allocate time-domain block scratch.
        var scratchBlock = ArrayPool<float>.Shared.Rent(n);
        var scratchSpectrum = ArrayPool<float>.Shared.Rent(n2);
        try
        {
            var mdct = isLong ? (_mdctLong ??= new VorbisMdct(n)) : (_mdctShort ??= new VorbisMdct(n));

            for (int ch = 0; ch < channels; ch++)
            {
                var spectrum = scratchSpectrum.AsSpan(0, n2);
                spectrum.Clear();

                int submap = mapping.ChannelMux[ch];
                var floor = setup.Floors[mapping.SubmapFloor[submap]];
                if (!noResidue[ch] && floorYs[ch] is not null && floor.Type == 1)
                {
                    Span<float> floorCurve = spectrum;
                    bool hasFloor = VorbisFloor1.Synthesize(floorYs[ch], floor, n2, floorCurve);
                    if (hasFloor)
                    {
                        var residue = residues[ch];
                        for (int i = 0; i < n2; i++) spectrum[i] = floorCurve[i] * residue[i];
                    }
                    else
                    {
                        spectrum.Clear();
                    }
                }
                else
                {
                    spectrum.Clear();
                }

                var block = scratchBlock.AsSpan(0, n);
                mdct.Inverse(spectrum, block);
                VorbisWindow.Apply(block, leftWinLen, rightWinLen);
                lap.Accumulate(ch, block, n);
            }

            // Commit and emit.
            int emit = lap.PeekEmit(n);
            int total = emit * channels;
            if (total <= 0)
            {
                lap.Commit([], n);
                return default;
            }
            var owner = MemoryPool<float>.Shared.Rent(total);
            var mem = owner.Memory[..total];
            var perChannel = new Memory<float>[channels];
            for (int ch = 0; ch < channels; ch++)
            {
                perChannel[ch] = mem.Slice(ch * emit, emit);
            }
            int produced = lap.Commit(perChannel, n);
            if (produced != emit)
                throw new InvalidOperationException("Lap commit produced unexpected sample count.");

            long framePts = _samplesProduced;
            _samplesProduced += produced;
            return new DecodedAudioFrame
            {
                Channels = channels,
                SampleRate = sampleRate,
                SamplesPerChannel = produced,
                Pts = pts >= 0 ? pts : framePts,
                Samples = mem,
                Owner = owner,
            };
        }
        finally
        {
            ArrayPool<float>.Shared.Return(scratchBlock);
            ArrayPool<float>.Shared.Return(scratchSpectrum);
        }
    }

    private void ConsumeHeader(ReadOnlySpan<byte> packet)
    {
        if (packet.Length < 1) throw new InvalidDataException("Empty Vorbis header packet.");
        byte type = packet[0];
        switch (type)
        {
            case 1:
                _id = VorbisHeaders.ParseIdentification(packet);
                break;
            case 3:
                _comment = VorbisHeaders.ParseComment(packet);
                break;
            case 5:
                if (_id is null)
                    throw new InvalidDataException("Setup header arrived before identification header.");
                _setup = VorbisSetup.Parse(packet, _id.Channels);
                _lap = new VorbisLap(_id.Channels, _id.Blocksize1);
                break;
            default:
                throw new InvalidDataException($"Unexpected Vorbis header type {type}.");
        }
        _primed++;
    }

    /// <inheritdoc/>
    public void Reset()
    {
        _samplesProduced = 0;
        _lap?.Reset();
    }

    /// <inheritdoc/>
    public void Dispose()
    {
    }
}

/// <summary>Factory that produces <see cref="VorbisDecoder"/> instances.</summary>
public sealed class VorbisDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec == CodecId.Vorbis;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new VorbisDecoder(parameters);
}

