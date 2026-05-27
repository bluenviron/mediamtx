using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Cr3;

/// <summary>
/// Test-only synthesiser for CR3 byte streams. Lays out the standard
/// ISO-BMFF box tree with an <c>ftyp</c> (brand "crx "), a <c>moov</c>
/// containing the Canon UUID 85c0b687-820f-11e0-8111-f4ce462b6a48 with
/// CMT1 (TIFF IFD) and THMB (JPEG thumbnail) sub-boxes, plus an
/// optional <c>uuid</c> box with the PRVW UUID
/// eaf42b5e-1c98-4b88-b9fb-b7dc406e4d16 containing the PRVW JPEG.
/// </summary>
internal static class TestCr3Builder
{
    private static readonly byte[] s_canonUuid =
    [
        0x85, 0xC0, 0xB6, 0x87, 0x82, 0x0F, 0x11, 0xE0,
        0x81, 0x11, 0xF4, 0xCE, 0x46, 0x2B, 0x6A, 0x48,
    ];

    private static readonly byte[] s_prvwUuid =
    [
        0xEA, 0xF4, 0x2B, 0x5E, 0x1C, 0x98, 0x4B, 0x88,
        0xB9, 0xFB, 0xB7, 0xDC, 0x40, 0x6E, 0x4D, 0x16,
    ];

    public sealed record Cr3Spec
    {
        public string Brand { get; init; } = "crx ";
        public uint MinorVersion { get; init; } = 1;
        public IReadOnlyList<string> CompatibleBrands { get; init; } = ["crx ", "isom"];
        public string? Make { get; init; }
        public string? Model { get; init; }
        public string? Software { get; init; }
        public string? DateTime { get; init; }
        public string? Artist { get; init; }
        public string? Copyright { get; init; }
        public byte[]? ThmbJpeg { get; init; }
        public int ThmbDeclaredWidth { get; init; } = 160;
        public int ThmbDeclaredHeight { get; init; } = 120;
        public byte[]? PrvwJpeg { get; init; }
        public int PrvwDeclaredWidth { get; init; } = 1920;
        public int PrvwDeclaredHeight { get; init; } = 1280;
        public byte[]? MdatPayload { get; init; }

        // CMT2 (EXIF sub-IFD) fields. When any non-null, a CMT2 box is emitted.
        public (uint Num, uint Den)? ExposureTime { get; init; }
        public (uint Num, uint Den)? FNumber { get; init; }
        public ushort? IsoSpeedRatings { get; init; }
        public string? DateTimeOriginal { get; init; }
        public string? DateTimeDigitized { get; init; }
        public (int Num, int Den)? ExposureBiasValue { get; init; }
        public (uint Num, uint Den)? FocalLength { get; init; }
        public string? LensModel { get; init; }
        public string? LensMake { get; init; }
        public ushort? Flash { get; init; }
        public ushort? MeteringMode { get; init; }
        public ushort? ExposureProgram { get; init; }
        public ushort? WhiteBalance { get; init; }

        // CMT3 (Canon MakerNote) raw payload. When non-null, a CMT3 box is emitted.
        public byte[]? Cmt3RawPayload { get; init; }

        // CMT4 (GPS IFD) fields. When any non-null, a CMT4 box is emitted.
        public string? GpsLatitudeRef { get; init; }
        public (uint, uint, uint, uint, uint, uint)? GpsLatitudeDms { get; init; }
        public string? GpsLongitudeRef { get; init; }
        public (uint, uint, uint, uint, uint, uint)? GpsLongitudeDms { get; init; }
        public byte? GpsAltitudeRef { get; init; }
        public (uint Num, uint Den)? GpsAltitude { get; init; }
        public (uint, uint, uint, uint, uint, uint)? GpsTimeStampHms { get; init; }
        public string? GpsDateStamp { get; init; }
    }

