package model

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/memcache"
)

type AdminXSRFToken struct {
	Token      string
	Expiration time.Time
}

func (t *AdminXSRFToken) Validate(provided string) bool {
	if t == nil {
		return false
	}
	if !time.Now().Before(t.Expiration) {
		return false
	}
	return provided == t.Token
}

func marshalToken(token interface{}) ([]byte, error) {
	t, ok := token.(*AdminXSRFToken)
	if !ok {
		return nil, fmt.Errorf("Cannot marshal %T as an admin token", token)
	}
	return []byte(fmt.Sprintf("%s|%d", t.Token, t.Expiration.Unix())), nil
}

func unmarshalToken(value []byte, token interface{}) error {
	t, ok := token.(*AdminXSRFToken)
	if !ok {
		return fmt.Errorf("Cannot unmarshal token to %T", token)
	}
	v := string(value)
	parts := strings.Split(v, "|")
	expirationSeconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return err
	}
	t.Token = parts[0]
	t.Expiration = time.Unix(expirationSeconds, 0)
	return nil
}

var tokenCodec = &memcache.Codec{
	Marshal:   marshalToken,
	Unmarshal: unmarshalToken,
}

func xsrfTokenKey(id string) string {
	return "adminxsrftoken|" + id
}

func lookupCachedToken(c appengine.Context, id string) *AdminXSRFToken {
	token := &AdminXSRFToken{}
	if _, err := tokenCodec.Get(c, xsrfTokenKey(id), token); err != nil {
		return nil
	}
	return token
}

func lookupStoredToken(c appengine.Context, id string) (*AdminXSRFToken, error) {
	key := datastore.NewKey(c, "AdminXSRFToken", id, 0, nil)
	token := &AdminXSRFToken{}
	if err := datastore.Get(c, key, token); err != nil {
		return nil, err
	}
	cacheToken(c, id, token)
	return token, nil
}

func GetXSRFToken(c appengine.Context, id string) (*AdminXSRFToken, error) {
	token := lookupCachedToken(c, id)
	var err error
	if token == nil {
		token, err = lookupStoredToken(c, id)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return nil, err
		}
	}
	now := time.Now()
	if token != nil && now.Before(token.Expiration) {
		return token, nil
	}
	return MakeXSRFToken(c, id)
}

func token(id string) string {
	hash := sha512.New()
	hash.Write([]byte(time.Now().String()))
	hash.Write([]byte(id))
	return strings.Trim(base64.URLEncoding.EncodeToString(hash.Sum(nil)), "=")
}

func cacheToken(c appengine.Context, id string, token *AdminXSRFToken) {
	item := &memcache.Item{
		Key:    xsrfTokenKey(id),
		Object: token,
	}
	if err := tokenCodec.Set(c, item); err != nil {
		c.Errorf("Error caching admin token: %s", err)
	}
}

func storeToken(c appengine.Context, id string, token *AdminXSRFToken) error {
	key := datastore.NewKey(c, "AdminXSRFToken", id, 0, nil)
	if _, err := datastore.Put(c, key, token); err != nil {
		return err
	}
	return nil
}

func MakeXSRFToken(c appengine.Context, id string) (*AdminXSRFToken, error) {
	token := &AdminXSRFToken{
		Token:      token(id),
		Expiration: time.Now().AddDate(0, 0, 1),
	}
	if err := storeToken(c, id, token); err != nil {
		return nil, err
	}
	cacheToken(c, id, token)
	return token, nil
}

func ValidXSRFToken(c appengine.Context, id, providedToken string) bool {
	token, err := GetXSRFToken(c, id)
	if err != nil {
		return false
	}
	if !time.Now().Before(token.Expiration) {
		return false
	}
	if token.Token != providedToken {
		return false
	}
	return true
}

func MakeSessionID() string {
	hash := sha512.New()
	hash.Write([]byte(time.Now().String()))
	hash.Write([]byte{byte(rand.Intn(256))})
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}
