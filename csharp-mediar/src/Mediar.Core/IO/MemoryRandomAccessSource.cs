namespace Mediar.IO;

/// <summary>
/// A <see cref="IRandomAccessSource"/> backed by an in-memory buffer.
/// Useful for tests and for inputs that have already been buffered.
/// </summary>
public sealed class MemoryRandomAccessSource : IRandomAccessSource
{
    private readonly ReadOnlyMemory<byte> _memory;
    private bool _disposed;

    /// <summary>Wrap <paramref name="memory"/> as a read-only random-access source.</summary>
    public MemoryRandomAccessSource(ReadOnlyMemory<byte> memory)
    {
        _memory = memory;
    }

    /// <inheritdoc/>
    public long Length => _memory.Length;

    /// <inheritdoc/>
    public int Read(long offset, Span<byte> buffer)
    {
        ObjectDisposedException.ThrowIf(_disposed, this);
        if (offset < 0 || offset > _memory.Length)
        {
            throw new ArgumentOutOfRangeException(nameof(offset));
        }
        int available = _memory.Length - (int)offset;
        if (available <= 0) return 0;
        int take = Math.Min(buffer.Length, available);
        _memory.Span.Slice((int)offset, take).CopyTo(buffer);
        return take;
    }

    /// <inheritdoc/>
    public ValueTask<int> ReadAsync(long offset, Memory<byte> buffer, CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        return new ValueTask<int>(Read(offset, buffer.Span));
    }

    /// <inheritdoc/>
    public void Dispose() => _disposed = true;

    /// <inheritdoc/>
    public ValueTask DisposeAsync()
    {
        Dispose();
        return ValueTask.CompletedTask;
    }
}
