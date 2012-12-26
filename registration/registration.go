package registration

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"
	"appengine/user"
)

var (
	registrationForm = template.Must(template.ParseFiles("registration/form.html"))
	adminPage        = template.Must(template.ParseFiles("registration/admin.html"))
)

func init() {
	http.HandleFunc("/registration", register)
	http.HandleFunc("/registration/new", newRegistration)
	http.HandleFunc("/registration/admin", admin)
}

type Class struct {
	ShortName       string
	LongName        string `datstore: ",noindex"`
	MaxStudents     int32  `datastore: ",noindex"`
	CurrentStudents int32
}

type Student struct {
	First string `datastore: ",noindex"`
	Last  string `datastore: ",noindex"`
	Email string
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	url, err := user.LoginURL(c, r.URL.String())
	if err != nil {
		c.Errorf("Error creating login url: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusSeeOther)
}

func admin(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if !user.IsAdmin(c) {
		redirectToLogin(w, r)
		return
	}
	data := make(map[string]string, 0)
	url, err := user.LogoutURL(c, r.URL.String())
	if err != nil {
		c.Errorf("Error creating logout URL: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data["LogoutURL"] = url
	if err := adminPage.Execute(w, data); err != nil {
		c.Errorf("Error writing admin page template: %s", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func register(w http.ResponseWriter, r *http.Request) {
	if err := registrationForm.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func newRegistration(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "new registration")
}
