using System.Buffers.Binary;
using System.Text;

namespace Mediar.Containers.Voc;

/// <summary>
/// Writes Creative Voice files using a single "new-format" type-9 sound-data
/// block (Sound Blaster 16 / Sound Blaster Pro era). PCM U8 / S16LE / G.711 are
/// supported.
/// </summary>
public sealed class VocMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private MediaTrack? _track;
    private long _blockSizePos;
    private long _payloadStart;
    private long _bytesWritten;
    private bool _started;
    private bool _finished;

    /// <summary>Create a VOC muxer.</summary>
    public VocMuxer(Stream output, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite || !output.CanSeek)
            throw new ArgumentException("Stream must be writable and seekable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
    }

    /// <inheritdoc/>
    public string FormatName => "voc";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_track is not null) throw new InvalidOperationException("VOC supports a single track.");
        if (track.Codec is not AudioCodecParameters a)
            throw new ArgumentException("VOC muxer requires an audio track.", nameof(track));
        _ = ToVocCodecId(a.Codec); // validates
        _track = track;
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("AddTrack must be called first.");
        if (_started) return;
        _started = true;
        var a = (AudioCodecParameters)_track.Codec;

        // 26-byte header.
        byte[] header = new byte[26];
        Encoding.ASCII.GetBytes("Creative Voice File").CopyTo(header.AsSpan(0, 19));
        header[19] = 0x1A;
        BinaryPrimitives.WriteUInt16LittleEndian(header.AsSpan(20, 2), 26); // data start
        BinaryPrimitives.WriteUInt16LittleEndian(header.AsSpan(22, 2), 0x010A); // version 1.10
        // Validation code = (~version + 0x1234) mod 0x10000
        ushort code = unchecked((ushort)(~(0x010A + 0x1234) + 0x1234));
        BinaryPrimitives.WriteUInt16LittleEndian(header.AsSpan(24, 2), code);
        await _output.WriteAsync(header, cancellationToken).ConfigureAwait(false);

        // Block 9 header (type + 3-byte size patched in FinishAsync), then 12-byte format header.
        byte[] hdr = new byte[4 + 12];
        hdr[0] = 9;
        _blockSizePos = _output.Position + 1;
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.AsSpan(4, 4), (uint)a.SampleRate);
        hdr[8] = (byte)a.BitsPerSample;
        hdr[9] = (byte)a.Channels;
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.AsSpan(10, 2), ToVocCodecId(a.Codec));
        // hdr[12..16] reserved = 0
        await _output.WriteAsync(hdr, cancellationToken).ConfigureAwait(false);
        _payloadStart = _output.Position;
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
        // Terminator block (type 0)
        _output.WriteByte(0);
        long endPos = _output.Position;
        // Patch the type-9 block size = (12 header bytes + payload).
        long blockPayloadSize = 12 + _bytesWritten;
        if (blockPayloadSize > 0xFFFFFF)
            throw new InvalidOperationException("VOC block size limit (16MB) exceeded; split into multiple files.");
        byte[] sz = new byte[3];
        sz[0] = (byte)(blockPayloadSize & 0xFF);
        sz[1] = (byte)((blockPayloadSize >> 8) & 0xFF);
        sz[2] = (byte)((blockPayloadSize >> 16) & 0xFF);
        _output.Seek(_blockSizePos, SeekOrigin.Begin);
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

    private static ushort ToVocCodecId(CodecId codec) => codec switch
    {
        CodecId.PcmU8 => 0,
        CodecId.PcmS16Le => 4,
        CodecId.G711ALaw => 6,
        CodecId.G711MuLaw => 7,
        _ => throw new NotSupportedException($"VOC muxer cannot write codec '{codec}'."),
    };
}
