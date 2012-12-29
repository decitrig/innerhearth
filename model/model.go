package model

import (
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"
)

var (
	ErrClassExists = errors.New("Class already exists")
)

type ClassFullError struct {
	Class string
}

func (e *ClassFullError) Error() string {
	return fmt.Sprintf("Class %s is full", e.Class)
}

type Class struct {
	Name          string
	Capacity      int32
	Registrations int32
}

func NewClass(name string, capacity int32) *Class {
	return &Class{
		Name:     name,
		Capacity: capacity,
	}
}

func classFullError(class string) error {
	return &ClassFullError{class}
}

func (c *Class) Insert(context appengine.Context) error {
	key := datastore.NewKey(context, "Class", c.Name, 0, nil)
	err := datastore.RunInTransaction(context, func(context appengine.Context) error {
		var existing *Class
		if err := datastore.Get(context, key, existing); err != datastore.ErrNoSuchEntity {
			return ErrClassExists
		}
		_, err := datastore.Put(context, key, c)
		return err
	}, nil)
	return err
}

func ListClasses(c appengine.Context) ([]Class, error) {
	cs := make([]Class, 0)
	q := datastore.NewQuery("Class").
		Order("Name")
	if _, err := q.GetAll(c, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

func DeleteClass(c appengine.Context, className string) error {
	if len(className) == 0 {
		return fmt.Errorf("Must provide class name")
	}
	key := datastore.NewKey(c, "Class", className, 0, nil)
	if err := datastore.Delete(c, key); err != nil {
		return fmt.Errorf("Error deleting %s: %s", className, err)
	}
	return nil
}

type Registration struct {
	ClassName        string
	StudentEmail     string
	Created          time.Time
	Confirmed        time.Time
	ConfirmationCode string `datastore: ",noindex"`
}

func newConfirmationCode(class, email string) string {
	hash := sha512.New()
	hash.Write([]byte(class))
	hash.Write([]byte(email))
	hash.Write([]byte(time.Now().String()))
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

func NewRegistration(c appengine.Context, className, studentEmail string) *Registration {
	return &Registration{
		ClassName:        className,
		StudentEmail:     studentEmail,
		Created:          time.Now(),
		ConfirmationCode: newConfirmationCode(className, studentEmail),
	}
}

func (r *Registration) createKey(c appengine.Context) *datastore.Key {
	classKey := datastore.NewKey(c, "Class", r.ClassName, 0, nil)
	return datastore.NewKey(c, "Registration", r.StudentEmail, 0, classKey)
}

func (r *Registration) Insert(c appengine.Context) error {
	key := r.createKey(c)
	classKey := datastore.NewKey(c, "Class", r.ClassName, 0, nil)
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		// Check that the registration is not a duplicate.
		old := &Registration{}
		if err := datastore.Get(c, key, old); err != nil {
			if err != datastore.ErrNoSuchEntity {
				return fmt.Errorf("Could not read registration %v: %s", r, err)
			}
		} else {
			return fmt.Errorf("Registration %v is a duplicate", r)
		}

		// Check that there is space in the class & reserve space.
		class := &Class{}
		if err := datastore.Get(c, classKey, class); err != nil {
			return fmt.Errorf("Could not read class %s: %s", r.ClassName, err)
		}
		if class.Registrations >= class.Capacity {
			return classFullError(class.Name)
		}
		class.Registrations++
		if _, err := datastore.Put(c, classKey, class); err != nil {
			return fmt.Errorf("Could not write class %s: %s", class.Name, err)
		}

		// Write the registration info.
		if _, err := datastore.Put(c, key, r); err != nil {
			return fmt.Errorf("Could not write new registration %v: %s", r, err)
		}
		return nil
	}, nil)
	return err
}
