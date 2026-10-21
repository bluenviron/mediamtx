package rtsp

import (
	"testing"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/require"
)

func TestSourceMediaAllowed(t *testing.T) {
	video := &description.Media{Type: description.MediaTypeVideo}
	audio := &description.Media{Type: description.MediaTypeAudio}
	application := &description.Media{Type: description.MediaTypeApplication}
	backChannel := &description.Media{Type: description.MediaTypeAudio, IsBackChannel: true}
	require.True(t, sourceMediaAllowed(video, nil))
	require.True(t, sourceMediaAllowed(audio, nil))
	require.True(t, sourceMediaAllowed(application, nil))
	require.False(t, sourceMediaAllowed(backChannel, nil))
	require.True(t, sourceMediaAllowed(video, new([]string{"video"})))
	require.False(t, sourceMediaAllowed(audio, new([]string{"video"})))
	require.False(t, sourceMediaAllowed(application, new([]string{"video"})))
	require.True(t, sourceMediaAllowed(audio, new([]string{"audio"})))
	require.False(t, sourceMediaAllowed(video, new([]string{"audio"})))
	require.False(t, sourceMediaAllowed(backChannel, new([]string{"audio"})))
}
