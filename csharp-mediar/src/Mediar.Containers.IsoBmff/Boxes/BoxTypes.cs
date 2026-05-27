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
    /// <summary>
    /// AOM's proposed sample-entry FourCC for AV2. The exact code is still
    /// under registration as the AV2 spec finalises; "av02" is the
    /// commonly-cited placeholder used by AOM tooling. Tracks tagged with
    /// it parse cleanly so that container-level passthrough works.
    /// </summary>
    public static readonly FourCc Av02 = new("av02");
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

    // Metadata: udta / meta / ilst tree + 3GPP location.
    public static readonly FourCc Udta = new("udta");
    public static readonly FourCc Ilst = new("ilst");
    public static readonly FourCc Data = new("data");
    public static readonly FourCc Loci = new("loci");

    // iTunes-style ilst atoms — names prefixed with the copyright sign (0xA9).
    public static readonly FourCc IlNam = new(((uint)0xA9 << 24) | ((uint)'n' << 16) | ((uint)'a' << 8) | (uint)'m');
    public static readonly FourCc IlArt = new(((uint)0xA9 << 24) | ((uint)'A' << 16) | ((uint)'R' << 8) | (uint)'T');
    public static readonly FourCc IlAlb = new(((uint)0xA9 << 24) | ((uint)'a' << 16) | ((uint)'l' << 8) | (uint)'b');
    public static readonly FourCc IlDay = new(((uint)0xA9 << 24) | ((uint)'d' << 16) | ((uint)'a' << 8) | (uint)'y');
    public static readonly FourCc IlGen = new(((uint)0xA9 << 24) | ((uint)'g' << 16) | ((uint)'e' << 8) | (uint)'n');
    public static readonly FourCc IlCmt = new(((uint)0xA9 << 24) | ((uint)'c' << 16) | ((uint)'m' << 8) | (uint)'t');
    public static readonly FourCc IlWrt = new(((uint)0xA9 << 24) | ((uint)'w' << 16) | ((uint)'r' << 8) | (uint)'t');
    public static readonly FourCc IlToo = new(((uint)0xA9 << 24) | ((uint)'t' << 16) | ((uint)'o' << 8) | (uint)'o');
    public static readonly FourCc IlLyr = new(((uint)0xA9 << 24) | ((uint)'l' << 16) | ((uint)'y' << 8) | (uint)'r');
    public static readonly FourCc IlXyz = new(((uint)0xA9 << 24) | ((uint)'x' << 16) | ((uint)'y' << 8) | (uint)'z');
    public static readonly FourCc IlGrp = new("aART");
    public static readonly FourCc IlTrk = new("trkn");
    public static readonly FourCc IlDsk = new("disk");
    public static readonly FourCc IlCpy = new("cprt");
    // Extended iTunes / Apple atoms.
    public static readonly FourCc IlWrk = new(((uint)0xA9 << 24) | ((uint)'w' << 16) | ((uint)'r' << 8) | (uint)'k');
    public static readonly FourCc IlGroup = new(((uint)0xA9 << 24) | ((uint)'g' << 16) | ((uint)'r' << 8) | (uint)'p');
    public static readonly FourCc IlSt3 = new(((uint)0xA9 << 24) | ((uint)'s' << 16) | ((uint)'t' << 8) | (uint)'3');
    public static readonly FourCc IlCon = new(((uint)0xA9 << 24) | ((uint)'c' << 16) | ((uint)'o' << 8) | (uint)'n');
    public static readonly FourCc IlDir = new(((uint)0xA9 << 24) | ((uint)'d' << 16) | ((uint)'i' << 8) | (uint)'r');
    public static readonly FourCc IlMvN = new(((uint)0xA9 << 24) | ((uint)'m' << 16) | ((uint)'v' << 8) | (uint)'n');
    public static readonly FourCc IlTmpo = new("tmpo");
    public static readonly FourCc IlCpil = new("cpil");
    public static readonly FourCc IlDesc = new("desc");
    public static readonly FourCc IlLdes = new("ldes");
    public static readonly FourCc IlKey = new("\u00A9key");
    public static readonly FourCc IlPub = new("\u00A9pub");
    // Freeform iTunes atom container ("----") plus its three named children.
    public static readonly FourCc IlFreeForm = new("----");
    public static readonly FourCc Mean = new("mean");
    public static readonly FourCc Name = new("name");
}
