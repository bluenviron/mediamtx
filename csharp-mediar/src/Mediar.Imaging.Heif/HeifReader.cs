using System.Buffers.Binary;
using System.Collections.Frozen;
using System.Collections.Immutable;
using System.Text;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Reader for ISO/IEC 23008-12 image-format files: HEIF, HEIC, AVIF, and the
/// Canon CR3 raw container (which shares the same outer ISO-BMFF box structure).
/// </summary>
/// <remarks>
/// <para>
/// The reader does a complete pass over the box tree and surfaces every
/// item-level construct defined in the HEIF specification: the file-type box
/// (<c>ftyp</c>), handler, primary item, item info entries (<c>iinf</c>),
/// item locations (<c>iloc</c>), references (<c>iref</c>), property
/// associations (<c>iprp</c>/<c>ipma</c>) and the property container
/// (<c>ipco</c>) with <c>ispe</c>, <c>pixi</c>, <c>pasp</c>, <c>colr</c>,
/// <c>auxC</c>, <c>irot</c>, and <c>imir</c>.
/// </para>
/// <para>
/// HEVC, AV1, and VVC bitstream decoding are out of scope for this Mediar
/// release. <see cref="ReadFramesAsync"/> throws
/// <see cref="NotSupportedException"/> for any item that resolves to one of
/// those codecs; the container metadata is still fully exposed.
/// </para>
/// </remarks>
public sealed class HeifReader : IImageReader
{
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private bool _disposed;
    private readonly byte[] _fileBytes;
    private readonly ReadOnlyMemory<byte> _idatBytes;
    private readonly FrozenDictionary<uint, byte> _constructionMethods;

    /// <inheritdoc/>
    public ImageFormat Format { get; }
    /// <inheritdoc/>
    public ImageInfo Info { get; }
    /// <inheritdoc/>
    public ImageMetadata Metadata { get; }
    /// <inheritdoc/>
    public bool CanDecodePixels => false;

    /// <summary>Major brand from the <c>ftyp</c> box (e.g. <c>"heic"</c>, <c>"mif1"</c>, <c>"avif"</c>, <c>"crx "</c>).</summary>
    public string MajorBrand { get; }

    /// <summary>Compatible brands declared by <c>ftyp</c>.</summary>
    public ImmutableArray<string> CompatibleBrands { get; }

    /// <summary>
    /// Typed view over the <c>ftyp</c> brand list: codec, container kind,
    /// multilayer / range-extended / tone-mapped / Apple multi-image flags.
    /// </summary>
    public HeifBrandInfo BrandInfo { get; }

    /// <summary>Primary item id (from <c>pitm</c>); 0 if none.</summary>
    public uint PrimaryItemId { get; }

    /// <summary>All items declared by the meta box.</summary>
    public ImmutableArray<HeifItem> Items { get; }

    /// <summary>Decoded property container (<c>ipco</c>).</summary>
    public ImmutableArray<HeifProperty> Properties { get; }

    /// <summary>Property associations (<c>ipma</c>): item id -> property indices into <see cref="Properties"/>.</summary>
    public FrozenDictionary<uint, ImmutableArray<int>> Associations { get; }

    /// <summary>Item references (<c>iref</c>): grouped by reference type.</summary>
    public ImmutableArray<HeifReference> References { get; }

    /// <summary>
    /// Typed lookup over <see cref="References"/> that resolves the
    /// thumbnail / derivation / auxiliary / metadata graph without
    /// the caller needing to iterate the flat reference table.
    /// </summary>
    public HeifReferenceGraph ReferenceGraph { get; }

    /// <summary>
    /// Thumbnails of the <see cref="PrimaryItemId"/>, in declaration
    /// order. Empty when the file declares no thumbnails or no
    /// primary item.
    /// </summary>
    public ImmutableArray<uint> PrimaryThumbnailIds =>
        PrimaryItemId == 0 ? [] : ReferenceGraph.GetThumbnailsFor(PrimaryItemId);

    /// <summary>
    /// Auxiliary items (alpha, depth, HDR layer, etc.) of the
    /// <see cref="PrimaryItemId"/>, in declaration order.
    /// </summary>
    public ImmutableArray<uint> PrimaryAuxiliaryIds =>
        PrimaryItemId == 0 ? [] : ReferenceGraph.GetAuxiliariesFor(PrimaryItemId);

    private HeifReader(Stream s, bool owns, ImageFormat fmt, ImageInfo info, ImageMetadata meta,
                       string majorBrand, ImmutableArray<string> compat, uint primary,
                       ImmutableArray<HeifItem> items, ImmutableArray<HeifProperty> props,
                       FrozenDictionary<uint, ImmutableArray<int>> assoc, ImmutableArray<HeifReference> refs,
                       byte[] fileBytes, ReadOnlyMemory<byte> idatBytes,
                       FrozenDictionary<uint, byte> constructionMethods)
    {
        _stream = s; _ownsStream = owns;
        Format = fmt; Info = info; Metadata = meta;
        MajorBrand = majorBrand; CompatibleBrands = compat;
        BrandInfo = HeifBrandInfo.From(majorBrand, compat);
        PrimaryItemId = primary; Items = items; Properties = props;
        Associations = assoc; References = refs;
        ReferenceGraph = new HeifReferenceGraph(refs);
        _fileBytes = fileBytes;
        _idatBytes = idatBytes;
        _constructionMethods = constructionMethods;
    }

