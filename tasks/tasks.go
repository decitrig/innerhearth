package tasks

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"text/template"

	"appengine"
	"appengine/mail"

	"model"
)

var (
	confirmationEmail   = template.Must(template.ParseFiles("registration/confirmation-email.txt"))
	accountConfirmEmail = template.Must(template.ParseFiles("login/account-confirm-email.txt"))
	noReply             = "no-reply@innerhearthyoga.appspotmail.com"
)

func init() {
	http.HandleFunc("/task/email-confirmation", emailConfirmation)
	http.HandleFunc("/task/email-account-confirmation", emailAccountConfirmation)
}

func emailConfirmation(w http.ResponseWriter, r *http.Request) {
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	c := appengine.NewContext(r)
	if err != nil {
		c.Criticalf("Could not parse class ID %s from request: %s", r.FormValue("class"), err)
		return
	}
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		c.Errorf("Couldn't find class %d", classID)
		http.Error(w, "Missing class", http.StatusInternalServerError)
	}
	roster := model.NewRoster(c, class)
	reg := roster.LookupRegistration(r.FormValue("account"))
	if reg == nil {
		c.Errorf("Couldn't find registration of %s in %s", r.FormValue("account"), r.FormValue("class"))
		http.Error(w, "Missing registration", http.StatusInternalServerError)
		return
	}
	account, err := model.GetAccountByID(c, r.FormValue("account"))
	if err != nil {
		c.Errorf("Couldn't find account %s: %s", r.FormValue("account"), err)
		http.Error(w, "Missing account", http.StatusInternalServerError)
		return
	}
	buf := &bytes.Buffer{}
	if err := confirmationEmail.Execute(buf, map[string]interface{}{
		"Class": class,
		"Email": account.Email,
	}); err != nil {
		c.Criticalf("Couldn't create email to '%s': %s", account.Email, err)
		return
	}
	msg := &mail.Message{
		Sender:  noReply,
		To:      []string{account.Email},
		Subject: fmt.Sprintf("Registration for %s at Inner Hearth Yoga", class.Title),
		Body:    buf.String(),
	}
	if err := mail.Send(c, msg); err != nil {
		c.Criticalf("Couldn't send email to '%s': %s", account.Email, err)
	}
}

func emailAccountConfirmation(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	email := r.FormValue("email")
	if email == "" {
		c.Errorf("Couldn't parse email from task request: %s", r)
		return
	}
	code := r.FormValue("code")
	if code == "" {
		c.Errorf("Couldn't parse code from task request: %s", r)
		return
	}
	buf := &bytes.Buffer{}
	if err := accountConfirmEmail.Execute(buf, map[string]interface{}{
		"Email": email,
		"Code":  code,
	}); err != nil {
		c.Criticalf("Couldn't create account confirm template: %s", err)
		return
	}
	msg := &mail.Message{
		Sender:  noReply,
		To:      []string{email},
		Subject: "Inner Hearth Yoga account confirmation",
		Body:    buf.String(),
	}
	if err := mail.Send(c, msg); err != nil {
		c.Criticalf("Couldn't send email to '%s': %s", email, err)
	}
}
