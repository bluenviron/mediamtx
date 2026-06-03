using Mediar.Codecs.Opus.Encoder.Silk;
using Xunit;

namespace Mediar.Tests;

public class SilkVadTests
{
    private const int FrameLen = 320; // 20 ms @ 16 kHz, divisible by 4.

    [Fact]
    public void Silence_ReportsNotActiveAndZeroSnr()
    {
        var vad = new SilkVad();
        var snr = new int[SilkVad.SubframeCount];
        // Warm the history with silence.
        for (int i = 0; i < 8; i++)
            vad.Analyze(new float[FrameLen], snr);

        var result = vad.Analyze(new float[FrameLen], snr);
        Assert.False(result.IsVoiceActive);
        for (int s = 0; s < SilkVad.SubframeCount; s++)
            // SNR(silence / silence) collapses to log(ε/ε) = 0 dB.
            Assert.InRange(snr[s], -16, 16);
    }

    [Fact]
    public void LoudSignalAfterSilenceWarmup_ReportsActive()
    {
        var vad = new SilkVad();
        var snr = new int[SilkVad.SubframeCount];

        // Warm the noise-floor history with silence so the loud frame
        // has something to be loud relative to.
        for (int i = 0; i < SilkVad.SubframeCount + 1; i++)
            vad.Analyze(new float[FrameLen], snr);

        // Speech-shaped: 200 Hz tone at amplitude 0.5 on top of low noise.
        var x = new float[FrameLen];
        var rng = new Random(0xA1B2);
        for (int i = 0; i < FrameLen; i++)
        {
            x[i] = 0.5f * MathF.Sin(2f * MathF.PI * 200f * i / 16000f)
                 + 0.001f * (float)(rng.NextDouble() * 2 - 1);
        }
        var result = vad.Analyze(x, snr);
        Assert.True(result.IsVoiceActive,
            "VAD must flag a 200 Hz tone after silence-warmed noise floor; got SNR Q7 = ["
            + string.Join(", ", snr) + "].");
    }

    [Fact]
    public void Analyze_RejectsFrameLengthNotDivisibleBySubframeCount()
    {
        var vad = new SilkVad();
        var snr = new int[SilkVad.SubframeCount];
        Assert.Throws<ArgumentException>(() =>
            vad.Analyze(new float[FrameLen + 1], snr));
    }

    [Fact]
    public void Analyze_RejectsTooSmallSnrDestination()
    {
        var vad = new SilkVad();
        var snr = new int[SilkVad.SubframeCount - 1];
        Assert.Throws<ArgumentException>(() =>
            vad.Analyze(new float[FrameLen], snr));
    }

    [Fact]
    public void Reset_ClearsNoiseHistory()
    {
        var vad = new SilkVad();
        var snr = new int[SilkVad.SubframeCount];
        // Push a few loud frames to elevate the historical "minimum".
        var loud = new float[FrameLen];
        for (int i = 0; i < FrameLen; i++) loud[i] = 0.9f;
        for (int i = 0; i < 5; i++) vad.Analyze(loud, snr);

        vad.Reset();

        // After reset, a silence frame should compute SNR against
        // itself (no prior history), giving ~0 dB.
        vad.Analyze(new float[FrameLen], snr);
        for (int s = 0; s < SilkVad.SubframeCount; s++)
            Assert.InRange(snr[s], -16, 16);
    }
}
