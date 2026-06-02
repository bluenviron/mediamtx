using System.Globalization;
using System.Text;

namespace Mediar.Imaging.Xpm;

/// <summary>
/// Writer for the X11 XPM3 pixmap format. Emits a C source fragment of
/// the form <c>static char *image[] = { "&lt;header&gt;", "&lt;colors&gt;...",
/// "&lt;pixels&gt;..." };</c> with one character per pixel for palettes of
/// up to 92 colors, two characters per pixel for up to ~8000, and so on.
/// The variable name defaults to <c>image</c> and can be overridden via
/// the constructor. Supports <see cref="PixelFormat.Rgb24"/>,
/// <see cref="PixelFormat.Bgr24"/>, <see cref="PixelFormat.Rgba32"/>,
/// <see cref="PixelFormat.Bgra32"/>, <see cref="PixelFormat.Argb32"/>,
/// <see cref="PixelFormat.Gray8"/>, and <see cref="PixelFormat.Indexed8"/>.
/// </summary>
public sealed class XpmWriter : IImageWriter
{
    // Character set XPM allows: printable ASCII excluding space, quote and backslash.
    // ' ' is reserved for "None"-style transparent keys in some toolchains, and
    // " / \ break the C-string-literal we emit.
    private static readonly char[] s_charset = BuildCharset();
    private readonly Stream _stream;
    private readonly bool _ownsStream;
    private readonly string _name;
    private bool _written;
    private bool _disposed;

    /// <inheritdoc/>
    public ImageFormat Format => ImageFormat.Xpm;

    /// <summary>Construct a writer that emits to <paramref name="stream"/>.</summary>
    /// <param name="stream">Destination stream.</param>
    /// <param name="ownsStream">When <c>true</c>, disposes the stream alongside the writer.</param>
    /// <param name="variableName">Identifier used for the emitted C array variable.</param>
    public XpmWriter(Stream stream, bool ownsStream = false, string variableName = "image")
    {
        ArgumentNullException.ThrowIfNull(stream);
        if (!stream.CanWrite) throw new ArgumentException("Stream must be writable.", nameof(stream));
        if (string.IsNullOrWhiteSpace(variableName))
            throw new ArgumentException("Variable name required.", nameof(variableName));
        _stream = stream;
        _ownsStream = ownsStream;
        _name = variableName;
    }

    /// <summary>Create an XPM writer for <paramref name="path"/>.</summary>
    public static XpmWriter Create(string path, string variableName = "image")
        => new(new FileStream(path, FileMode.Create, FileAccess.Write, FileShare.None),
               ownsStream: true, variableName);

    /// <inheritdoc/>
    public async ValueTask WriteFrameAsync(ImageFrame frame, CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(frame);
        if (_written) throw new InvalidOperationException("XPM supports a single frame.");
        _written = true;

        int width = frame.Width;
        int height = frame.Height;
        var (perPixelColor, _) = ResolveColorReader(frame.PixelFormat);
        if (perPixelColor is null)
            throw new NotSupportedException($"XPM writer does not support pixel format {frame.PixelFormat}.");

        // Build unique color palette (RGBA8888 with R in low byte) -> key.
        var paletteIndex = new Dictionary<uint, int>(64);
        var paletteOrder = new List<uint>(64);
        for (int y = 0; y < height; y++)
        {
            ReadOnlySpan<byte> row = frame.Pixels.Span.Slice(y * frame.Stride);
            for (int x = 0; x < width; x++)
            {
                uint c = perPixelColor(frame, row, x);
                if (!paletteIndex.ContainsKey(c))
                {
                    paletteIndex[c] = paletteOrder.Count;
                    paletteOrder.Add(c);
                }
            }
        }

        int cpp = CharsPerPixel(paletteOrder.Count);
        var keys = new string[paletteOrder.Count];
        for (int i = 0; i < paletteOrder.Count; i++) keys[i] = KeyForIndex(i, cpp);

        var sb = new StringBuilder(width * cpp * height + paletteOrder.Count * 32);
        sb.Append("/* XPM */\n")
          .Append("static char *").Append(_name).Append("[] = {\n")
          .Append('"').Append(width.ToString(CultureInfo.InvariantCulture)).Append(' ')
          .Append(height.ToString(CultureInfo.InvariantCulture)).Append(' ')
          .Append(paletteOrder.Count.ToString(CultureInfo.InvariantCulture)).Append(' ')
          .Append(cpp.ToString(CultureInfo.InvariantCulture)).Append("\",\n");
        for (int i = 0; i < paletteOrder.Count; i++)
        {
            uint rgba = paletteOrder[i];
            byte a = (byte)((rgba >> 24) & 0xFF);
            sb.Append('"').Append(keys[i]).Append(" c ");
            if (a == 0)
            {
                sb.Append("None");
            }
            else
            {
                byte r = (byte)(rgba & 0xFF);
                byte g = (byte)((rgba >> 8) & 0xFF);
                byte b = (byte)((rgba >> 16) & 0xFF);
                sb.Append('#')
                  .Append(r.ToString("X2", CultureInfo.InvariantCulture))
                  .Append(g.ToString("X2", CultureInfo.InvariantCulture))
                  .Append(b.ToString("X2", CultureInfo.InvariantCulture));
            }
            sb.Append('"').Append(',').Append('\n');
        }
        for (int y = 0; y < height; y++)
        {
            sb.Append('"');
            ReadOnlySpan<byte> row = frame.Pixels.Span.Slice(y * frame.Stride);
            for (int x = 0; x < width; x++)
            {
                uint c = perPixelColor(frame, row, x);
                int idx = paletteIndex[c];
                sb.Append(keys[idx]);
            }
            sb.Append('"');
            if (y + 1 < height) sb.Append(',');
            sb.Append('\n');
        }
        sb.Append("};\n");

        byte[] bytes = Encoding.ASCII.GetBytes(sb.ToString());
        await _stream.WriteAsync(bytes, cancellationToken).ConfigureAwait(false);
    }