    /// <summary>Open a HEIF/HEIC/AVIF/CR3 file from a path.</summary>
    public static HeifReader Open(string path)
    {
        var fs = new FileStream(path, FileMode.Open, FileAccess.Read, FileShare.Read, 4096, FileOptions.SequentialScan);
        try { return Open(fs, ImageFormat.Heif, ownsStream: true); }
        catch { fs.Dispose(); throw; }
    }

    /// <summary>Open a HEIF/HEIC/AVIF/CR3 file from a stream.</summary>
    public static HeifReader Open(Stream stream, ImageFormat expected = ImageFormat.Heif, bool ownsStream = false)
    {
        ArgumentNullException.ThrowIfNull(stream);
        using var ms = new MemoryStream();
        stream.CopyTo(ms);
        byte[] bytes = ms.ToArray();

        var parsed = Parse(bytes, expected);
        return new HeifReader(stream, ownsStream, parsed.Format, parsed.Info, parsed.Metadata,
                              parsed.MajorBrand, parsed.CompatibleBrands, parsed.PrimaryItemId,
                              parsed.Items, parsed.Properties, parsed.Associations, parsed.References,
                              bytes, parsed.IdatBytes, parsed.ConstructionMethods);
    }

    /// <inheritdoc/>
    public IAsyncEnumerable<ImageFrame> ReadFramesAsync(CancellationToken cancellationToken = default) =>
        throw new NotSupportedException(
            $"Pixel decoding for {Format} requires an HEVC / AV1 / VVC decoder which is not " +
            "included in this Mediar release. The full HEIF container, item-list, property table, " +
            "color profile, and transform metadata are exposed for inspection.");

    /// <summary>
    /// Construction method (per ISO/IEC 14496-12 § 8.11.3) for the given item:
    /// 0 = file_offset (resolve via iloc extents against the raw file),
    /// 1 = idat_offset (resolve via iloc extents against the <c>idat</c> box),
    /// 2 = item_offset (chained item, not supported). Returns 0 when the
    /// item is unknown or the iloc box used version 0 (which has no
    /// construction_method field and is always interpreted as file_offset).
    /// </summary>
    public byte GetConstructionMethod(uint itemId) =>
        _constructionMethods.TryGetValue(itemId, out byte m) ? m : (byte)0;

    /// <summary>
    /// Resolves the raw bytes of <paramref name="itemId"/> by joining the
    /// extents declared in the iloc box. Supports construction methods 0
    /// (file offset) and 1 (idat offset). Returns <c>false</c> for unknown
    /// items, item_offset chained items (method 2), or extents that exceed
    /// the underlying buffer.
    /// </summary>
    public bool TryGetItemData(uint itemId, out ReadOnlyMemory<byte> data)
    {
        data = default;
        int idx = -1;
        for (int i = 0; i < Items.Length; i++)
        {
            if (Items[i].Id == itemId) { idx = i; break; }
        }
        if (idx < 0) return false;

        var item = Items[idx];
        byte method = GetConstructionMethod(itemId);
        if (method == 2) return false;

        var source = method == 1 ? _idatBytes : _fileBytes.AsMemory();
        if (source.IsEmpty && item.Extents.Length > 0) return false;

        int totalLen = 0;
        foreach (var ext in item.Extents)
        {
            totalLen = checked(totalLen + (int)ext.Length);
        }

        if (totalLen == 0) return true;
        var buffer = new byte[totalLen];
        int written = 0;
        foreach (var ext in item.Extents)
        {
            ulong absOffset = method == 1 ? ext.Offset : item.Location.BaseOffset + ext.Offset;
            if (absOffset > (ulong)source.Length) return false;
            ulong endOffset = absOffset + ext.Length;
            if (endOffset > (ulong)source.Length) return false;
            source.Slice((int)absOffset, (int)ext.Length).CopyTo(buffer.AsMemory(written));
            written += (int)ext.Length;
        }
        data = buffer;
        return true;
    }

    /// <summary>
    /// Resolves a <c>grid</c> item (HEIF tile composition) into a typed
    /// <see cref="HeifGridDerivation"/>. Returns <c>false</c> when the item
    /// is not a grid, its data cannot be resolved, or the payload is
    /// malformed.
    /// </summary>
    public bool TryGetGridDerivation(uint itemId, out HeifGridDerivation derivation)
    {
        derivation = null!;
        if (!TryGetItemTypedData(itemId, "grid", out var data)) return false;
        return HeifGridDerivation.TryParse(data.Span, out derivation);
    }

    /// <summary>
    /// Resolves an <c>iovl</c> item (HEIF overlay composition) into a typed
    /// <see cref="HeifOverlayDerivation"/>. Returns <c>false</c> when the
    /// item is not an overlay, its data cannot be resolved, or the payload
    /// is malformed.
    /// </summary>
    public bool TryGetOverlayDerivation(uint itemId, out HeifOverlayDerivation derivation)
    {
        derivation = null!;
        if (!TryGetItemTypedData(itemId, "iovl", out var data)) return false;
        // Overlay payload size depends on the number of dimg sources.
        int refCount = ReferenceGraph.GetDerivedSourcesOf(itemId).Length;
        return HeifOverlayDerivation.TryParse(data.Span, refCount, out derivation);
    }

