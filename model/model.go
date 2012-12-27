package model

import (
	"errors"

	"appengine"
	"appengine/datastore"
)

var (
	ErrClassExists = errors.New("Class already exists")
)

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

type Registration struct {
	Student *datastore.Key
}

type Student struct {
	Email     string
	FirstName string `datastore: ",noindex"`
	LastName  string
}
