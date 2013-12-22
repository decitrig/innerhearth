package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/delay"
	"appengine/mail"
	"appengine/taskqueue"
	"appengine/user"
)

// Errors returned from login-related functions.
var (
	ErrUserNotFound          = fmt.Errorf("User not found")
	ErrAlreadyConfirmed      = fmt.Errorf("User is already confirmed")
	ErrWrongConfirmationCode = fmt.Errorf("Wrong confirmation code")
	ErrEmailAlreadyClaimed   = fmt.Errorf("The email is already claimed")
)

var (
	noReply = "no-reply@innerhearthyoga.appspotmail.com"
)

var (
	delayedConfirmAccount = delay.Func("confirmAccount", func(c appengine.Context, user InnerHearthUser) error {
		buf := &bytes.Buffer{}
		if err := accountConfirmationEmail.Execute(buf, user); err != nil {
			c.Criticalf("Couldn't execute account confirm email: %s", err)
			return nil
		}
		msg := &mail.Message{
			Sender:  fmt.Sprintf("no-reply@%s.appspotmail.com", appengine.AppID(c)),
			To:      []string{user.Email},
			Subject: "Confirm your account registration with Inner Hearth Yoga",
			Body:    buf.String(),
		}
		if err := mail.Send(c, msg); err != nil {
			c.Criticalf("Couldn't send email to %q: %s", user.Email, err)
			return fmt.Errorf("failed to send email")
		}
		return nil
	})
)

var (
	// OpenIDProviders is a list of the OpenID providers we support.
	OpenIDProviders = []Provider{
		{"Google", "https://www.google.com/accounts/o8/id"},
		{"Yahoo", "yahoo.com"},
		{"AOL", "aol.com"},
	}
)

// provider represents an OpenID provider which we use for login.
type Provider struct {
	// The display name of the provider.
	Name string

	// The OpenID identifier given internally to AppEngine to create a login link.
	Identifier string
}

// AsLink returns a LoginLink to allow the user to login with the provider.
func (p Provider) AsLink(c appengine.Context, continueURL string) (LoginLink, error) {
	url, err := user.LoginURLFederated(c, continueURL, p.Identifier)
	if err != nil {
		return LoginLink{}, fmt.Errorf("failed to create login link for %q: %s", p.Name, err)
	}
	return LoginLink{p.Name, url}, nil
}

