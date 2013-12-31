package login

import (
	"fmt"

	"appengine"
	"appengine/user"
)

var (
	// OpenIDProviders is a list of the OpenID providers we support.
	OpenIDProviders = []Provider{
		{"Google", "https://www.google.com/accounts/o8/id"},
		{"Yahoo", "yahoo.com"},
		{"AOL", "aol.com"},
	}
)

// Provider represents an OpenID provider which we use for login.
type Provider struct {
	// The display name of the provider.
	Name string

	// The OpenID identifier given internally to AppEngine to create a login link.
	Identifier string
}

// Link returns a LoginLink to allow the user to login with the provider.
func (p Provider) Link(c appengine.Context, continueURL string) (Link, error) {
	url, err := user.LoginURLFederated(c, continueURL, p.Identifier)
	if err != nil {
		return Link{}, fmt.Errorf("failed to create login link for %q: %s", p.Name, err)
	}
	return Link{p.Name, url}, nil
}

// Links converts a list of Provider structs to a list of login links
// for display to a user.
func Links(c appengine.Context, providers []Provider, continueURL string) ([]Link, error) {
	links := make([]Link, len(providers))
	for i, provider := range providers {
		link, err := provider.Link(c, continueURL)
		if err != nil {
			c.Errorf("Couldn't create login link for %q: %s", provider.Identifier, err)
			return nil, fmt.Errorf("invalid provider ID %q", provider.Identifier)
		}
		links[i] = link
	}
	return links, nil
}

// A LoginLink is a login redirect URL associated with the name of the
// OpenID provider to which it redirects.
type Link struct {
	ProviderName string
	URL          string
}
