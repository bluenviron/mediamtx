using System.Buffers.Binary;
using System.Text;

namespace Mediar.Containers.Iff8Svx;

/// <summary>
/// Writes Amiga 8SVX files containing a single 8-bit signed-PCM voice.
/// Multi-octave authoring is not supported (Mediar produces single-octave
/// "one-shot" voices, which is what 99% of modern callers want).
/// </summary>
public sealed class Iff8SvxMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private long _formSizePos;
    private long _bodySizePos;
    private long _bodyStart;
    private long _bytesWritten;
    private bool _started;
    private bool _finished;
    private string? _title, _artist, _comment, _copyright;

    /// <summary>Create an 8SVX muxer writing to <paramref name="output"/>.</summary>
    public Iff8SvxMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite || !output.CanSeek)
            throw new ArgumentException("Stream must be writable and seekable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "8svx";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_track is not null) throw new InvalidOperationException("8SVX supports a single track.");
        if (track.Codec is not AudioCodecParameters a)
            throw new ArgumentException("8SVX muxer requires an audio track.", nameof(track));
        if (a.Codec != CodecId.PcmS8 || a.Channels != 1 || a.BitsPerSample != 8)
            throw new ArgumentException("8SVX only supports 8-bit signed PCM mono.", nameof(track));
        if (a.SampleRate <= 0 || a.SampleRate > ushort.MaxValue)
            throw new ArgumentException("8SVX sample rate must fit in a UInt16.", nameof(track));
        _track = track;
    }

    /// <summary>Set the file's title (written as NAME chunk).</summary>
    public void SetTitle(string? value) => _title = value;
    /// <summary>Set the file's author (written as AUTH chunk).</summary>
    public void SetArtist(string? value) => _artist = value;
    /// <summary>Set an annotation (written as ANNO chunk).</summary>
    public void SetComment(string? value) => _comment = value;
    /// <summary>Set the copyright (written as <c>(c) </c> chunk).</summary>
    public void SetCopyright(string? value) => _copyright = value;

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("AddTrack must be called first.");
        if (_started) return;
        _started = true;

        // FORM chunk header (size patched later)
        byte[] form = [
            (byte)'F',(byte)'O',(byte)'R',(byte)'M', 0,0,0,0,
            (byte)'8',(byte)'S',(byte)'V',(byte)'X',
        ];
        _formSizePos = _output.Position + 4;
        await _output.WriteAsync(form, cancellationToken).ConfigureAwait(false);

        // VHDR chunk
        var a = (AudioCodecParameters)_track.Codec;
        byte[] vhdr = new byte[8 + 20];
        BinaryPrimitives.WriteUInt32BigEndian(vhdr.AsSpan(0, 4), ChunkIds.VHDR);
        BinaryPrimitives.WriteUInt32BigEndian(vhdr.AsSpan(4, 4), 20);
        // OneShotHiSamples / RepeatHiSamples / SamplesPerHiCycle (filled in FinishAsync)
        // SamplesPerSec / Octaves / Compression / Volume
        BinaryPrimitives.WriteUInt16BigEndian(vhdr.AsSpan(8 + 12, 2), (ushort)a.SampleRate);
        vhdr[8 + 14] = 1; // octaves
        vhdr[8 + 15] = 0; // compression = none
        BinaryPrimitives.WriteUInt32BigEndian(vhdr.AsSpan(8 + 16, 4), 0x10000); // unity volume (16.16)
        await _output.WriteAsync(vhdr, cancellationToken).ConfigureAwait(false);

        await WriteTextChunkAsync(ChunkIds.NAME, _title, cancellationToken).ConfigureAwait(false);
        await WriteTextChunkAsync(ChunkIds.AUTH, _artist, cancellationToken).ConfigureAwait(false);
        await WriteTextChunkAsync(ChunkIds.ANNO, _comment, cancellationToken).ConfigureAwait(false);
        await WriteTextChunkAsync(ChunkIds.COPY, _copyright, cancellationToken).ConfigureAwait(false);

        // BODY chunk header (size patched later)
        byte[] body = new byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(body.AsSpan(0, 4), ChunkIds.BODY);
        _bodySizePos = _output.Position + 4;
        await _output.WriteAsync(body, cancellationToken).ConfigureAwait(false);
        _bodyStart = _output.Position;
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        if (!_started) await StartAsync(cancellationToken).ConfigureAwait(false);
        await _output.WriteAsync(sample.Data, cancellationToken).ConfigureAwait(false);
        _bytesWritten += sample.Data.Length;
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (!_started || _finished) return;
        _finished = true;
        long endPos = _output.Position;
        // Pad BODY to even length
        if ((_bytesWritten & 1) != 0)
        {
            _output.WriteByte(0);
            endPos = _output.Position;
        }

        byte[] sz = new byte[4];
        // BODY size
        _output.Seek(_bodySizePos, SeekOrigin.Begin);
        BinaryPrimitives.WriteUInt32BigEndian(sz, (uint)_bytesWritten);
        await _output.WriteAsync(sz, cancellationToken).ConfigureAwait(false);
        // VHDR OneShotHiSamples: at file position
        //   formSizePos (4) + size field (4) + "8SVX" (4) + "VHDR" (4) + VHDR size field (4) = 20.
        long vhdrOneShotPos = _formSizePos + 4 /* size field */ + 4 /* "8SVX" */ + 4 /* "VHDR" */ + 4 /* VHDR size */;
        _output.Seek(vhdrOneShotPos, SeekOrigin.Begin);
        BinaryPrimitives.WriteUInt32BigEndian(sz, (uint)_bytesWritten);
        await _output.WriteAsync(sz, cancellationToken).ConfigureAwait(false);
        // FORM size = (endPos - 8)
        _output.Seek(_formSizePos, SeekOrigin.Begin);
        BinaryPrimitives.WriteUInt32BigEndian(sz, (uint)(endPos - _formSizePos - 4));
        await _output.WriteAsync(sz, cancellationToken).ConfigureAwait(false);
        _output.Seek(endPos, SeekOrigin.Begin);
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        await FinishAsync().ConfigureAwait(false);
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }
    /// <inheritdoc/>
    public void Dispose()
    {
        FinishAsync().AsTask().GetAwaiter().GetResult();
        if (!_leaveOpen) _output.Dispose();
    }

    private async ValueTask WriteTextChunkAsync(uint id, string? value, CancellationToken ct)
    {
        if (string.IsNullOrEmpty(value)) return;
        byte[] payload = Encoding.UTF8.GetBytes(value);
        byte[] hdr = new byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr.AsSpan(0, 4), id);
        BinaryPrimitives.WriteUInt32BigEndian(hdr.AsSpan(4, 4), (uint)payload.Length);
        await _output.WriteAsync(hdr, ct).ConfigureAwait(false);
        await _output.WriteAsync(payload, ct).ConfigureAwait(false);
        if ((payload.Length & 1) != 0) _output.WriteByte(0);
    }

    private static class ChunkIds
    {
        public const uint VHDR = 0x56484452;
        public const uint BODY = 0x424F4459;
        public const uint NAME = 0x4E414D45;
        public const uint AUTH = 0x41555448;
        public const uint ANNO = 0x414E4E4F;
        public const uint COPY = 0x28632920;
    }
}
