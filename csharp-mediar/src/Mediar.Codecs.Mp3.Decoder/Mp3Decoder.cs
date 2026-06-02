using System.Buffers;
using Mediar.Containers.Mp3;

namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// MPEG-1/2/2.5 Audio Layer III (MP3) decoder. Each <see cref="Decode"/> call
/// expects one complete MPEG audio frame (sync header through end of main
/// data) — the boundary that <c>Mediar.Containers.Mp3.Mp3Demuxer</c> already
/// produces.
/// </summary>
/// <remarks>
/// <para>
/// Pipeline (per ISO 11172-3 §2.4.3 and ISO 13818-3 §2.4.3):
/// header parse → side info → bit reservoir → scalefactor decode → Huffman
/// decode → requantize → MS/IS stereo → reorder → antialias → IMDCT +
/// overlap-add → polyphase synthesis → interleaved PCM float32.
/// </para>
/// <para>
/// Conformance: pipeline is structurally complete. Large lookup tables — the
/// 30 Huffman big_values tables (Annex B Table B.7) and the 512-point D-window
/// (Annex B Table B.4) — are sourced analytically or from a small embedded
/// subset, NOT from the ISO reference data. As a result silence frames decode
/// to silence and low-entropy frames decode to coherent audio, but bit-exact
/// reference-decoder agreement is not yet guaranteed. Replacing the table
/// fallbacks in <see cref="Mp3HuffmanTables"/> and the analytic D-window in
/// <see cref="Mp3Polyphase"/> with the ISO reference values is the path to
/// full conformance.
/// </para>
/// <para>
/// MP3 patents (last: US 5,742,735) expired in April 2017; the format and
/// these tables are no longer encumbered.
/// </para>
/// </remarks>
public sealed class Mp3Decoder : IAudioDecoder
{
    private readonly MainDataReader _reservoir = new();
    private readonly Mp3Hybrid[] _hybrid;
    private readonly Mp3Polyphase[] _polyphase;
    private readonly Mp3Scalefactors[,] _scalefactors;
    private readonly int[] _is576 = new int[576];
    private readonly float[][] _xrPerChannel;
    private readonly float[,] _hybridOut = new float[32, 18];
    private readonly float[] _polyphaseRow = new float[32];
    private readonly float[] _polyphasePcm = new float[32];

    // Reusable side-info instance sized for the worst case (MPEG-1, stereo).
    // Avoids per-frame allocation of the side-info wrapper, its Granules array,
    // and the per-granule TableSelect / SubblockGain arrays.
    private readonly Mp3SideInfo _sideInfo = new(granuleCount: 2, channelCount: 2);

    /// <inheritdoc/>
    public CodecId Codec => CodecId.Mp3;

    /// <inheritdoc/>
    public AudioCodecParameters Parameters { get; }

    /// <summary>Create an MP3 decoder. <paramref name="parameters"/> must have <see cref="CodecId.Mp3"/>.</summary>
    public Mp3Decoder(AudioCodecParameters parameters)
    {
        ArgumentNullException.ThrowIfNull(parameters);
        if (parameters.Codec != CodecId.Mp3)
            throw new ArgumentException("Mp3Decoder requires Codec=Mp3.", nameof(parameters));
        Parameters = parameters;

        _hybrid = new[] { new Mp3Hybrid(), new Mp3Hybrid() };
        _polyphase = new[] { new Mp3Polyphase(), new Mp3Polyphase() };
        _scalefactors = new Mp3Scalefactors[2, 2];
        for (int g = 0; g < 2; g++)
            for (int c = 0; c < 2; c++)
                _scalefactors[g, c] = new Mp3Scalefactors();
        _xrPerChannel = new[] { new float[576], new float[576] };
    }

