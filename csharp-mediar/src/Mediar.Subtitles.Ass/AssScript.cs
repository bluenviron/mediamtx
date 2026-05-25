using System.Globalization;
using System.Text;

namespace Mediar.Subtitles.Ass;

/// <summary>
/// Parsed Advanced SubStation Alpha (ASS / SSA v4+) script.
/// Sections that aren't needed for round-trip are kept verbatim so
/// writing back yields a byte-equivalent file.
/// </summary>
public sealed class AssScript
{
    /// <summary>Script-info key-value pairs (e.g. <c>PlayResX=1280</c>).</summary>
    public List<KeyValuePair<string, string>> ScriptInfo { get; } = new();

    /// <summary>Raw lines of the <c>[V4+ Styles]</c> section (Format + Style lines).</summary>
    public List<string> StyleSection { get; } = new();

    /// <summary>Field order of the <c>[Events]</c> section's <c>Format:</c> line.</summary>
    public List<string> EventFields { get; } = new();

    /// <summary>Parsed events.</summary>
    public List<AssEvent> Events { get; } = new();
}

/// <summary>Reader for ASS / SSA v4+ scripts.</summary>
public static class AssReader
{
    /// <summary>Parse an ASS file from disk.</summary>
    public static AssScript ReadFile(string path)
    {
        using var sr = new StreamReader(path, Encoding.UTF8, detectEncodingFromByteOrderMarks: true);
        return Read(sr);
    }

    /// <summary>Parse an ASS document from a string.</summary>
    public static AssScript ReadString(string content)
    {
        using var sr = new StringReader(content);
        return Read(sr);
    }

    /// <summary>Parse an ASS document from <paramref name="reader"/>.</summary>
    public static AssScript Read(TextReader reader)
    {
        ArgumentNullException.ThrowIfNull(reader);
        var script = new AssScript();
        string? section = null;
        string? line;
        while ((line = reader.ReadLine()) is not null)
        {
            string trimmed = line.Trim();
            if (trimmed.Length == 0 || trimmed.StartsWith(';') ||
                trimmed.StartsWith("!:", StringComparison.Ordinal))
            {
                continue;
            }
            if (trimmed.StartsWith('[') && trimmed.EndsWith(']'))
            {
                section = trimmed[1..^1].Trim();
                continue;
            }
            switch (section)
            {
                case "Script Info":
                    int eq = trimmed.IndexOf(':');
                    if (eq > 0)
                    {
                        string key = trimmed[..eq].Trim();
                        string val = trimmed[(eq + 1)..].Trim();
                        script.ScriptInfo.Add(new KeyValuePair<string, string>(key, val));
                    }
                    break;
                case "V4+ Styles":
                case "V4 Styles":
                    script.StyleSection.Add(trimmed);
                    break;
                case "Events":
                    if (trimmed.StartsWith("Format:", StringComparison.OrdinalIgnoreCase))
                    {
                        script.EventFields.Clear();
                        foreach (var f in trimmed[7..].Split(','))
                        {
                            script.EventFields.Add(f.Trim());
                        }
                    }
                    else
                    {
                        int colon = trimmed.IndexOf(':');
                        if (colon > 0)
                        {
                            string kind = trimmed[..colon];
                            string rest = trimmed[(colon + 1)..].TrimStart();
                            script.Events.Add(ParseEvent(kind, rest, script.EventFields));
                        }
                    }
                    break;
            }
        }
        return script;
    }

    private static AssEvent ParseEvent(string kind, string body, List<string> fields)
    {
        // The last field is "Text" which may contain commas; split only up to fields.Count-1 commas.
        var parts = new string[fields.Count];
        int idx = 0;
        int start = 0;
        for (int i = 0; i < body.Length && idx < fields.Count - 1; i++)
        {
            if (body[i] == ',')
            {
                parts[idx++] = body[start..i];
                start = i + 1;
            }
        }
        parts[idx] = body[start..];

        var ev = new AssEvent
        {
            Kind = kind,
        };
        for (int i = 0; i < fields.Count; i++)
        {
            string field = fields[i];
            string value = parts[i] ?? string.Empty;
            ev = ApplyField(ev, field, value);
        }
        return ev;
    }

