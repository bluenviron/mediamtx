package gstpipe

import (
	"errors"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
)

type StatServer struct {
	stats    map[string]*gstPipeStat
	PathConf map[string]*conf.Path
	Parent   logger.Writer
	mutex    sync.RWMutex
}

// ErrStatNotFound is returned when a Stat is not found.
var ErrStatNotFound = errors.New("stat not found")

// Initialize initializes StatServer.
func (w *StatServer) Initialize() error {

	// For every path in Config, create a new gstPipeStat
	// and add it to the stats map

	w.stats = make(map[string]*gstPipeStat)

	w.ReloadPathNames(w.PathConf)

	w.Log(logger.Info, "gstpipe stats server initialized")

	return nil

}

// Log implements logger.Writer.
func (w *StatServer) Log(level logger.Level, format string, args ...interface{}) {
	w.Parent.Log(level, "[GstPipeStats] "+format, args...)
}

// ReloadPathNames is called by core.Core.
func (w *StatServer) ReloadPathNames(pathConfs map[string]*conf.Path) {

	// Check if any of the conf.Path.Name is not in the stats map. If not, add it to the stats map
	w.Log(logger.Info, "Reloading path names")

	w.mutex.Lock()
	defer w.mutex.Unlock()

	for pathConfName := range pathConfs {
		w.Log(logger.Debug, "Checking path %s", pathConfName)

		for path := range w.stats {

			if path == pathConfName {
				w.Log(logger.Debug, "Path %s already exists", path)
				continue
			}

		}
		w.Log(logger.Debug, "Path %s does not exist. Adding it", pathConfName)
		w.stats[pathConfName] = &gstPipeStat{path: pathConfName}

	}

	w.Log(logger.Info, "Path names reloaded")

}

func (w *StatServer) GetStats(path string) (*gstPipeStat, error) {

	if w.stats[path] == nil {
		return nil, ErrStatNotFound
	}

	w.mutex.RLock()
	defer w.mutex.RUnlock()

	return w.stats[path], nil
}

func (w *StatServer) SetStats(path string, stats *gstPipeStat) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.stats[path] = stats
}

func (w *StatServer) SetOnlyPathWithZeroStats(path string) {
	w.stats[path] = &gstPipeStat{path: path}
}

func (w *StatServer) GetStatsList() map[string]*gstPipeStat {

	return w.stats
}

func (w *StatServer) SetJitterStats(path string, bufferStats jitterBufferStats) {

	jitterStats := jitterBufferStats{
		numLost:         bufferStats.numLost,
		numLate:         bufferStats.numLate,
		numDuplicates:   bufferStats.numDuplicates,
		avgJitter:       bufferStats.avgJitter,
		rtxCount:        bufferStats.rtxCount,
		rtxSuccessCount: bufferStats.rtxSuccessCount,
		rtxPerPacket:    bufferStats.rtxPerPacket,
		rtxRtt:          bufferStats.rtxRtt,
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.stats[path].jitterStats = jitterStats

}

func (w *StatServer) SetRtpSourceStats(path string, sourceStats rtpSourceStats) {

	w.stats[path] = &gstPipeStat{path: path}
	stats := rtpSourceStats{
		packetsLost:     sourceStats.packetsLost,
		packetsReceived: sourceStats.packetsReceived,
		bitrate:         sourceStats.bitrate,
		jitter:          sourceStats.jitter,
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.stats[path].rtpSourceStats = stats

}

func (w *StatServer) SetRtpSessionStats(path string, sessionStats rtpSessionStats) {

	w.stats[path] = &gstPipeStat{path: path}
	stats := rtpSessionStats{
		rtxDropCount:  sessionStats.rtxDropCount,
		sentNackCount: sessionStats.sentNackCount,
		recvNackCount: sessionStats.recvNackCount,
	}

	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.stats[path].rtpSessionStats = stats

}

// APIGstPipeGet is called by api.

func (w *StatServer) APIGstPipeGet(path string) (*defs.APIGstPipe, error) {

	w.mutex.RLock()
	defer w.mutex.RUnlock()

	stat, err := w.GetStats(path)

	if err != nil {
		return nil, err
	}

	return stat.apiItem(), err

}

// APIGstPipeList is called by api.
func (w *StatServer) APIGstPipeList() (*defs.APIGstPipeList, error) {

	stats := w.GetStatsList()

	data := &defs.APIGstPipeList{
		Items: []*defs.APIGstPipe{},
	}

	for _, stat := range stats {
		data.Items = append(data.Items, stat.apiItem())
	}

	return data, nil
}

// APIGstJitterBufferStatPut is called by api.
func (w *StatServer) APIGstJitterBufferStatPut(path string, data *defs.APIGstJitterBufferStats) {

	bufferStats := jitterBufferStats{
		numLost:         data.NumLost,
		numLate:         data.NumLate,
		numDuplicates:   data.NumDuplicates,
		avgJitter:       data.AvgJitter,
		rtxCount:        data.RtxCount,
		rtxSuccessCount: data.RtxSuccessCount,
		rtxPerPacket:    data.RtxPerPacket,
		rtxRtt:          data.RtxRtt,
	}

	w.SetJitterStats(path, bufferStats)

}

// APIGstRtpSourceStatPut is called by api.
func (w *StatServer) APIGstRtpSourceStatPut(path string, data *defs.APIGstRtpSourceStats) {

	sourceStats := rtpSourceStats{
		packetsLost:     data.PacketsLost,
		packetsReceived: data.PacketsReceived,
		bitrate:         data.Bitrate,
		jitter:          data.Jitter,
	}

	w.SetRtpSourceStats(path, sourceStats)

}

// APIGstRtpSessionStatPut is called by api.
func (w *StatServer) APIGstRtpSessionStatPut(path string, data *defs.APIGstRtpSessionStats) {

	sessionStats := rtpSessionStats{
		rtxDropCount:  data.RtxDropCount,
		sentNackCount: data.SentNackCount,
		recvNackCount: data.RecvNackCount,
	}

	w.SetRtpSessionStats(path, sessionStats)

}
