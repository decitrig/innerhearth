package auth

import (
	"reflect"
	"testing"
	"time"

	"appengine/aetest"
)

var (
	userID = "foo"
	path   = "/bar/baz"
	now    = time.Unix(100, 0)
)

func TestCreateXSRFTokens(t *testing.T) {
	tokens := make([]*Token, 100)
	for i, _ := range tokens {
		var err error
		tokens[i], err = NewToken(userID, path, now)
		if err != nil {
			t.Fatalf("Error making token: %s", err)
		}
		if exp := tokens[i].Expiration; !exp.After(now) {
			t.Fatalf("Wrong expiration for token: %s not after %s", exp, now)
		}
	}
	for i := 0; i < len(tokens)-1; i++ {
		for j := i + 1; j < len(tokens); j++ {
			if tok1, tok2 := tokens[i], tokens[j]; reflect.DeepEqual(tok1, tok2) {
				t.Fatalf("Got two identical tokens, %v and %v", tok1, tok2)
			}
		}
	}
}

func TestEquals(t *testing.T) {
	tok1, err := NewToken(userID, path, now)
	if err != nil {
		t.Fatalf("Error creating token: %s", err)
	}
	tok2, err := NewToken(userID, path, now)
	if err != nil {
		t.Fatalf("Error creating token: %s", err)
	}
	tok1a := *tok1
	if tok1.Equals(tok2) {
		t.Errorf("%v should not equal %v", tok1, tok2)
	}
	if !tok1.Equals(&tok1a) {
		t.Errorf("%v should equal %v", tok1, tok1a)
	}
}

func TestLoadStoreTokens(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatalf("Couldn't start local datastore: %s", err)
	}
	defer c.Close()

	tok, err := NewToken(userID, path, now)
	if err != nil {
		t.Fatalf("Error creating token: %s", err)
	}
	if err := tok.Store(c); err != nil {
		t.Fatalf("Error storing token: %s", err)
	}
	tok1, err := LookupToken(c, userID, path)
	if err != nil {
		t.Fatalf("Error looking up token: %s", err)
	}
	if !reflect.DeepEqual(tok, tok1) {
		t.Errorf("Tokens don't match; %v instead of %v", tok1, tok)
	}
	_, err = LookupToken(c, userID, path)
	if err != ErrTokenNotFound {
		t.Errorf("Should not have found token on second lookup")
	}
}

func TestValidation(t *testing.T) {
	tok1, _ := NewToken(userID, path, now)
	tok2, _ := NewToken(userID, path, now)
	unexpired := now.Add(59 * time.Minute)
	expired := now.Add(61 * time.Minute)
	if !tok1.IsValid(tok1.Encode(), unexpired) {
		t.Errorf("%v should have been valid at %s", tok1, unexpired)
	}
	if tok1.IsValid(tok1.Encode(), expired) {
		t.Errorf("%v should not have been valid at %s", tok1, unexpired)
	}
	if tok1.IsValid(tok2.Encode(), unexpired) {
		t.Errorf("%v should not have validated %s", tok1, tok2)
	}
}