    public static byte[] Build(Cr3Spec spec)
    {
        using var ms = new MemoryStream();

        WriteBox(ms, "ftyp", w =>
        {
            byte[] brand = Encoding.ASCII.GetBytes(spec.Brand);
            if (brand.Length != 4) throw new ArgumentException("ftyp brand must be 4 chars.");
            w.Write(brand);
            Span<byte> minor = stackalloc byte[4];
            BinaryPrimitives.WriteUInt32BigEndian(minor, spec.MinorVersion);
            w.Write(minor);
            foreach (var cb in spec.CompatibleBrands)
            {
                byte[] b = Encoding.ASCII.GetBytes(cb);
                if (b.Length != 4) throw new ArgumentException("Compatible brand must be 4 chars.");
                w.Write(b);
            }
        });

        WriteBox(ms, "moov", moov =>
        {
            WriteBox(moov, "uuid", uuid =>
            {
                uuid.Write(s_canonUuid);
                // CMT1: TIFF IFD with the EXIF strings.
                if (spec.Make is not null || spec.Model is not null || spec.Software is not null
                    || spec.DateTime is not null || spec.Artist is not null || spec.Copyright is not null)
                {
                    WriteBox(uuid, "CMT1", cmt1 =>
                    {
                        WriteTiffIfd(cmt1, spec);
                    });
                }
                if (spec.ThmbJpeg is not null)
                {
                    WriteBox(uuid, "THMB", thmb =>
                    {
                        Span<byte> prelude = stackalloc byte[12];
                        // 0-3: version + flags = 0
                        BinaryPrimitives.WriteUInt16BigEndian(prelude.Slice(4, 2), (ushort)spec.ThmbDeclaredWidth);
                        BinaryPrimitives.WriteUInt16BigEndian(prelude.Slice(6, 2), (ushort)spec.ThmbDeclaredHeight);
                        BinaryPrimitives.WriteUInt32BigEndian(prelude.Slice(8, 4), (uint)spec.ThmbJpeg.Length);
                        thmb.Write(prelude);
                        thmb.Write(spec.ThmbJpeg);
                    });
                }
                if (HasAnyCmt2Field(spec))
                {
                    WriteBox(uuid, "CMT2", cmt2 => WriteCmt2Ifd(cmt2, spec));
                }
                if (spec.Cmt3RawPayload is not null)
                {
                    WriteBox(uuid, "CMT3", cmt3 => cmt3.Write(spec.Cmt3RawPayload));
                }
                if (HasAnyCmt4Field(spec))
                {
                    WriteBox(uuid, "CMT4", cmt4 => WriteCmt4Ifd(cmt4, spec));
                }
            });
        });

        if (spec.PrvwJpeg is not null)
        {
            WriteBox(ms, "uuid", uuid =>
            {
                uuid.Write(s_prvwUuid);
                WriteBox(uuid, "PRVW", prvw =>
                {
                    Span<byte> prelude = stackalloc byte[16];
                    // 0-3: version + flags
                    // 4-5: reserved
                    BinaryPrimitives.WriteUInt16BigEndian(prelude.Slice(6, 2), (ushort)spec.PrvwDeclaredWidth);
                    BinaryPrimitives.WriteUInt16BigEndian(prelude.Slice(8, 2), (ushort)spec.PrvwDeclaredHeight);
                    // 10-11: reserved
                    BinaryPrimitives.WriteUInt32BigEndian(prelude.Slice(12, 4), (uint)spec.PrvwJpeg.Length);
                    prvw.Write(prelude);
                    prvw.Write(spec.PrvwJpeg);
                });
            });
        }

        if (spec.MdatPayload is not null)
        {
            WriteBox(ms, "mdat", mdat => mdat.Write(spec.MdatPayload));
        }

        return ms.ToArray();
    }

    private static void WriteTiffIfd(MemoryStream cmt1, Cr3Spec spec)
    {
        var asciiTags = new List<(ushort Tag, string Value)>();
        if (spec.Make is not null) asciiTags.Add((0x010F, spec.Make));
        if (spec.Model is not null) asciiTags.Add((0x0110, spec.Model));
        if (spec.Software is not null) asciiTags.Add((0x0131, spec.Software));
        if (spec.DateTime is not null) asciiTags.Add((0x0132, spec.DateTime));
        if (spec.Artist is not null) asciiTags.Add((0x013B, spec.Artist));
        if (spec.Copyright is not null) asciiTags.Add((0x8298, spec.Copyright));

        // II, magic 42, IFD0 offset (always 8).
        Span<byte> header = stackalloc byte[8];
        header[0] = 0x49; header[1] = 0x49;
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(2, 2), 42);
        BinaryPrimitives.WriteUInt32LittleEndian(header.Slice(4, 4), 8u);
        cmt1.Write(header);

        // Compute data pool offsets relative to CMT1 start.
        // IFD layout: u16 count + 12*N entries + u32 next-IFD offset.
        int ifdSize = 2 + asciiTags.Count * 12 + 4;
        int poolStart = 8 + ifdSize;
        var pool = new List<byte>();
        var entryDataAt = new int[asciiTags.Count];
        for (int i = 0; i < asciiTags.Count; i++)
        {
            string val = asciiTags[i].Value;
            byte[] bytes = Encoding.ASCII.GetBytes(val + "\0");
            if (bytes.Length <= 4)
            {
                entryDataAt[i] = -1; // inline
            }
            else
            {
                entryDataAt[i] = poolStart + pool.Count;
                pool.AddRange(bytes);
            }
        }

        Span<byte> ifd = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(ifd, (ushort)asciiTags.Count);
        cmt1.Write(ifd);

