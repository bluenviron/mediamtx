using System.Buffers.Binary;

namespace Mediar.Tests.Gdi;

/// <summary>
/// Tiny EMF byte-stream synthesiser for the GDI tests. Writes just enough of
/// the MS-EMF spec to feed <c>EmfPlayer</c>: the mandatory EMR_HEADER, a
/// handful of object table + draw records, then EMR_EOF.
/// </summary>
internal sealed class EmfWriter : IDisposable
{
    private readonly MemoryStream _ms = new();
    private uint _nextHandle = 1;
    private int _recordCount;

    public void Dispose() => _ms.Dispose();

    public EmfWriter(int left, int top, int right, int bottom)
    {
        Span<byte> hdr = stackalloc byte[88];
        BinaryPrimitives.WriteUInt32LittleEndian(hdr[..4], 1);   // EMR_HEADER
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(4, 4), 88);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(8, 4), left);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(12, 4), top);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(16, 4), right);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(20, 4), bottom);
        // Frame (0.01mm) - same as bounds for tests.
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(24, 4), left);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(28, 4), top);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(32, 4), right);
        BinaryPrimitives.WriteInt32LittleEndian(hdr.Slice(36, 4), bottom);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(40, 4), 0x464D4520);  // " EMF" signature
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(44, 4), 0x00010000);  // version
        _ms.Write(hdr);
        _recordCount = 1;
    }

    /// <summary>Emit EMR_EOF and return this writer for fluent use.</summary>
    public EmfWriter Eof()
    {
        Span<byte> eof = stackalloc byte[20];
        BinaryPrimitives.WriteUInt32LittleEndian(eof[..4], 14);   // EMR_EOF
        BinaryPrimitives.WriteUInt32LittleEndian(eof.Slice(4, 4), 20);
        _ms.Write(eof);
        _recordCount++;
        return this;
    }

    public byte[] Build() => _ms.ToArray();

    public int RecordCount => _recordCount;

    /// <summary>EMR_CREATEBRUSHINDIRECT (39). Returns the assigned handle.</summary>
    public uint CreateSolidBrush(byte r, byte g, byte b)
    {
        uint handle = _nextHandle++;
        Span<byte> payload = stackalloc byte[16];
        BinaryPrimitives.WriteUInt32LittleEndian(payload[..4], handle);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.Slice(4, 4), 0); // BS_SOLID
        BinaryPrimitives.WriteUInt32LittleEndian(payload.Slice(8, 4), ColorRef(r, g, b));
        BinaryPrimitives.WriteUInt32LittleEndian(payload.Slice(12, 4), 0); // hatch
        WriteRecord(39, payload);
        return handle;
    }

    /// <summary>EMR_CREATEPEN (38). Returns the assigned handle.</summary>
    public uint CreatePen(uint style, int width, byte r, byte g, byte b)
    {
        uint handle = _nextHandle++;
        Span<byte> payload = stackalloc byte[24];
        BinaryPrimitives.WriteUInt32LittleEndian(payload[..4], handle);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.Slice(4, 4), style);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(8, 4), width);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(12, 4), 0);
        BinaryPrimitives.WriteUInt32LittleEndian(payload.Slice(16, 4), ColorRef(r, g, b));
        BinaryPrimitives.WriteUInt32LittleEndian(payload.Slice(20, 4), 0);
        WriteRecord(38, payload);
        return handle;
    }

    /// <summary>EMR_SELECTOBJECT (37).</summary>
    public void SelectObject(uint handle)
    {
        Span<byte> payload = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32LittleEndian(payload, handle);
        WriteRecord(37, payload);
    }

    /// <summary>EMR_SELECTOBJECT with a stock-object handle (high bit set).</summary>
    public void SelectStockObject(uint stockId)
    {
        SelectObject(0x80000000u | stockId);
    }

    /// <summary>EMR_RECTANGLE (43).</summary>
    public void Rectangle(int l, int t, int r, int b)
    {
        Span<byte> payload = stackalloc byte[16];
        BinaryPrimitives.WriteInt32LittleEndian(payload[..4], l);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(4, 4), t);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(8, 4), r);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(12, 4), b);
        WriteRecord(43, payload);
    }

    /// <summary>EMR_POLYGON16 (86).</summary>
    public void Polygon16(IReadOnlyList<(int X, int Y)> points)
    {
        int n = points.Count;
        // Payload: BoundsRect (16) + Count (4) + N * (2 + 2)
        int len = 16 + 4 + n * 4;
        Span<byte> payload = stackalloc byte[len];
        int minX = int.MaxValue, minY = int.MaxValue, maxX = int.MinValue, maxY = int.MinValue;
        for (int i = 0; i < n; i++)
        {
            if (points[i].X < minX) minX = points[i].X;
            if (points[i].Y < minY) minY = points[i].Y;
            if (points[i].X > maxX) maxX = points[i].X;
            if (points[i].Y > maxY) maxY = points[i].Y;
        }
        BinaryPrimitives.WriteInt32LittleEndian(payload[..4], minX);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(4, 4), minY);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(8, 4), maxX);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(12, 4), maxY);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(16, 4), n);
        for (int i = 0; i < n; i++)
        {
            BinaryPrimitives.WriteInt16LittleEndian(payload.Slice(20 + i * 4, 2), (short)points[i].X);
            BinaryPrimitives.WriteInt16LittleEndian(payload.Slice(22 + i * 4, 2), (short)points[i].Y);
        }
        WriteRecord(86, payload);
    }

    /// <summary>EMR_MOVETOEX (27).</summary>
    public void MoveToEx(int x, int y)
    {
        Span<byte> payload = stackalloc byte[8];
        BinaryPrimitives.WriteInt32LittleEndian(payload[..4], x);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(4, 4), y);
        WriteRecord(27, payload);
    }

    /// <summary>EMR_LINETO (54).</summary>
    public void LineTo(int x, int y)
    {
        Span<byte> payload = stackalloc byte[8];
        BinaryPrimitives.WriteInt32LittleEndian(payload[..4], x);
        BinaryPrimitives.WriteInt32LittleEndian(payload.Slice(4, 4), y);
        WriteRecord(54, payload);
    }

    public void BeginPath() => WriteRecord(59, ReadOnlySpan<byte>.Empty);
    public void EndPath() => WriteRecord(60, ReadOnlySpan<byte>.Empty);
    public void CloseFigure() => WriteRecord(61, ReadOnlySpan<byte>.Empty);
    public void FillPath() => WriteRecord(62, ReadOnlySpan<byte>.Empty);
    public void StrokeAndFillPath() => WriteRecord(63, ReadOnlySpan<byte>.Empty);
    public void StrokePath() => WriteRecord(64, ReadOnlySpan<byte>.Empty);
    public void AbortPath() => WriteRecord(68, ReadOnlySpan<byte>.Empty);

    public void SaveDc() => WriteRecord(33, ReadOnlySpan<byte>.Empty);

    public void RestoreDc(int n)
    {
        Span<byte> payload = stackalloc byte[4];
        BinaryPrimitives.WriteInt32LittleEndian(payload, n);
        WriteRecord(34, payload);
    }

    public void RawRecord(uint recordType, ReadOnlySpan<byte> payload) => WriteRecord(recordType, payload);

    private void WriteRecord(uint recordType, ReadOnlySpan<byte> payload)
    {
        // FillPath family has a 16-byte rectangle bounds prefix (the spec).
        Span<byte> hdr = stackalloc byte[8];
        uint size = (uint)(8 + payload.Length);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr[..4], recordType);
        BinaryPrimitives.WriteUInt32LittleEndian(hdr.Slice(4, 4), size);
        _ms.Write(hdr);
        if (payload.Length > 0) _ms.Write(payload);
        _recordCount++;
    }

    private static uint ColorRef(byte r, byte g, byte b) => (uint)r | ((uint)g << 8) | ((uint)b << 16);
}
