package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type StreamEvent struct {
	Path   string   `json:"path"`
	Active bool     `json:"active"`
	Tracks []string `json:"tracks,omitempty"`
}

func WebHookEventPublish(URL string, event StreamEvent) {
	if URL != "" {
		log.Print("Webhook Triggering")

		jsonData, jsonError := json.Marshal(event)
		if jsonError != nil {
			log.Fatal(jsonError)
		}
		response, err := http.Post(URL, "application/json", bytes.NewBuffer(jsonData))
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
