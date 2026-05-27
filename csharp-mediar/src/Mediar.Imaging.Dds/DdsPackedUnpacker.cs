using System.Buffers.Binary;

namespace Mediar.Imaging.Dds;

/// <summary>
/// Unpacks DXGI packed bit-field uncompressed formats into the SDK's
/// canonical pixel layouts. Used for surfaces whose on-disk word layout
/// does not match any 1:1 byte format (e.g. R11G11B10_FLOAT, where 3
/// floats are packed into a single 32-bit word).
/// </summary>
internal static class DdsPackedUnpacker
{
    /// <summary>
    /// Unpacks an R11G11B10_FLOAT surface (DXGI_FORMAT 26) into
    /// 3 x float32 (PixelFormat.Rgb96Float) at the destination.
    /// Each source word encodes R (bits 0-10, 5-exp 6-mant), G
    /// (bits 11-21, 5-exp 6-mant) and B (bits 22-31, 5-exp 5-mant).
    /// </summary>
    public static byte[] UnpackR11G11B10Float(ReadOnlySpan<byte> src, int width, int height)
    {
        if (src.Length < width * height * 4)
        {
            throw new ImageFormatException("Truncated R11G11B10_FLOAT pixel data.");
        }
        var dst = new byte[width * height * 12];
        var dstSpan = dst.AsSpan();
        for (int i = 0; i < width * height; i++)
        {
            uint packed = BinaryPrimitives.ReadUInt32LittleEndian(src.Slice(i * 4, 4));
            float r = DecodeF11((ushort)(packed & 0x7FF));
            float g = DecodeF11((ushort)((packed >> 11) & 0x7FF));
            float b = DecodeF10((ushort)((packed >> 22) & 0x3FF));
            BinaryPrimitives.WriteSingleLittleEndian(dstSpan.Slice(i * 12 + 0, 4), r);
            BinaryPrimitives.WriteSingleLittleEndian(dstSpan.Slice(i * 12 + 4, 4), g);
            BinaryPrimitives.WriteSingleLittleEndian(dstSpan.Slice(i * 12 + 8, 4), b);
        }
        return dst;
    }

    /// <summary>
    /// Unpacks an R10G10B10A2_UNORM surface (DXGI_FORMAT 24) into 4 x byte
    /// (PixelFormat.Rgba32). 10-bit channels are rescaled to 8 bits via
    /// (value10 * 255 + 511) / 1023; the 2-bit alpha is replicated.
    /// </summary>
    public static byte[] UnpackR10G10B10A2Unorm(ReadOnlySpan<byte> src, int width, int height)
    {
        if (src.Length < width * height * 4)
        {
            throw new ImageFormatException("Truncated R10G10B10A2_UNORM pixel data.");
        }
        var dst = new byte[width * height * 4];
        for (int i = 0; i < width * height; i++)
        {
            uint packed = BinaryPrimitives.ReadUInt32LittleEndian(src.Slice(i * 4, 4));
            uint r10 = packed & 0x3FF;
            uint g10 = (packed >> 10) & 0x3FF;
            uint b10 = (packed >> 20) & 0x3FF;
            uint a2 = (packed >> 30) & 0x3;
            dst[i * 4 + 0] = (byte)((r10 * 255 + 511) / 1023);
            dst[i * 4 + 1] = (byte)((g10 * 255 + 511) / 1023);
            dst[i * 4 + 2] = (byte)((b10 * 255 + 511) / 1023);
            dst[i * 4 + 3] = (byte)((a2 * 255 + 1) / 3);
        }
        return dst;
    }

    /// <summary>
    /// Unpacks an R9G9B9E5_SHAREDEXP surface (DXGI_FORMAT 67) into 3 x
    /// float32 (PixelFormat.Rgb96Float). Each word encodes 9-bit mantissas
    /// for R/G/B sharing a single 5-bit biased exponent in the top bits.
    /// </summary>
    public static byte[] UnpackR9G9B9E5SharedExp(ReadOnlySpan<byte> src, int width, int height)
    {
        if (src.Length < width * height * 4)
        {
            throw new ImageFormatException("Truncated R9G9B9E5_SHAREDEXP pixel data.");
        }
        var dst = new byte[width * height * 12];
        var dstSpan = dst.AsSpan();
        for (int i = 0; i < width * height; i++)
        {
            uint packed = BinaryPrimitives.ReadUInt32LittleEndian(src.Slice(i * 4, 4));
            uint rm = packed & 0x1FF;
            uint gm = (packed >> 9) & 0x1FF;
            uint bm = (packed >> 18) & 0x1FF;
            uint exp = (packed >> 27) & 0x1F;
            float scale = MathF.Pow(2.0f, (int)exp - 15 - 9);
            float r = rm * scale;
            float g = gm * scale;
            float b = bm * scale;
            BinaryPrimitives.WriteSingleLittleEndian(dstSpan.Slice(i * 12 + 0, 4), r);
            BinaryPrimitives.WriteSingleLittleEndian(dstSpan.Slice(i * 12 + 4, 4), g);
            BinaryPrimitives.WriteSingleLittleEndian(dstSpan.Slice(i * 12 + 8, 4), b);
        }
        return dst;
    }

    private static float DecodeF11(ushort v)
    {
        uint exp = (uint)(v >> 6) & 0x1Fu;
        uint mant = v & 0x3Fu;
        if (exp == 0)
        {
            if (mant == 0) return 0.0f;
            return mant * MathF.Pow(2.0f, -14 - 6);
        }
        if (exp == 0x1F)
        {
            return mant == 0 ? float.PositiveInfinity : float.NaN;
        }
        return (1.0f + mant / 64.0f) * MathF.Pow(2.0f, (int)exp - 15);
    }

    private static float DecodeF10(ushort v)
    {
        uint exp = (uint)(v >> 5) & 0x1Fu;
        uint mant = v & 0x1Fu;
        if (exp == 0)
        {
            if (mant == 0) return 0.0f;
            return mant * MathF.Pow(2.0f, -14 - 5);
        }
        if (exp == 0x1F)
        {
            return mant == 0 ? float.PositiveInfinity : float.NaN;
        }
        return (1.0f + mant / 32.0f) * MathF.Pow(2.0f, (int)exp - 15);
    }
}
