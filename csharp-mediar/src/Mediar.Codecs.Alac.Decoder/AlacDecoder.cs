using System.Buffers;
using Mediar.IO;

namespace Mediar.Codecs.Alac.Decoder;

/// <summary>
/// Apple Lossless (ALAC) decoder. Consumes one ALAC packet per call (an
/// element sequence terminated by an <c>END</c> element, as produced by
/// MP4, M4A, CAF, and Matroska demuxers) and emits an interleaved float
/// PCM frame in <c>[-1, 1)</c>.
/// </summary>
/// <remarks>
/// <para>
/// Clean-room implementation of the format documented in Apple's ALAC
/// reference (Apache 2.0). The pipeline per channel is:
/// element header → predictor info → wasted-bits skip → adaptive
/// rice / Golomb residuals → adaptive FIR predictor → re-insertion of
/// wasted bits → stereo un-mix (CPE only) → float conversion.
/// </para>
/// <para>
/// Coverage in this release:
/// </para>
/// <list type="bullet">
/// <item>SCE (mono) and CPE (stereo) elements; LFE shares the SCE code
/// path. Multi-element channel layouts (5.1 / 7.1) are rejected with a
/// clear <see cref="NotSupportedException"/> — see the README TODO.</item>
/// <item>Bit depths 16, 20, 24 and 32, including the wasted-bytes
/// (<c>bytesShifted</c>) LSB-stuffing path used by 20/24/32-bit streams
/// to keep the predictor working at 16-bit precision.</item>
/// <item>The <c>partialFrame</c> per-packet sample-count override used by
/// the final (short) packet of a stream.</item>
/// <item>Uncompressed (escape) frames and compressed frames with
/// predictor <c>mode == 0</c> or <c>mode != 0</c> (the latter applies an
/// extra identity pass per Apple reference).</item>
/// </list>
/// </remarks>
public sealed class AlacDecoder : IAudioDecoder
{
    private readonly AlacSpecificConfig _config;

    private int[] _residuals = Array.Empty<int>();
    private int[] _mixU = Array.Empty<int>();
    private int[] _mixV = Array.Empty<int>();
    private ushort[] _shiftU = Array.Empty<ushort>();
    private ushort[] _shiftV = Array.Empty<ushort>();
    private readonly short[] _coefsU = new short[32];
    private readonly short[] _coefsV = new short[32];

