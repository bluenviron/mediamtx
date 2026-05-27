using System.Buffers.Binary;

namespace Mediar.Imaging;

/// <summary>
/// Sniffs the first bytes of an image file and identifies its
/// <see cref="ImageFormat"/> by magic numbers. Pure, allocation-free.
/// </summary>
public static class ImageFormatDetector
{
    /// <summary>
    /// Maximum number of leading bytes <see cref="Detect(ReadOnlySpan{byte})"/>
    /// inspects. Callers should pre-buffer at least this much.
    /// </summary>
    public const int RecommendedHeaderBytes = 64;

    /// <summary>
    /// Inspects <paramref name="header"/> for a known image-format magic
    /// sequence and returns the matching format, or <see cref="ImageFormat.Unknown"/>.
    /// </summary>
    /// <remarks>
    /// Pass at least the first <see cref="RecommendedHeaderBytes"/> bytes; the
    /// detector tolerates shorter inputs but degrades to fuzzy matches.
    /// </remarks>
    public static ImageFormat Detect(ReadOnlySpan<byte> header)
    {
        // Detection is ordered: longest / most-specific magic first.
        if (header.Length >= 8)
        {
            // PNG: 89 50 4E 47 0D 0A 1A 0A
            if (header[0] == 0x89 && header[1] == 0x50 && header[2] == 0x4E && header[3] == 0x47 &&
                header[4] == 0x0D && header[5] == 0x0A && header[6] == 0x1A && header[7] == 0x0A)
            {
                // Distinguish APNG / MNG / PNJ inline by inspecting the first chunk.
                if (header.Length >= 16 && ContainsAcTl(header)) return ImageFormat.Apng;
                return ImageFormat.Png;
            }

            // MNG: 8A 4D 4E 47 0D 0A 1A 0A
            if (header[0] == 0x8A && header[1] == 0x4D && header[2] == 0x4E && header[3] == 0x47 &&
                header[4] == 0x0D && header[5] == 0x0A && header[6] == 0x1A && header[7] == 0x0A)
            {
                return ImageFormat.Mng;
            }
        }

        if (header.Length >= 12)
        {
            // RIFF: "RIFF"...."WEBP" / "WEBA"
            if (header[0] == (byte)'R' && header[1] == (byte)'I' &&
                header[2] == (byte)'F' && header[3] == (byte)'F')
            {
                if (header[8] == (byte)'W' && header[9] == (byte)'E' &&
                    header[10] == (byte)'B' && header[11] == (byte)'P')
                {
                    return ImageFormat.WebP;
                }
                if (header[8] == (byte)'C' && header[9] == (byte)'D' &&
                    header[10] == (byte)'R')
                {
                    return ImageFormat.Cdr;
                }
            }

            // ISO BMFF: "....ftyp...."
            if (header[4] == (byte)'f' && header[5] == (byte)'t' &&
                header[6] == (byte)'y' && header[7] == (byte)'p')
            {
                var brand = header.Slice(8, 4);
                return DetectIsoBmffBrand(brand);
            }
        }

        if (header.Length >= 6)
        {
            // GIF: GIF87a or GIF89a
            if (header[0] == (byte)'G' && header[1] == (byte)'I' && header[2] == (byte)'F' &&
                header[3] == (byte)'8' &&
                (header[4] == (byte)'7' || header[4] == (byte)'9') && header[5] == (byte)'a')
            {
                return ImageFormat.Gif;
            }
        }

        if (header.Length >= 4)
        {
            // JPEG / JFIF: FF D8 FF
            if (header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF)
            {
                return ImageFormat.Jpeg;
            }

            // BMP / DIB: "BM"
            if (header[0] == (byte)'B' && header[1] == (byte)'M')
            {
                return ImageFormat.Bmp;
            }

            // TIFF / many RAW: "II*\0" (little-endian) or "MM\0*" (big-endian)
            if ((header[0] == (byte)'I' && header[1] == (byte)'I' &&
                 header[2] == 0x2A && header[3] == 0x00) ||
                (header[0] == (byte)'M' && header[1] == (byte)'M' &&
                 header[2] == 0x00 && header[3] == 0x2A))
            {
                // CR2 (Canon Raw v2) - identical TIFF preamble but with
                // a "CR\x02\x00" sentinel at offset 8.
                if (header.Length >= 12 &&
                    header[8] == (byte)'C' && header[9] == (byte)'R' &&
                    header[10] == 0x02 && header[11] == 0x00)
                {
                    return ImageFormat.Cr2;
                }
                return ImageFormat.Tiff;
            }

            // BigTIFF: II + 0x2B 0x00 / MM 0x00 0x2B (used by some new RAW)
            if ((header[0] == (byte)'I' && header[1] == (byte)'I' &&
                 header[2] == 0x2B && header[3] == 0x00) ||
                (header[0] == (byte)'M' && header[1] == (byte)'M' &&
                 header[2] == 0x00 && header[3] == 0x2B))
            {
                return ImageFormat.Tiff;
            }

            // RW2 (Panasonic / Leica RAW): identical TIFF byte-order mark "II"
            // followed by magic 0x0055 (85) instead of TIFF's 0x002A (42).
            if (header[0] == (byte)'I' && header[1] == (byte)'I' &&
                header[2] == 0x55 && header[3] == 0x00)
            {
                return ImageFormat.Rw2;
            }

            // ORF (Olympus RAW): non-standard magic word. The byte-order mark
            // "II" is followed by either 0x4F52 ('RO', little-endian sense) or
            // 0x5253 ('RS', older Olympus E-1 lineup). The big-endian variant
            // "MM" + 0x524F ('OR') is rare but spec'd. All other Olympus files
            // simply use the standard TIFF magic 0x002A and are identified at
            // open time by the EXIF Make tag.
            if (header[0] == (byte)'I' && header[1] == (byte)'I' &&
                ((header[2] == 0x52 && header[3] == 0x4F) ||
                 (header[2] == 0x52 && header[3] == 0x53)))
            {
                return ImageFormat.Orf;
            }
            if (header[0] == (byte)'M' && header[1] == (byte)'M' &&
                header[2] == 0x4F && header[3] == 0x52)
            {
                return ImageFormat.Orf;
            }

            // DDS: "DDS "
            if (header[0] == (byte)'D' && header[1] == (byte)'D' &&
                header[2] == (byte)'S' && header[3] == (byte)' ')
            {
                return ImageFormat.Dds;
            }

            // KTX / KTX2: 12-byte Khronos texture identifier
            //   KTX 1.x  -> "«KTX 11»\r\n\x1A\n"  (bytes 4..6 = "KTX")
            //   KTX 2.x  -> "«KTX 20»\r\n\x1A\n"  (bytes 4..6 = "KTX")
            if (header.Length >= 12 &&
                header[0] == 0xAB && header[1] == 0x4B && header[2] == 0x54 && header[3] == 0x58 &&
                header[4] == 0x20 &&
                header[7] == 0xBB && header[8] == 0x0D && header[9] == 0x0A &&
                header[10] == 0x1A && header[11] == 0x0A)
            {
                if (header[5] == 0x31 && header[6] == 0x31) return ImageFormat.Ktx;
                if (header[5] == 0x32 && header[6] == 0x30) return ImageFormat.Ktx2;
            }

            // PSD: "8BPS" (V1 = PSD, V2 = PSB)
            if (header[0] == (byte)'8' && header[1] == (byte)'B' &&
                header[2] == (byte)'P' && header[3] == (byte)'S')
            {
                if (header.Length >= 6)
                {
                    var ver = BinaryPrimitives.ReadUInt16BigEndian(header.Slice(4, 2));
                    return ver == 2 ? ImageFormat.Psb : ImageFormat.Psd;
                }
                return ImageFormat.Psd;
            }

            // ICO / CUR header: 00 00 ti 00 where ti = 1 (icon) / 2 (cursor)
            if (header[0] == 0x00 && header[1] == 0x00 && header[3] == 0x00)
            {
                if (header[2] == 0x01) return ImageFormat.Ico;
                if (header[2] == 0x02) return ImageFormat.Cur;
            }

            // ICNS: "icns"
            if (header[0] == (byte)'i' && header[1] == (byte)'c' &&
                header[2] == (byte)'n' && header[3] == (byte)'s')
            {
                return ImageFormat.Icns;
            }

            // PCX: 0x0A xx 01 xx, version byte is small
            if (header[0] == 0x0A && header[2] == 0x01 && header[1] <= 0x05)
            {
                return ImageFormat.Pcx;
            }

            // DCX: 0xB168DE3A little endian
            if (header[0] == 0xB1 && header[1] == 0x68 && header[2] == 0xDE && header[3] == 0x3A)
            {
                return ImageFormat.Dcx;
            }
        }

        if (header.Length >= 2)
        {
            // GZIP-wrapped (SVGZ / EMZ / WMZ): we can't distinguish without the extension.
            if (header[0] == 0x1F && header[1] == 0x8B)
            {
                return ImageFormat.Svgz;
            }
        }

        // X PixMap: "/* XPM */"
        if (StartsWithAscii(header, "/* XPM */"))
        {
            return ImageFormat.Xpm;
        }

        // Portable AnyMap: "P1".."P7" followed by whitespace
        if (header.Length >= 3 &&
            header[0] == (byte)'P' && header[1] >= (byte)'1' && header[1] <= (byte)'7' &&
            (header[2] == 0x20 || header[2] == 0x0A || header[2] == 0x0D || header[2] == 0x09))
        {
            return header[1] switch
            {
                (byte)'1' or (byte)'4' => ImageFormat.Pbm,
                (byte)'2' or (byte)'5' => ImageFormat.Pgm,
                (byte)'3' or (byte)'6' => ImageFormat.Ppm,
                _ => ImageFormat.Pnm,
            };
        }

        // Radiance HDR: "#?RADIANCE" or "#?RGBE"
        if (StartsWithAscii(header, "#?RADIANCE") || StartsWithAscii(header, "#?RGBE"))
        {
            return ImageFormat.Hdr;
        }

        // BPG: 0x42 0x50 0x47 0xFB
        if (header.Length >= 4 &&
            header[0] == 0x42 && header[1] == 0x50 && header[2] == 0x47 && header[3] == 0xFB)
        {
            return ImageFormat.Bpg;
        }

        // FLIF: "FLIF"
        if (StartsWithAscii(header, "FLIF"))
        {
            return ImageFormat.Flif;
        }

        // JPEG 2000 codestream: FF 4F FF 51 (SOC + SIZ marker start)
        if (header.Length >= 4 &&
            header[0] == 0xFF && header[1] == 0x4F && header[2] == 0xFF && header[3] == 0x51)
        {
            return ImageFormat.J2k;
        }

        // JPEG 2000 file: 00 00 00 0C 6A 50 20 20 0D 0A 87 0A
        if (header.Length >= 12 &&
            header[0] == 0x00 && header[1] == 0x00 && header[2] == 0x00 && header[3] == 0x0C &&
            header[4] == (byte)'j' && header[5] == (byte)'P' &&
            header[6] == 0x20 && header[7] == 0x20 &&
            header[8] == 0x0D && header[9] == 0x0A && header[10] == 0x87 && header[11] == 0x0A)
        {
            return ImageFormat.Jp2;
        }

        // JPEG XL: codestream FF 0A or container 00 00 00 0C 4A 58 4C 20
        if (header.Length >= 2 && header[0] == 0xFF && header[1] == 0x0A)
        {
            return ImageFormat.Jxl;
        }
        if (header.Length >= 12 &&
            header[0] == 0x00 && header[1] == 0x00 && header[2] == 0x00 && header[3] == 0x0C &&
            header[4] == (byte)'J' && header[5] == (byte)'X' &&
            header[6] == (byte)'L' && header[7] == (byte)' ')
        {
            return ImageFormat.Jxl;
        }

        // JPEG XR: 0x49 0x49 0xBC ...
        if (header.Length >= 3 && header[0] == 0x49 && header[1] == 0x49 && header[2] == 0xBC)
        {
            return ImageFormat.Jxr;
        }

        // EMF: header begins with EMR_HEADER record type 0x00000001 and a
        // signature "ENHMETAFILE" 44 bytes in. Cheap check: 1 0 0 0 + size.
        if (header.Length >= 44 &&
            BinaryPrimitives.ReadUInt32LittleEndian(header[..4]) == 1u &&
            header[40] == (byte)' ' && header[41] == (byte)'E' &&
            header[42] == (byte)'M' && header[43] == (byte)'F')
        {
            return ImageFormat.Emf;
        }

        // WMF placeable header: 0xD7 0xCD 0xC6 0x9A
        if (header.Length >= 4 &&
            header[0] == 0xD7 && header[1] == 0xCD &&
            header[2] == 0xC6 && header[3] == 0x9A)
        {
            return ImageFormat.Apm;
        }
        // WMF (plain): word 0x0001 + word 0x0009
        if (header.Length >= 4 &&
            header[0] == 0x01 && header[1] == 0x00 &&
            header[2] == 0x09 && header[3] == 0x00)
        {
            return ImageFormat.Wmf;
        }

        // RAF (Fujifilm RAW): 15-byte ASCII signature "FUJIFILMCCD-RAW".
        if (header.Length >= 15 &&
            header[0] == (byte)'F' && header[1] == (byte)'U' && header[2] == (byte)'J' &&
            header[3] == (byte)'I' && header[4] == (byte)'F' && header[5] == (byte)'I' &&
            header[6] == (byte)'L' && header[7] == (byte)'M' && header[8] == (byte)'C' &&
            header[9] == (byte)'C' && header[10] == (byte)'D' && header[11] == (byte)'-' &&
            header[12] == (byte)'R' && header[13] == (byte)'A' && header[14] == (byte)'W')
        {
            return ImageFormat.Raf;
        }

        // MRW (Konica Minolta RAW): 4-byte magic "\0MRM" = 0x00 0x4D 0x52 0x4D.
        // Used by Minolta DiMage A-series and 7-series, Dynax/Maxxum 5D/7D,
        // and Konica Minolta α-Sweet Digital before Sony absorbed the line.
        if (header.Length >= 4 &&
            header[0] == 0x00 && header[1] == (byte)'M' &&
            header[2] == (byte)'R' && header[3] == (byte)'M')
        {
            return ImageFormat.Mrw;
        }

        // X3F (Sigma Foveon RAW): 4-byte ASCII signature "FOVb".
        if (StartsWithAscii(header, "FOVb"))
        {
            return ImageFormat.X3f;
        }

        // CRW (Canon CIFF v1): 14-byte signature - 2-byte byte-order ("II" or
        // "MM"), 4-byte header length, then ASCII "HEAPCCDR". Used by Canon
        // EOS-D30/D60/10D/300D/1D/1Ds bodies before CR2 superseded CIFF.
        if (header.Length >= 14 &&
            ((header[0] == (byte)'I' && header[1] == (byte)'I') ||
             (header[0] == (byte)'M' && header[1] == (byte)'M')) &&
            header[6] == (byte)'H' && header[7] == (byte)'E' &&
            header[8] == (byte)'A' && header[9] == (byte)'P' &&
            header[10] == (byte)'C' && header[11] == (byte)'C' &&
            header[12] == (byte)'D' && header[13] == (byte)'R')
        {
            return ImageFormat.Crw;
        }

        // DjVu: "AT&TFORM"
        if (StartsWithAscii(header, "AT&TFORM"))
        {
            return ImageFormat.Djvu;
        }

        // DICOM: bytes 128..131 = "DICM"
        if (header.Length >= 132 &&
            header[128] == (byte)'D' && header[129] == (byte)'I' &&
            header[130] == (byte)'C' && header[131] == (byte)'M')
        {
            return ImageFormat.Dicom;
        }

        // ECW: ERS file header is text "FileType = ECW"; raw ECW starts with 0x80 'B' 'B'
        if (header.Length >= 4 &&
            header[0] == 0x80 && header[1] == 0x42 && header[2] == 0x42 && header[3] == 0x42)
        {
            return ImageFormat.Ecw;
        }

        // PDF / AI: "%PDF-"
        if (header.Length >= 5 &&
            header[0] == (byte)'%' && header[1] == (byte)'P' && header[2] == (byte)'D' &&
            header[3] == (byte)'F' && header[4] == (byte)'-')
        {
            return ImageFormat.Ai;
        }

        // ZIP container: "PK\x03\x04" - could be ODG/OTG/FODG/AFPHOTO/AFX/CLIP/INK/SKP/...
        if (header.Length >= 4 &&
            header[0] == (byte)'P' && header[1] == (byte)'K' &&
            header[2] == 0x03 && header[3] == 0x04)
        {
            return ImageFormat.Odg;
        }

        // Targa file footer signature is at end; the only reliable leading
        // signature in a TGA file is the bytes "TRUEVISION-XFILE." at EOF.
        // We attempt a weak heuristic if no other match: image-type 0..11.
        if (header.Length >= 18)
        {
            byte imageType = header[2];
            byte colorMapType = header[1];
            if ((imageType is 0 or 1 or 2 or 3 or 9 or 10 or 11) &&
                colorMapType is 0 or 1)
            {
                return ImageFormat.Tga;
            }
        }

        return ImageFormat.Unknown;
    }

