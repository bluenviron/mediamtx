using System.Text;
using Mediar.Midi;
using Xunit;

namespace Mediar.Tests;

public sealed class MidiRoundTripTests
{
    [Fact]
    public void Format0_RoundTrips_With_NoteOn_NoteOff_And_Meta()
    {
        var events = new List<MidiEvent>
        {
            new() {
                Tick = 0, Type = MidiMessageType.Meta, MetaType = MidiMetaType.TrackName,
                Payload = Encoding.UTF8.GetBytes("Lead"),
            },
            new() {
                Tick = 0, Type = MidiMessageType.Meta, MetaType = MidiMetaType.SetTempo,
                Payload = new byte[] { 0x07, 0xA1, 0x20 }, // 500000 µs/quarter = 120 BPM
            },
            new() { Tick = 0,   Type = MidiMessageType.ProgramChange, Channel = 0, Data1 = 1 },
            new() { Tick = 0,   Type = MidiMessageType.NoteOn,  Channel = 0, Data1 = 60, Data2 = 100 },
            new() { Tick = 240, Type = MidiMessageType.NoteOn,  Channel = 0, Data1 = 60, Data2 = 0 },
            new() { Tick = 240, Type = MidiMessageType.NoteOn,  Channel = 0, Data1 = 62, Data2 = 100 },
            new() { Tick = 480, Type = MidiMessageType.NoteOff, Channel = 0, Data1 = 62, Data2 = 64 },
        };
        var track = new MidiTrack { Name = "Lead", Events = events };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(480), Tracks = [track] };

        byte[] bytes = MidiWriter.Write(file);
        Assert.Equal((byte)'M', bytes[0]);
        Assert.Equal((byte)'T', bytes[1]);
        Assert.Equal((byte)'h', bytes[2]);
        Assert.Equal((byte)'d', bytes[3]);

        MidiFile roundTripped = MidiReader.Read(bytes);
        Assert.Equal(0, roundTripped.Format);
        Assert.True(roundTripped.Division.IsTicksPerQuarter);
        Assert.Equal((ushort)480, roundTripped.Division.TicksPerQuarter);
        var rt = Assert.Single(roundTripped.Tracks);
        Assert.Equal("Lead", rt.Name);

        // Reader will auto-append EndOfTrack if missing; writer also auto-appends.
        // So expected events = 7 input + 1 EOT.
        Assert.Equal(events.Count + 1, rt.Events.Count);
        Assert.Equal(MidiMetaType.EndOfTrack, rt.Events[^1].MetaType);

