package auth

// Error is an authentication error.
type Error struct {
	Wrapped        error
	AskCredentials bool
}

// Error implements the error interface.
func (e *Error) Error() string {
	return "failed to authenticate: " + e.Wrapped.Error()
}