    /// <summary>Reads up to <see cref="RecommendedHeaderBytes"/> from <paramref name="stream"/> and detects.</summary>
    public static async ValueTask<ImageFormat> DetectAsync(
        Stream stream, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(stream);
        var buf = new byte[RecommendedHeaderBytes + 132];
        int read = 0;
        while (read < buf.Length)
        {
            int n = await stream.ReadAsync(buf.AsMemory(read), cancellationToken).ConfigureAwait(false);
            if (n == 0) break;
            read += n;
        }
        return Detect(buf.AsSpan(0, read));
    }

    private static ImageFormat DetectIsoBmffBrand(ReadOnlySpan<byte> brand)
    {
        // HEIF / HEIC: "heic","heix","heif","heim","heis","mif1","msf1",
        //              "hevc","hevx","mfx1","mfx2"
        // AVIF       : "avif","avis"
        // CR3        : "crx ","crxc"
        // QTIF       : "qif "
        // generic    : MP4 brands → not an image.
        if (Equal(brand, "heic") || Equal(brand, "heix") || Equal(brand, "heim") ||
            Equal(brand, "heis"))
        {
            return ImageFormat.Heic;
        }
        if (Equal(brand, "heif") || Equal(brand, "mif1") || Equal(brand, "msf1") ||
            Equal(brand, "hevc") || Equal(brand, "hevx") || Equal(brand, "mfx1") ||
            Equal(brand, "mfx2"))
        {
            return ImageFormat.Heif;
        }
        if (Equal(brand, "avif") || Equal(brand, "avis"))
        {
            return ImageFormat.Avif;
        }
        if (Equal(brand, "crx "))
        {
            return ImageFormat.Cr3;
        }
        if (Equal(brand, "qt  ") || Equal(brand, "qif "))
        {
            return ImageFormat.Qtif;
        }
        return ImageFormat.Unknown;
    }

    private static bool ContainsAcTl(ReadOnlySpan<byte> header)
    {
        // APNG identifies itself with an "acTL" chunk appearing before "IDAT".
        // Cheap pattern match over the available prefix.
        ReadOnlySpan<byte> ac = "acTL"u8;
        for (int i = 8; i + 4 <= header.Length; i++)
        {
            if (header[i] == ac[0] && header[i + 1] == ac[1] &&
                header[i + 2] == ac[2] && header[i + 3] == ac[3])
            {
                return true;
            }
        }
        return false;
    }

    private static bool StartsWithAscii(ReadOnlySpan<byte> bytes, string ascii)
    {
        if (bytes.Length < ascii.Length) return false;
        for (int i = 0; i < ascii.Length; i++)
        {
            if (bytes[i] != (byte)ascii[i]) return false;
        }
        return true;
    }

    private static bool Equal(ReadOnlySpan<byte> a, string b)
    {
        if (a.Length != b.Length) return false;
        for (int i = 0; i < b.Length; i++)
        {
            if (a[i] != (byte)b[i]) return false;
        }
        return true;
    }
}
