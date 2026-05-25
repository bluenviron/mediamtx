using System.Buffers.Binary;

namespace Mediar.Containers.Ogg;

/// <summary>
/// Single-stream Ogg muxer. Wraps caller-supplied packets in Ogg pages with
/// the proper segment table, granule positions and CRC32. Suitable for Opus,
/// Vorbis or FLAC-in-Ogg payloads.
/// </summary>
/// <remarks>
/// <para>
/// Each call to <see cref="WriteSampleAsync"/> produces one Ogg packet. The
/// muxer batches multiple small packets into a single page for efficiency
/// (lacing table up to 255 segments per page) and splits long packets across
/// continuation pages.
/// </para>
/// <para>
/// Header packets (OpusHead/OpusTags for Opus, the three Vorbis setup packets
/// for Vorbis, or the FLAC mapping packet for FLAC-in-Ogg) must be queued via
/// <see cref="AddHeaderPacket"/> before <see cref="StartAsync"/>. For Opus, if
/// no header packets are queued and the track's <see cref="CodecParameters.ExtraData"/>
/// contains a valid OpusHead, the muxer synthesizes a minimal OpusTags page.
/// </para>
/// </remarks>
public sealed class OggMuxer : IMediaMuxer
{
    private readonly Stream _output;
    private readonly bool _leaveOpen;
    private readonly uint _serialNumber;
    private MediaTrack? _track;
    private bool _started;
    private bool _finished;
    private uint _sequenceNumber;
    private bool _wroteBos;

    private readonly List<byte[]> _headerPackets = new();
    private readonly byte[] _laceTable = new byte[255];
    private readonly List<byte> _pagePayload = new(8192);
    private int _laceCount;
    private long _pendingGranule = -1;
    private bool _nextPageContinuation;

    /// <summary>Create a muxer writing to <paramref name="output"/>.</summary>
    public OggMuxer(Stream output, uint serialNumber = 0x4D454449u /* "MEDI" */, bool leaveOpen = false)
    {
        ArgumentNullException.ThrowIfNull(output);
        if (!output.CanWrite) throw new ArgumentException("Output stream must be writable.", nameof(output));
        _output = output;
        _leaveOpen = leaveOpen;
        _serialNumber = serialNumber;
    }

    /// <inheritdoc/>
    public string FormatName => "ogg";

    /// <inheritdoc/>
    public void AddTrack(MediaTrack track)
    {
        ArgumentNullException.ThrowIfNull(track);
        if (_started) throw new InvalidOperationException("Cannot add tracks after Start.");
        if (_track is not null) throw new InvalidOperationException("OggMuxer supports a single logical stream.");
        if (track.Codec is not AudioCodecParameters audio)
        {
            throw new ArgumentException("OggMuxer accepts only audio tracks for now.", nameof(track));
        }
        if (audio.Codec is not (CodecId.Opus or CodecId.Vorbis or CodecId.Flac))
        {
            throw new ArgumentException($"OggMuxer does not support codec {audio.Codec}.", nameof(track));
        }
        _track = track;
    }

    /// <summary>
    /// Queue a codec setup header packet. Each codec has a fixed number of
    /// header packets (Opus=2, Vorbis=3, FLAC=1+). Call before <see cref="StartAsync"/>.
    /// </summary>
    public void AddHeaderPacket(ReadOnlyMemory<byte> packet)
    {
        if (_started) throw new InvalidOperationException("Cannot add header packets after Start.");
        _headerPackets.Add(packet.ToArray());
    }

    /// <inheritdoc/>
    public async ValueTask StartAsync(CancellationToken cancellationToken = default)
    {
        if (_track is null) throw new InvalidOperationException("Add a track before starting.");
        if (_started) return;
        _started = true;

        var audio = (AudioCodecParameters)_track.Codec;

        // Auto-fill header packets when caller hasn't supplied them.
        if (_headerPackets.Count == 0)
        {
            if (!_track.Codec.ExtraData.IsEmpty)
            {
                _headerPackets.Add(_track.Codec.ExtraData.ToArray());
            }
            if (audio.Codec == CodecId.Opus && _headerPackets.Count == 1)
            {
                _headerPackets.Add(BuildMinimalOpusTags());
            }
        }
        if (_headerPackets.Count == 0)
        {
            throw new InvalidOperationException("No codec header packets are available.");
        }

        // Write each header packet as its own page (per Ogg mapping convention).
        for (int i = 0; i < _headerPackets.Count; i++)
        {
            byte headerType = (byte)((i == 0) ? 0x02 /* BOS */ : 0x00);
            await EmitSinglePacketPageAsync(headerType, granulePosition: 0, _headerPackets[i], cancellationToken).ConfigureAwait(false);
            if (i == 0) _wroteBos = true;
        }
    }

