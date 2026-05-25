namespace Mediar.Codecs.Vorbis.Decoder;

/// <summary>
/// Parsed Vorbis setup header (Vorbis I §4.2.4). Captures the bookkeeping
/// needed for audio decode: codebooks, floor configurations, residue
/// configurations, mapping descriptors, and mode descriptors.
///
/// Floor and residue tables are parsed as raw configuration so the decoder
/// can dispatch them to the appropriate synthesis routine. Floor 0 (LSP) is
/// captured but rarely used in practice; the synthesis path supports floor 1.
/// </summary>
internal sealed class VorbisSetup
{
    public VorbisCodebook[] Codebooks { get; init; } = Array.Empty<VorbisCodebook>();
    public Floor[] Floors { get; init; } = Array.Empty<Floor>();
    public Residue[] Residues { get; init; } = Array.Empty<Residue>();
    public Mapping[] Mappings { get; init; } = Array.Empty<Mapping>();
    public Mode[] Modes { get; init; } = Array.Empty<Mode>();

    internal sealed class Floor
    {
        public int Type { get; init; }
        // Floor 1 fields
        public int[] PartitionClassList { get; init; } = Array.Empty<int>();
        public int Multiplier { get; init; }
        public int RangeBits { get; init; }
        public int[] XList { get; init; } = Array.Empty<int>();
        public ClassConfig[] Classes { get; init; } = Array.Empty<ClassConfig>();

        internal sealed class ClassConfig
        {
            public int Dimensions { get; init; }
            public int SubclassBits { get; init; }
            public int MasterBook { get; init; }
            public int[] SubclassBooks { get; init; } = Array.Empty<int>();
        }
    }

    internal sealed class Residue
    {
        public int Type { get; init; }
        public int Begin { get; init; }
        public int End { get; init; }
        public int PartitionSize { get; init; }
        public int Classifications { get; init; }
        public int ClassBook { get; init; }
        public int[] Cascade { get; init; } = Array.Empty<int>();
        public int[,] Books { get; init; } = new int[0, 0];
    }

    internal sealed class Mapping
    {
        public int[] SubmapFloor { get; init; } = Array.Empty<int>();
        public int[] SubmapResidue { get; init; } = Array.Empty<int>();
        public int[] ChannelMux { get; init; } = Array.Empty<int>();
        public CouplingStep[] CouplingSteps { get; init; } = Array.Empty<CouplingStep>();

        internal sealed record CouplingStep(int MagnitudeChannel, int AngleChannel);
    }

    internal sealed record Mode(bool BlockFlag, int WindowType, int TransformType, int Mapping);

