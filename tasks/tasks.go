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
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"text/template"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/mail"
	"appengine/taskqueue"

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
	http.HandleFunc("/task/delete-expired-tokens", deleteExpiredTokens)
}

type ConfirmationTask struct {
	Email   string
	ClassID int64
}

func parseConfirmationTask(r *http.Request) (*ConfirmationTask, error) {
	task := &ConfirmationTask{}
	b, err := urlDecode(r.FormValue("gob"))
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode gob: %s", err)
	}
	gobDecode(b, task)
	return task, nil
}

func (t ConfirmationTask) Post(c appengine.Context) error {
	posted := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
		"gob": {urlEncode(gobEncode(t))},
	})
	posted.RetryOptions = &taskqueue.RetryOptions{
		RetryLimit: 3,
	}
	if _, err := taskqueue.Add(c, posted, ""); err != nil {
		return err
	}
	return nil
}

func ConfirmRegistration(c appengine.Context, email string, class *model.Class) error {
	c.Infof("Sending confirmation email to %q", email)
	t := &ConfirmationTask{Email: email, ClassID: class.ID}
	return t.Post(c)
}

func sendRegistrationConfirmation(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	task, err := parseConfirmationTask(r)
	if err != nil {
		c.Criticalf("Could not parse gob for confirmation task: %s", err)
		return
	}
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(task.ClassID)
	if class == nil {
		c.Errorf("Couldn't find class %d", task.ClassID)
		http.Error(w, "Missing class", http.StatusInternalServerError)
		return
	}
	teacher := scheduler.GetTeacher(class)
	roster := model.NewRoster(c, class)
	student := roster.LookupStudent(task.Email)
	if student == nil {
		c.Errorf("Couldn't find student %q in %d", task.Email, class.ID)
		http.Error(w, "Missing registration", http.StatusInternalServerError)
		return
	}
	buf := &bytes.Buffer{}
	if err := confirmationEmail.Execute(buf, map[string]interface{}{
		"Class":   class,
		"Teacher": teacher,
		"Student": student,
	}); err != nil {
		c.Criticalf("Couldn't create email to '%s': %s", student.Email, err)
		return
	}
	sender := fmt.Sprintf("no-reply@%s.appspotmail.com", appengine.AppID(c))
	msg := &mail.Message{
		Sender:  sender,
		To:      []string{student.Email},
		Subject: fmt.Sprintf("Registration for %s at Inner Hearth Yoga", class.Title),
		Body:    buf.String(),
	}
	if err := mail.Send(c, msg); err != nil {
		c.Criticalf("Couldn't send email to '%s': %s", student.Email, err)
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

func deleteExpiredTokens(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	q := datastore.NewQuery("XSRFToken").
		Filter("Expiration <", time.Now()).
		KeysOnly()
	keys, err := q.GetAll(c, nil)
	if err != nil {
		c.Criticalf("Failed to get expired token keys: %s", err)
		return
	}
	if err := datastore.DeleteMulti(c, keys); err != nil {
		c.Warningf("Failed to delete expired tokens: %s", err)
	}
}

func gobEncode(d interface{}) []byte {
	buf := &bytes.Buffer{}
	e := gob.NewEncoder(buf)
	e.Encode(d)
	return buf.Bytes()
}

func urlEncode(data []byte) string {
	buf := &bytes.Buffer{}
	e := base64.NewEncoder(base64.URLEncoding, buf)
	e.Write(data)
	e.Close()
	return buf.String()
}

func gobDecode(b []byte, e interface{}) {
	decoder := gob.NewDecoder(bytes.NewReader(b))
	decoder.Decode(e)
}

func urlDecode(s string) ([]byte, error) {
	decoder := base64.NewDecoder(base64.URLEncoding, strings.NewReader(s))
	b, err := ioutil.ReadAll(decoder)
	if err != nil {
		return nil, err
	}
	return b, nil
}