    /// <inheritdoc/>
    public CodecId Codec => CodecId.Alac;

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>
    /// Create an ALAC decoder. <paramref name="parameters"/>.ExtraData must
    /// carry the ALAC Specific Config (the 24-byte magic cookie body, or a
    /// container-wrapped variant accepted by <see cref="AlacExtraData.NormalizeCookie"/>).
    /// </summary>
    public AlacDecoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec != CodecId.Alac)
            throw new ArgumentException("AlacDecoder requires Codec=Alac.", nameof(parameters));
        Parameters = parameters;

        var cookieBody = AlacExtraData.NormalizeCookie(parameters.ExtraData.Span);
        if (cookieBody.IsEmpty)
        {
            throw new ArgumentException(
                "AlacDecoder requires AudioCodecParameters.ExtraData to contain the ALAC Specific Config magic cookie.",
                nameof(parameters));
        }

        _config = AlacSpecificConfig.Parse(cookieBody);

        EnsureScratch(_config.FrameLength);
    }

    /// <summary>The parsed ALAC Specific Config (magic cookie) in use.</summary>
    public AlacSpecificConfig Config => _config;

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;
        if (encoded.Length < 1)
            throw new InvalidDataException("ALAC packet too short.");

        var br = new BitReader(encoded);
        int channelIndex = 0;
        int channels = _config.NumChannels;
        int defaultSamples = _config.FrameLength;
        int actualSamples = defaultSamples;
        int sampleRate = _config.SampleRate;
        int bitDepth = _config.BitDepth;

        // First pass: collect per-channel int32 PCM into mixU/mixV.
        // We support at most stereo (one CPE) or mono (one SCE/LFE).
        // We grow scratch buffers if the first element bumps the frame size.
        int producedChannels = 0;

        while (br.CanRead(3))
        {
            uint tag = br.ReadBits(3);
            if (tag == 7) // END
            {
                br.AlignToByte();
                break;
            }

            switch (tag)
            {
                case 0: // SCE
                case 3: // LFE
                {
                    if (producedChannels + 1 > channels)
                        throw new InvalidDataException("ALAC element exceeds channel count from cookie.");

                    DecodeElement(ref br, isPair: false, defaultSamples, bitDepth,
                        out int sceSamples);
                    actualSamples = sceSamples;
                    EnsureScratch(actualSamples);
                    // mono channel data is in _mixU; copy into the output
                    // assembly slot (we delay interleaving until both
                    // channels are decoded for CPE; for SCE we just keep it
                    // in _mixU and remember the channel index).
                    if (producedChannels == 0)
                    {
                        // already in _mixU
                    }
                    else
                    {
                        // For >1 SCE in a stream, copy into _mixV
                        Array.Copy(_mixU, _mixV, actualSamples);
                    }
                    producedChannels += 1;
                    channelIndex += 1;
                    break;
                }

                case 1: // CPE
                {
                    if (producedChannels + 2 > channels)
                        throw new InvalidDataException("ALAC CPE exceeds channel count from cookie.");

                    DecodeElement(ref br, isPair: true, defaultSamples, bitDepth,
                        out int cpeSamples);
                    actualSamples = cpeSamples;
                    EnsureScratch(actualSamples);
                    producedChannels += 2;
                    channelIndex += 2;
                    break;
                }

                case 2: // CCE — not used by ALAC channel layouts; reject explicitly.
                case 4: // DSE — data stream element; no audio.
                case 5: // PCE — program config; not used by ALAC.
                case 6: // FIL — fill; not used by ALAC.
                    throw new NotSupportedException(
                        $"ALAC element tag {tag} is not used by standard ALAC channel layouts.");

                default:
                    throw new InvalidDataException($"Unrecognised ALAC element tag {tag}.");
            }
        }

        if (producedChannels == 0)
            throw new InvalidDataException("ALAC packet contained no audio elements.");
        if (producedChannels != channels)
            throw new InvalidDataException(
                $"ALAC packet produced {producedChannels} channels but cookie reports {channels}.");

        // Pack into interleaved float PCM in [-1, 1).
        int total = actualSamples * channels;
        var owner = MemoryPool<float>.Shared.Rent(total);
        var floats = owner.Memory.Span[..total];
        float scale = 1f / (1 << (bitDepth - 1));

        if (channels == 1)
        {
            for (int i = 0; i < actualSamples; i++) floats[i] = _mixU[i] * scale;
        }
        else
        {
            for (int i = 0; i < actualSamples; i++)
            {
                floats[i * 2 + 0] = _mixU[i] * scale;
                floats[i * 2 + 1] = _mixV[i] * scale;
            }
        }

        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = sampleRate,
            SamplesPerChannel = actualSamples,
            Pts = pts,
            Samples = owner.Memory[..total],
            Owner = owner,
        };
    }

    private void DecodeElement(ref BitReader br, bool isPair, int defaultSamples,
        int bitDepth, out int numSamples)
    {
        // 4 bits instance tag (ignored), 12 bits unused (must be 0).
        br.SkipBits(4);
        uint unused = br.ReadBits(12);
        if (unused != 0)
            throw new InvalidDataException("ALAC element has non-zero reserved bits.");

        // 4-bit header byte: partialFrame(1) | bytesShifted(2) | escapeFlag(1)
        uint headerByte = br.ReadBits(4);
        bool partialFrame = ((headerByte >> 3) & 0x1) != 0;
        int bytesShifted = (int)((headerByte >> 1) & 0x3);
        bool escapeFlag = (headerByte & 0x1) != 0;

        if (bytesShifted == 3)
            throw new InvalidDataException("ALAC bytesShifted = 3 is reserved and not legal.");

        int shift = bytesShifted * 8;
        int chanBits = bitDepth - shift + (isPair ? 1 : 0);

        numSamples = defaultSamples;
        if (partialFrame)
        {
            uint hi = br.ReadBits(16);
            uint lo = br.ReadBits(16);
            numSamples = (int)((hi << 16) | lo);
            if (numSamples <= 0 || numSamples > 0x40000)
                throw new InvalidDataException($"ALAC partial-frame numSamples implausible: {numSamples}.");
        }

        EnsureScratch(numSamples);

        if (escapeFlag)
        {
            // Apple's reference resets chanBits to the raw bit depth in the
            // escape path (ignoring bytesShifted and the CPE +1 mid/side bit
            // since neither prediction nor unmix is applied).
            int rawBits = bitDepth;
            int rawShift = 32 - rawBits;
            for (int i = 0; i < numSamples; i++)
            {
                int u = SignedRead(ref br, rawBits, rawShift);
                _mixU[i] = u;
                if (isPair)
                {
                    int v = SignedRead(ref br, rawBits, rawShift);
                    _mixV[i] = v;
                }
            }
            return;
        }

        // Compressed element.
        int mixBits = (int)br.ReadBits(8);
        int mixRes = (sbyte)br.ReadBits(8);

        ReadPredictorInfo(ref br, out int modeU, out int denShiftU,
            out int pbFactorU, out int numU, _coefsU);

        int modeV = 0, denShiftV = 0, pbFactorV = 4, numV = 0;
        if (isPair)
        {
            ReadPredictorInfo(ref br, out modeV, out denShiftV,
                out pbFactorV, out numV, _coefsV);
        }

        // If bytesShifted > 0, the per-sample shift bits live in a separate
        // region immediately after the predictor info, BEFORE the rice
        // residual blocks. We snapshot the bit position, skip past the
        // shift region, decode the predictor blocks, then come back to
        // read the shift bits.
        long shiftRegionStart = 0;
        if (bytesShifted != 0)
        {
            shiftRegionStart = br.BitPosition;
            int shiftRegionBits = shift * (isPair ? 2 : 1) * numSamples;
            br.SkipBits(shiftRegionBits);
        }

        // Channel U
        DecompressAndPredict(ref br, _mixU, numSamples,
            modeU, denShiftU, pbFactorU, numU, _coefsU, chanBits);

        // Channel V
        if (isPair)
        {
            DecompressAndPredict(ref br, _mixV, numSamples,
                modeV, denShiftV, pbFactorV, numV, _coefsV, chanBits);
        }

        // Re-read shift bits if present.
        if (bytesShifted != 0)
        {
            long resumePos = br.BitPosition;
            br.SeekToBit(shiftRegionStart);
            for (int i = 0; i < numSamples; i++)
            {
                _shiftU[i] = (ushort)br.ReadBits(shift);
                if (isPair) _shiftV[i] = (ushort)br.ReadBits(shift);
            }
            br.SeekToBit(resumePos);
        }

        // Stereo unmix (in-place, sample buffers stay in _mixU/_mixV at chanBits precision).
        if (isPair)
        {
            AlacMatrix.Unmix(_mixU.AsSpan(0, numSamples), _mixV.AsSpan(0, numSamples),
                _mixU.AsSpan(0, numSamples), _mixV.AsSpan(0, numSamples),
                numSamples, mixBits, mixRes);
        }

        // Re-insert wasted bits as LSBs.
        if (bytesShifted != 0)
        {
            for (int i = 0; i < numSamples; i++)
            {
                _mixU[i] = (_mixU[i] << shift) | _shiftU[i];
            }
            if (isPair)
            {
                for (int i = 0; i < numSamples; i++)
                {
                    _mixV[i] = (_mixV[i] << shift) | _shiftV[i];
                }
            }
        }
    }

    private void ReadPredictorInfo(ref BitReader br,
        out int mode, out int denShift, out int pbFactor, out int numCoeffs,
        short[] coeffs)
    {
        uint hb1 = br.ReadBits(8);
        mode = (int)(hb1 >> 4) & 0xF;
        denShift = (int)hb1 & 0xF;

        uint hb2 = br.ReadBits(8);
        pbFactor = (int)(hb2 >> 5) & 0x7;
        numCoeffs = (int)hb2 & 0x1F;

        for (int i = 0; i < numCoeffs; i++)
        {
            coeffs[i] = (short)br.ReadBits(16);
        }
    }

    private void DecompressAndPredict(
        ref BitReader br, int[] outBuffer, int numSamples,
        int mode, int denShift, int pbFactor, int numCoeffs,
        short[] coeffs, int chanBits)
    {
        int chanMb = _config.Mb;
        int chanKb = _config.Kb;
        int chanPb = (_config.Pb * pbFactor) / 4;

        AlacRice.DecodeBlock(ref br, _residuals.AsSpan(0, numSamples),
            numSamples, chanMb, chanPb, chanKb, chanBits);

        if (mode == 0)
        {
            AlacPredictor.Unpc(_residuals, outBuffer, numSamples,
                coeffs, numCoeffs, chanBits, denShift);
        }
        else
        {
            // Mode != 0: identity pass over the residuals, then the real
            // predictor pass. The first pass writes back into the residual
            // buffer; the second consumes that into outBuffer.
            AlacPredictor.Unpc(_residuals, _residuals, numSamples,
                Span<short>.Empty, 31, chanBits, 0);
            AlacPredictor.Unpc(_residuals, outBuffer, numSamples,
                coeffs, numCoeffs, chanBits, denShift);
        }
    }

    private static int SignedRead(ref BitReader br, int chanBits, int chanShift)
    {
        // ReadBits handles up to 32 bits; sign-extend by shifting up then
        // arithmetically back down (matches Apple's `(val << shift) >> shift`).
        int val;
        if (chanBits <= 16)
        {
            val = (int)br.ReadBits(chanBits);
            val = (val << chanShift) >> chanShift;
        }
        else
        {
            int hi = (int)br.ReadBits(16);
            int lo = (int)br.ReadBits(chanBits - 16);
            val = (hi << (chanBits - 16)) | lo;
            val = (val << chanShift) >> chanShift;
        }
        return val;
    }

    private void EnsureScratch(int numSamples)
    {
        if (_residuals.Length < numSamples)
        {
            _residuals = new int[numSamples];
            _mixU = new int[numSamples];
            _mixV = new int[numSamples];
            _shiftU = new ushort[numSamples];
            _shiftV = new ushort[numSamples];
        }
    }

    /// <inheritdoc/>
    public void Reset() { /* ALAC packets are independent (each carries enough state) */ }

    /// <inheritdoc/>
    public void Dispose() { /* no unmanaged resources */ }
}

/// <summary>Factory that creates <see cref="AlacDecoder"/> instances.</summary>
public sealed class AlacDecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec == CodecId.Alac;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new AlacDecoder(parameters);
}
