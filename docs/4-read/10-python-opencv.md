# Python and OpenCV

Python-based software can read streams from the server with the OpenCV library, acting as a [RTSP client](03-rtsp.md).

```python
import cv2

cap = cv2.VideoCapture('rtsp://localhost:8554/mystream')
if not cap.isOpened():
    raise Exception("can't open video capture")

while True:
    ret, frame = cap.read()
    if not ret:
        raise Exception("can't receive frame")

    cv2.imshow('frame', frame)

    if cv2.waitKey(1) == ord('q'):
        break

cap.release()
cv2.destroyAllWindows()
```
