package auth

// Credentials is a set of credentials (either user+pass or a token).
type Credentials struct {
	User  string
	Pass  string
	Token string
}
