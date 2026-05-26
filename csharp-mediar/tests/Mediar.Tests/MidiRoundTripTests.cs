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
}