    public static VorbisSetup Parse(ReadOnlySpan<byte> packet, int channels)
    {
        if (packet.Length < 7) throw new InvalidDataException("Setup header too short.");
        if (packet[0] != 5) throw new InvalidDataException("Expected packet type 5 (setup).");
        if (!packet.Slice(1, 6).SequenceEqual("vorbis"u8))
            throw new InvalidDataException("Bad Vorbis setup magic.");

        var r = new VorbisBitReader(packet[7..]);

        // 4.2.4.1 — codebooks
        int codebookCount = (int)r.ReadBits(8) + 1;
        var codebooks = new VorbisCodebook[codebookCount];
        for (int i = 0; i < codebookCount; i++)
        {
            codebooks[i] = VorbisCodebook.Parse(ref r);
        }

        // 4.2.4.2 — time domain transforms (all reserved 0)
        int timeCount = (int)r.ReadBits(6) + 1;
        for (int i = 0; i < timeCount; i++)
        {
            uint placeholder = r.ReadBits(16);
            if (placeholder != 0) throw new InvalidDataException("Reserved time-domain-transform field non-zero.");
        }

        // 4.2.4.3 — floors
        int floorCount = (int)r.ReadBits(6) + 1;
        var floors = new Floor[floorCount];
        for (int i = 0; i < floorCount; i++)
        {
            int type = (int)r.ReadBits(16);
            if (type == 0)
            {
                // Floor 0 — parse and skip fields without retaining them.
                _ = r.ReadBits(8);   // order
                _ = r.ReadBits(16);  // rate
                _ = r.ReadBits(16);  // bark map size
                _ = r.ReadBits(6);   // amplitude bits
                _ = r.ReadBits(8);   // amplitude offset
                int numBooks = (int)r.ReadBits(4) + 1;
                for (int b = 0; b < numBooks; b++) _ = r.ReadBits(8);
                floors[i] = new Floor { Type = 0 };
                continue;
            }
            if (type != 1) throw new InvalidDataException($"Unknown floor type {type}.");

            int partitions = (int)r.ReadBits(5);
            var partitionClassList = new int[partitions];
            int maxClass = -1;
            for (int p = 0; p < partitions; p++)
            {
                partitionClassList[p] = (int)r.ReadBits(4);
                if (partitionClassList[p] > maxClass) maxClass = partitionClassList[p];
            }
            var classes = new Floor.ClassConfig[maxClass + 1];
            for (int c = 0; c <= maxClass; c++)
            {
                int dims = (int)r.ReadBits(3) + 1;
                int subclassBits = (int)r.ReadBits(2);
                int masterBook = subclassBits > 0 ? (int)r.ReadBits(8) : 0;
                int subN = 1 << subclassBits;
                var subBooks = new int[subN];
                for (int j = 0; j < subN; j++) subBooks[j] = (int)r.ReadBits(8) - 1;
                classes[c] = new Floor.ClassConfig
                {
                    Dimensions = dims,
                    SubclassBits = subclassBits,
                    MasterBook = masterBook,
                    SubclassBooks = subBooks,
                };
            }
            int multiplier = (int)r.ReadBits(2) + 1;
            int rangeBits = (int)r.ReadBits(4);
            var xList = new List<int> { 0, 1 << rangeBits };
            for (int p = 0; p < partitions; p++)
            {
                var cls = classes[partitionClassList[p]];
                int n = cls.Dimensions;
                for (int j = 0; j < n; j++) xList.Add((int)r.ReadBits(rangeBits));
            }
            floors[i] = new Floor
            {
                Type = 1,
                PartitionClassList = partitionClassList,
                Multiplier = multiplier,
                RangeBits = rangeBits,
                XList = xList.ToArray(),
                Classes = classes,
            };
        }

        // 4.2.4.4 — residues
        int residueCount = (int)r.ReadBits(6) + 1;
        var residues = new Residue[residueCount];
        for (int i = 0; i < residueCount; i++)
        {
            int type = (int)r.ReadBits(16);
            if (type < 0 || type > 2)
                throw new InvalidDataException($"Unknown residue type {type}.");
            int begin = (int)r.ReadBits(24);
            int end = (int)r.ReadBits(24);
            int partitionSize = (int)r.ReadBits(24) + 1;
            int classifications = (int)r.ReadBits(6) + 1;
            int classBook = (int)r.ReadBits(8);
            var cascade = new int[classifications];
            for (int c = 0; c < classifications; c++)
            {
                int low = (int)r.ReadBits(3);
                bool bigBit = r.ReadBit();
                int high = bigBit ? (int)r.ReadBits(5) : 0;
                cascade[c] = (high << 3) | low;
            }
            var books = new int[classifications, 8];
            for (int c = 0; c < classifications; c++)
            {
                for (int b = 0; b < 8; b++)
                {
                    if ((cascade[c] & (1 << b)) != 0) books[c, b] = (int)r.ReadBits(8);
                    else books[c, b] = -1;
                }
            }
            residues[i] = new Residue
            {
                Type = type,
                Begin = begin,
                End = end,
                PartitionSize = partitionSize,
                Classifications = classifications,
                ClassBook = classBook,
                Cascade = cascade,
                Books = books,
            };
        }

        // 4.2.4.5 — mappings
        int mappingCount = (int)r.ReadBits(6) + 1;
        var mappings = new Mapping[mappingCount];
        for (int i = 0; i < mappingCount; i++)
        {
            int mappingType = (int)r.ReadBits(16);
            if (mappingType != 0) throw new InvalidDataException($"Unknown mapping type {mappingType}.");

            int submaps = r.ReadBit() ? (int)r.ReadBits(4) + 1 : 1;
            Mapping.CouplingStep[] couplings;
            if (r.ReadBit())
            {
                int couplingSteps = (int)r.ReadBits(8) + 1;
                couplings = new Mapping.CouplingStep[couplingSteps];
                int chBits = VorbisBitReader.Ilog(channels - 1);
                for (int c = 0; c < couplingSteps; c++)
                {
                    int mag = (int)r.ReadBits(chBits);
                    int ang = (int)r.ReadBits(chBits);
                    if (mag == ang || mag >= channels || ang >= channels)
                        throw new InvalidDataException("Invalid mapping coupling step.");
                    couplings[c] = new Mapping.CouplingStep(mag, ang);
                }
            }
            else couplings = Array.Empty<Mapping.CouplingStep>();

            uint reserved = r.ReadBits(2);
            if (reserved != 0) throw new InvalidDataException("Reserved mapping field non-zero.");

            int[] mux;
            if (submaps > 1)
            {
                mux = new int[channels];
                for (int c = 0; c < channels; c++) mux[c] = (int)r.ReadBits(4);
            }
            else mux = new int[channels];

            var submapFloor = new int[submaps];
            var submapResidue = new int[submaps];
            for (int s = 0; s < submaps; s++)
            {
                _ = r.ReadBits(8); // unused submap "time configuration"
                submapFloor[s] = (int)r.ReadBits(8);
                submapResidue[s] = (int)r.ReadBits(8);
            }
            mappings[i] = new Mapping
            {
                ChannelMux = mux,
                SubmapFloor = submapFloor,
                SubmapResidue = submapResidue,
                CouplingSteps = couplings,
            };
        }

        // 4.2.4.6 — modes
        int modeCount = (int)r.ReadBits(6) + 1;
        var modes = new Mode[modeCount];
        for (int i = 0; i < modeCount; i++)
        {
            bool blockFlag = r.ReadBit();
            int windowType = (int)r.ReadBits(16);
            int transformType = (int)r.ReadBits(16);
            int mapping = (int)r.ReadBits(8);
            if (windowType != 0 || transformType != 0)
                throw new InvalidDataException("Unsupported mode window/transform.");
            modes[i] = new Mode(blockFlag, windowType, transformType, mapping);
        }

        bool framing = r.ReadBit();
        if (!framing) throw new InvalidDataException("Setup header framing bit missing.");

        return new VorbisSetup
        {
            Codebooks = codebooks,
            Floors = floors,
            Residues = residues,
            Mappings = mappings,
            Modes = modes,
        };
    }
}
