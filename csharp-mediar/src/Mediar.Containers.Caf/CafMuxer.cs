using System.Buffers.Binary;
using System.Text;

namespace Mediar.Containers.Caf;

/// <summary>
/// Writes a Core Audio Format (<c>.caf</c>) file from PCM samples written by
/// callers. Supports any of the linear-PCM variants that
/// <see cref="CafDemuxer"/> reads back; the codec is inferred from the
/// track's <see cref="AudioCodecParameters"/>.
/// </summary>
public sealed class CafMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private long _descPos = -1;
    private long _dataStartPos = -1;
    private long _bytesWritten;
    private bool _started;
    private bool _finished;
    private readonly Dictionary<string, string> _info = new(StringComparer.OrdinalIgnoreCase);

    /// <summary>Create a CAF muxer that writes to <paramref name="output"/>.</summary>
    public CafMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_track is not null) throw new InvalidOperationException("CAF only supports a single track.");
        if (track.Codec is not AudioCodecParameters)
            throw new ArgumentException("CAF muxer only supports audio tracks.", nameof(track));
        _track = track;
    }

    /// <inheritdoc/>
    public string FormatName => "caf";

    /// <summary>Add an <c>info</c> key/value pair (title, artist, …).</summary>
    public void AddInfo(string key, string value)
    {
        ArgumentException.ThrowIfNullOrEmpty(key);
        _info[key] = value ?? string.Empty;
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("AddTrack must be called first.");
        if (_started) return;
        _started = true;

        // File header: 'caff' + version (1) + flags (0)
        byte[] hdr = [
            (byte)'c', (byte)'a', (byte)'f', (byte)'f',
            0, 1,
            0, 0,
        ];
        await _output.WriteAsync(hdr, cancellationToken).ConfigureAwait(false);

        // 'desc' chunk
        _descPos = _output.Position;
        var audio = (AudioCodecParameters)_track.Codec;
        byte[] desc = new byte[12 + 32];
        WriteFourCcBE(desc, 0, "desc");
        BinaryPrimitives.WriteInt64BigEndian(desc.AsSpan(4, 8), 32);
        BinaryPrimitives.WriteInt64BigEndian(desc.AsSpan(12, 8),
            BitConverter.DoubleToInt64Bits(audio.SampleRate));
        WriteFourCcBE(desc, 20, CodecToFormatId(audio.Codec));
        BinaryPrimitives.WriteUInt32BigEndian(desc.AsSpan(24, 4), FlagsFor(audio.Codec));
        int bytesPerFrame = ((audio.BitsPerSample + 7) / 8) * audio.Channels;
        BinaryPrimitives.WriteUInt32BigEndian(desc.AsSpan(28, 4), (uint)bytesPerFrame); // bytes/packet
        BinaryPrimitives.WriteUInt32BigEndian(desc.AsSpan(32, 4), 1u);                  // frames/packet
        BinaryPrimitives.WriteUInt32BigEndian(desc.AsSpan(36, 4), (uint)audio.Channels);
        BinaryPrimitives.WriteUInt32BigEndian(desc.AsSpan(40, 4), (uint)audio.BitsPerSample);
        await _output.WriteAsync(desc, cancellationToken).ConfigureAwait(false);

        // info chunk (optional - written before data so streams have it early)
        if (_info.Count > 0)
        {
            using var ms = new MemoryStream();
            byte[] entries = new byte[4]; // count
            BinaryPrimitives.WriteUInt32BigEndian(entries, (uint)_info.Count);
            ms.Write(entries);
            foreach (var (k, v) in _info)
            {
                byte[] kb = Encoding.UTF8.GetBytes(k);
                byte[] vb = Encoding.UTF8.GetBytes(v);
                ms.Write(kb); ms.WriteByte(0);
                ms.Write(vb); ms.WriteByte(0);
            }
            byte[] payload = ms.ToArray();
            byte[] infoHdr = new byte[12];
            WriteFourCcBE(infoHdr, 0, "info");
            BinaryPrimitives.WriteInt64BigEndian(infoHdr.AsSpan(4, 8), payload.Length);
            await _output.WriteAsync(infoHdr, cancellationToken).ConfigureAwait(false);
            await _output.WriteAsync(payload, cancellationToken).ConfigureAwait(false);
        }

        // 'data' chunk header (size patched in FinishAsync); 4-byte edit-count follows
        byte[] dataHdr = new byte[12 + 4];
        WriteFourCcBE(dataHdr, 0, "data");
        BinaryPrimitives.WriteInt64BigEndian(dataHdr.AsSpan(4, 8), -1); // patched
        await _output.WriteAsync(dataHdr, cancellationToken).ConfigureAwait(false);
        _dataStartPos = _output.Position;
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
        long end = _output.Position;
        // Patch data chunk size = (edit-count uint32 + audio bytes)
        _output.Seek(_dataStartPos - 8, SeekOrigin.Begin);
        byte[] sizeBuf = new byte[8];
        BinaryPrimitives.WriteInt64BigEndian(sizeBuf, _bytesWritten + 4);
        await _output.WriteAsync(sizeBuf, cancellationToken).ConfigureAwait(false);
        _output.Seek(end, SeekOrigin.Begin);
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

    private static void WriteFourCcBE(Span<byte> dst, int offset, string fourcc)
    {
        for (int i = 0; i < 4; i++) dst[offset + i] = (byte)fourcc[i];
    }

    private static string CodecToFormatId(CodecId codec) => codec switch
    {
        CodecId.PcmS8 or CodecId.PcmU8 or
        CodecId.PcmS16Le or CodecId.PcmS16Be or
        CodecId.PcmS24Le or CodecId.PcmS32Le or CodecId.PcmF32Le => "lpcm",
        CodecId.Alac => "alac",
        CodecId.Aac  => "aac ",
        CodecId.G711MuLaw => "ulaw",
        CodecId.G711ALaw  => "alaw",
        CodecId.Opus => "opus",
        _ => throw new NotSupportedException($"CAF muxer cannot write codec '{codec}'."),
    };

    private static uint FlagsFor(CodecId codec) => codec switch
    {
        // kCAFLinearPCMFormatFlagIsFloat = 1 << 0
        // kCAFLinearPCMFormatFlagIsLittleEndian = 1 << 1
        // (kCAFLinearPCMFormatFlagIsSigned is bit 2 — we set it for signed PCM where bits != 16)
        CodecId.PcmS8     => 0x4,
        CodecId.PcmU8     => 0x0,
        CodecId.PcmS16Le  => 0x2,
        CodecId.PcmS16Be  => 0x0,
        CodecId.PcmS24Le  => 0x2,
        CodecId.PcmS32Le  => 0x2,
        CodecId.PcmF32Le  => 0x1 | 0x2,
        _ => 0,
    };
}
