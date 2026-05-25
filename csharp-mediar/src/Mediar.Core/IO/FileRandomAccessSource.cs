using Microsoft.Win32.SafeHandles;

namespace Mediar.IO;

/// <summary>
/// A <see cref="IRandomAccessSource"/> backed by a positioned file handle.
/// Uses <see cref="System.IO.RandomAccess"/> for thread-safe, position-free I/O
/// (no <see cref="FileStream.Position"/> mutation).
/// </summary>
public sealed class FileRandomAccessSource : IRandomAccessSource
{
    private readonly SafeFileHandle _handle;
    private readonly bool _ownsHandle;
    private readonly long _length;
    private bool _disposed;

    /// <summary>Open <paramref name="path"/> for read-only sequential/random access.</summary>
    public FileRandomAccessSource(string path)
    {
        _handle = File.OpenHandle(
            path,
            FileMode.Open,
            FileAccess.Read,
            FileShare.Read,
            FileOptions.RandomAccess);
        _ownsHandle = true;
        _length = RandomAccess.GetLength(_handle);
    }

    /// <summary>Wrap an existing handle (does not take ownership unless requested).</summary>
    public FileRandomAccessSource(SafeFileHandle handle, bool ownsHandle = false)
    {
        ArgumentNullException.ThrowIfNull(handle);
        _handle = handle;
        _ownsHandle = ownsHandle;
        _length = RandomAccess.GetLength(_handle);
    }

    /// <inheritdoc/>
    public long Length => _length;

    /// <inheritdoc/>
    public int Read(long offset, Span<byte> buffer)
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
        if (offset < 0 || offset > _length)
        {
            throw new ArgumentOutOfRangeException(nameof(offset));
        }
        return RandomAccess.Read(_handle, buffer, offset);
    }

    /// <inheritdoc/>
    public ValueTask<int> ReadAsync(long offset, Memory<byte> buffer, CancellationToken cancellationToken = default)
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
        return RandomAccess.ReadAsync(_handle, buffer, offset, cancellationToken);
    }

    /// <inheritdoc/>
    public void Dispose()
    {
        if (_disposed) return;
        _disposed = true;
        if (_ownsHandle) _handle.Dispose();
    }

    /// <inheritdoc/>
    public ValueTask DisposeAsync()
    {
        Dispose();
        return ValueTask.CompletedTask;
    }
}
