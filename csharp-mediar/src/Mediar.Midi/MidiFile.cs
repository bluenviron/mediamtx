namespace Mediar.Midi;

/// <summary>MIDI channel-voice message types (status nibble &amp; 0xF0).</summary>
public enum MidiMessageType : byte
{
    /// <summary>Note Off (0x80).</summary>
    NoteOff = 0x80,
    /// <summary>Note On (0x90); note-on with velocity 0 is treated as note-off.</summary>
    NoteOn = 0x90,
    /// <summary>Polyphonic key pressure / aftertouch (0xA0).</summary>
    PolyphonicKeyPressure = 0xA0,
    /// <summary>Control change (0xB0).</summary>
    ControlChange = 0xB0,
    /// <summary>Program change (0xC0).</summary>
    ProgramChange = 0xC0,
    /// <summary>Channel pressure / aftertouch (0xD0).</summary>
    ChannelPressure = 0xD0,
    /// <summary>Pitch wheel (0xE0).</summary>
    PitchBend = 0xE0,
/// <summary>System-exclusive payload (0xF0/0xF7).</summary>
SystemExclusive = 0xF0,
    /// <summary>Meta event (0xFF — file-level only, not transmitted on the wire).</summary>
    Meta = 0xFF,
}

/// <summary>
/// Standard MIDI File "meta event" subtype identifiers (the byte that
/// follows the 0xFF status byte). The list covers the events defined by the
/// Standard MIDI File 1.0 specification.
/// </summary>
public enum MidiMetaType : byte
{
    /// <summary>Sequence number meta event (0x00).</summary>
    SequenceNumber = 0x00,
    /// <summary>Generic text meta event (0x01).</summary>
    Text = 0x01,
    /// <summary>Copyright notice (0x02).</summary>
    Copyright = 0x02,
    /// <summary>Sequence / track name (0x03).</summary>
    TrackName = 0x03,
    /// <summary>Instrument name (0x04).</summary>
    InstrumentName = 0x04,
    /// <summary>Lyric (0x05).</summary>
    Lyric = 0x05,
    /// <summary>Marker (0x06).</summary>
    Marker = 0x06,
    /// <summary>Cue point (0x07).</summary>
    CuePoint = 0x07,
    /// <summary>MIDI channel prefix (0x20).</summary>
    ChannelPrefix = 0x20,
    /// <summary>MIDI port (0x21).</summary>
    MidiPort = 0x21,
    /// <summary>End of track (0x2F).</summary>
    EndOfTrack = 0x2F,
    /// <summary>Set tempo, 3 bytes µs/quarter (0x51).</summary>
    SetTempo = 0x51,
    /// <summary>SMPTE offset (0x54).</summary>
    SmpteOffset = 0x54,
    /// <summary>Time signature (0x58).</summary>
    TimeSignature = 0x58,
    /// <summary>Key signature (0x59).</summary>
    KeySignature = 0x59,
    /// <summary>Sequencer-specific event (0x7F).</summary>
    SequencerSpecific = 0x7F,
}

/// <summary>
/// A single MIDI event (channel-voice, SysEx, or Meta) with its absolute
/// delta-time tick offset from the start of the track.
/// </summary>
public sealed record MidiEvent
{
    /// <summary>Absolute time in ticks from the start of the track.</summary>
    public required long Tick { get; init; }
    /// <summary>Status byte category.</summary>
    public required MidiMessageType Type { get; init; }
    /// <summary>Channel (0..15) for channel-voice messages; 0 otherwise.</summary>
    public byte Channel { get; init; }
    /// <summary>For Meta events, the meta subtype byte.</summary>
    public MidiMetaType MetaType { get; init; }
    /// <summary>First data byte (note number, controller, program, pitch LSB...).</summary>
    public byte Data1 { get; init; }
    /// <summary>Second data byte (velocity, value, pitch MSB...).</summary>
    public byte Data2 { get; init; }
    /// <summary>Optional payload for SysEx / Meta events.</summary>
    public ReadOnlyMemory<byte> Payload { get; init; }

    /// <summary>Pitch-bend value as a signed 14-bit centred on 0.</summary>
    public int PitchBend14 => Type == MidiMessageType.PitchBend
        ? ((Data2 << 7) | Data1) - 0x2000
        : 0;
}

/// <summary>
/// One track in a Standard MIDI File. SMF format 0 has exactly one track;
/// format 1 has many (typically one per instrument/voice).
/// </summary>
public sealed class MidiTrack
{
    /// <summary>Optional track name (from the 0xFF 0x03 meta event).</summary>
    public string? Name { get; init; }
    /// <summary>Optional instrument name (from the 0xFF 0x04 meta event).</summary>
    public string? Instrument { get; init; }
    /// <summary>All events in track order.</summary>
    public required IReadOnlyList<MidiEvent> Events { get; init; }
}

/// <summary>
/// Tempo specification carried by the SMF header. Pulses-per-quarter-note
/// (the common case) and SMPTE time-codes are both supported.
/// </summary>
public sealed record MidiDivision
{
    /// <summary>True when this division uses ticks-per-quarter-note (the common form).</summary>
    public bool IsTicksPerQuarter { get; init; }
    /// <summary>Ticks per quarter note (when <see cref="IsTicksPerQuarter"/> is true).</summary>
    public ushort TicksPerQuarter { get; init; }
    /// <summary>SMPTE frame rate (24, 25, 29, 30).</summary>
    public byte SmpteFrames { get; init; }
    /// <summary>Ticks per SMPTE frame.</summary>
    public byte TicksPerFrame { get; init; }

    /// <summary>Convenience constructor for the common PPQ form.</summary>
    public static MidiDivision Ppq(ushort ticksPerQuarter) =>
        new() { IsTicksPerQuarter = true, TicksPerQuarter = ticksPerQuarter };
}

/// <summary>
/// Top-level Standard MIDI File model. Use <see cref="MidiReader"/> to load
/// one from disk or a stream and <see cref="MidiWriter"/> to write one back.
/// </summary>
public sealed class MidiFile
{
    /// <summary>SMF format: 0 (single track), 1 (multi-track), 2 (independent patterns).</summary>
    public required ushort Format { get; init; }
    /// <summary>Time-division metadata from the header.</summary>
    public required MidiDivision Division { get; init; }
    /// <summary>All tracks in file order.</summary>
    public required IReadOnlyList<MidiTrack> Tracks { get; init; }
}
