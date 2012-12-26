package registration

import (
	"fmt"
	"html/template"
	"net/http"
)

var (
	registrationForm = template.Must(template.ParseFiles("registration/form.html"))
)

func init() {
	http.HandleFunc("/register", register)
	http.HandleFunc("/register/new", newRegistration)
}

func register(w http.ResponseWriter, r *http.Request) {
	if err := registrationForm.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func newRegistration(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "new registration")
}
