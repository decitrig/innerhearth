package tasks

import (
	"bytes"
	"fmt"
	"net/http"
	"text/template"

	"appengine"
	"appengine/mail"
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
	reg, err := model.GetRegistration(c, r.FormValue("class"), r.FormValue("email"))
	if err != nil {
		return fmt.Errorf("Error looking up registration: %s", err)
	}
	buf := &bytes.Buffer{}
	if err := confirmationEmail.Execute(buf, map[string]interface{}{
		"Class":       r.FormValue("class"),
		"Email":       r.FormValue("email"),
		"ConfirmLink": "http://innerhearthyoga.appspot.com/registration/confirm?code=" + reg.ConfirmationCode,
		"RejectLink":  "http://innerhearthyoga.appspot.com/registration/reject?code=" + reg.ConfirmationCode,
	}); err != nil {
		return fmt.Errorf("Error executing email template: %s", err)
	}
	msg := &mail.Message{
		Sender:  "no-reply@innerhearthyoga.com",
		To:      []string{r.FormValue("email")},
		Subject: fmt.Sprintf("Please confirm registration for %s at Inner Hearth Yoga", r.FormValue("class")),
		Body:    buf.String(),
	}
	c := appengine.NewContext(r)
	if err := mail.Send(c, msg); err != nil {
		return fmt.Errorf("Error sending confirmation email to %s: %s", r.FormValue("email"), err)
	}
	return nil
}
