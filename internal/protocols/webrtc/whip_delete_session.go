package webrtc

import (
	"context"
	"fmt"
	"net/http"
)

// WHIPDeleteSession deletes a WHIP/WHEP session.
func WHIPDeleteSession(
	ctx context.Context,
	hc *http.Client,
	ur string,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, ur, nil)
	if err != nil {
		return err
	}

	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}

	return nil
}
