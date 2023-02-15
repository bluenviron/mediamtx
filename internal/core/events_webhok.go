package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/aler9/rtsp-simple-server/internal/logger"
	"io"
	"log"
	"net/http"
)

type webhookServerParent interface {
	Log(logger.Level, string, ...interface{})
}

type WebhookInstance struct {
	URL    string
	parent webhookServerParent
}

func NewWebHook(url string, parent webhookServerParent) *WebhookInstance {
	web := &WebhookInstance{URL: url, parent: parent}
	web.log(logger.Info, "Web Hook Initialized (%s)", url)
	return web
}

func (receiver *WebhookInstance) Publish(event EventStream) {
	webhook := receiver.URL

	log.Print("Webhook Triggering")
	log.Print(webhook)

	jsonData, jsonError := json.Marshal(event)
	if jsonError != nil {
		log.Fatal(jsonError)
	}

	if webhook != "" {
		response, err := http.Post(webhook, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Print("Web Hook Failed")
			return
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				return
			}
		}(response.Body)
		body, bodyError := io.ReadAll(response.Body)
		if bodyError != nil {
			log.Fatal(bodyError)
		}
		if body != nil {
			logTest := fmt.Sprintf("webhook is triggred with path")
			log.Print(logTest)
		}
	}
}

func (receiver *WebhookInstance) log(level logger.Level, format string, args ...interface{}) {
	label := "WEBHOOK"
	receiver.parent.Log(level, "[%s] "+format, append([]interface{}{label}, args...)...)
}