// MakeLinkList converts a list of Provider structs to a list of login
// links for display to a user.
func MakeLinkList(c appengine.Context, providers []Provider, continueURL string) ([]LoginLink, error) {
	links := make([]LoginLink, len(providers))
	for i, provider := range providers {
		link, err := provider.AsLink(c, continueURL)
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
type LoginLink struct {
	ProviderName string
	URL          string
}

// UserInfo stores basic user contact & identification information.
type UserInfo struct {
	FirstName string `datastore: ",noindex"`
	LastName  string
	Email     string
	Phone     string `datastore: ",noindex"`
}

// An InnerHearthUser is a registered user of the site.
type InnerHearthUser struct {
	AccountID string `datastore: "-"`

	UserInfo

	Confirmed        time.Time `datastore: ",noindex"`
	ConfirmationCode string    `datastore: ",noindex"`
}

func newConfirmationCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// NewInnerHearthUser creates a new InnerHearthUser for the given
// user.
func NewInnerHearthUser(u *user.User, info UserInfo) (*InnerHearthUser, error) {
	confirmCode, err := newConfirmationCode()
	if err != nil {
		return nil, fmt.Errorf("couldnt' create confirmation code: %s", err)
	}
	return &InnerHearthUser{
		AccountID:        SaltAndHashString(u.FederatedIdentity),
		UserInfo:         info,
		ConfirmationCode: confirmCode,
	}, nil
}

func userKeyFromID(c appengine.Context, id string) *datastore.Key {
	return datastore.NewKey(c, "InnerHearthUser", id, 0, nil)
}

func userKeyFromFederatedIdentity(c appengine.Context, u *user.User) *datastore.Key {
	return userKeyFromID(c, SaltAndHashString(u.FederatedIdentity))
}

func (u *InnerHearthUser) key(c appengine.Context) *datastore.Key {
	return userKeyFromID(c, u.AccountID)
}

func lookupUserByKey(c appengine.Context, key *datastore.Key) (*InnerHearthUser, error) {
	ihu := &InnerHearthUser{}
	if err := datastore.Get(c, key, ihu); err != nil {
		if err != datastore.ErrNoSuchEntity {
			c.Errorf("Failed looking up user %q: %s", key.StringID(), err)
		}
		return nil, ErrUserNotFound
	}
	ihu.AccountID = key.StringID()
	return ihu, nil
}

// LookupUser returns the InnerHearthUser for the given AppEngine
// user's federated identity, if any.
func LookupUser(c appengine.Context, u *user.User) (*InnerHearthUser, error) {
	return lookupUserByKey(c, userKeyFromFederatedIdentity(c, u))
}

// LookupUserByID returns the InnerHearthUser with the given ID, if any.
func LookupUserByID(c appengine.Context, id string) (*InnerHearthUser, error) {
	return lookupUserByKey(c, userKeyFromID(c, id))
}

// LookupUserByEmail returns the InnerHearthUser with the given email, if any.
func LookupUserByEmail(c appengine.Context, email string) (*InnerHearthUser, error) {
	q := datastore.NewQuery("InnerHearthUser").
		KeysOnly().
		Filter("Email =", email).
		Limit(1)
	keys, err := q.GetAll(c, nil)
	if err != nil {
		c.Errorf("Failure looking for user %q: %s", email, err)
		return nil, ErrUserNotFound
	}
	if len(keys) == 0 {
		return nil, ErrUserNotFound
	}
	return lookupUserByKey(c, keys[0])
}

// Store persists the InnerHearthUser to the datastore.
func (u *InnerHearthUser) Store(c appengine.Context) error {
	key := u.key(c)
	if _, err := datastore.Put(c, key, u); err != nil {
		return fmt.Errorf("failed to store user %q: %s", u.AccountID, err)
	}
	return nil
}

// SendConfirmation schedules a task to email a confirmation request
// to a new user.
func (u *InnerHearthUser) SendConfirmation(c appengine.Context) error {
	t, err := delayedConfirmAccount.Task(*u)
	if err != nil {
		return fmt.Errorf("error getting function task: %s", err)
	}
	t.RetryOptions = &taskqueue.RetryOptions{
		RetryLimit: 3,
	}
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		return fmt.Errorf("error adding confirmation to taskqueue: %s", err)
	}
	return nil
}

// Confirm marks the user as having confirmed their registration and
// stores the confirmation time back to the datastore.
func (u *InnerHearthUser) Confirm(c appengine.Context, code string, now time.Time) error {
	if u.ConfirmationCode == "" {
		return ErrAlreadyConfirmed
	}
	if code != u.ConfirmationCode {
		return ErrWrongConfirmationCode
	}
	u.Confirmed = now.In(time.UTC)
	u.ConfirmationCode = ""
	return u.Store(c)
}

// A UserEmail associates a user ID with an email address, enforcing uniqueness among email addresses.
type UserEmail struct {
	user  *datastore.Key
	Email string
}

// Creates a new UserEmail struct associating the user with their email.
func NewUserEmail(c appengine.Context, u *user.User, email string) *UserEmail {
	return &UserEmail{
		user:  userKeyFromFederatedIdentity(c, u),
		Email: email,
	}
}

func (e *UserEmail) key(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, "UserEmail", e.Email, 0, nil)
}

// Claim attempts to uniquely associate the user and email.
func (e *UserEmail) Claim(c appengine.Context) error {
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		key := e.key(c)
		old := &UserEmail{}
		lookupErr := datastore.Get(c, key, old)
		switch {
		case lookupErr == nil:
			return ErrEmailAlreadyClaimed
		case lookupErr == datastore.ErrNoSuchEntity:
			// Didn't find old claim: all is well.
			break
		default:
			return fmt.Errorf("failed to look up old UserEmail: %s", lookupErr)
		}

		if _, storeErr := datastore.Put(c, key, e); storeErr != nil {
			return fmt.Errorf("failed to store UserEmail: %s", storeErr)
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	return nil
}