    private static AssEvent ApplyField(AssEvent ev, string field, string value)
    {
        return field switch
        {
            "Layer" => ev with { Layer = ParseInt(value) },
            "Start" => AssTime.TryParse(value, out var s) ? ev with { Start = s } : ev,
            "End" => AssTime.TryParse(value, out var e) ? ev with { End = e } : ev,
            "Style" => ev with { Style = value },
            "Name" or "Actor" => ev with { Name = value },
            "MarginL" => ev with { MarginL = ParseInt(value) },
            "MarginR" => ev with { MarginR = ParseInt(value) },
            "MarginV" => ev with { MarginV = ParseInt(value) },
            "Effect" => ev with { Effect = value },
            "Text" => ev with { Text = value },
            _ => ev,
        };
    }

    private static int ParseInt(string s) => int.TryParse(s, NumberStyles.Integer, CultureInfo.InvariantCulture, out int v) ? v : 0;
}

/// <summary>Writer for ASS / SSA v4+ scripts.</summary>
public static class AssWriter
{
    /// <summary>Write <paramref name="script"/> to <paramref name="path"/> as UTF-8.</summary>
    public static void WriteFile(string path, AssScript script)
    {
        using var sw = new StreamWriter(path, append: false, Encoding.UTF8);
        Write(sw, script);
    }

    /// <summary>Serialize <paramref name="script"/> to a string.</summary>
    public static string WriteString(AssScript script)
    {
        using var sw = new StringWriter(CultureInfo.InvariantCulture);
        Write(sw, script);
        return sw.ToString();
    }

    /// <summary>Write <paramref name="script"/> to <paramref name="writer"/>.</summary>
    public static void Write(TextWriter writer, AssScript script)
    {
        ArgumentNullException.ThrowIfNull(writer);
        ArgumentNullException.ThrowIfNull(script);
        writer.WriteLine("[Script Info]");
        foreach (var kv in script.ScriptInfo)
        {
            writer.WriteLine($"{kv.Key}: {kv.Value}");
        }
        writer.WriteLine();
        if (script.StyleSection.Count > 0)
        {
            writer.WriteLine("[V4+ Styles]");
            foreach (var line in script.StyleSection)
            {
                writer.WriteLine(line);
            }
            writer.WriteLine();
        }
        writer.WriteLine("[Events]");
        if (script.EventFields.Count == 0)
        {
            writer.WriteLine("Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text");
        }
        else
        {
            writer.Write("Format: ");
            writer.WriteLine(string.Join(", ", script.EventFields));
        }
        var fields = script.EventFields.Count > 0
            ? script.EventFields
            : (IReadOnlyList<string>)new[] { "Layer", "Start", "End", "Style", "Name", "MarginL", "MarginR", "MarginV", "Effect", "Text" };
        foreach (var ev in script.Events)
        {
            writer.WriteLine(FormatEvent(ev, fields));
        }
    }

    private static string FormatEvent(AssEvent ev, IReadOnlyList<string> fields)
    {
        var sb = new StringBuilder();
        sb.Append(ev.Kind);
        sb.Append(": ");
        for (int i = 0; i < fields.Count; i++)
        {
            if (i > 0) sb.Append(',');
            sb.Append(FieldValue(ev, fields[i]));
        }
        return sb.ToString();
    }

    private static string FieldValue(AssEvent ev, string field) => field switch
    {
        "Layer" => ev.Layer.ToString(CultureInfo.InvariantCulture),
        "Start" => AssTime.Format(ev.Start),
        "End" => AssTime.Format(ev.End),
        "Style" => ev.Style,
        "Name" or "Actor" => ev.Name,
        "MarginL" => ev.MarginL.ToString("D4", CultureInfo.InvariantCulture),
        "MarginR" => ev.MarginR.ToString("D4", CultureInfo.InvariantCulture),
        "MarginV" => ev.MarginV.ToString("D4", CultureInfo.InvariantCulture),
        "Effect" => ev.Effect,
        "Text" => ev.Text,
        _ => string.Empty,
    };
}
