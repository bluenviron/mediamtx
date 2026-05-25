using System.Buffers;

namespace Mediar.IO;

/// <summary>
/// Abstraction over a positioned byte source. Implementations may back this with
/// a <see cref="FileStream"/>, a memory-mapped file, or an in-memory buffer.
/// </summary>
/// <remarks>
/// All read methods are bounds-checked. Implementations are expected to be safe for
/// use from one thread at a time; <see cref="FileRandomAccessSource"/> uses
/// <see cref="RandomAccess"/> internally and is safe to call concurrently against the
/// same file handle, but Mediar's demuxers serialize access for simplicity.
/// </remarks>
public interface IRandomAccessSource : IAsyncDisposable, IDisposable
{
    /// <summary>Total length in bytes.</summary>
    long Length { get; }

    /// <summary>Read into <paramref name="buffer"/> starting at <paramref name="offset"/>.</summary>
    /// <returns>The number of bytes actually read; may be less than buffer.Length only at EOF.</returns>
    int Read(long offset, Span<byte> buffer);

    /// <summary>Async variant of <see cref="Read"/>.</summary>
    ValueTask<int> ReadAsync(long offset, Memory<byte> buffer, CancellationToken cancellationToken = default);

    /// <summary>
    /// Convenience: rent a pooled buffer and read exactly <paramref name="count"/> bytes.
    /// Throws <see cref="EndOfStreamException"/> if EOF is reached before that.
    /// Caller must dispose the returned owner.
    /// </summary>
    public IMemoryOwner<byte> ReadExact(long offset, int count)
    {
        var owner = MemoryPool<byte>.Shared.Rent(count);
        int total = 0;
        while (total < count)
        {
            int n = Read(offset + total, owner.Memory.Span.Slice(total, count - total));
            if (n <= 0)
            {
                owner.Dispose();
                throw new EndOfStreamException(
                    $"Expected {count} bytes at offset {offset}, got {total}.");
            }
            total += n;
        }
        return owner;
    }
}
