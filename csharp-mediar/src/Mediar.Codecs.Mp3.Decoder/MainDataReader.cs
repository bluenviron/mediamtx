namespace Mediar.Codecs.Mp3.Decoder;

/// <summary>
/// MP3 main-data bit reservoir reader. A Layer III frame's main_data does
/// not begin at the byte immediately after the side-info: it can be located
/// up to 511 bytes back into the previous frames' main-data area (the
/// "back-pointer" addressed by <c>main_data_begin</c> in side-info).
/// </summary>
/// <remarks>
/// <para>
/// This class owns a circular byte buffer (default 8 KiB — more than enough
/// to hold 511 bytes of back-pointer plus the current frame's main-data,
/// which is at most ~1730 bytes at 320 kbps stereo MPEG-1).
/// </para>
/// <para>
/// Usage per frame:
/// </para>
/// <list type="number">
///   <item><see cref="ResetCursor(int)"/> with <c>main_data_begin</c>.</item>
///   <item>If <see cref="HasEnoughHistory"/> is false, the frame cannot be
///         decoded (post-seek or start-of-stream) — emit silence and
///         <see cref="Append(System.ReadOnlySpan{byte})"/> only.</item>
///   <item>Otherwise call read-bit accessors as the main-data layout dictates.</item>
///   <item>After decoding (success or skip), <see cref="Append(System.ReadOnlySpan{byte})"/>
///         the current frame's main-data bytes for future frames.</item>
/// </list>
/// </remarks>
internal sealed class MainDataReader
{
    private readonly byte[] _buffer;
    private int _writePos;
    private int _validBytes;
    private int _bitCursor;
    private bool _hasEnoughHistory;

    public MainDataReader(int capacity = 8192)
    {
        if (capacity < 2048)
            throw new ArgumentOutOfRangeException(nameof(capacity), "Reservoir capacity must be at least 2 KiB.");
        _buffer = new byte[capacity];
    }

    public int Capacity => _buffer.Length;

    /// <summary>True if the previous-frame back-pointer fits in the reservoir for the current frame.</summary>
    public bool HasEnoughHistory => _hasEnoughHistory;

    /// <summary>Remaining bytes of reservoir not yet consumed by current frame's cursor.</summary>
    public int RemainingBits => _validBytes * 8 - _bitCursor;

    /// <summary>Clear all reservoir state (call on Reset / start-of-stream).</summary>
    public void Clear()
    {
        _writePos = 0;
        _validBytes = 0;
        _bitCursor = 0;
        _hasEnoughHistory = false;
    }

    /// <summary>Append the current frame's main-data bytes to the reservoir.</summary>
    public void Append(ReadOnlySpan<byte> mainData)
    {
        int remaining = mainData.Length;
        while (remaining > 0)
        {
            int chunk = Math.Min(remaining, _buffer.Length - _writePos);
            mainData.Slice(mainData.Length - remaining, chunk).CopyTo(_buffer.AsSpan(_writePos));
            _writePos = (_writePos + chunk) % _buffer.Length;
            remaining -= chunk;
        }
        _validBytes = Math.Min(_validBytes + mainData.Length, _buffer.Length);
    }

    /// <summary>
    /// Position the bit cursor so that subsequent reads start
    /// <c>main_data_begin</c> bytes before the end of the currently-appended
    /// reservoir. Call AFTER appending the current frame's main_data bytes.
    /// </summary>
    public void ResetCursor(int mainDataBegin)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(mainDataBegin);

        // The cursor positions to a point mainDataBegin bytes before the start
        // of the current frame's bytes (i.e., into the previous reservoir).
        // Since we already appended this frame, "back from end" = mainDataBegin
        // PLUS the current frame's appended bytes — but we don't know that here.
        // The caller passes the already-known position-from-end (in bits).
        // For simplicity we expose Append-then-Position via SetCursorBytesFromEnd.
        throw new InvalidOperationException(
            "Use SetCursorBytesFromEnd(bytesFromEnd) to position the cursor.");
    }

    /// <summary>Set the bit cursor a given number of bytes from the END of valid reservoir data.</summary>
    public void SetCursorBytesFromEnd(int bytesFromEnd)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(bytesFromEnd);
        if (bytesFromEnd > _validBytes)
        {
            _hasEnoughHistory = false;
            _bitCursor = _validBytes * 8;
            return;
        }
        _hasEnoughHistory = true;
        _bitCursor = (_validBytes - bytesFromEnd) * 8;
    }

    /// <summary>Read up to 24 bits MSB-first. Returns 0 if cursor is past end.</summary>
    public uint ReadBits(int n)
    {
        if (n <= 0) return 0;
        if (n > 24) throw new ArgumentOutOfRangeException(nameof(n), "Use ReadBits32 for >24 bits.");
        uint result = 0;
        int totalBits = _validBytes * 8;
        while (n > 0)
        {
            if (_bitCursor >= totalBits) return result << n; // ran past end: zero-fill
            int byteIdx = ResolveByteIndex(_bitCursor >> 3);
            int bitInByte = 8 - (_bitCursor & 7);
            int take = Math.Min(n, bitInByte);
            int shift = bitInByte - take;
            uint chunk = (uint)((_buffer[byteIdx] >> shift) & ((1 << take) - 1));
            result = (result << take) | chunk;
            _bitCursor += take;
            n -= take;
        }
        return result;
    }

    public uint ReadBits32(int n)
    {
        if (n <= 24) return ReadBits(n);
        uint hi = ReadBits(n - 24);
        uint lo = ReadBits(24);
        return (hi << 24) | lo;
    }

    public bool ReadBit() => ReadBits(1) != 0;

    public void SkipBits(int n)
    {
        if (n <= 0) return;
        _bitCursor = Math.Min(_bitCursor + n, _validBytes * 8);
    }

    /// <summary>Current bit cursor position relative to the start of valid reservoir data.</summary>
    public int BitPosition => _bitCursor;

    /// <summary>Set the bit cursor to an absolute position relative to the start of valid reservoir data.</summary>
    public void SetBitPosition(int bitPosition)
    {
        ArgumentOutOfRangeException.ThrowIfNegative(bitPosition);
        _bitCursor = Math.Min(bitPosition, _validBytes * 8);
    }

    private int ResolveByteIndex(int positionFromStart)
    {
        // _writePos points one past the last appended byte. The first valid byte
        // is _writePos - _validBytes (mod capacity).
        int start = (_writePos - _validBytes + _buffer.Length) % _buffer.Length;
        return (start + positionFromStart) % _buffer.Length;
    }
}
