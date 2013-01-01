package tasks

import (
	"bytes"
	"fmt"
	"net/http"
	"text/template"

	"appengine"
	"appengine/mail"

	"model"
)

var (
	confirmationEmail = template.Must(template.ParseFiles("registration/confirmation-email.txt"))
)

type taskHandler func(w http.ResponseWriter, r *http.Request) error

func (fn taskHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("Task error: %s", err)
		http.Error(w, "An error occurred", http.StatusInternalServerError)
	}
}

func handle(url string, handler taskHandler) {
	http.Handle(url, handler)
}

func init() {
	handle("/task/email-confirmation", emailConfirmation)
}

func emailConfirmation(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	account, err := model.GetAccountByID(c, r.FormValue("account"))
	if err != nil {
		return fmt.Errorf("Error looking up account: %s", err)
	}
	reg, err := model.GetRegistration(c, r.FormValue("class"), account.AccountID)
	if err != nil {
		return fmt.Errorf("Error looking up registration: %s", err)
	}
	buf := &bytes.Buffer{}
	if err := confirmationEmail.Execute(buf, map[string]interface{}{
		"Class": reg.ClassName,
		"Email": account.Email,
	}); err != nil {
		return fmt.Errorf("Error executing email template: %s", err)
	}
	msg := &mail.Message{
		Sender:  "no-reply@innerhearthyoga.appspotmail.com",
		To:      []string{account.Email},
		Subject: fmt.Sprintf("Registration for %s at Inner Hearth Yoga", r.FormValue("class")),
		Body:    buf.String(),
	}
	if err := mail.Send(c, msg); err != nil {
		return fmt.Errorf("Error sending confirmation email to %s: %s", r.FormValue("email"), err)
	}
	return nil
}
