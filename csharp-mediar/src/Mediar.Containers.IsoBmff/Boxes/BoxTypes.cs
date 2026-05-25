using Mediar.IO;

namespace Mediar.Containers.IsoBmff;

/// <summary>
/// Well-known ISO BMFF / MP4 box type FourCCs.
/// </summary>
internal static class BoxTypes
{
    // File-type
    public static readonly FourCc Ftyp = new("ftyp");
    public static readonly FourCc Styp = new("styp");

    // Movie
    public static readonly FourCc Moov = new("moov");
    public static readonly FourCc Mvhd = new("mvhd");
    public static readonly FourCc Trak = new("trak");
    public static readonly FourCc Tkhd = new("tkhd");
    public static readonly FourCc Edts = new("edts");
    public static readonly FourCc Elst = new("elst");
    public static readonly FourCc Mdia = new("mdia");
    public static readonly FourCc Mdhd = new("mdhd");
    public static readonly FourCc Hdlr = new("hdlr");
    public static readonly FourCc Minf = new("minf");
    public static readonly FourCc Dinf = new("dinf");
    public static readonly FourCc Dref = new("dref");
    public static readonly FourCc Stbl = new("stbl");
    public static readonly FourCc Stsd = new("stsd");
    public static readonly FourCc Stts = new("stts");
    public static readonly FourCc Ctts = new("ctts");
    public static readonly FourCc Stsc = new("stsc");
    public static readonly FourCc Stsz = new("stsz");
    public static readonly FourCc Stz2 = new("stz2");
    public static readonly FourCc Stco = new("stco");
    public static readonly FourCc Co64 = new("co64");
    public static readonly FourCc Stss = new("stss");
    public static readonly FourCc Smhd = new("smhd");
    public static readonly FourCc Vmhd = new("vmhd");
    public static readonly FourCc Nmhd = new("nmhd");

    // Data
    public static readonly FourCc Mdat = new("mdat");
    public static readonly FourCc Free = new("free");
    public static readonly FourCc Skip = new("skip");
    public static readonly FourCc Wide = new("wide");

    // Codec sample entries (a few common ones)
    public static readonly FourCc Avc1 = new("avc1");
    public static readonly FourCc Avc3 = new("avc3");
    public static readonly FourCc Hvc1 = new("hvc1");
    public static readonly FourCc Hev1 = new("hev1");
    public static readonly FourCc Av01 = new("av01");
    public static readonly FourCc Vp09 = new("vp09");
    public static readonly FourCc Mp4a = new("mp4a");
    public static readonly FourCc Mp4v = new("mp4v");
    public static readonly FourCc Alac = new("alac");
    public static readonly FourCc Opus = new("Opus");
    public static readonly FourCc FlacEntry = new("fLaC");
    public static readonly FourCc Tx3g = new("tx3g");
    public static readonly FourCc Stpp = new("stpp");
    public static readonly FourCc Wvtt = new("wvtt");
    public static readonly FourCc EsDs = new("esds");
    public static readonly FourCc AvcC = new("avcC");
    public static readonly FourCc HvcC = new("hvcC");
    public static readonly FourCc Av1C = new("av1C");

    // Track handlers
    public static readonly FourCc Vide = new("vide");
    public static readonly FourCc Soun = new("soun");
    public static readonly FourCc Subt = new("subt");
    public static readonly FourCc Text = new("text");
    public static readonly FourCc Sbtl = new("sbtl");
    public static readonly FourCc Meta = new("meta");
}
