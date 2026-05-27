using System.Collections.Frozen;
using System.Collections.Immutable;

namespace Mediar.Imaging.Heif;

/// <summary>
/// Pre-computed lookup tables over the HEIF item-reference graph
/// (<c>iref</c>) that let callers walk the typed relationships
/// between primary items, thumbnails, derived images, auxiliary
/// layers and metadata items without iterating
/// <see cref="HeifReader.References"/> by hand on every query.
/// </summary>
/// <remarks>
/// <para>
/// ISO/IEC 23008-12 stores item references as directed edges
/// <c>(type, from, [to_0, to_1, ...])</c>. The semantics of the
/// edge direction depend on the 4-CC reference type, which is what
/// makes the raw graph hard to traverse without per-type knowledge.
/// This type encodes that knowledge once:
/// </para>
/// <list type="bullet">
/// <item><c>thmb</c> — <em>from</em> is the thumbnail, <em>to</em> is the master image.</item>
/// <item><c>dimg</c> — <em>from</em> is the derived image, <em>to</em> are the source images.</item>
/// <item><c>auxl</c> — <em>from</em> is the auxiliary layer (alpha / depth / hdr), <em>to</em> is the master.</item>
/// <item><c>cdsc</c> — <em>from</em> is the metadata item (Exif / XMP), <em>to</em> are the described items.</item>
/// <item><c>base</c> — <em>from</em> is the predictively-coded item, <em>to</em> are the references.</item>
/// </list>
/// <para>
/// Every accessor returns an empty immutable array when no edges of
/// the requested type touch the queried item, so callers can
/// unconditionally iterate without null checks.
/// </para>
/// </remarks>
public sealed class HeifReferenceGraph
{
    private readonly FrozenDictionary<uint, ImmutableArray<uint>> _thumbnailsOfMaster;
    private readonly FrozenDictionary<uint, uint> _masterOfThumbnail;
    private readonly FrozenDictionary<uint, ImmutableArray<uint>> _derivedSourcesOf;
    private readonly FrozenDictionary<uint, ImmutableArray<uint>> _derivedConsumersOf;
    private readonly FrozenDictionary<uint, ImmutableArray<uint>> _auxiliariesOfMaster;
    private readonly FrozenDictionary<uint, uint> _masterOfAuxiliary;
    private readonly FrozenDictionary<uint, ImmutableArray<uint>> _metadataDescribing;
    private readonly FrozenDictionary<uint, ImmutableArray<uint>> _metadataReferencingItem;

    /// <summary>Build the graph from a flat <see cref="HeifReference"/> table.</summary>
    public HeifReferenceGraph(ImmutableArray<HeifReference> references)
    {
        var thumbsOfMaster = new Dictionary<uint, List<uint>>();
        var masterOfThumb = new Dictionary<uint, uint>();
        var sources = new Dictionary<uint, ImmutableArray<uint>>();
        var consumers = new Dictionary<uint, List<uint>>();
        var auxOfMaster = new Dictionary<uint, List<uint>>();
        var masterOfAux = new Dictionary<uint, uint>();
        var describes = new Dictionary<uint, List<uint>>();
        var describedBy = new Dictionary<uint, List<uint>>();

        foreach (var r in references)
        {
            switch (r.Type)
            {
                case "thmb":
                    foreach (var master in r.ToItemIds)
                    {
                        if (!thumbsOfMaster.TryGetValue(master, out var list))
                            thumbsOfMaster[master] = list = [];
                        list.Add(r.FromItemId);
                        masterOfThumb.TryAdd(r.FromItemId, master);
                    }
                    break;
                case "dimg":
                    sources[r.FromItemId] = r.ToItemIds;
                    foreach (var source in r.ToItemIds)
                    {
                        if (!consumers.TryGetValue(source, out var list))
                            consumers[source] = list = [];
                        list.Add(r.FromItemId);
                    }
                    break;
                case "auxl":
                    foreach (var master in r.ToItemIds)
                    {
                        if (!auxOfMaster.TryGetValue(master, out var list))
                            auxOfMaster[master] = list = [];
                        list.Add(r.FromItemId);
                        masterOfAux.TryAdd(r.FromItemId, master);
                    }
                    break;
                case "cdsc":
                    if (!describes.TryGetValue(r.FromItemId, out var dlist))
                        describes[r.FromItemId] = dlist = [];
                    foreach (var described in r.ToItemIds)
                    {
                        dlist.Add(described);
                        if (!describedBy.TryGetValue(described, out var dblist))
                            describedBy[described] = dblist = [];
                        dblist.Add(r.FromItemId);
                    }
                    break;
            }
        }

        _thumbnailsOfMaster = thumbsOfMaster.ToFrozenDictionary(
            kv => kv.Key, kv => kv.Value.ToImmutableArray());
        _masterOfThumbnail = masterOfThumb.ToFrozenDictionary();
        _derivedSourcesOf = sources.ToFrozenDictionary();
        _derivedConsumersOf = consumers.ToFrozenDictionary(
            kv => kv.Key, kv => kv.Value.ToImmutableArray());
        _auxiliariesOfMaster = auxOfMaster.ToFrozenDictionary(
            kv => kv.Key, kv => kv.Value.ToImmutableArray());
        _masterOfAuxiliary = masterOfAux.ToFrozenDictionary();
        _metadataDescribing = describes.ToFrozenDictionary(
            kv => kv.Key, kv => kv.Value.ToImmutableArray());
        _metadataReferencingItem = describedBy.ToFrozenDictionary(
            kv => kv.Key, kv => kv.Value.ToImmutableArray());
    }

