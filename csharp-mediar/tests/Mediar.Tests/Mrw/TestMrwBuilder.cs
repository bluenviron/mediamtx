using System.Buffers.Binary;
using System.Text;

namespace Mediar.Tests.Mrw;

/// <summary>
/// Synthesises minimal but spec-conforming Konica Minolta MRW byte streams
/// used by <see cref="MrwReaderTests"/>. Layout per libopenraw / dcraw:
/// 4-byte "\0MRM" magic + BE u32 envelope length + sub-block stream
/// (each sub-block = 4-byte "\0XXX" tag + BE u32 length + payload),
/// followed by the raw Bayer mosaic CFA payload.
/// </summary>
internal static class TestMrwBuilder
{
    internal sealed record MrwSpec
    {
        /// <summary>Optional overrides for the 4-byte magic. Defaults to "\0MRM".</summary>
        public byte[]? OverrideMagic { get; init; }

        /// <summary>If non-null the PRD sub-block is included with the given fields.</summary>
        public PrdSpec? Prd { get; init; }

        /// <summary>If non-null the TTW sub-block is included with the given embedded TIFF bytes.</summary>
        public byte[]? TtwBytes { get; init; }

        /// <summary>Optional WBG payload bytes. Sub-block is omitted when null.</summary>
        public byte[]? WbgBytes { get; init; }

        /// <summary>Optional RIF payload bytes. Sub-block is omitted when null.</summary>
        public byte[]? RifBytes { get; init; }

        /// <summary>Optional CFA payload bytes (raw Bayer mosaic) appended after the envelope.</summary>
        public byte[]? CfaBytes { get; init; }

        /// <summary>If true, the produced byte array is truncated to <see cref="TruncateTo"/> bytes.</summary>
        public bool Truncate { get; init; }
        public int TruncateTo { get; init; }

        /// <summary>If non-zero, overrides the envelope length field. Used to force overrun rejection.</summary>
        public uint OverrideEnvelopeLength { get; init; }

        /// <summary>If non-null, replaces the first byte of an inserted bogus sub-block tag for testing strict validation.</summary>
        public byte? InsertBogusSubBlockFirstByte { get; init; }
    }

    internal sealed record PrdSpec
    {
        public string VersionNumber { get; init; } = "27WB0002";
        public ushort SensorHeight { get; init; } = 2160;
        public ushort SensorWidth { get; init; } = 3008;
        public ushort ImageHeight { get; init; } = 2136;
        public ushort ImageWidth { get; init; } = 2868;
        public byte DataSize { get; init; } = 12;
        public byte PixelSize { get; init; } = 12;
        public byte StorageMethod { get; init; } = 0x52;
        public byte BayerPattern { get; init; } = 0x01;
        /// <summary>If true, the PRD payload is only 16 bytes (older firmware) - no bayer pattern byte.</summary>
        public bool ShortLayout { get; init; }
    }

    public static byte[] Build(MrwSpec spec)
    {
        using var blocks = new MemoryStream();

        if (spec.Prd is { } prd)
        {
            byte[] prdPayload = BuildPrdPayload(prd);
            WriteSubBlock(blocks, [0x00, (byte)'P', (byte)'R', (byte)'D'], prdPayload);
        }

        if (spec.TtwBytes is { } ttw && ttw.Length > 0)
        {
            WriteSubBlock(blocks, [0x00, (byte)'T', (byte)'T', (byte)'W'], ttw);
        }

        if (spec.WbgBytes is { } wbg && wbg.Length > 0)
        {
            WriteSubBlock(blocks, [0x00, (byte)'W', (byte)'B', (byte)'G'], wbg);
        }

        if (spec.RifBytes is { } rif && rif.Length > 0)
        {
            WriteSubBlock(blocks, [0x00, (byte)'R', (byte)'I', (byte)'F'], rif);
        }

        if (spec.InsertBogusSubBlockFirstByte is { } bogus)
        {
            // Append a sub-block whose tag byte 0 is not 0x00 - the reader must reject.
            WriteSubBlock(blocks, [bogus, (byte)'X', (byte)'X', (byte)'X'], [0x00]);
        }

        byte[] envelope = blocks.ToArray();
        using var ms = new MemoryStream();
        var magic = spec.OverrideMagic ?? [0x00, (byte)'M', (byte)'R', (byte)'M'];
        ms.Write(magic, 0, magic.Length);
        Span<byte> lenBytes = stackalloc byte[4];
        uint envLen = spec.OverrideEnvelopeLength != 0
            ? spec.OverrideEnvelopeLength
            : (uint)envelope.Length;
        BinaryPrimitives.WriteUInt32BigEndian(lenBytes, envLen);
        ms.Write(lenBytes);
        ms.Write(envelope, 0, envelope.Length);
        if (spec.CfaBytes is { } cfa && cfa.Length > 0)
        {
            ms.Write(cfa, 0, cfa.Length);
        }

        var bytes = ms.ToArray();
        if (spec.Truncate)
        {
            int keep = Math.Max(0, Math.Min(spec.TruncateTo, bytes.Length));
            var truncated = new byte[keep];
            Array.Copy(bytes, truncated, keep);
            return truncated;
        }
        return bytes;
    }

    private static byte[] BuildPrdPayload(PrdSpec prd)
    {
        int len = prd.ShortLayout ? 16 : 24;
        var payload = new byte[len];
        var version = Encoding.ASCII.GetBytes(prd.VersionNumber);
        Array.Copy(version, 0, payload, 0, Math.Min(version.Length, 8));
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(8, 2), prd.SensorHeight);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(10, 2), prd.SensorWidth);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(12, 2), prd.ImageHeight);
        BinaryPrimitives.WriteUInt16BigEndian(payload.AsSpan(14, 2), prd.ImageWidth);
        if (len > 16)
        {
            payload[16] = prd.DataSize;
            payload[17] = prd.PixelSize;
            payload[18] = prd.StorageMethod;
            // bytes 19-22 reserved/unknown - left zero
            payload[23] = prd.BayerPattern;
        }
        return payload;
    }

    private static void WriteSubBlock(MemoryStream s, byte[] tag, byte[] payload)
    {
        s.Write(tag, 0, 4);
        Span<byte> lenBytes = stackalloc byte[4];
        BinaryPrimitives.WriteUInt32BigEndian(lenBytes, (uint)payload.Length);
        s.Write(lenBytes);
        s.Write(payload, 0, payload.Length);
    }
}
