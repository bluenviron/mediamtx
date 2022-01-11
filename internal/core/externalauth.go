package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func externalAuth(
	ur string,
	ip string,
	user string,
	password string,
	path string,
	action string,
	query string,
) error {
	enc, _ := json.Marshal(struct {
		IP       string `json:"ip"`
		User     string `json:"user"`
		Password string `json:"password"`
		Path     string `json:"path"`
		Action   string `json:"action"`
		Query    string `json:"query"`
	}{
		IP:       ip,
		User:     user,
		Password: password,
		Path:     path,
		Action:   action,
		Query:    query,
	})
	res, err := http.Post(ur, "application/json", bytes.NewReader(enc))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	return nil
}