        // Compare salient fields (ignore EOT at end).
        for (int i = 0; i < events.Count; i++)
        {
            Assert.Equal(events[i].Tick, rt.Events[i].Tick);
            Assert.Equal(events[i].Type, rt.Events[i].Type);
            Assert.Equal(events[i].Channel, rt.Events[i].Channel);
            Assert.Equal(events[i].Data1, rt.Events[i].Data1);
            if (events[i].Type != MidiMessageType.ProgramChange
                && events[i].Type != MidiMessageType.ChannelPressure
                && events[i].Type != MidiMessageType.Meta)
            {
                Assert.Equal(events[i].Data2, rt.Events[i].Data2);
            }
        }
    }

    [Fact]
    public void Format1_Multi_Track_RoundTrips()
    {
        var tA = new MidiTrack
        {
            Events =
            [
                new() { Tick = 0, Type = MidiMessageType.NoteOn,  Channel = 0, Data1 = 60, Data2 = 100 },
                new() { Tick = 96, Type = MidiMessageType.NoteOff, Channel = 0, Data1 = 60, Data2 = 0 },
            ],
        };
        var tB = new MidiTrack
        {
            Events =
            [
                new() { Tick = 0, Type = MidiMessageType.NoteOn,  Channel = 1, Data1 = 64, Data2 = 100 },
                new() { Tick = 96, Type = MidiMessageType.NoteOff, Channel = 1, Data1 = 64, Data2 = 0 },
            ],
        };
        var file = new MidiFile { Format = 1, Division = MidiDivision.Ppq(96), Tracks = [tA, tB] };

        byte[] bytes = MidiWriter.Write(file);
        MidiFile rt = MidiReader.Read(bytes);
        Assert.Equal(1, rt.Format);
        Assert.Equal(2, rt.Tracks.Count);
        Assert.Equal(MidiMessageType.NoteOn, rt.Tracks[0].Events[0].Type);
        Assert.Equal(0, rt.Tracks[0].Events[0].Channel);
        Assert.Equal(MidiMessageType.NoteOn, rt.Tracks[1].Events[0].Type);
        Assert.Equal(1, rt.Tracks[1].Events[0].Channel);
    }

    [Fact]
    public void SysEx_RoundTrips()
    {
        var track = new MidiTrack
        {
            Events =
            [
                new() {
                    Tick = 0, Type = MidiMessageType.SystemExclusive,
                    Data1 = 0xF0,
                    Payload = new byte[] { 0x43, 0x12, 0x00, 0xF7 }, // arbitrary MFR payload, ending with EOX
                },
            ],
        };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        byte[] bytes = MidiWriter.Write(file);
        MidiFile rt = MidiReader.Read(bytes);

        var firstEvent = rt.Tracks[0].Events[0];
        Assert.Equal(MidiMessageType.SystemExclusive, firstEvent.Type);
        Assert.Equal(0xF0, firstEvent.Data1);
        Assert.Equal(4, firstEvent.Payload.Length);
        Assert.Equal((byte)0x43, firstEvent.Payload.Span[0]);
    }

    [Fact]
    public void Running_Status_Compression_Is_Decoded()
    {
        // Six consecutive NoteOn events on channel 0 with the same status byte.
        // The writer should emit one status byte and 6 data-pairs;
        // the reader must restore them as 6 separate events.
        var events = new List<MidiEvent>();
        for (int i = 0; i < 6; i++)
        {
            events.Add(new MidiEvent
            {
                Tick = i * 10, Type = MidiMessageType.NoteOn,
                Channel = 0, Data1 = (byte)(60 + i), Data2 = 100,
            });
        }
        var track = new MidiTrack { Events = events };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        byte[] bytes = MidiWriter.Write(file);
        MidiFile rt = MidiReader.Read(bytes);

        // EOT auto-appended → 7 events.
        Assert.Equal(7, rt.Tracks[0].Events.Count);
        for (int i = 0; i < 6; i++)
        {
            Assert.Equal(MidiMessageType.NoteOn, rt.Tracks[0].Events[i].Type);
            Assert.Equal((byte)(60 + i), rt.Tracks[0].Events[i].Data1);
        }
    }

    [Fact]
    public void Smpte_Division_RoundTrips()
    {
        var file = new MidiFile
        {
            Format = 0,
            Division = new MidiDivision
            {
                IsTicksPerQuarter = false,
                SmpteFrames = 25,
                TicksPerFrame = 80,
            },
            Tracks =
            [
                new() { Events = [] },
            ],
        };
        byte[] bytes = MidiWriter.Write(file);
        MidiFile rt = MidiReader.Read(bytes);
        Assert.False(rt.Division.IsTicksPerQuarter);
        Assert.Equal((byte)25, rt.Division.SmpteFrames);
        Assert.Equal((byte)80, rt.Division.TicksPerFrame);
    }

    [Fact]
    public void Throws_On_Missing_MThd_Header()
    {
        byte[] junk = new byte[16];
        Assert.Throws<InvalidDataException>(() => MidiReader.Read(junk));
    }

    [Fact]
    public void Throws_On_Too_Small_Input()
    {
        Assert.Throws<InvalidDataException>(() => MidiReader.Read(new byte[5]));
    }

    [Fact]
    public void Throws_On_Truncated_Header_Chunk_Length()
    {
        // MThd present, but advertised header length < 6 bytes.
        byte[] bad = new byte[16];
        bad[0] = (byte)'M'; bad[1] = (byte)'T'; bad[2] = (byte)'h'; bad[3] = (byte)'d';
        // header length = 4 (too small)
        bad[4] = 0; bad[5] = 0; bad[6] = 0; bad[7] = 4;
        Assert.Throws<InvalidDataException>(() => MidiReader.Read(bad));
    }

    [Fact]
    public void Throws_On_Truncated_MTrk_Body()
    {
        // Header advertises one track of 100 bytes but body is missing.
        var file = new MidiFile
        {
            Format = 0,
            Division = MidiDivision.Ppq(96),
            Tracks = [new MidiTrack { Events = [
                new() { Tick = 0, Type = MidiMessageType.NoteOn, Channel = 0, Data1 = 60, Data2 = 100 },
            ] }],
        };
        byte[] good = MidiWriter.Write(file);
        // Truncate to just header + first 8 bytes of MTrk.
        byte[] truncated = good.AsSpan(0, 14 + 8 + 4).ToArray();
        // Inflate the trkLen field to something larger than what's present.
        truncated[18] = 0xFF;
        // Some malformations surface as InvalidDataException, others
        // as IndexOutOfRangeException; either signals a parse failure.
        Assert.ThrowsAny<Exception>(() => MidiReader.Read(truncated));
    }

    [Fact]
    public void ReadFile_NullOrEmpty_Path_Throws()
    {
        Assert.Throws<ArgumentNullException>(() => MidiReader.ReadFile(null!));
        Assert.Throws<ArgumentException>(() => MidiReader.ReadFile(string.Empty));
    }

    [Fact]
    public void ReadFile_Missing_Path_Throws_FileNotFound()
    {
        var path = Path.Combine(Path.GetTempPath(),
            "missing-midi-" + Guid.NewGuid().ToString("N") + ".mid");
        Assert.Throws<FileNotFoundException>(() => MidiReader.ReadFile(path));
    }

    [Fact]
    public void ControlChange_RoundTrips()
    {
        var track = new MidiTrack
        {
            Events =
            [
                new() { Tick = 0,  Type = MidiMessageType.ControlChange, Channel = 0, Data1 = 7,  Data2 = 100 }, // volume
                new() { Tick = 10, Type = MidiMessageType.ControlChange, Channel = 0, Data1 = 64, Data2 = 127 }, // sustain
            ],
        };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        var ev0 = rt.Tracks[0].Events[0];
        Assert.Equal(MidiMessageType.ControlChange, ev0.Type);
        Assert.Equal((byte)7, ev0.Data1);
        Assert.Equal((byte)100, ev0.Data2);
        Assert.Equal((byte)64, rt.Tracks[0].Events[1].Data1);
    }

    [Fact]
    public void PitchBend_RoundTrips_And_PitchBend14_Computes_Centered_Value()
    {
        // Pitch bend value 0x2000 = centered (0)
        var track = new MidiTrack
        {
            Events =
            [
                new() { Tick = 0, Type = MidiMessageType.PitchBend, Channel = 0, Data1 = 0x00, Data2 = 0x40 },
            ],
        };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        var ev = rt.Tracks[0].Events[0];
        Assert.Equal(MidiMessageType.PitchBend, ev.Type);
        Assert.Equal(0, ev.PitchBend14);
    }

    [Fact]
    public void PolyphonicKeyPressure_RoundTrips()
    {
        var track = new MidiTrack
        {
            Events =
            [
                new() { Tick = 0, Type = MidiMessageType.PolyphonicKeyPressure, Channel = 2, Data1 = 60, Data2 = 80 },
            ],
        };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        var ev = rt.Tracks[0].Events[0];
        Assert.Equal(MidiMessageType.PolyphonicKeyPressure, ev.Type);
        Assert.Equal(2, ev.Channel);
        Assert.Equal((byte)60, ev.Data1);
        Assert.Equal((byte)80, ev.Data2);
    }

    [Fact]
    public void ChannelPressure_RoundTrips()
    {
        var track = new MidiTrack
        {
            Events =
            [
                new() { Tick = 0, Type = MidiMessageType.ChannelPressure, Channel = 3, Data1 = 64 },
            ],
        };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        var ev = rt.Tracks[0].Events[0];
        Assert.Equal(MidiMessageType.ChannelPressure, ev.Type);
        Assert.Equal(3, ev.Channel);
        Assert.Equal((byte)64, ev.Data1);
    }

    [Fact]
    public void Multiple_Meta_Subtypes_RoundTrip()
    {
        var events = new List<MidiEvent>
        {
            new() {
                Tick = 0, Type = MidiMessageType.Meta, MetaType = MidiMetaType.Copyright,
                Payload = Encoding.UTF8.GetBytes("(c) Mediar"),
            },
            new() {
                Tick = 0, Type = MidiMessageType.Meta, MetaType = MidiMetaType.Text,
                Payload = Encoding.UTF8.GetBytes("Some text"),
            },
            new() {
                Tick = 5, Type = MidiMessageType.Meta, MetaType = MidiMetaType.Marker,
                Payload = Encoding.UTF8.GetBytes("Verse 1"),
            },
            new() {
                Tick = 10, Type = MidiMessageType.Meta, MetaType = MidiMetaType.CuePoint,
                Payload = Encoding.UTF8.GetBytes("Cue A"),
            },
            new() {
                Tick = 15, Type = MidiMessageType.Meta, MetaType = MidiMetaType.Lyric,
                Payload = Encoding.UTF8.GetBytes("La"),
            },
            new() {
                Tick = 20, Type = MidiMessageType.Meta, MetaType = MidiMetaType.TimeSignature,
                Payload = new byte[] { 4, 2, 24, 8 }, // 4/4
            },
            new() {
                Tick = 20, Type = MidiMessageType.Meta, MetaType = MidiMetaType.KeySignature,
                Payload = new byte[] { 0x00, 0x00 }, // C major
            },
        };
        var track = new MidiTrack { Events = events };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        // EOT auto-appended -> events.Count + 1.
        Assert.Equal(events.Count + 1, rt.Tracks[0].Events.Count);
        Assert.Equal(MidiMetaType.Copyright, rt.Tracks[0].Events[0].MetaType);
        Assert.Equal(MidiMetaType.KeySignature, rt.Tracks[0].Events[^2].MetaType);
        Assert.Equal(MidiMetaType.EndOfTrack, rt.Tracks[0].Events[^1].MetaType);
    }

    [Fact]
    public void Empty_Track_Yields_Only_EndOfTrack()
    {
        var file = new MidiFile
        {
            Format = 0, Division = MidiDivision.Ppq(96),
            Tracks = [new MidiTrack { Events = [] }],
        };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        var ev = Assert.Single(rt.Tracks[0].Events);
        Assert.Equal(MidiMessageType.Meta, ev.Type);
        Assert.Equal(MidiMetaType.EndOfTrack, ev.MetaType);
    }

    [Fact]
    public void Header_Format_1_Track_Count_Matches()
    {
        var file = new MidiFile
        {
            Format = 1, Division = MidiDivision.Ppq(96),
            Tracks =
            [
                new() { Events = [] },
                new() { Events = [] },
                new() { Events = [] },
            ],
        };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        Assert.Equal(1, rt.Format);
        Assert.Equal(3, rt.Tracks.Count);
    }

    [Fact]
    public void PitchBend14_Returns_Zero_For_NonPitchBend_Event()
    {
        var ev = new MidiEvent
        {
            Tick = 0, Type = MidiMessageType.NoteOn,
            Channel = 0, Data1 = 60, Data2 = 100,
        };
        Assert.Equal(0, ev.PitchBend14);
    }

    [Fact]
    public void PitchBend14_Min_And_Max_Return_Expected_Signed_Values()
    {
        var lo = new MidiEvent
        {
            Tick = 0, Type = MidiMessageType.PitchBend,
            Channel = 0, Data1 = 0, Data2 = 0, // raw 0
        };
        var hi = new MidiEvent
        {
            Tick = 0, Type = MidiMessageType.PitchBend,
            Channel = 0, Data1 = 0x7F, Data2 = 0x7F, // raw 16383
        };
        Assert.Equal(-0x2000, lo.PitchBend14);
        Assert.Equal(0x1FFF, hi.PitchBend14);
    }

    [Fact]
    public void Tracks_Read_Back_With_Preserved_Tick_Ordering()
    {
        // Generate 50 events with increasing tick values and ensure the
        // round-tripped track preserves the order.
        var events = new List<MidiEvent>();
        long t = 0;
        for (int i = 0; i < 50; i++)
        {
            t += 7;
            events.Add(new MidiEvent
            {
                Tick = t, Type = MidiMessageType.NoteOn,
                Channel = 0, Data1 = (byte)(60 + (i % 12)), Data2 = 100,
            });
        }
        var track = new MidiTrack { Events = events };
        var file = new MidiFile { Format = 0, Division = MidiDivision.Ppq(96), Tracks = [track] };
        var rt = MidiReader.Read(MidiWriter.Write(file));
        long lastTick = -1;
        for (int i = 0; i < 50; i++)
        {
            Assert.True(rt.Tracks[0].Events[i].Tick >= lastTick);
            lastTick = rt.Tracks[0].Events[i].Tick;
        }
    }

    [Fact]
    public void ReadFile_Reads_Same_Bytes_As_Read_Span()
    {
        var file = new MidiFile
        {
            Format = 0, Division = MidiDivision.Ppq(96),
            Tracks = [new MidiTrack { Events = [] }],
        };
        byte[] bytes = MidiWriter.Write(file);
        string path = Path.Combine(Path.GetTempPath(),
            "mediar-midi-" + Guid.NewGuid().ToString("N") + ".mid");
        try
        {
            File.WriteAllBytes(path, bytes);
            var fromPath = MidiReader.ReadFile(path);
            var fromSpan = MidiReader.Read(bytes);
            Assert.Equal(fromPath.Format, fromSpan.Format);
            Assert.Equal(fromPath.Tracks.Count, fromSpan.Tracks.Count);
        }
        finally
        {
            if (File.Exists(path)) File.Delete(path);
        }
    }
}