    /// <summary>
    /// Returns <c>true</c> when <paramref name="itemId"/> is an <c>iden</c>
    /// identity derivation. An identity item simply re-presents its single
    /// <c>dimg</c> source with the property transforms (irot/imir/clap)
    /// associated to the iden item itself.
    /// </summary>
    public bool IsIdentityDerivation(uint itemId)
    {
        foreach (var it in Items)
        {
            if (it.Id == itemId) return it.Type == "iden";
        }
        return false;
    }

    /// <summary>
    /// Resolves the <c>clli</c> (Content Light Level) HDR property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifContentLightLevel"/>. Returns <c>false</c> when
    /// no <c>clli</c> property is associated or the payload is malformed.
    /// </summary>
    public bool TryGetContentLightLevel(uint itemId, out HeifContentLightLevel info)
    {
        info = null!;
        if (!TryGetPropertyBytes(itemId, "clli", out var data)) return false;
        return HeifContentLightLevel.TryParse(data.Span, out info);
    }

    /// <summary>
    /// Resolves the <c>mdcv</c> (Mastering Display Colour Volume) HDR
    /// property associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifMasteringDisplayColourVolume"/>.
    /// </summary>
    public bool TryGetMasteringDisplayColourVolume(uint itemId, out HeifMasteringDisplayColourVolume info)
    {
        info = null!;
        if (!TryGetPropertyBytes(itemId, "mdcv", out var data)) return false;
        return HeifMasteringDisplayColourVolume.TryParse(data.Span, out info);
    }

    /// <summary>
    /// Resolves the <c>clap</c> (Clean Aperture) cropping property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifCleanAperture"/>.
    /// </summary>
    public bool TryGetCleanAperture(uint itemId, out HeifCleanAperture info)
    {
        info = null!;
        if (!TryGetPropertyBytes(itemId, "clap", out var data)) return false;
        return HeifCleanAperture.TryParse(data.Span, out info);
    }

    /// <summary>
    /// Resolves the <c>av1C</c> (AV1 Codec Configuration) property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="Av1CodecConfigurationRecord"/> per the AV1 ISOBMFF
    /// binding §2.3. Use this on AVIF / AVIF-tiled images to learn the
    /// AV1 sequence profile, level, tier, bit depth, and chroma format
    /// without re-decoding the bitstream.
    /// </summary>
    public bool TryGetAv1CodecConfiguration(uint itemId, out Av1CodecConfigurationRecord record)
    {
        record = null!;
        if (!TryGetPropertyBytes(itemId, "av1C", out var data)) return false;
        return Av1CodecConfigurationRecord.TryParse(data.Span, out record);
    }

    /// <summary>
    /// Resolves the <c>hvcC</c> property associated with <paramref name="itemId"/>
    /// into a typed <see cref="HevcCodecConfigurationRecord"/> per ISO/IEC
    /// 14496-15 §8.3.3.1.2. Use this on HEIC / HEIF images to learn the
    /// HEVC profile / tier / level, chroma format, bit depth, and the
    /// VPS / SPS / PPS parameter sets without re-decoding the bitstream.
    /// </summary>
    public bool TryGetHevcCodecConfiguration(uint itemId, out HevcCodecConfigurationRecord record)
    {
        record = null!;
        if (!TryGetPropertyBytes(itemId, "hvcC", out var data)) return false;
        return HevcCodecConfigurationRecord.TryParse(data.Span, out record);
    }

    /// <summary>
    /// Resolves the <c>udes</c> property associated with <paramref name="itemId"/>
    /// into a typed <see cref="HeifUserDescription"/> per ISO/IEC 23008-12
    /// §6.5.20. Use this to surface author-provided language tag, name,
    /// description, and free-form tags without re-walking the property
    /// boxes.
    /// </summary>
    public bool TryGetUserDescription(uint itemId, out HeifUserDescription record)
    {
        record = null!;
        if (!TryGetPropertyBytes(itemId, "udes", out var data)) return false;
        return HeifUserDescription.TryParse(data.Span, out record);
    }

    /// <summary>
    /// Resolves the <c>vvcC</c> property associated with <paramref name="itemId"/>
    /// into a typed <see cref="VvcCodecConfigurationRecord"/> per ISO/IEC
    /// 14496-15 §11.2.4.2. Use this on VVC-encoded HEIF images (brand
    /// <c>vvic</c>) to learn the operating-point index, Profile / Tier /
    /// Level, chroma format, bit depth, maximum picture dimensions, and
    /// the VPS / SPS / PPS / DCI / OPI parameter sets without re-decoding
    /// the bitstream.
    /// </summary>
    public bool TryGetVvcCodecConfiguration(uint itemId, out VvcCodecConfigurationRecord record)
    {
        record = null!;
        if (!TryGetPropertyBytes(itemId, "vvcC", out var data)) return false;
        return VvcCodecConfigurationRecord.TryParse(data.Span, out record);
    }

    /// <summary>
    /// Resolves the <c>irot</c> rotation property associated with
    /// <paramref name="itemId"/> into a typed
    /// <see cref="HeifImageRotation"/>. Returns <c>false</c> when no
    /// <c>irot</c> property is associated with the item.
    /// </summary>
    public bool TryGetImageRotation(uint itemId, out HeifImageRotation rotation)
    {
        rotation = HeifImageRotation.None;
        if (!Associations.TryGetValue(itemId, out var indices)) return false;
        foreach (int idx in indices)
        {
            if (idx <= 0 || idx > Properties.Length) continue;
            var prop = Properties[idx - 1];
            if (prop.Type == "irot")
            {
                rotation = prop.A switch
                {
                    90 => HeifImageRotation.Rotate90Ccw,
                    180 => HeifImageRotation.Rotate180,
                    270 => HeifImageRotation.Rotate270Ccw,
                    _ => HeifImageRotation.None,
                };
                return true;
            }
        }
        return false;
    }