    /// <summary>
    /// Thumbnails of the given master item id (resolved via the
    /// <c>thmb</c> reference type). An item can carry multiple
    /// thumbnails of different resolutions.
    /// </summary>
    public ImmutableArray<uint> GetThumbnailsFor(uint masterItemId) =>
        _thumbnailsOfMaster.TryGetValue(masterItemId, out var v) ? v : [];

    /// <summary>
    /// Master item id of the given thumbnail (the inverse of
    /// <see cref="GetThumbnailsFor"/>). Returns <see langword="null"/>
    /// when the item is not a thumbnail.
    /// </summary>
    public uint? GetMasterOfThumbnail(uint thumbnailItemId) =>
        _masterOfThumbnail.TryGetValue(thumbnailItemId, out var v) ? v : null;

    /// <summary>
    /// Source items the given derived item is composited from (via
    /// the <c>dimg</c> reference type). For HEIF grids the sources
    /// are the tile items in row-major order.
    /// </summary>
    public ImmutableArray<uint> GetDerivedSourcesOf(uint derivedItemId) =>
        _derivedSourcesOf.TryGetValue(derivedItemId, out var v) ? v : [];

    /// <summary>
    /// Derived items that include the given item among their sources
    /// (the inverse of <see cref="GetDerivedSourcesOf"/>).
    /// </summary>
    public ImmutableArray<uint> GetDerivedConsumersOf(uint sourceItemId) =>
        _derivedConsumersOf.TryGetValue(sourceItemId, out var v) ? v : [];

    /// <summary>
    /// Auxiliary items (alpha mask, depth map, HDR layer, etc.)
    /// referenced by the given master via the <c>auxl</c> reference
    /// type.
    /// </summary>
    public ImmutableArray<uint> GetAuxiliariesFor(uint masterItemId) =>
        _auxiliariesOfMaster.TryGetValue(masterItemId, out var v) ? v : [];

    /// <summary>
    /// Master item id of the given auxiliary item (the inverse of
    /// <see cref="GetAuxiliariesFor"/>). Returns <see langword="null"/>
    /// when the item is not an auxiliary.
    /// </summary>
    public uint? GetMasterOfAuxiliary(uint auxiliaryItemId) =>
        _masterOfAuxiliary.TryGetValue(auxiliaryItemId, out var v) ? v : null;

    /// <summary>
    /// Items the given metadata item (e.g. Exif / XMP item) describes
    /// (via the <c>cdsc</c> reference type).
    /// </summary>
    public ImmutableArray<uint> GetItemsDescribedBy(uint metadataItemId) =>
        _metadataDescribing.TryGetValue(metadataItemId, out var v) ? v : [];

    /// <summary>
    /// Metadata items (Exif / XMP) that describe the given item (the
    /// inverse of <see cref="GetItemsDescribedBy"/>).
    /// </summary>
    public ImmutableArray<uint> GetMetadataItemsFor(uint itemId) =>
        _metadataReferencingItem.TryGetValue(itemId, out var v) ? v : [];
}
