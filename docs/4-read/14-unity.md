# Unity

Software written with the Unity Engine can read a stream from the server by using the [WebRTC protocol](03-webrtc.md).

Create a new Unity project or open an existing one.

Open _Window -> Package Manager_, click on the plus sign, _Add Package by name..._ and insert `com.unity.webrtc`. Wait for the package to be installed.

In the _Project_ window, under `Assets`, create a new C# Script called `WebRTCReader.cs` with this content:

```cs
using System.Collections;
using UnityEngine;
using Unity.WebRTC;

public class WebRTCReader : MonoBehaviour
{
    public string url = "http://localhost:8889/stream/whep";

    private RTCPeerConnection pc;
    private MediaStream receiveStream;

    void Start()
    {
        UnityEngine.UI.RawImage rawImage = gameObject.GetComponentInChildren<UnityEngine.UI.RawImage>();
        AudioSource audioSource = gameObject.GetComponentInChildren<AudioSource>();
        pc = new RTCPeerConnection();
        receiveStream = new MediaStream();

        pc.OnTrack = e =>
        {
            receiveStream.AddTrack(e.Track);
        };

        receiveStream.OnAddTrack = e =>
        {
            if (e.Track is VideoStreamTrack videoTrack)
            {
                videoTrack.OnVideoReceived += (tex) =>
                {
                    rawImage.texture = tex;
                };
            }
            else if (e.Track is AudioStreamTrack audioTrack)
            {
                audioSource.SetTrack(audioTrack);
                audioSource.loop = true;
                audioSource.Play();
            }
        };

        RTCRtpTransceiverInit init = new RTCRtpTransceiverInit();
        init.direction = RTCRtpTransceiverDirection.RecvOnly;
        pc.AddTransceiver(TrackKind.Audio, init);
        pc.AddTransceiver(TrackKind.Video, init);

        StartCoroutine(WebRTC.Update());
        StartCoroutine(createOffer());
    }

    private IEnumerator createOffer()
    {
        var op = pc.CreateOffer();
        yield return op;
        if (op.IsError) {
            Debug.LogError("CreateOffer() failed");
            yield break;
        }

        yield return setLocalDescription(op.Desc);
    }

    private IEnumerator setLocalDescription(RTCSessionDescription offer)
    {
        var op = pc.SetLocalDescription(ref offer);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetLocalDescription() failed");
            yield break;
        }

        yield return postOffer(offer);
    }

    private IEnumerator postOffer(RTCSessionDescription offer)
    {
        var content = new System.Net.Http.StringContent(offer.sdp);
        content.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("application/sdp");
        var client = new System.Net.Http.HttpClient();

        var task = System.Threading.Tasks.Task.Run(async () => {
            var res = await client.PostAsync(new System.UriBuilder(url).Uri, content);
            res.EnsureSuccessStatusCode();
            return await res.Content.ReadAsStringAsync();
        });
        yield return new WaitUntil(() => task.IsCompleted);
        if (task.Exception != null) {
            Debug.LogError(task.Exception);
            yield break;
        }

        yield return setRemoteDescription(task.Result);
    }

    private IEnumerator setRemoteDescription(string answer)
    {
        RTCSessionDescription desc = new RTCSessionDescription();
        desc.type = RTCSdpType.Answer;
        desc.sdp = answer;
        var op = pc.SetRemoteDescription(ref desc);
        yield return op;
        if (op.IsError) {
            Debug.LogError("SetRemoteDescription() failed");
            yield break;
        }

        yield break;
    }

    void OnDestroy()
    {
        pc?.Close();
        pc?.Dispose();
        receiveStream?.Dispose();
    }
}
```

Edit the `url` variable according to your needs.

In the _Hierarchy_ window, find or create a scene. Inside the scene, add a _Canvas_. Inside the Canvas, add a _Raw Image_ and an _Audio Source_. Then add the `WebRTCReader.cs` script as component of the canvas, by dragging it inside the _Inspector_ window. then Press the _Play_ button at the top of the page.