    /// <summary>
    /// Resolves the <c>imir</c> mirror property associated with
    /// <paramref name="itemId"/> into a typed
    /// <see cref="HeifImageMirrorAxis"/>. Returns <c>false</c> when no
    /// <c>imir</c> property is associated with the item.
    /// </summary>
    public bool TryGetImageMirror(uint itemId, out HeifImageMirrorAxis axis)
    {
        axis = HeifImageMirrorAxis.Vertical;
        if (!Associations.TryGetValue(itemId, out var indices)) return false;
        foreach (int idx in indices)
        {
            if (idx <= 0 || idx > Properties.Length) continue;
            var prop = Properties[idx - 1];
            if (prop.Type == "imir")
            {
                axis = prop.A == 1 ? HeifImageMirrorAxis.Horizontal : HeifImageMirrorAxis.Vertical;
                return true;
            }
        }
        return false;
    }

    /// <summary>
    /// Resolves the <c>pasp</c> pixel aspect ratio property associated
    /// with <paramref name="itemId"/> into a typed
    /// <see cref="HeifPixelAspectRatio"/>. Returns <c>false</c> when no
    /// <c>pasp</c> property is associated with the item.
    /// </summary>
    public bool TryGetPixelAspectRatio(uint itemId, out HeifPixelAspectRatio aspect)
    {
        aspect = null!;
        if (!Associations.TryGetValue(itemId, out var indices)) return false;
        foreach (int idx in indices)
        {
            if (idx <= 0 || idx > Properties.Length) continue;
            var prop = Properties[idx - 1];
            if (prop.Type == "pasp")
            {
                aspect = new HeifPixelAspectRatio
                {
                    HorizontalSpacing = (uint)prop.C,
                    VerticalSpacing = (uint)prop.D,
                };
                return true;
            }
        }
        return false;
    }

    /// <summary>
    /// Resolves the <c>pixi</c> pixel information property associated
    /// with <paramref name="itemId"/> into a typed
    /// <see cref="HeifPixelInformation"/> exposing the per-channel
    /// bit-depth array. Returns <c>false</c> when no <c>pixi</c>
    /// property is associated or the payload is malformed.
    /// </summary>
    public bool TryGetPixelInformation(uint itemId, out HeifPixelInformation info)
    {
        info = null!;
        if (!TryGetPropertyBytes(itemId, "pixi", out var data)) return false;
        return HeifPixelInformation.TryParse(data.Span, out info);
    }

    /// <summary>
    /// Resolves the <c>auxC</c> auxiliary type property associated with
    /// <paramref name="itemId"/> into a typed
    /// <see cref="HeifAuxiliaryType"/> exposing the aux URN and any
    /// trailing subtype bytes. Returns <c>false</c> when no <c>auxC</c>
    /// property is associated or the payload is malformed.
    /// </summary>
    public bool TryGetAuxiliaryType(uint itemId, out HeifAuxiliaryType type)
    {
        type = null!;
        if (!TryGetPropertyBytes(itemId, "auxC", out var data)) return false;
        return HeifAuxiliaryType.TryParse(data.Span, out type);
    }

    /// <summary>
    /// Resolves the <c>a1op</c> AV1 operating-point selector property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifAv1OperatingPoint"/>. Returns <c>false</c> when
    /// no <c>a1op</c> property is associated with the item.
    /// </summary>
    public bool TryGetAv1OperatingPoint(uint itemId, out HeifAv1OperatingPoint op)
    {
        op = null!;
        if (!TryGetPropertyBytes(itemId, "a1op", out var data)) return false;
        return HeifAv1OperatingPoint.TryParse(data.Span, out op);
    }

    /// <summary>
    /// Resolves the <c>a1lx</c> AV1 layered-image-indexing property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifAv1LayeredImageIndexing"/>. Returns <c>false</c>
    /// when no <c>a1lx</c> property is associated or the payload is
    /// malformed.
    /// </summary>
    public bool TryGetAv1LayeredImageIndexing(uint itemId, out HeifAv1LayeredImageIndexing rec)
    {
        rec = null!;
        if (!TryGetPropertyBytes(itemId, "a1lx", out var data)) return false;
        return HeifAv1LayeredImageIndexing.TryParse(data.Span, out rec);
    }

    /// <summary>
    /// Resolves the <c>cclv</c> Content Colour Volume property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifContentColourVolume"/>. Returns <c>false</c>
    /// when no <c>cclv</c> property is associated or the payload is
    /// malformed.
    /// </summary>
    public bool TryGetContentColourVolume(uint itemId, out HeifContentColourVolume volume)
    {
        volume = null!;
        if (!TryGetPropertyBytes(itemId, "cclv", out var data)) return false;
        return HeifContentColourVolume.TryParse(data.Span, out volume);
    }

