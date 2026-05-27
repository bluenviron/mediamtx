using System.Buffers.Binary;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Typed metadata for an <c>iovl</c> (overlay) item per ISO/IEC 23008-12 § 6.6.2.3.1.
/// The overlay derivation paints each referenced source (via the <c>dimg</c> iref)
/// onto a canvas of size (<see cref="OutputWidth"/>, <see cref="OutputHeight"/>)
/// at the per-source (<c>HorizontalOffset</c>, <c>VerticalOffset</c>) position,
/// filling the unpainted background with <see cref="CanvasFill"/> (RGBA).
/// </summary>
public sealed record HeifOverlayDerivation
{
    /// <summary>Box-format version (0 in the current spec).</summary>
    public required byte Version { get; init; }

    /// <summary>Box-format flags. Bit 0 controls 16-vs-32-bit field width.</summary>
    public required byte Flags { get; init; }

    /// <summary>16-bit per-channel RGBA fill colour for unpainted canvas regions.</summary>
    public required (ushort R, ushort G, ushort B, ushort A) CanvasFill { get; init; }

    /// <summary>Output canvas width in pixels.</summary>
    public required uint OutputWidth { get; init; }

    /// <summary>Output canvas height in pixels.</summary>
    public required uint OutputHeight { get; init; }

    /// <summary>Per-source horizontal/vertical offset (signed) into the canvas.</summary>
    public required ImmutableArray<(int HorizontalOffset, int VerticalOffset)> Offsets { get; init; }

    /// <summary>
    /// Parses a raw <c>iovl</c> payload into a typed <see cref="HeifOverlayDerivation"/>.
    /// <paramref name="referenceCount"/> must match the number of <c>dimg</c>
    /// sources declared in the iref box for this item.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, int referenceCount, out HeifOverlayDerivation derivation)
    {
        derivation = null!;
        if (data.Length < 2) return false;
        byte version = data[0];
        byte flags = data[1];
        int fieldBits = ((flags & 1) + 1) * 16;
        int fieldBytes = fieldBits / 8;
        int fixedLen = 2 + 8 + 2 * fieldBytes;
        if (data.Length < fixedLen + 2 * fieldBytes * referenceCount) return false;

        int p = 2;
        ushort r = BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
        ushort g = BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
        ushort b = BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
        ushort a = BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;

        uint outW, outH;
        if (fieldBytes == 2)
        {
            outW = BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
            outH = BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
        }
        else
        {
            outW = BinaryPrimitives.ReadUInt32BigEndian(data[p..]); p += 4;
            outH = BinaryPrimitives.ReadUInt32BigEndian(data[p..]); p += 4;
        }

        var offsets = ImmutableArray.CreateBuilder<(int, int)>(referenceCount);
        for (int i = 0; i < referenceCount; i++)
        {
            int hx, vy;
            if (fieldBytes == 2)
            {
                hx = (short)BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
                vy = (short)BinaryPrimitives.ReadUInt16BigEndian(data[p..]); p += 2;
            }
            else
            {
                hx = (int)BinaryPrimitives.ReadUInt32BigEndian(data[p..]); p += 4;
                vy = (int)BinaryPrimitives.ReadUInt32BigEndian(data[p..]); p += 4;
            }
            offsets.Add((hx, vy));
        }

        derivation = new HeifOverlayDerivation
        {
            Version = version,
            Flags = flags,
            CanvasFill = (r, g, b, a),
            OutputWidth = outW,
            OutputHeight = outH,
            Offsets = offsets.ToImmutable(),
        };
        return true;
    }
}

/// <summary>
/// Typed metadata for a <c>grid</c> item per ISO/IEC 23008-12 § 6.6.2.3.2.
/// The grid derivation tiles each referenced source (via the <c>dimg</c> iref)
/// into a (<see cref="Rows"/>, <see cref="Columns"/>) layout cropped to a
/// (<see cref="OutputWidth"/>, <see cref="OutputHeight"/>) output canvas.
/// Tiles are listed in row-major order.
/// </summary>
public sealed record HeifGridDerivation
{
    /// <summary>Box-format version (0 in the current spec).</summary>
    public required byte Version { get; init; }

    /// <summary>Box-format flags. Bit 0 controls 16-vs-32-bit output width/height field width.</summary>
    public required byte Flags { get; init; }

    /// <summary>Number of rows in the grid layout (always &gt;= 1).</summary>
    public required int Rows { get; init; }

    /// <summary>Number of columns in the grid layout (always &gt;= 1).</summary>
    public required int Columns { get; init; }

    /// <summary>Output canvas width in pixels (cropped from the tile lattice).</summary>
    public required uint OutputWidth { get; init; }

    /// <summary>Output canvas height in pixels (cropped from the tile lattice).</summary>
    public required uint OutputHeight { get; init; }

    /// <summary>
    /// Parses a raw <c>grid</c> payload into a typed <see cref="HeifGridDerivation"/>.
    /// </summary>
    public static bool TryParse(ReadOnlySpan<byte> data, out HeifGridDerivation derivation)
    {
        derivation = null!;
        if (data.Length < 8) return false;
        byte version = data[0];
        byte flags = data[1];
        int rowsMinus1 = data[2];
        int colsMinus1 = data[3];
        bool wide = (flags & 1) == 1;
        int needed = 4 + (wide ? 8 : 4);
        if (data.Length < needed) return false;

        uint outW, outH;
        if (wide)
        {
            outW = BinaryPrimitives.ReadUInt32BigEndian(data[4..]);
            outH = BinaryPrimitives.ReadUInt32BigEndian(data[8..]);
        }
        else
        {
            outW = BinaryPrimitives.ReadUInt16BigEndian(data[4..]);
            outH = BinaryPrimitives.ReadUInt16BigEndian(data[6..]);
        }

        derivation = new HeifGridDerivation
        {
            Version = version,
            Flags = flags,
            Rows = rowsMinus1 + 1,
            Columns = colsMinus1 + 1,
            OutputWidth = outW,
            OutputHeight = outH,
        };
        return true;
    }
}
