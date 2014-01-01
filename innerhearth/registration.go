package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"appengine"
	"appengine/user"

	"github.com/decitrig/innerhearth/account"
	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/classes"
	"github.com/decitrig/innerhearth/students"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	classFullPage = template.Must(template.ParseFiles("templates/base.html", "templates/registration/class-full.html"))
)

func init() {
	webapp.HandleFunc("/register/session", registerForSession)
	webapp.HandleFunc("/register/oneday", registerForOneDay)
	webapp.HandleFunc("/register/paper", registerPaperStudent)
}

func classAndUser(w http.ResponseWriter, r *http.Request) (*account.Account, *classes.Class, *webapp.Error) {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		return nil, nil, badRequest(w, "Must be logged in.")
	}
	a, err := account.ForUser(c, u)
	if err != nil {
		return nil, nil, badRequest(w, "Must be registered.")
	}
	id, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return nil, nil, invalidData(w, "Couldn't parse class ID")
	}
	class, err := classes.ClassWithID(c, id)
	switch err {
	case nil:
		break
	case classes.ErrClassNotFound:
		return nil, nil, invalidData(w, "No such class")
	default:
		return nil, nil, webapp.InternalError(fmt.Errorf("failed to look up class %d: %s", id, err))
	}
	return a, class, nil
}

func registerForSession(w http.ResponseWriter, r *http.Request) *webapp.Error {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Method not allowed")
		return nil
	}
	c := appengine.NewContext(r)
	user, class, err := classAndUser(w, r)
	if err != nil {
		return err
	}
	token, ok := checkToken(c, user.ID, r.URL.Path, r.FormValue(auth.TokenFieldName))
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("Invalid auth token"))
	}
	student := students.New(user, class)
	switch err := student.Add(c, time.Now()); err {
	case nil:
		break
	case students.ErrClassIsFull:
		if err := classFullPage.Execute(w, class); err != nil {
			return webapp.InternalError(err)
		}
	default:
		return webapp.InternalError(fmt.Errorf("failed to write student: %s", err))
	}
	token.Delete(c)
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func registerForOneDay(w http.ResponseWriter, r *http.Request) *webapp.Error {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Method not allowed")
		return nil
	}
	c := appengine.NewContext(r)
	user, class, err := classAndUser(w, r)
	if err != nil {
		return err
	}
	token, ok := checkToken(c, user.ID, r.URL.Path, r.FormValue(auth.TokenFieldName))
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("Invalid auth token"))
	}
	date, dateErr := parseLocalDate(r.FormValue("date"))
	if dateErr != nil {
		return invalidData(w, "Invalid date; please use mm/dd/yyyy format")
	}
	// TODO(rwsims): The date here should really be the end time of the
	// class on the given day.
	student := students.NewDropIn(user, class, date)
	switch err := student.Add(c, time.Now()); err {
	case nil:
		break
	case students.ErrClassIsFull:
		if err := classFullPage.Execute(w, class); err != nil {
			return webapp.InternalError(err)
		}
	default:
		return webapp.InternalError(fmt.Errorf("failed to write student: %s", err))
	}
	token.Delete(c)
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func registerPaperStudent(w http.ResponseWriter, r *http.Request) *webapp.Error {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "Method not allowed")
		return nil
	}
	c := appengine.NewContext(r)
	user, class, werr := classAndUser(w, r)
	if werr != nil {
		return werr
	}
	token, ok := checkToken(c, user.ID, r.URL.Path, r.FormValue(auth.TokenFieldName))
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("Invalid auth token"))
	}
	fields, err := webapp.ParseRequiredValues(r, "firstname", "lastname", "email", "type")
	if err != nil {
		return missingFields(w)
	}
	acct, err := account.WithEmail(c, fields["email"])
	switch err {
	case nil:
		// Register with existing account
		break
	case account.ErrUserNotFound:
		// Need to create a paper account for this registration. This account will not be stored.
		info := account.Info{
			FirstName: fields["firstname"],
			LastName:  fields["lastname"],
			Email:     fields["email"],
		}
		if phone := fields["phone"]; phone != "" {
			info.Phone = phone
		}
		acct = account.Paper(info, class.ID)
		break
	default:
		return webapp.InternalError(fmt.Errorf("failed to look up account for %q: %s", fields["email"], err))
	}
	var student *students.Student
	if fields["type"] == "dropin" {
		// TODO(rwsims): The date here should really be the end time of the
		// class on the given day.
		date, err := parseLocalDate(r.FormValue("date"))
		if err != nil {
			return invalidData(w, "Invalid date; please use mm/dd/yyyy format")
		}
		student = students.NewDropIn(acct, class, date)
	} else {
		student = students.New(acct, class)
	}
	switch err := student.Add(c, time.Now()); err {
	case nil:
		break
	case students.ErrClassIsFull:
		if err := classFullPage.Execute(w, class); err != nil {
			return webapp.InternalError(err)
		}
	default:
		return webapp.InternalError(fmt.Errorf("failed to write student: %s", err))
	}
	token.Delete(c)
	http.Redirect(w, r, fmt.Sprintf("/roster?class=%d", class.ID), http.StatusSeeOther)
	return nil
}