    /// <inheritdoc/>
    public async ValueTask WriteSampleAsync(MediaSample sample, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(sample);
        if (!_started) throw new InvalidOperationException("Call StartAsync first.");
        if (_finished) throw new InvalidOperationException("Muxer already finalized.");

        var packet = sample.Data.ToArray();
        long granuleAtEnd = sample.Pts + sample.Duration;

        // Lay packet into one or more pages using the 255-segment lacing table.
        int pos = 0;
        int remaining = packet.Length;
        do
        {
            if (_laceCount == 255)
            {
                await FlushPageAsync(eos: false, granulePosition: -1, cancellationToken).ConfigureAwait(false);
                _nextPageContinuation = true;
            }

            int take = Math.Min(255, remaining);
            if (take > 0)
            {
                for (int i = 0; i < take; i++) _pagePayload.Add(packet[pos + i]);
                pos += take;
                remaining -= take;
            }
            _laceTable[_laceCount++] = (byte)take;

            // Trailing 0-byte segment when the packet is an exact multiple of 255.
            if (take == 255 && remaining == 0)
            {
                if (_laceCount == 255)
                {
                    await FlushPageAsync(eos: false, granulePosition: -1, cancellationToken).ConfigureAwait(false);
                    _nextPageContinuation = true;
                }
                _laceTable[_laceCount++] = 0;
                break;
            }
        } while (remaining > 0);

        _pendingGranule = granuleAtEnd;
    }

    /// <inheritdoc/>
    public async ValueTask FinishAsync(CancellationToken cancellationToken = default)
    {
        if (_finished) return;
        if (_laceCount > 0)
        {
            await FlushPageAsync(eos: true, granulePosition: _pendingGranule, cancellationToken).ConfigureAwait(false);
        }
        else
        {
            // Emit an empty EOS page so demuxers detect end-of-stream cleanly.
            await EmitEmptyEosPageAsync(cancellationToken).ConfigureAwait(false);
        }
        await _output.FlushAsync(cancellationToken).ConfigureAwait(false);
        _finished = true;
    }

    private async ValueTask EmitSinglePacketPageAsync(byte headerType, long granulePosition, byte[] packet, CancellationToken ct)
    {
        // For setup packets that fit in <=255 segments, emit as a fresh page.
        int segments = packet.Length / 255 + 1;
        if (segments > 255)
        {
            // Split into multiple pages; reuse the normal packet writer.
            int pos = 0;
            int remaining = packet.Length;
            do
            {
                if (_laceCount == 255)
                {
                    await FlushPageAsync(false, -1, ct).ConfigureAwait(false);
                    _nextPageContinuation = true;
                }
                int take = Math.Min(255, remaining);
                if (take > 0)
                {
                    _pagePayload.AddRange(new ReadOnlySpan<byte>(packet, pos, take).ToArray());
                    pos += take;
                    remaining -= take;
                }
                _laceTable[_laceCount++] = (byte)take;
                if (take == 255 && remaining == 0)
                {
                    if (_laceCount == 255)
                    {
                        await FlushPageAsync(false, -1, ct).ConfigureAwait(false);
                        _nextPageContinuation = true;
                    }
                    _laceTable[_laceCount++] = 0;
                    break;
                }
            } while (remaining > 0);

            await FlushPageAsync(false, granulePosition, ct, overrideHeaderType: headerType).ConfigureAwait(false);
        }
        else
        {
            int pos = 0;
            int remaining = packet.Length;
            do
            {
                int take = Math.Min(255, remaining);
                if (take > 0)
                {
                    _pagePayload.AddRange(new ReadOnlySpan<byte>(packet, pos, take).ToArray());
                    pos += take;
                    remaining -= take;
                }
                _laceTable[_laceCount++] = (byte)take;
                if (take == 255 && remaining == 0)
                {
                    _laceTable[_laceCount++] = 0;
                    break;
                }
            } while (remaining > 0);
            await FlushPageAsync(false, granulePosition, ct, overrideHeaderType: headerType).ConfigureAwait(false);
        }
    }

