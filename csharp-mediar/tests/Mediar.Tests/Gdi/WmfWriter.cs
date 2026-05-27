using System.Buffers.Binary;

namespace Mediar.Tests.Gdi;

/// <summary>
/// Tiny WMF byte-stream synthesiser for the GDI tests. Writes META_HEADER,
/// a handful of records using the legacy 16-bit Windows GDI ordering, then
/// META_EOF. Optionally prepends an Aldus Placeable preamble.
/// </summary>
internal sealed class WmfWriter : IDisposable
{
    private readonly MemoryStream _body = new();
    private readonly bool _placeable;
    private readonly short _l, _t, _r, _b;
    private readonly ushort _inch;

    public WmfWriter(bool placeable = false, short l = 0, short t = 0, short r = 100, short b = 100, ushort inch = 96)
    {
        _placeable = placeable;
        _l = l; _t = t; _r = r; _b = b; _inch = inch;
    }

    public void Dispose() => _body.Dispose();

    public byte[] Build()
    {
        using var ms = new MemoryStream();
        if (_placeable)
        {
            Span<byte> ph = stackalloc byte[22];
            BinaryPrimitives.WriteUInt32LittleEndian(ph[..4], 0x9AC6CDD7);
            BinaryPrimitives.WriteUInt16LittleEndian(ph.Slice(4, 2), 0); // HWmf
            BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(6, 2), _l);
            BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(8, 2), _t);
            BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(10, 2), _r);
            BinaryPrimitives.WriteInt16LittleEndian(ph.Slice(12, 2), _b);
            BinaryPrimitives.WriteUInt16LittleEndian(ph.Slice(14, 2), _inch);
            // Reserved (4) + Checksum (2) = 6 zero bytes already.
            ms.Write(ph);
        }
        // META_HEADER (18 bytes).
        Span<byte> hdr = stackalloc byte[18];
        BinaryPrimitives.WriteUInt16LittleEndian(hdr[..2], 1); // memory metafile
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(2, 2), 9); // header size in words
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(4, 2), 0x0300); // version
        ms.Write(hdr);
        ms.Write(_body.GetBuffer().AsSpan(0, (int)_body.Length));
        // META_EOF (3 words: size + function).
        Span<byte> eof = stackalloc byte[6];
        BinaryPrimitives.WriteUInt32LittleEndian(eof[..4], 3);
        BinaryPrimitives.WriteUInt16LittleEndian(eof.Slice(4, 2), 0);
        ms.Write(eof);
        return ms.ToArray();
    }

    /// <summary>META_SETWINDOWEXT — Y first, then X per WMF spec.</summary>
    public void SetWindowExt(short cx, short cy)
    {
        Span<byte> args = stackalloc byte[4];
        BinaryPrimitives.WriteInt16LittleEndian(args[..2], cy);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2, 2), cx);
        WriteRecord(0x020C, args);
    }

    /// <summary>META_CREATEBRUSHINDIRECT. Returns the assigned slot index.</summary>
    public ushort CreateSolidBrush(byte r, byte g, byte b, ref ushort nextSlot)
    {
        Span<byte> args = stackalloc byte[8];
        BinaryPrimitives.WriteUInt16LittleEndian(args[..2], 0); // BS_SOLID
        BinaryPrimitives.WriteUInt32LittleEndian(args.Slice(2, 4), (uint)r | ((uint)g << 8) | ((uint)b << 16));
        // hatch (2 bytes) = 0
        WriteRecord(0x02FC, args);
        return nextSlot++;
    }

    /// <summary>META_CREATEPENINDIRECT.</summary>
    public ushort CreatePen(ushort style, short width, byte r, byte g, byte b, ref ushort nextSlot)
    {
        Span<byte> args = stackalloc byte[10];
        BinaryPrimitives.WriteUInt16LittleEndian(args[..2], style);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2, 2), width); // X
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(4, 2), 0);     // Y
        BinaryPrimitives.WriteUInt32LittleEndian(args.Slice(6, 4), (uint)r | ((uint)g << 8) | ((uint)b << 16));
        WriteRecord(0x02FA, args);
        return nextSlot++;
    }

    /// <summary>META_SELECTOBJECT.</summary>
    public void SelectObject(ushort slot)
    {
        Span<byte> args = stackalloc byte[2];
        BinaryPrimitives.WriteUInt16LittleEndian(args, slot);
        WriteRecord(0x012D, args);
    }

    /// <summary>META_RECTANGLE — order is bottom, right, top, left.</summary>
    public void Rectangle(short l, short t, short r, short b)
    {
        Span<byte> args = stackalloc byte[8];
        BinaryPrimitives.WriteInt16LittleEndian(args[..2], b);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2, 2), r);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(4, 2), t);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(6, 2), l);
        WriteRecord(0x041B, args);
    }

    /// <summary>META_POLYGON — count + N (X,Y) pairs (the impl reads X first).</summary>
    public void Polygon(IReadOnlyList<(short X, short Y)> points)
    {
        int len = 2 + points.Count * 4;
        Span<byte> args = stackalloc byte[len];
        BinaryPrimitives.WriteUInt16LittleEndian(args[..2], (ushort)points.Count);
        for (int i = 0; i < points.Count; i++)
        {
            BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2 + i * 4, 2), points[i].X);
            BinaryPrimitives.WriteInt16LittleEndian(args.Slice(4 + i * 4, 2), points[i].Y);
        }
        WriteRecord(0x0324, args);
    }

    /// <summary>META_MOVETO — Y first then X.</summary>
    public void MoveTo(short x, short y)
    {
        Span<byte> args = stackalloc byte[4];
        BinaryPrimitives.WriteInt16LittleEndian(args[..2], y);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2, 2), x);
        WriteRecord(0x0214, args);
    }

    /// <summary>META_LINETO — Y first then X.</summary>
    public void LineTo(short x, short y)
    {
        Span<byte> args = stackalloc byte[4];
        BinaryPrimitives.WriteInt16LittleEndian(args[..2], y);
        BinaryPrimitives.WriteInt16LittleEndian(args.Slice(2, 2), x);
        WriteRecord(0x0213, args);
    }

    /// <summary>Append a raw record (no args validation).</summary>
    public void RawRecord(ushort func, ReadOnlySpan<byte> args) => WriteRecord(func, args);

    private void WriteRecord(ushort func, ReadOnlySpan<byte> args)
    {
        // Size in words = (4 (size field) + 2 (func) + args.Length) / 2.
        uint sizeWords = (uint)((6 + args.Length) / 2);
        Span<byte> hdr = stackalloc byte[6];
        BinaryPrimitives.WriteUInt32LittleEndian(hdr[..4], sizeWords);
        BinaryPrimitives.WriteUInt16LittleEndian(hdr.Slice(4, 2), func);
        _body.Write(hdr);
        if (args.Length > 0) _body.Write(args);
    }
}
