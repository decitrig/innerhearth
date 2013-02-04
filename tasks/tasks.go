/*
 *  Copyright 2013 Ryan W Sims (rwsims@gmail.com)
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
package tasks

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"text/template"

	"appengine"
	"appengine/mail"

	"github.com/decitrig/innerhearth/model"
)

var (
	confirmationEmail   = template.Must(template.ParseFiles("templates/registration/confirmation-email.txt"))
	accountConfirmEmail = template.Must(template.ParseFiles("templates/account-confirm-email.txt"))
	noReply             = "no-reply@innerhearthyoga.appspotmail.com"
)

func init() {
	http.HandleFunc("/task/email-confirmation", sendRegistrationConfirmation)
	http.HandleFunc("/task/email-account-confirmation", sendNewAccountConfirmation)
}

func sendRegistrationConfirmation(w http.ResponseWriter, r *http.Request) {
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
	teacher := scheduler.GetTeacher(class)
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
		"Class":   class,
		"Email":   account.Email,
		"Teacher": teacher,
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

func sendNewAccountConfirmation(w http.ResponseWriter, r *http.Request) {
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