    private static int CharsPerPixel(int colors)
    {
        int n = s_charset.Length;
        int cpp = 1;
        long capacity = n;
        while (capacity < colors)
        {
            cpp++;
            capacity *= n;
        }
        return cpp;
    }

    private static string KeyForIndex(int index, int cpp)
    {
        int n = s_charset.Length;
        Span<char> buf = stackalloc char[cpp];
        for (int i = cpp - 1; i >= 0; i--)
        {
            buf[i] = s_charset[index % n];
            index /= n;
        }
        return new string(buf);
    }

    private static char[] BuildCharset()
    {
        var list = new List<char>(96);
        for (int c = 33; c <= 126; c++)
        {
            char ch = (char)c;
            if (ch == '"' || ch == '\\' || ch == '?') continue; // ? avoids trigraphs.
            list.Add(ch);
        }
        return [.. list];
    }

    private delegate uint PixelColorReader(ImageFrame frame, ReadOnlySpan<byte> row, int x);

    private static (PixelColorReader? reader, int bpp) ResolveColorReader(PixelFormat pf) => pf switch
    {
        PixelFormat.Rgb24 => (static (f, r, x) => 0xFF000000u | ((uint)r[x * 3 + 2] << 16) | ((uint)r[x * 3 + 1] << 8) | r[x * 3 + 0], 3),
        PixelFormat.Bgr24 => (static (f, r, x) => 0xFF000000u | ((uint)r[x * 3 + 0] << 16) | ((uint)r[x * 3 + 1] << 8) | r[x * 3 + 2], 3),
        PixelFormat.Rgba32 => (static (f, r, x) => ((uint)r[x * 4 + 3] << 24) | ((uint)r[x * 4 + 2] << 16) | ((uint)r[x * 4 + 1] << 8) | r[x * 4 + 0], 4),
        PixelFormat.Bgra32 => (static (f, r, x) => ((uint)r[x * 4 + 3] << 24) | ((uint)r[x * 4 + 0] << 16) | ((uint)r[x * 4 + 1] << 8) | r[x * 4 + 2], 4),
        PixelFormat.Argb32 => (static (f, r, x) => ((uint)r[x * 4 + 0] << 24) | ((uint)r[x * 4 + 1] << 16) | ((uint)r[x * 4 + 2] << 8) | r[x * 4 + 3], 4),
        PixelFormat.Gray8 => (static (f, r, x) => 0xFF000000u | ((uint)r[x] << 16) | ((uint)r[x] << 8) | r[x], 1),
        PixelFormat.Indexed8 => (static (f, r, x) =>
        {
            byte idx = r[x];
            var pal = f.Palette.Span;
            return idx < pal.Length ? pal[idx] : (0xFF000000u | ((uint)idx << 16) | ((uint)idx << 8) | idx);
        }, 1),
        _ => (null, 0),
    };

    /// <inheritdoc/>
    public ValueTask FinishAsync(CancellationToken cancellationToken = default)
        => ValueTask.CompletedTask;

    /// <inheritdoc/>
    public async ValueTask DisposeAsync()
    {
        if (_disposed) return;
        _disposed = true;
        await _stream.FlushAsync().ConfigureAwait(false);
        if (_ownsStream) await _stream.DisposeAsync().ConfigureAwait(false);
    }
}