    private async ValueTask EmitEmptyEosPageAsync(CancellationToken ct)
    {
        _laceTable[_laceCount++] = 0;
        await FlushPageAsync(eos: true, granulePosition: _pendingGranule, ct).ConfigureAwait(false);
    }

    private async ValueTask FlushPageAsync(bool eos, long granulePosition, CancellationToken ct, byte? overrideHeaderType = null)
    {
        byte headerType;
        if (overrideHeaderType.HasValue)
        {
            headerType = overrideHeaderType.Value;
            if (eos) headerType |= 0x04;
        }
        else
        {
            headerType = 0;
            if (_nextPageContinuation) headerType |= 0x01;
            if (eos) headerType |= 0x04;
            if (!_wroteBos)
            {
                headerType |= 0x02;
                _wroteBos = true;
            }
        }

        int headerLen = 27 + _laceCount;
        byte[] page = new byte[headerLen + _pagePayload.Count];
        page[0] = (byte)'O'; page[1] = (byte)'g'; page[2] = (byte)'g'; page[3] = (byte)'S';
        page[4] = 0; // version
        page[5] = headerType;
        BinaryPrimitives.WriteInt64LittleEndian(page.AsSpan(6, 8), granulePosition);
        BinaryPrimitives.WriteUInt32LittleEndian(page.AsSpan(14, 4), _serialNumber);
        BinaryPrimitives.WriteUInt32LittleEndian(page.AsSpan(18, 4), _sequenceNumber++);
        // CRC field (page[22..26]) left zero for the CRC computation.
        page[26] = (byte)_laceCount;
        Buffer.BlockCopy(_laceTable, 0, page, 27, _laceCount);
        for (int i = 0; i < _pagePayload.Count; i++) page[headerLen + i] = _pagePayload[i];

        uint crc = OggCrc.Compute(page);
        BinaryPrimitives.WriteUInt32LittleEndian(page.AsSpan(22, 4), crc);

        await _output.WriteAsync(page, ct).ConfigureAwait(false);

        _laceCount = 0;
        _pagePayload.Clear();
        _nextPageContinuation = false;
    }

    private static byte[] BuildMinimalOpusTags()
    {
        // "OpusTags" + LE32 vendor_len + vendor + LE32 user_comment_count(=0)
        const string vendor = "Mediar";
        byte[] vendorBytes = System.Text.Encoding.UTF8.GetBytes(vendor);
        byte[] pkt = new byte[8 + 4 + vendorBytes.Length + 4];
        pkt[0] = (byte)'O'; pkt[1] = (byte)'p'; pkt[2] = (byte)'u'; pkt[3] = (byte)'s';
        pkt[4] = (byte)'T'; pkt[5] = (byte)'a'; pkt[6] = (byte)'g'; pkt[7] = (byte)'s';
        BinaryPrimitives.WriteUInt32LittleEndian(pkt.AsSpan(8, 4), (uint)vendorBytes.Length);
        Buffer.BlockCopy(vendorBytes, 0, pkt, 12, vendorBytes.Length);
        BinaryPrimitives.WriteUInt32LittleEndian(pkt.AsSpan(12 + vendorBytes.Length, 4), 0);
        return pkt;
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (!_finished)
        {
            try { _output.Flush(); } catch { /* swallow */ }
            _finished = true;
        }
        if (!_leaveOpen) _output.Dispose();
    }

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (!_finished)
        {
            try { await _output.FlushAsync().ConfigureAwait(false); } catch { /* swallow */ }
            _finished = true;
        }
        if (!_leaveOpen) await _output.DisposeAsync().ConfigureAwait(false);
    }
}
