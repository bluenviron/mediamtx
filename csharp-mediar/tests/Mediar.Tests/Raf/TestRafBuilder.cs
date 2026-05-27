using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Raf;

/// <summary>
/// Synthesises minimal but spec-conforming Fujifilm RAF byte streams used
/// by <see cref="RafReaderTests"/>. The layout is byte-exact per
/// libopenraw's RAF documentation: 16-byte magic, 4-byte format version,
/// 8-byte reserved id, 32-byte zero-terminated camera-model slot,
/// 4-byte directory version, 20-byte padding, then six big-endian
/// uint32 offset/length pairs (JPEG / Meta / CFA).
/// </summary>
internal static class TestRafBuilder
{
    private const int HeaderSize = 0x6C;

    internal sealed record RafSpec
    {
        public string FormatVersion { get; init; } = "0201";
        public string CameraModel { get; init; } = "X-T4";
        public string DirectoryVersion { get; init; } = "0100";

        /// <summary>Bytes of the embedded JPEG preview. Must be a real, parseable JPEG.</summary>
        public byte[] JpegBytes { get; init; } = [];

        /// <summary>Optional Meta-container payload. Can be empty.</summary>
        public byte[] MetaBytes { get; init; } = [];

        /// <summary>Optional CFA TIFF payload. Can be empty.</summary>
        public byte[] CfaBytes { get; init; } = [];

        /// <summary>If non-null, overrides the 15-byte ASCII magic at offset 0.</summary>
        public byte[]? OverrideMagic { get; init; }

        /// <summary>If true, the produced byte array is truncated to <see cref="TruncateTo"/> bytes.</summary>
        public bool Truncate { get; init; }
        public int TruncateTo { get; init; }

        /// <summary>If true, the JPEG offset/length pair is zeroed (used to test rejection).</summary>
        public bool ZeroJpegSlot { get; init; }
    }

    public static byte[] Build(RafSpec spec)
    {
        // Layout: header (108 bytes) + JPEG + Meta + CFA, in that order.
        int jpegOffset = HeaderSize;
        int metaOffset = jpegOffset + spec.JpegBytes.Length;
        int cfaOffset = metaOffset + spec.MetaBytes.Length;
        int total = cfaOffset + spec.CfaBytes.Length;

        var bytes = new byte[total];

        // Magic.
        var magic = spec.OverrideMagic ?? "FUJIFILMCCD-RAW"u8.ToArray();
        Array.Copy(magic, 0, bytes, 0, Math.Min(magic.Length, 16));

        // Format version (offset 0x10, 4 ASCII bytes).
        WriteAsciiFixed(bytes, 0x10, spec.FormatVersion, 4);

        // 8 bytes reserved/id at 0x14 left zero.
        // Camera model (offset 0x1C, 32 bytes, zero-terminated).
        WriteAsciiZeroPadded(bytes, 0x1C, spec.CameraModel, 32);

        // Directory version (offset 0x3C, 4 ASCII bytes).
        WriteAsciiFixed(bytes, 0x3C, spec.DirectoryVersion, 4);
        // 20 bytes unknown at 0x40 left zero.

        // Offset/length table at 0x54.
        if (spec.ZeroJpegSlot)
        {
            // Zero the JPEG offset slot - reader must reject.
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x54, 4), 0u);
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x58, 4), 0u);
        }
        else
        {
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x54, 4), (uint)jpegOffset);
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x58, 4), (uint)spec.JpegBytes.Length);
        }
        if (spec.MetaBytes.Length > 0)
        {
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x5C, 4), (uint)metaOffset);
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x60, 4), (uint)spec.MetaBytes.Length);
        }
        if (spec.CfaBytes.Length > 0)
        {
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x64, 4), (uint)cfaOffset);
            BinaryPrimitives.WriteUInt32BigEndian(bytes.AsSpan(0x68, 4), (uint)spec.CfaBytes.Length);
        }

        // Payload regions.
        Array.Copy(spec.JpegBytes, 0, bytes, jpegOffset, spec.JpegBytes.Length);
        Array.Copy(spec.MetaBytes, 0, bytes, metaOffset, spec.MetaBytes.Length);
        Array.Copy(spec.CfaBytes, 0, bytes, cfaOffset, spec.CfaBytes.Length);

        if (spec.Truncate)
        {
            int keep = Math.Max(0, Math.Min(spec.TruncateTo, total));
            var truncated = new byte[keep];
            Array.Copy(bytes, truncated, keep);
            return truncated;
        }

        return bytes;
    }

    private static void WriteAsciiFixed(byte[] dst, int offset, string value, int length)
    {
        var raw = Encoding.ASCII.GetBytes(value);
        int n = Math.Min(raw.Length, length);
        Array.Copy(raw, 0, dst, offset, n);
        // remainder already zero-initialised
    }

    private static void WriteAsciiZeroPadded(byte[] dst, int offset, string value, int length)
    {
        WriteAsciiFixed(dst, offset, value, length - 1); // reserve last byte for terminator
    }
}
