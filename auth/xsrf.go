package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"
)

const (
	TokenFieldName = "xsrf_token"
)

// Errors returned from Token-related functions.
var (
	ErrTokenNotFound = fmt.Errorf("token not found")
)

// A Token is an unguessable challenge token sent along with requests
// to prevent CSRF attacks.
type Token struct {
	// Cryptographically random bytes.
	Token []byte

	// The ID string of the user who requested the token.
	UserID string

	// The path of the request for which the token is valid.
	Path string

	// The time after which the token is no longer valid.
	Expiration time.Time
}

// NewToken creates a new Token for a user making a request which will
// expire 1 hour after the given time.
func NewToken(userID, path string, now time.Time) (*Token, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("failed to make token: %s", err)
	}
	return &Token{
		Token:      b,
		UserID:     userID,
		Path:       path,
		Expiration: now.Add(1 * time.Hour),
	}, nil
}

func tokenKey(c appengine.Context, userID, path string) *datastore.Key {
	return datastore.NewKey(c, "Token", fmt.Sprintf("%s:%s", userID, path), 0, nil)
}

// key returns a datastore key for a token.
func (t *Token) key(c appengine.Context) *datastore.Key {
	return tokenKey(c, t.UserID, t.Path)
}

// TokenForRequest returns the stored auth token for a user request.
// If the token could not be found, returns ErrTokenNotFound.
func TokenForRequest(c appengine.Context, userID, path string) (*Token, error) {
	tok := &Token{}
	if err := datastore.Get(c, tokenKey(c, userID, path), tok); err != nil {
		if err != datastore.ErrNoSuchEntity {
			c.Errorf("failed to look up token: %s", err)
		}
		return nil, ErrTokenNotFound
	}
	if err := datastore.Delete(c, tokenKey(c, userID, path)); err != nil {
		c.Errorf("failed to delete token: %s", err)
	}
	return tok, nil
}

// Store persists a token into the datastore, and possibly an
// in-memory cache.
func (t *Token) Store(c appengine.Context) error {
	if _, err := datastore.Put(c, t.key(c), t); err != nil {
		return fmt.Errorf("failed to store token: %s", err)
	}
	return nil
}

// Encode returns an encoded string of the token, suitable for
// embedding in an HTML form.
func (t *Token) Encode() string {
	var buf bytes.Buffer
	buf.WriteString(t.UserID)
	buf.WriteString(t.Path)
	buf.Write(t.Token)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func (t *Token) String() string {
	return fmt.Sprintf("Token(%x %s %s %s)",
		t.Token, t.UserID, t.Path, t.Expiration.Format("2006-01-02 15:04"))
}

func (t *Token) Equals(u *Token) bool {
	return t.Encode() == u.Encode()
}

// IsValid returns true if the token's encoding matches the given
// encoded token and its expiration is not after now.
func (t *Token) IsValid(encoded string, now time.Time) bool {
	if t.Encode() != encoded {
		return false
	}
	return t.Expiration.After(now)
}

// Delete attempts to remove the token from the datastore.
func (t *Token) Delete(c appengine.Context) {
	datastore.Delete(c, t.key(c))
}
