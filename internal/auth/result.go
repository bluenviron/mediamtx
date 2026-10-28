package auth

import "time"

// Result is the result of Authenticate().
type Result struct {
	User      string
	ExpiresAt *time.Time
}
