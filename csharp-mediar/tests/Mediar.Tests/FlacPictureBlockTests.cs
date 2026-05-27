using System.Buffers.Binary;
using System.Text;
using Xunit;

namespace Mediar.Tests;

/// <summary>
/// Tests for the FLAC PICTURE metadata-block parser (RFC 9639 § 8.8).
/// The same byte layout is reused by Vorbis Comments' base64-encoded
/// <c>METADATA_BLOCK_PICTURE</c> field, so the parser is exercised
/// through both FLAC and the Vorbis-comment wiring.
/// </summary>
public sealed class FlacPictureBlockTests
{
    private static readonly byte[] JpegMagic = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46];
    private static readonly byte[] PngMagic = [0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00];

    [Fact]
    public void TryParse_Decodes_All_Fields()
    {
        byte[] block = BuildPictureBlock(
            type: (uint)MediaPictureType.CoverFront,
            mime: "image/jpeg",
            description: "Front cover",
            width: 600,
            height: 600,
            depth: 24,
            palette: 0,
            data: JpegMagic);

        var picture = FlacPictureBlock.TryParse(block);
        Assert.NotNull(picture);
        Assert.Equal(MediaPictureType.CoverFront, picture!.Type);
        Assert.Equal("image/jpeg", picture.MimeType);
        Assert.Equal("Front cover", picture.Description);
        Assert.Equal(600, picture.Width);
        Assert.Equal(600, picture.Height);
        Assert.Equal(24, picture.ColorDepth);
        Assert.Equal(0, picture.IndexedColors);
        Assert.Equal(JpegMagic, picture.Data.ToArray());
    }

    [Fact]
    public void TryParse_Handles_Empty_Mime_And_Description()
    {
        byte[] block = BuildPictureBlock(
            type: (uint)MediaPictureType.Other,
            mime: "",
            description: "",
            width: 0,
            height: 0,
            depth: 0,
            palette: 0,
            data: PngMagic);

        var picture = FlacPictureBlock.TryParse(block);
        Assert.NotNull(picture);
        Assert.Equal("", picture!.MimeType);
        Assert.Equal("", picture.Description);
        Assert.Equal(PngMagic, picture.Data.ToArray());
    }

    [Fact]
    public void TryParse_Out_Of_Range_PictureType_Falls_Back_To_Other()
    {
        byte[] block = BuildPictureBlock(
            type: 99u,
            mime: "image/png",
            description: "",
            width: 1,
            height: 1,
            depth: 8,
            palette: 0,
            data: PngMagic);

        var picture = FlacPictureBlock.TryParse(block);
        Assert.NotNull(picture);
        Assert.Equal(MediaPictureType.Other, picture!.Type);
    }

    [Fact]
    public void TryParse_Truncated_Header_Returns_Null()
    {
        Assert.Null(FlacPictureBlock.TryParse([0x00, 0x00, 0x00]));
    }

    [Fact]
    public void TryParse_Mime_Length_Exceeds_Buffer_Returns_Null()
    {
        byte[] block = new byte[12];
        // type (0) + mime length = 9999 (larger than buffer)
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(4, 4), 9999u);
        Assert.Null(FlacPictureBlock.TryParse(block));
    }

    [Fact]
    public void TryParse_Data_Length_Exceeds_Buffer_Returns_Null()
    {
        byte[] block = BuildPictureBlock(
            type: 3,
            mime: "image/jpeg",
            description: "",
            width: 0, height: 0, depth: 0, palette: 0,
            data: JpegMagic);
        // Patch the final data-length field to claim a value larger than payload.
        int dataLenOffset = block.Length - JpegMagic.Length - 4;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(dataLenOffset, 4), 100_000u);
        Assert.Null(FlacPictureBlock.TryParse(block));
    }

    [Fact]
    public void TryParse_Utf8_Description_Round_Trips()
    {
        byte[] block = BuildPictureBlock(
            type: 3,
            mime: "image/jpeg",
            description: "Forsiden — front",
            width: 0, height: 0, depth: 0, palette: 0,
            data: JpegMagic);
        var picture = FlacPictureBlock.TryParse(block);
        Assert.NotNull(picture);
        Assert.Equal("Forsiden — front", picture!.Description);
    }

    internal static byte[] BuildPictureBlock(
        uint type, string mime, string description,
        uint width, uint height, uint depth, uint palette,
        byte[] data)
    {
        byte[] mimeBytes = Encoding.ASCII.GetBytes(mime);
        byte[] descBytes = Encoding.UTF8.GetBytes(description);
        int totalLen = 4 + 4 + mimeBytes.Length + 4 + descBytes.Length + 4 * 5 + data.Length;
        byte[] block = new byte[totalLen];
        int p = 0;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), type); p += 4;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), (uint)mimeBytes.Length); p += 4;
        mimeBytes.CopyTo(block.AsSpan(p)); p += mimeBytes.Length;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), (uint)descBytes.Length); p += 4;
        descBytes.CopyTo(block.AsSpan(p)); p += descBytes.Length;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), width); p += 4;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), height); p += 4;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), depth); p += 4;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), palette); p += 4;
        BinaryPrimitives.WriteUInt32BigEndian(block.AsSpan(p, 4), (uint)data.Length); p += 4;
        data.CopyTo(block.AsSpan(p));
        return block;
    }
}