        Span<byte> entry = stackalloc byte[12];
        for (int i = 0; i < asciiTags.Count; i++)
        {
            entry.Clear();
            BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(0, 2), asciiTags[i].Tag);
            BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), 2); // ASCII
            byte[] valBytes = Encoding.ASCII.GetBytes(asciiTags[i].Value + "\0");
            BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), (uint)valBytes.Length);
            if (entryDataAt[i] < 0)
            {
                // inline
                valBytes.AsSpan().CopyTo(entry.Slice(8, valBytes.Length));
            }
            else
            {
                BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), (uint)entryDataAt[i]);
            }
            cmt1.Write(entry);
        }

        // Next-IFD offset (0 = no next).
        Span<byte> next = stackalloc byte[4];
        cmt1.Write(next);

        cmt1.Write(pool.ToArray());
    }

    private static bool HasAnyCmt2Field(Cr3Spec s) =>
        s.ExposureTime is not null || s.FNumber is not null
        || s.IsoSpeedRatings is not null
        || s.DateTimeOriginal is not null || s.DateTimeDigitized is not null
        || s.ExposureBiasValue is not null || s.FocalLength is not null
        || s.LensModel is not null || s.LensMake is not null
        || s.Flash is not null || s.MeteringMode is not null
        || s.ExposureProgram is not null || s.WhiteBalance is not null;

    private static bool HasAnyCmt4Field(Cr3Spec s) =>
        s.GpsLatitudeRef is not null || s.GpsLatitudeDms is not null
        || s.GpsLongitudeRef is not null || s.GpsLongitudeDms is not null
        || s.GpsAltitudeRef is not null || s.GpsAltitude is not null
        || s.GpsTimeStampHms is not null || s.GpsDateStamp is not null;

    private static void WriteCmt2Ifd(MemoryStream stream, Cr3Spec spec)
    {
        var tags = new List<TiffTag>();
        if (spec.ExposureTime is { } et) tags.Add(TiffTag.Rational(0x829A, et.Num, et.Den));
        if (spec.FNumber is { } fn) tags.Add(TiffTag.Rational(0x829D, fn.Num, fn.Den));
        if (spec.ExposureProgram is { } ep) tags.Add(TiffTag.Short(0x8822, ep));
        if (spec.IsoSpeedRatings is { } iso) tags.Add(TiffTag.Short(0x8827, iso));
        if (spec.DateTimeOriginal is { } dto) tags.Add(TiffTag.Ascii(0x9003, dto));
        if (spec.DateTimeDigitized is { } dtd) tags.Add(TiffTag.Ascii(0x9004, dtd));
        if (spec.ExposureBiasValue is { } ev) tags.Add(TiffTag.SRational(0x9204, ev.Num, ev.Den));
        if (spec.MeteringMode is { } mm) tags.Add(TiffTag.Short(0x9207, mm));
        if (spec.Flash is { } fl) tags.Add(TiffTag.Short(0x9209, fl));
        if (spec.FocalLength is { } focal) tags.Add(TiffTag.Rational(0x920A, focal.Num, focal.Den));
        if (spec.WhiteBalance is { } wb) tags.Add(TiffTag.Short(0xA403, wb));
        if (spec.LensMake is { } lk) tags.Add(TiffTag.Ascii(0xA433, lk));
        if (spec.LensModel is { } lm) tags.Add(TiffTag.Ascii(0xA434, lm));

        // Tags must be in ascending order per TIFF 6.0 spec.
        tags.Sort((a, b) => a.Tag.CompareTo(b.Tag));
        WriteTiffIfdGeneric(stream, tags);
    }

    private static void WriteCmt4Ifd(MemoryStream stream, Cr3Spec spec)
    {
        var tags = new List<TiffTag>();
        if (spec.GpsLatitudeRef is { } lr) tags.Add(TiffTag.Ascii(0x0001, lr));
        if (spec.GpsLatitudeDms is { } latDms)
            tags.Add(TiffTag.RationalArray(0x0002,
                [latDms.Item1, latDms.Item3, latDms.Item5], [latDms.Item2, latDms.Item4, latDms.Item6]));
        if (spec.GpsLongitudeRef is { } lor) tags.Add(TiffTag.Ascii(0x0003, lor));
        if (spec.GpsLongitudeDms is { } lonDms)
            tags.Add(TiffTag.RationalArray(0x0004,
                [lonDms.Item1, lonDms.Item3, lonDms.Item5], [lonDms.Item2, lonDms.Item4, lonDms.Item6]));
        if (spec.GpsAltitudeRef is { } altRef) tags.Add(TiffTag.Byte(0x0005, altRef));
        if (spec.GpsAltitude is { } alt) tags.Add(TiffTag.Rational(0x0006, alt.Num, alt.Den));
        if (spec.GpsTimeStampHms is { } hms)
            tags.Add(TiffTag.RationalArray(0x0007,
                [hms.Item1, hms.Item3, hms.Item5], [hms.Item2, hms.Item4, hms.Item6]));
        if (spec.GpsDateStamp is { } ds) tags.Add(TiffTag.Ascii(0x001D, ds));

        tags.Sort((a, b) => a.Tag.CompareTo(b.Tag));
        WriteTiffIfdGeneric(stream, tags);
    }

    private readonly record struct TiffTag(ushort Tag, ushort Type, uint Count, byte[] Bytes)
    {
        public static TiffTag Ascii(ushort tag, string value)
        {
            byte[] bytes = Encoding.ASCII.GetBytes(value + "\0");
            return new TiffTag(tag, 2, (uint)bytes.Length, bytes);
        }
        public static TiffTag Byte(ushort tag, byte value) => new(tag, 1, 1, [value]);
        public static TiffTag Short(ushort tag, ushort value)
        {
            byte[] bytes = new byte[2];
            BinaryPrimitives.WriteUInt16LittleEndian(bytes, value);
            return new TiffTag(tag, 3, 1, bytes);
        }
        public static TiffTag Rational(ushort tag, uint num, uint den)
        {
            byte[] bytes = new byte[8];
            BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(0, 4), num);
            BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(4, 4), den);
            return new TiffTag(tag, 5, 1, bytes);
        }
        public static TiffTag SRational(ushort tag, int num, int den)
        {
            byte[] bytes = new byte[8];
            BinaryPrimitives.WriteInt32LittleEndian(bytes.AsSpan(0, 4), num);
            BinaryPrimitives.WriteInt32LittleEndian(bytes.AsSpan(4, 4), den);
            return new TiffTag(tag, 10, 1, bytes);
        }
        public static TiffTag RationalArray(ushort tag, uint[] nums, uint[] dens)
        {
            if (nums.Length != dens.Length) throw new ArgumentException("nums.Length != dens.Length");
            byte[] bytes = new byte[8 * nums.Length];
            for (int i = 0; i < nums.Length; i++)
            {
                BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(i * 8, 4), nums[i]);
                BinaryPrimitives.WriteUInt32LittleEndian(bytes.AsSpan(i * 8 + 4, 4), dens[i]);
            }
            return new TiffTag(tag, 5, (uint)nums.Length, bytes);
        }
    }

    private static void WriteTiffIfdGeneric(MemoryStream stream, List<TiffTag> tags)
    {
        // II, magic 42, IFD0 offset = 8.
        Span<byte> header = stackalloc byte[8];
        header[0] = 0x49; header[1] = 0x49;
        BinaryPrimitives.WriteUInt16LittleEndian(header.Slice(2, 2), 42);
        BinaryPrimitives.WriteUInt32LittleEndian(header.Slice(4, 4), 8u);
        stream.Write(header);

        int ifdSize = 2 + tags.Count * 12 + 4;
        int poolStart = 8 + ifdSize;
        var pool = new List<byte>();
        var entryDataAt = new int[tags.Count];
        for (int i = 0; i < tags.Count; i++)
        {
            if (tags[i].Bytes.Length <= 4)
            {
                entryDataAt[i] = -1;
            }
            else
            {
                entryDataAt[i] = poolStart + pool.Count;
                pool.AddRange(tags[i].Bytes);
            }
        }

        Span<byte> count = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(count, (ushort)tags.Count);
        stream.Write(count);

        Span<byte> entry = stackalloc byte[12];
        for (int i = 0; i < tags.Count; i++)
        {
            entry.Clear();
            BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(0, 2), tags[i].Tag);
            BinaryPrimitives.WriteUInt16LittleEndian(entry.Slice(2, 2), tags[i].Type);
            BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(4, 4), tags[i].Count);
            if (entryDataAt[i] < 0)
            {
                tags[i].Bytes.AsSpan().CopyTo(entry.Slice(8, tags[i].Bytes.Length));
            }
            else
            {
                BinaryPrimitives.WriteUInt32LittleEndian(entry.Slice(8, 4), (uint)entryDataAt[i]);
            }
            stream.Write(entry);
        }

        Span<byte> next = stackalloc byte[4];
        stream.Write(next);

        stream.Write(pool.ToArray());
    }

    private static void WriteBox(Stream s, string type, Action<MemoryStream> writePayload)
    {
        using var inner = new MemoryStream();
        writePayload(inner);
        byte[] payload = inner.ToArray();
        int total = payload.Length + 8;
        Span<byte> hdr = stackalloc byte[8];
        BinaryPrimitives.WriteUInt32BigEndian(hdr.Slice(0, 4), (uint)total);
        Encoding.ASCII.GetBytes(type).CopyTo(hdr.Slice(4, 4));
        s.Write(hdr);
        s.Write(payload);
    }
}
