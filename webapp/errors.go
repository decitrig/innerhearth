package webapp

import (
	"net/http"
	"time"

	"appengine"
	"appengine/datastore"
)

type ErrorLog struct {
	ID      int64 `datastore:"-"`
	Time    time.Time
	Message string
	Path    string
}

func NewErrorLog(c appengine.Context, r *http.Request, message string) (*ErrorLog, error) {
	log := &ErrorLog{
		ID:      0,
		Path:    r.URL.String(),
		Time:    time.Now(),
		Message: message,
	}
	tmpKey := datastore.NewIncompleteKey(c, "ErrorLog", nil)
	key, err := datastore.Put(c, tmpKey, log)
	if err != nil {
		return nil, err
	}
	log.ID = key.IntID()
	return log, nil
}