    /// <summary>
    /// Resolves the <c>lsel</c> Layer Selector property associated
    /// with <paramref name="itemId"/> into a typed
    /// <see cref="HeifLayerSelector"/>. Returns <c>false</c> when no
    /// <c>lsel</c> property is associated or the payload is
    /// malformed.
    /// </summary>
    public bool TryGetLayerSelector(uint itemId, out HeifLayerSelector selector)
    {
        selector = null!;
        if (!TryGetPropertyBytes(itemId, "lsel", out var data)) return false;
        return HeifLayerSelector.TryParse(data.Span, out selector);
    }

    /// <summary>
    /// Resolves the <c>rref</c> Required Reference Types property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifRequiredReference"/>. Returns <c>false</c> when
    /// no <c>rref</c> property is associated or the payload is
    /// malformed.
    /// </summary>
    public bool TryGetRequiredReference(uint itemId, out HeifRequiredReference required)
    {
        required = null!;
        if (!TryGetPropertyBytes(itemId, "rref", out var data)) return false;
        return HeifRequiredReference.TryParse(data.Span, out required);
    }

    /// <summary>
    /// Resolves the <c>jpgC</c> JPEG Codec Configuration property
    /// associated with <paramref name="itemId"/> into a typed
    /// <see cref="HeifJpegConfiguration"/> exposing the shared JPEG
    /// prefix bytes that must be prepended to the item payload to
    /// form a complete JPEG bitstream. Returns <c>false</c> when no
    /// <c>jpgC</c> property is associated or the payload is empty.
    /// </summary>
    public bool TryGetJpegConfiguration(uint itemId, out HeifJpegConfiguration config)
    {
        config = null!;
        if (!TryGetPropertyBytes(itemId, "jpgC", out var data)) return false;
        return HeifJpegConfiguration.TryParse(data.Span, out config);
    }

    /// <summary>
    /// Decode the L-HEVC <c>tols</c> Target Output Layer Set
    /// property associated with the given item, when present. The
    /// returned index references an operating point declared by the
    /// companion <c>oinf</c> property. Returns false when no
    /// <c>tols</c> property is associated or the payload is malformed.
    /// </summary>
    public bool TryGetTargetOutputLayerSet(uint itemId, out HeifTargetOutputLayerSet tols)
    {
        tols = null!;
        if (!TryGetPropertyBytes(itemId, "tols", out var data)) return false;
        if (!HeifTargetOutputLayerSet.TryParse(data.Span, out var parsed) || parsed is null) return false;
        tols = parsed;
        return true;
    }

    /// <summary>
    /// Decode the L-HEVC <c>oinf</c> Operating Points Information
    /// property associated with the given item, when present. The
    /// returned record carries the scalability mask, profile-tier-
    /// level entries, operating points and layer dependency graph.
    /// Returns false when no <c>oinf</c> property is associated or
    /// the payload is structurally inconsistent.
    /// </summary>
    public bool TryGetOperatingPointsInformation(uint itemId, out HeifOperatingPointsInformation oinf)
    {
        oinf = null!;
        if (!TryGetPropertyBytes(itemId, "oinf", out var data)) return false;
        if (!HeifOperatingPointsInformation.TryParse(data.Span, out var parsed) || parsed is null) return false;
        oinf = parsed;
        return true;
    }

    private bool TryGetPropertyBytes(uint itemId, string type, out ReadOnlyMemory<byte> data)
    {
        data = default;
        if (!Associations.TryGetValue(itemId, out var indices)) return false;
        foreach (int idx in indices)
        {
            if (idx <= 0 || idx > Properties.Length) continue;
            var prop = Properties[idx - 1];
            if (prop.Type == type && !prop.IccProfile.IsEmpty)
            {
                data = prop.IccProfile;
                return true;
            }
        }
        return false;
    }

