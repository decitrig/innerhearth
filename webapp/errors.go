package webapp

import (
	"runtime"
	"time"

	"appengine"
	"appengine/datastore"
)

type ErrorLog struct {
	ID         int64 `datastore:"-"`
	Time       time.Time
	Message    string
	Stacktrace []Caller
}

type Caller struct {
	Func string
	File string
	Line int
}

func getCallers(max int) []Caller {
	pcs := make([]uintptr, max)
	l := runtime.Callers(3, pcs)
	callers := make([]Caller, l)
	for i, pc := range pcs[:l] {
		f := runtime.FuncForPC(pc)
		file, line := f.FileLine(pc)
		callers[i] = Caller{
			Func: f.Name(),
			File: file,
			Line: line,
		}
	}
	return callers
}

func NewErrorLog(c appengine.Context, message string) (*ErrorLog, error) {
	log := &ErrorLog{
		ID:         0,
		Time:       time.Now(),
		Message:    message,
		Stacktrace: getCallers(10),
	}
	tmpKey := datastore.NewIncompleteKey(c, "ErrorLog", nil)
	key, err := datastore.Put(c, tmpKey, log)
	if err != nil {
		return nil, err
	}
	log.ID = key.IntID()
	return log, nil
}