/// <summary>
/// Tests for Vorbis-comment cover-art extraction via the
/// <c>METADATA_BLOCK_PICTURE</c> (base64-encoded FLAC PICTURE block)
/// and legacy <c>COVERART</c> (raw JPEG bytes, base64-encoded) keys.
/// </summary>
public sealed class VorbisCommentPictureTests
{
    private static readonly byte[] JpegMagic = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46];

    [Fact]
    public void MetadataBlockPicture_Is_Decoded_From_Base64()
    {
        byte[] block = FlacPictureBlockTests.BuildPictureBlock(
            type: (uint)MediaPictureType.CoverFront,
            mime: "image/jpeg",
            description: "Album cover",
            width: 500, height: 500, depth: 24, palette: 0,
            data: JpegMagic);
        string base64 = Convert.ToBase64String(block);

        byte[] payload = BuildVorbisComment("MyEncoder", [
            "TITLE=Spring",
            "METADATA_BLOCK_PICTURE=" + base64,
        ]);
        var builder = new MediaMetadataBuilder();
        VorbisComment.ReadInto(payload, builder);
        var meta = builder.Build();

        Assert.Equal("Spring", meta.Title);
        Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", meta.Pictures[0].MimeType);
        Assert.Equal("Album cover", meta.Pictures[0].Description);
        Assert.False(meta.Tags.ContainsKey("METADATA_BLOCK_PICTURE"));
    }

    [Fact]
    public void Legacy_Coverart_Carries_Raw_Jpeg_Bytes()
    {
        string base64 = Convert.ToBase64String(JpegMagic);
        byte[] payload = BuildVorbisComment("MyEncoder", [
            "COVERART=" + base64,
        ]);
        var builder = new MediaMetadataBuilder();
        VorbisComment.ReadInto(payload, builder);
        var meta = builder.Build();

        Assert.Single(meta.Pictures);
        Assert.Equal("image/jpeg", meta.Pictures[0].MimeType);
        Assert.Equal(JpegMagic, meta.Pictures[0].Data.ToArray());
    }

    [Fact]
    public void Malformed_Base64_Is_Silently_Dropped()
    {
        byte[] payload = BuildVorbisComment("MyEncoder", [
            "TITLE=Spring",
            "METADATA_BLOCK_PICTURE=NOT-VALID-BASE64!!!",
        ]);
        var builder = new MediaMetadataBuilder();
        VorbisComment.ReadInto(payload, builder);
        var meta = builder.Build();

        Assert.Equal("Spring", meta.Title);
        Assert.Empty(meta.Pictures);
    }

    private static byte[] BuildVorbisComment(string vendor, string[] comments)
    {
        byte[] vendorBytes = Encoding.UTF8.GetBytes(vendor);
        var entries = new byte[comments.Length][];
        int totalLen = 4 + vendorBytes.Length + 4;
        for (int i = 0; i < comments.Length; i++)
        {
            entries[i] = Encoding.UTF8.GetBytes(comments[i]);
            totalLen += 4 + entries[i].Length;
        }
        byte[] payload = new byte[totalLen];
        int p = 0;
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(p, 4), (uint)vendorBytes.Length); p += 4;
        vendorBytes.CopyTo(payload.AsSpan(p)); p += vendorBytes.Length;
        BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(p, 4), (uint)entries.Length); p += 4;
        for (int i = 0; i < entries.Length; i++)
        {
            BinaryPrimitives.WriteUInt32LittleEndian(payload.AsSpan(p, 4), (uint)entries[i].Length); p += 4;
            entries[i].CopyTo(payload.AsSpan(p)); p += entries[i].Length;
        }
        return payload;
    }
}
