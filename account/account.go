package account

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

	"github.com/decitrig/innerhearth/auth"
)

var (
	ErrUserNotFound          = fmt.Errorf("User not found")
	ErrWrongConfirmationCode = fmt.Errorf("Wrong confirmation code")
	ErrEmailAlreadyClaimed   = fmt.Errorf("The email is already claimed")
)

var (
	delayedConfirmAccount = delay.Func("confirmAccount", func(c appengine.Context, user Account) error {
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

// Info stores basic user contact & identification data.
type Info struct {
	FirstName string `datastore: ",noindex"`
	LastName  string
	Email     string
	Phone     string `datastore: ",noindex"`
}

// An Account stores data about a registered user of the site.
type Account struct {
	ID string `datastore: "-"`
	Info

	Confirmed        time.Time `datastore: ",noindex"`
	ConfirmationCode string    `datastore: ",noindex"`
}

func newConfirmationCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// ID returns the internal ID for an appengine User.
func ID(u *user.User) (string, error) {
	if appengine.IsDevAppServer() {
		return auth.SaltAndHashString(u.Email), nil
	}
	var internal string
	switch {
	case u.Email != "":
		internal = u.Email
	case u.ID != "":
		internal = u.ID
	case u.FederatedIdentity != "":
		internal = u.FederatedIdentity
	}
	return auth.SaltAndHashString(internal), nil
}

// New creates a new Account for the given user.
func New(u *user.User, info Info) (*Account, error) {
	confirmCode, err := newConfirmationCode()
	if err != nil {
		return nil, fmt.Errorf("couldnt' create confirmation code: %s", err)
	}
	id, err := ID(u)
	if err != nil {
		return nil, err
	}
	return &Account{
		ID:               id,
		Info:             info,
		ConfirmationCode: confirmCode,
	}, nil
}

// Paper returns a stand-in account for handing paper registrations in
// the case where no Account yet exists.
func Paper(info Info, classID int64) *Account {
	return &Account{
		ID:   fmt.Sprintf("paper|%s|%d", info.Email, classID),
		Info: info,
	}
}

func keyForID(c appengine.Context, id string) *datastore.Key {
	return datastore.NewKey(c, "UserAccount", id, 0, nil)
}

func keyForUser(c appengine.Context, u *user.User) (*datastore.Key, error) {
	id, err := ID(u)
	if err != nil {
		return nil, err
	}
	return keyForID(c, id), nil
}

func isFieldMismatch(err error) bool {
	_, ok := err.(*datastore.ErrFieldMismatch)
	return ok
}

func byKey(c appengine.Context, key *datastore.Key) (*Account, error) {
	acct := &Account{}
	if err := datastore.Get(c, key, acct); err != nil {
		switch {
		case err == datastore.ErrNoSuchEntity:
			return nil, ErrUserNotFound
		case isFieldMismatch(err):
			c.Warningf("Type mismatch on user %q: %+v", key.StringID(), err)
			return acct, nil
		default:
			c.Errorf("Failed looking up user %q: %s", key.StringID(), err)
			return nil, ErrUserNotFound
		}
	}
	acct.ID = key.StringID()
	return acct, nil
}

func ForUser(c appengine.Context, u *user.User) (*Account, error) {
	key, err := keyForUser(c, u)
	if err != nil {
		return nil, err
	}
	return byKey(c, key)
}

func OldAccountForUser(c appengine.Context, u *user.User) (*Account, error) {
	return WithID(c, u.ID)
}

func WithID(c appengine.Context, id string) (*Account, error) {
	return byKey(c, keyForID(c, id))
}

func WithEmail(c appengine.Context, email string) (*Account, error) {
	q := datastore.NewQuery("UserAccount").
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
	return byKey(c, keys[0])
}

// Put persists the Account to the datastore.
func (u *Account) Put(c appengine.Context) error {
	if _, err := datastore.Put(c, keyForID(c, u.ID), u); err != nil {
		return err
	}
	return nil
}

// RewriteID transactionally rewrites the Account under the
// correct (i.e., obfuscated) key.
func (a *Account) RewriteID(c appengine.Context, u *user.User) error {
	var err error
	a.ID, err = ID(u)
	if err != nil {
		return fmt.Errorf("couldn't create ID for %v", u)
	}
	var txnErr error
	for i := 0; i < 10; i++ {
		txnErr = datastore.RunInTransaction(c, func(c appengine.Context) error {
			if err := a.Put(c); err != nil {
				return err
			}
			oldKey := datastore.NewKey(c, "UserAccount", u.ID, 0, nil)
			if err := datastore.Delete(c, oldKey); err != nil {
				return err
			}
			return nil
		}, &datastore.TransactionOptions{XG: true})
		if txnErr != datastore.ErrConcurrentTransaction {
			break
		}
	}
	if txnErr != nil {
		return txnErr
	}
	return nil
}

// SendConfirmation schedules a task to email a confirmation request
// to a new user.
func (u *Account) SendConfirmation(c appengine.Context) error {
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
func (u *Account) Confirm(c appengine.Context, code string, now time.Time) error {
	if u.ConfirmationCode == "" {
		// Already confirmed.
		return nil
	}
	if code != u.ConfirmationCode {
		return ErrWrongConfirmationCode
	}
	u.Confirmed = now.In(time.UTC)
	u.ConfirmationCode = ""
	return u.Put(c)
}

// A UserEmail associates a user ID with an email address, enforcing uniqueness among email addresses.
type ClaimedEmail struct {
	ClaimedBy *datastore.Key
	Email     string
}

// Creates a new ClaimedEmail struct associating the user with their email.
func NewClaimedEmail(c appengine.Context, id string, email string) *ClaimedEmail {
	return &ClaimedEmail{
		ClaimedBy: keyForID(c, id),
		Email:     email,
	}
}

func (e *ClaimedEmail) key(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, "ClaimedEmail", e.Email, 0, nil)
}

// Claim attempts to uniquely associate the user and email.
func (e *ClaimedEmail) Claim(c appengine.Context) error {
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		key := e.key(c)
		old := &ClaimedEmail{}
		lookupErr := datastore.Get(c, key, old)
		switch {
		case lookupErr == nil:
			return ErrEmailAlreadyClaimed
		case lookupErr == datastore.ErrNoSuchEntity:
			// Didn't find old claim: all is well.
			break
		default:
			return lookupErr
		}

		if _, storeErr := datastore.Put(c, key, e); storeErr != nil {
			return storeErr
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	return nil
}

// Delete removes a ClaimedEmail from the datastore, freeing that
// email for reclamation. Should only be used by site admins.
func (e *ClaimedEmail) Delete(c appengine.Context) error {
	key := e.key(c)
	if err := datastore.Delete(c, key); err != nil {
		return err
	}
	return nil
}