    private bool TryGetItemTypedData(uint itemId, string expectedType, out ReadOnlyMemory<byte> data)
    {
        data = default;
        HeifItem? match = null;
        foreach (var it in Items)
        {
            if (it.Id == itemId) { match = it; break; }
        }
        if (match is null || match.Type != expectedType) return false;
        return TryGetItemData(itemId, out data);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsStream) _stream.Dispose();
    }

    // ---------------- parser ----------------

    private sealed class ParseResult
    {
        public ImageFormat Format;
        public ImageInfo Info;
        public ImageMetadata Metadata = ImageMetadata.Empty;
        public string MajorBrand = "";
        public ImmutableArray<string> CompatibleBrands = ImmutableArray<string>.Empty;
        public uint PrimaryItemId;
        public ImmutableArray<HeifItem> Items = ImmutableArray<HeifItem>.Empty;
        public ImmutableArray<HeifProperty> Properties = ImmutableArray<HeifProperty>.Empty;
        public FrozenDictionary<uint, ImmutableArray<int>> Associations = FrozenDictionary<uint, ImmutableArray<int>>.Empty;
        public ImmutableArray<HeifReference> References = ImmutableArray<HeifReference>.Empty;
        public ReadOnlyMemory<byte> IdatBytes;
        public FrozenDictionary<uint, byte> ConstructionMethods = FrozenDictionary<uint, byte>.Empty;
    }

    private static ParseResult Parse(byte[] b, ImageFormat expected)
    {
        var r = new ParseResult { Format = expected };
        var items = new List<HeifItem>();
        var props = new List<HeifProperty>();
        var assoc = new Dictionary<uint, ImmutableArray<int>>();
        var refs = new List<HeifReference>();
        var ctorMethods = new Dictionary<uint, byte>();

        WalkTop(b, 0, b.Length);

        void WalkTop(byte[] buf, int start, int end)
        {
            int p = start;
            while (p + 8 <= end)
            {
                if (!ReadBoxHeader(buf, p, end, out string ty, out int contentStart, out int contentLen, out int total)) break;
                if (ty == "ftyp")
                {
                    ParseFtyp(buf, contentStart, contentLen);
                }
                else if (ty == "meta")
                {
                    // FullBox: 4 bytes version/flags then children
                    WalkMeta(buf, contentStart + 4, contentStart + contentLen);
                }
                p += total;
            }
        }

        void ParseFtyp(byte[] buf, int s, int len)
        {
            if (len < 8) return;
            r.MajorBrand = Encoding.ASCII.GetString(buf, s, 4);
            var compat = ImmutableArray.CreateBuilder<string>();
            for (int q = s + 8; q + 4 <= s + len; q += 4)
                compat.Add(Encoding.ASCII.GetString(buf, q, 4));
            r.CompatibleBrands = compat.ToImmutable();
            // Refine the format from the brand if caller passed Heif.
            if (r.Format is ImageFormat.Heif or ImageFormat.Unknown)
            {
                r.Format = r.MajorBrand switch
                {
                    "heic" or "heix" or "hevc" or "hevx" => ImageFormat.Heic,
                    "avif" or "avis" => ImageFormat.Avif,
                    "crx " => ImageFormat.Cr3,
                    "mif1" or "msf1" or "heim" or "heis" or "mif2" or "mif3" or "tmap" or "unif" or "vvic" or "vvis" => ImageFormat.Heif,
                    _ => r.Format,
                };
            }
        }

        void WalkMeta(byte[] buf, int s, int end)
        {
            int p = s;
            while (p + 8 <= end)
            {
                if (!ReadBoxHeader(buf, p, end, out string ty, out int cs, out int cl, out int tot)) break;
                switch (ty)
                {
                    case "hdlr": /* parsed but not exposed individually */ break;
                    case "pitm": ParsePitm(buf, cs, cl); break;
                    case "iinf": ParseIinf(buf, cs, cl); break;
                    case "iloc": ParseIloc(buf, cs, cl); break;
                    case "iref": ParseIref(buf, cs, cl); break;
                    case "iprp": ParseIprp(buf, cs, cs + cl); break;
                    case "idat": r.IdatBytes = new ReadOnlyMemory<byte>(buf, cs, cl); break;
                }
                p += tot;
            }
        }

        void ParsePitm(byte[] buf, int s, int len)
        {
            if (len < 6) return;
            byte version = buf[s];
            int p = s + 4;
            r.PrimaryItemId = version == 0
                ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p))
                : BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
        }

        void ParseIinf(byte[] buf, int s, int len)
        {
            int p = s;
            byte version = buf[p]; p += 4;
            int count = version == 0
                ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p))
                : (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
            p += version == 0 ? 2 : 4;
            for (int i = 0; i < count && p + 8 <= s + len; i++)
            {
                if (!ReadBoxHeader(buf, p, s + len, out string ty, out int cs, out int cl, out int tot)) break;
                if (ty == "infe") ParseInfe(buf, cs, cl);
                p += tot;
            }
        }

        void ParseInfe(byte[] buf, int s, int len)
        {
            if (len < 4) return;
            byte version = buf[s];
            int p = s + 4;
            uint id;
            if (version >= 2)
            {
                id = version == 2
                    ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p))
                    : BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
                p += version == 2 ? 2 : 4;
                p += 2;  // protection_index
                string type = Encoding.ASCII.GetString(buf, p, 4);
                p += 4;
                string name = ReadCString(buf, ref p, s + len);
                items.Add(new HeifItem(id, type, name, default, ImmutableArray<HeifItemExtent>.Empty));
            }
            else
            {
                id = BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p));
                p += 4;
                string name = ReadCString(buf, ref p, s + len);
                items.Add(new HeifItem(id, "", name, default, ImmutableArray<HeifItemExtent>.Empty));
            }
        }

        void ParseIloc(byte[] buf, int s, int len)
        {
            int p = s;
            byte version = buf[p];
            p += 4;
            byte b1 = buf[p++];
            byte b2 = buf[p++];
            int offsetSize = (b1 >> 4) & 0xF;
            int lengthSize = b1 & 0xF;
            int baseOffsetSize = (b2 >> 4) & 0xF;
            int indexSize = version > 0 ? (b2 & 0xF) : 0;
            int itemCount = version < 2
                ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p))
                : (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
            p += version < 2 ? 2 : 4;
            for (int i = 0; i < itemCount; i++)
            {
                uint itemId = version < 2
                    ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p))
                    : BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
                p += version < 2 ? 2 : 4;
                if (version > 0)
                {
                    // 12 bits reserved + 4 bits construction_method.
                    ushort word = BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p));
                    p += 2;
                    ctorMethods[itemId] = (byte)(word & 0x0F);
                }
                ushort dataRefIndex = BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p)); p += 2;
                ulong baseOffset = ReadUInt(buf, ref p, baseOffsetSize);
                int extentCount = BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p)); p += 2;
                var extents = ImmutableArray.CreateBuilder<HeifItemExtent>(extentCount);
                for (int e = 0; e < extentCount; e++)
                {
                    ulong extentIndex = ReadUInt(buf, ref p, indexSize);
                    ulong extentOffset = ReadUInt(buf, ref p, offsetSize);
                    ulong extentLength = ReadUInt(buf, ref p, lengthSize);
                    extents.Add(new HeifItemExtent(extentIndex, extentOffset, extentLength));
                }
                // patch existing item if we already saw infe
                int idx = items.FindIndex(it => it.Id == itemId);
                if (idx >= 0)
                {
                    var prev = items[idx];
                    items[idx] = prev with { Location = new HeifItemLocation(baseOffset, dataRefIndex), Extents = extents.ToImmutable() };
                }
                else
                {
                    items.Add(new HeifItem(itemId, "", "", new HeifItemLocation(baseOffset, dataRefIndex), extents.ToImmutable()));
                }
            }
        }

        void ParseIref(byte[] buf, int s, int len)
        {
            int p = s;
            byte version = buf[p];
            p += 4;
            while (p + 8 <= s + len)
            {
                if (!ReadBoxHeader(buf, p, s + len, out string ty, out int cs, out int cl, out int tot)) break;
                int q = cs;
                uint fromId = version == 0
                    ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(q))
                    : BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(q));
                q += version == 0 ? 2 : 4;
                int n = BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(q)); q += 2;
                var tos = ImmutableArray.CreateBuilder<uint>(n);
                for (int i = 0; i < n; i++)
                {
                    uint toId = version == 0
                        ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(q))
                        : BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(q));
                    q += version == 0 ? 2 : 4;
                    tos.Add(toId);
                }
                refs.Add(new HeifReference(ty, fromId, tos.ToImmutable()));
                p += tot;
            }
        }

        void ParseIprp(byte[] buf, int s, int end)
        {
            int p = s;
            while (p + 8 <= end)
            {
                if (!ReadBoxHeader(buf, p, end, out string ty, out int cs, out int cl, out int tot)) break;
                if (ty == "ipco") ParseIpco(buf, cs, cs + cl);
                else if (ty == "ipma") ParseIpma(buf, cs, cl);
                p += tot;
            }
        }

        void ParseIpco(byte[] buf, int s, int end)
        {
            int p = s;
            while (p + 8 <= end)
            {
                if (!ReadBoxHeader(buf, p, end, out string ty, out int cs, out int cl, out int tot)) break;
                props.Add(ParseProperty(ty, buf, cs, cl));
                p += tot;
            }
        }

        void ParseIpma(byte[] buf, int s, int len)
        {
            int p = s;
            byte version = buf[p];
            byte flags = buf[p + 3];
            p += 4;
            int n = (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p)); p += 4;
            for (int i = 0; i < n && p + 4 <= s + len; i++)
            {
                uint id = version < 1
                    ? BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p))
                    : BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
                p += version < 1 ? 2 : 4;
                byte assocCount = buf[p++];
                var arr = ImmutableArray.CreateBuilder<int>(assocCount);
                for (int a = 0; a < assocCount; a++)
                {
                    int propIdx;
                    if ((flags & 1) == 1)
                    {
                        ushort word = BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p));
                        p += 2;
                        propIdx = word & 0x7FFF;
                    }
                    else
                    {
                        byte byteVal = buf[p++];
                        propIdx = byteVal & 0x7F;
                    }
                    arr.Add(propIdx);
                }
                assoc[id] = arr.ToImmutable();
            }
        }

        static HeifProperty ParseProperty(string ty, byte[] buf, int s, int len)
        {
            switch (ty)
            {
                case "ispe" when len >= 12:
                    return new HeifProperty(ty,
                        (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(s + 4)),
                        (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(s + 8)),
                        0, 0, "", default);
                case "pixi" when len >= 5:
                    {
                        int channels = buf[s + 4];
                        int bitsTotal = 0;
                        for (int i = 0; i < channels && 5 + i < len; i++) bitsTotal += buf[s + 5 + i];
                        var raw = buf.AsSpan(s, len).ToArray();
                        return new HeifProperty(ty, channels, bitsTotal, 0, 0, "", raw);
                    }
                case "pasp" when len >= 8:
                    return new HeifProperty(ty, 0, 0,
                        (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(s)),
                        (int)BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(s + 4)),
                        "", default);
                case "irot" when len >= 1:
                    return new HeifProperty(ty, (buf[s] & 3) * 90, 0, 0, 0, "rot", default);
                case "imir" when len >= 1:
                    return new HeifProperty(ty, buf[s] & 1, 0, 0, 0, "mir", default);
                case "auxC":
                    {
                        int q = s + 4;
                        string auxType = ReadCString(buf, ref q, s + len);
                        var raw = buf.AsSpan(s, len).ToArray();
                        return new HeifProperty(ty, 0, 0, 0, 0, auxType, raw);
                    }
                case "colr" when len >= 4:
                    {
                        string ct = Encoding.ASCII.GetString(buf, s, 4);
                        if (ct == "prof" || ct == "rICC")
                        {
                            var icc = buf.AsSpan(s + 4, len - 4).ToArray();
                            return new HeifProperty(ty, 0, 0, 0, 0, ct, icc);
                        }
                        return new HeifProperty(ty, 0, 0, 0, 0, ct, default);
                    }
                case "clli" when len >= 4:
                case "mdcv" when len >= 24:
                case "clap" when len >= 32:
                case "av1C" when len >= 4:
                case "hvcC" when len >= 23:
                case "udes" when len >= 4:
                case "vvcC" when len >= 4:
                case "a1op" when len >= 1:
                case "a1lx" when len >= 1:
                case "cclv" when len >= 5:
                case "lsel" when len >= 2:
                case "rref" when len >= 5:
                case "jpgC" when len >= 1:
                case "tols" when len >= 6:
                case "oinf" when len >= 10:
                    {
                        var raw = buf.AsSpan(s, len).ToArray();
                        return new HeifProperty(ty, 0, 0, 0, 0, "", raw);
                    }
                default:
                    return new HeifProperty(ty, 0, 0, 0, 0, "", default);
            }
        }

        // ----- finalize -----
        r.Items = items.ToImmutableArray();
        r.Properties = props.ToImmutableArray();
        r.Associations = assoc.ToFrozenDictionary();
        r.References = refs.ToImmutableArray();
        r.ConstructionMethods = ctorMethods.ToFrozenDictionary();

        // Derive ImageInfo from the primary item's ispe property.
        int width = 0, height = 0, channels = 0, bpp = 0;
        ReadOnlyMemory<byte> icc = default;
        string? colorSpace = null;
        if (r.PrimaryItemId != 0 && r.Associations.TryGetValue(r.PrimaryItemId, out var pa))
        {
            foreach (int idx in pa)
            {
                if (idx <= 0 || idx > r.Properties.Length) continue;
                var pr = r.Properties[idx - 1];
                if (pr.Type == "ispe") { width = pr.A; height = pr.B; }
                else if (pr.Type == "pixi") { channels = pr.A; bpp = pr.B; }
                else if (pr.Type == "colr") { colorSpace = pr.S; icc = pr.IccProfile; }
            }
        }
        // Fall back to first ispe property if no association is parsed.
        if (width == 0)
        {
            foreach (var pr in r.Properties)
                if (pr.Type == "ispe") { width = pr.A; height = pr.B; break; }
        }

        r.Info = new ImageInfo
        {
            Width = width,
            Height = height,
            BitsPerPixel = bpp,
            ChannelCount = channels,
            Format = r.Format,
            FrameCount = 1,
            ColorSpace = colorSpace,
            IccProfile = icc,
            IsAnimated = r.References.Any(x => x.Type == "auxl" || x.Type == "dimg"),
        };

        return r;
    }

    private static bool ReadBoxHeader(byte[] buf, int p, int end, out string type, out int contentStart, out int contentLength, out int totalLength)
    {
        type = ""; contentStart = 0; contentLength = 0; totalLength = 0;
        if (p + 8 > end) return false;
        uint sz = BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p));
        type = Encoding.ASCII.GetString(buf, p + 4, 4);
        if (sz == 1)
        {
            if (p + 16 > end) return false;
            ulong large = BinaryPrimitives.ReadUInt64BigEndian(buf.AsSpan(p + 8));
            if (large > int.MaxValue || large < 16 || p + (int)large > end) return false;
            contentStart = p + 16;
            totalLength = (int)large;
            contentLength = totalLength - 16;
            return true;
        }
        if (sz == 0)
        {
            contentStart = p + 8;
            totalLength = end - p;
            contentLength = totalLength - 8;
            return true;
        }
        if (sz < 8 || p + sz > end) return false;
        contentStart = p + 8;
        totalLength = (int)sz;
        contentLength = totalLength - 8;
        return true;
    }

    private static ulong ReadUInt(byte[] buf, ref int p, int size)
    {
        ulong v = size switch
        {
            0 => 0,
            1 => buf[p],
            2 => BinaryPrimitives.ReadUInt16BigEndian(buf.AsSpan(p)),
            4 => BinaryPrimitives.ReadUInt32BigEndian(buf.AsSpan(p)),
            8 => BinaryPrimitives.ReadUInt64BigEndian(buf.AsSpan(p)),
            _ => throw new ImageFormatException($"Unsupported HEIF integer size {size}"),
        };
        p += size;
        return v;
    }

    private static string ReadCString(byte[] buf, ref int p, int end)
    {
        int start = p;
        while (p < end && buf[p] != 0) p++;
        var s = Encoding.UTF8.GetString(buf, start, p - start);
        if (p < end) p++;  // consume NUL
        return s;
    }
}

/// <summary>A single item declared in the HEIF meta box.</summary>
public sealed record HeifItem(uint Id, string Type, string Name, HeifItemLocation Location, ImmutableArray<HeifItemExtent> Extents);

/// <summary>Location reference for a HEIF item (data-ref index plus base offset).</summary>
public readonly record struct HeifItemLocation(ulong BaseOffset, ushort DataReferenceIndex);

/// <summary>A single byte extent referenced by an item.</summary>
public readonly record struct HeifItemExtent(ulong Index, ulong Offset, ulong Length);

/// <summary>A decoded ipco property entry. <see cref="A"/>/<see cref="B"/>/<see cref="C"/>/<see cref="D"/> usage depends on <see cref="Type"/>.</summary>
public sealed record HeifProperty(string Type, int A, int B, int C, int D, string S, ReadOnlyMemory<byte> IccProfile);

/// <summary>An item reference (from one item to others, grouped by a 4-CC reference type).</summary>
public sealed record HeifReference(string Type, uint FromItemId, ImmutableArray<uint> ToItemIds);