    /// <inheritdoc/>
    public DecodedAudioFrame Decode(ReadOnlySpan<byte> encoded, long pts)
    {
        if (encoded.IsEmpty) return default;
        if (encoded.Length < 4)
            throw new InvalidDataException("MP3 frame too short to contain a header.");

        if (!Mp3FrameHeader.TryParse(encoded, out var header))
            throw new InvalidDataException("Not a valid MPEG audio frame header.");
        if (header.Layer != 3)
            throw new NotSupportedException($"MP3 decoder requires Layer III, got Layer {header.Layer}.");
        if (encoded.Length < header.FrameSize)
            throw new InvalidDataException(
                $"MP3 frame truncated: header reports {header.FrameSize} bytes, got {encoded.Length}.");

        bool hasCrc = (encoded[1] & 0x01) == 0;
        int channelMode = (encoded[3] >> 6) & 0x03;
        int modeExtension = (encoded[3] >> 4) & 0x03;
        int sampleRateIdx = (encoded[2] >> 2) & 0x03;

        bool jointStereo = channelMode == 1;
        if (!jointStereo) modeExtension = 0;

        int channels = header.Channels;
        int mpegVersion = header.Version == 1 ? 1 : 2; // MPEG-2 LSF and 2.5 share side-info layout
        bool mpeg2Lsf = mpegVersion != 1;

        int sfbRow = header.Version switch
        {
            1 => sampleRateIdx,
            2 => 3 + sampleRateIdx,
            25 => 6 + sampleRateIdx,
            _ => 0,
        };
        var longBands = Mp3SfbTables.LongBands[sfbRow];
        var shortBands = Mp3SfbTables.ShortBands[sfbRow];

        int headerSize = 4;
        int crcSize = hasCrc ? 2 : 0;
        int sideInfoSize = Mp3SideInfo.SideInfoBytes(mpegVersion, channels);
        int mainDataSize = header.FrameSize - headerSize - crcSize - sideInfoSize;
        if (mainDataSize < 0)
            throw new InvalidDataException("MP3 frame size too small for side info.");

        var sideInfoSpan = encoded.Slice(headerSize + crcSize, sideInfoSize);
        var mainDataSpan = encoded.Slice(headerSize + crcSize + sideInfoSize, mainDataSize);

        Mp3SideInfo.ParseInto(sideInfoSpan, mpegVersion, channels, _sideInfo);
        var si = _sideInfo;

        _reservoir.Append(mainDataSpan);
        _reservoir.SetCursorBytesFromEnd(mainDataSize + si.MainDataBegin);

        int granuleCount = mpegVersion == 1 ? 2 : 1;
        int samplesPerChannel = granuleCount * 576;
        int total = channels * samplesPerChannel;

        var owner = MemoryPool<float>.Shared.Rent(total);
        var floats = owner.Memory.Span[..total];
        floats.Clear();

        if (!_reservoir.HasEnoughHistory)
        {
            // First frames after reset / seek: insufficient back-pointer to
            // decode this frame's main data. Emit silence and let the
            // reservoir fill up.
            return new DecodedAudioFrame
            {
                Channels = channels,
                SampleRate = header.SampleRate,
                SamplesPerChannel = samplesPerChannel,
                Pts = pts,
                Samples = owner.Memory[..total],
                Owner = owner,
            };
        }

        for (int g = 0; g < granuleCount; g++)
        {
            // Decode each channel: scalefactors + Huffman + requantize.
            for (int c = 0; c < channels; c++)
            {
                var gr = si.Granules[g, c];
                int part2Bits;
                if (mpegVersion == 1)
                {
                    part2Bits = Mp3ScalefactorDecoder.DecodeMpeg1(
                        _reservoir, si, g, c, _scalefactors[g, c],
                        g == 1 ? _scalefactors[0, c] : null);
                }
                else
                {
                    bool isIntensityRight = jointStereo && (modeExtension & 0x1) != 0 && c == 1;
                    part2Bits = Mp3ScalefactorDecoder.DecodeMpeg2(
                        _reservoir, gr, isIntensityRight, _scalefactors[g, c], out bool pre);
                    gr.PreFlag = pre;
                }

                Mp3Huffman.Decode(_reservoir, si, g, c, part2Bits, _is576, longBands, out _);
                Mp3Requantize.Apply(_is576, _xrPerChannel[c], gr, _scalefactors[g, c], longBands, shortBands);
            }

            // Stereo (MS / Intensity) over both channels' xr.
            if (channels == 2 && modeExtension != 0)
            {
                Mp3Stereo.Apply(
                    _xrPerChannel[0], _xrPerChannel[1],
                    si.Granules[g, 1], _scalefactors[g, 1],
                    modeExtension, mpeg2Lsf,
                    longBands, shortBands);
            }

            // Hybrid filterbank + polyphase per channel.
            for (int c = 0; c < channels; c++)
            {
                var gr = si.Granules[g, c];
                _hybrid[c].Process(_xrPerChannel[c], gr, _hybridOut, shortBands);

                int granuleOff = (g * 576) * channels + c;
                for (int row = 0; row < 18; row++)
                {
                    for (int sb = 0; sb < 32; sb++) _polyphaseRow[sb] = _hybridOut[sb, row];
                    _polyphase[c].SynthesizeRow(_polyphaseRow, _polyphasePcm);
                    for (int s = 0; s < 32; s++)
                    {
                        int outIdx = granuleOff + (row * 32 + s) * channels;
                        floats[outIdx] = _polyphasePcm[s];
                    }
                }
            }
        }

        return new DecodedAudioFrame
        {
            Channels = channels,
            SampleRate = header.SampleRate,
            SamplesPerChannel = samplesPerChannel,
            Pts = pts,
            Samples = owner.Memory[..total],
            Owner = owner,
        };
    }

    /// <inheritdoc/>
    public void Reset()
    {
        _reservoir.Clear();
        foreach (var h in _hybrid) h.Reset();
        foreach (var p in _polyphase) p.Reset();
        for (int g = 0; g < 2; g++)
            for (int c = 0; c < 2; c++)
                _scalefactors[g, c].Clear();
    }

    /// <inheritdoc/>
    public void Dispose() { /* no unmanaged resources */ }
}

/// <summary>Factory that creates <see cref="Mp3Decoder"/> instances.</summary>
public sealed class Mp3DecoderFactory : IAudioDecoderFactory
{
    /// <inheritdoc/>
    public bool Supports(CodecId codec) => codec == CodecId.Mp3;

    /// <inheritdoc/>
    public IAudioDecoder Create(AudioCodecParameters parameters) => new Mp3Decoder(parameters);
}
