using Mediar;

if (args.Length == 0)
{
    PrintUsage();
    return 1;
}

try
{
    return args[0].ToLowerInvariant() switch
    {
        "info" => await RunInfoAsync(args).ConfigureAwait(false),
        "extract-audio" => await RunExtractAudioAsync(args).ConfigureAwait(false),
        "mux-av" => await RunMuxAvAsync(args).ConfigureAwait(false),
        "embed-srt" => await RunEmbedSrtAsync(args).ConfigureAwait(false),
        "extract-srt" => await RunExtractSrtAsync(args).ConfigureAwait(false),
        "transmux" => await RunTransmuxAsync(args).ConfigureAwait(false),
        "help" or "--help" or "-h" => PrintUsage(),
        _ => UnknownCommand(args[0]),
    };
}
catch (Exception ex)
{
    Console.Error.WriteLine($"error: {ex.Message}");
    return 2;
}

static int PrintUsage()
{
    Console.WriteLine("""
        Mediar CLI – container & subtitle operations (no codec re-encoding).

        Usage:
          mediar info <input>
          mediar extract-audio <input.mp4> <output.m4a>
          mediar mux-av <video> <audio> <output.mp4>
          mediar embed-srt <input.mp4> <input.srt> <output.mp4> [language]
          mediar extract-srt <input.mp4> <output.srt>
          mediar transmux <input> <output>
        """);
    return 0;
}

static int UnknownCommand(string command)
{
    Console.Error.WriteLine($"unknown command: {command}");
    PrintUsage();
    return 1;
}

static async Task<int> RunInfoAsync(string[] args)
{
    if (args.Length < 2) { Console.Error.WriteLine("info: missing <input>"); return 1; }
    var info = await MediarOperations.ProbeAsync(args[1]).ConfigureAwait(false);
    Console.WriteLine($"input:     {info.Path}");
    Console.WriteLine($"format:    {info.Format}");
    Console.WriteLine($"duration:  {info.Duration}");
    var m = info.Metadata;
    if (m.Title is not null) Console.WriteLine($"title:     {m.Title}");
    if (m.Artist is not null) Console.WriteLine($"artist:    {m.Artist}");
    if (m.Album is not null) Console.WriteLine($"album:     {m.Album}");
    if (m.AlbumArtist is not null) Console.WriteLine($"album-art: {m.AlbumArtist}");
    if (m.Date is not null) Console.WriteLine($"date:      {m.Date}");
    if (m.Genre is not null) Console.WriteLine($"genre:     {m.Genre}");
    if (m.TrackNumber is not null) Console.WriteLine($"track:     {m.TrackNumber}");
    if (m.DiscNumber is not null) Console.WriteLine($"disc:      {m.DiscNumber}");
    if (m.Composer is not null) Console.WriteLine($"composer:  {m.Composer}");
    if (m.Copyright is not null) Console.WriteLine($"copyright: {m.Copyright}");
    if (m.Encoder is not null) Console.WriteLine($"encoder:   {m.Encoder}");
    if (m.Comment is not null) Console.WriteLine($"comment:   {m.Comment}");
    if (m.Location is { } g) Console.WriteLine($"geo:       {g.Latitude:F6},{g.Longitude:F6}" + (g.Altitude is { } a ? $" ({a:F1}m)" : ""));
    if (m.Tags.Count > 0)
    {
        Console.WriteLine($"tags:      {m.Tags.Count}");
        foreach (var kv in m.Tags) Console.WriteLine($"  {kv.Key} = {kv.Value}");
    }
    Console.WriteLine($"tracks:    {info.Tracks.Count}");
    foreach (var t in info.Tracks)
    {
        Console.WriteLine($"  #{t.Index} {t.Kind,-9} codec={t.Codec,-10} lang={t.Language,-4} time-base={t.TimeBase} dur={t.Duration}");
    }
    return 0;
}

static async Task<int> RunExtractAudioAsync(string[] args)
{
    if (args.Length < 3)
    {
        Console.Error.WriteLine("extract-audio: usage <input> <output.m4a>");
        return 1;
    }
    await MediarOperations.ExtractAudioAsync(args[1], args[2]).ConfigureAwait(false);
    Console.WriteLine($"wrote {args[2]}");
    return 0;
}

static async Task<int> RunMuxAvAsync(string[] args)
{
    if (args.Length < 4)
    {
        Console.Error.WriteLine("mux-av: usage <video> <audio> <output.mp4>");
        return 1;
    }
    await MediarOperations.MuxAudioWithVideoAsync(args[1], args[2], args[3]).ConfigureAwait(false);
    Console.WriteLine($"wrote {args[3]}");
    return 0;
}

static async Task<int> RunEmbedSrtAsync(string[] args)
{
    if (args.Length < 4)
    {
        Console.Error.WriteLine("embed-srt: usage <input.mp4> <input.srt> <output.mp4> [language]");
        return 1;
    }
    string language = args.Length >= 5 ? args[4] : "und";
    await MediarOperations.EmbedSrtAsync(args[1], args[2], args[3], language).ConfigureAwait(false);
    Console.WriteLine($"wrote {args[3]}");
    return 0;
}

static async Task<int> RunExtractSrtAsync(string[] args)
{
    if (args.Length < 3)
    {
        Console.Error.WriteLine("extract-srt: usage <input.mp4> <output.srt>");
        return 1;
    }
    await MediarOperations.ExtractSrtAsync(args[1], args[2]).ConfigureAwait(false);
    Console.WriteLine($"wrote {args[2]}");
    return 0;
}

static async Task<int> RunTransmuxAsync(string[] args)
{
    if (args.Length < 3)
    {
        Console.Error.WriteLine("transmux: usage <input> <output>");
        return 1;
    }
    await MediarOperations.TransmuxAsync(args[1], args[2]).ConfigureAwait(false);
    Console.WriteLine($"wrote {args[2]}");
    return 0;
}
