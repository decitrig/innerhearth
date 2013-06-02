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
package model

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"appengine"
	"appengine/datastore"
)

var (
	ErrClassFull         = errors.New("Class is full")
	ErrClassExists       = errors.New("Class already exists")
	ErrClassNotEmpty     = errors.New("Class is not empty")
	ErrAlreadyRegistered = errors.New("Student is already registered for class")
	ErrInvalidDropInDate = errors.New("Invalid drop in date")
)

type Session struct {
	ID int64 `datastore: "-"`

	Name  string
	Start time.Time
	End   time.Time
}

type byStartDate []*Session

func (l byStartDate) Len() int      { return len(l) }
func (l byStartDate) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l byStartDate) Less(i, j int) bool {
	return l[i].Start.Before(l[j].Start)
}

func AddSession(c appengine.Context, s *Session) error {
	key := datastore.NewIncompleteKey(c, "Session", nil)
	key, err := datastore.Put(c, key, s)
	if err != nil {
		return fmt.Errorf("error writing session: %s", err)
	}
	s.ID = key.IntID()
	return nil
}

func GetSession(c appengine.Context, id int64) *Session {
	k := datastore.NewKey(c, "Session", "", id, nil)
	session := &Session{}
	if err := datastore.Get(c, k, session); err != nil {
		if err != datastore.ErrNoSuchEntity {
			c.Errorf("Error looking up session %d: %s", id, err)
		}
		return nil
	}
	session.ID = k.IntID()
	return session
}

func ListSessions(c appengine.Context, now time.Time) []*Session {
	q := datastore.NewQuery("Session").
		Filter("End >=", dateOnly(now))
	sessions := []*Session{}
	keys, err := q.GetAll(c, &sessions)
	if err != nil {
		c.Errorf("Error looking up sessions: %s", err)
		return nil
	}
	for i, key := range keys {
		sessions[i].ID = key.IntID()
	}
	sort.Sort(byStartDate(sessions))
	return sessions
}

func (s *Session) ListClasses(c appengine.Context) []*ClassCalendarData {
	q := datastore.NewQuery("Class").
		Filter("Session =", s.ID)
	classes := []*Class{}
	keys, err := q.GetAll(c, &classes)
	if err != nil {
		c.Errorf("Error looking up classes for session %d: %s", s.ID, err)
		return nil
	}
	for i, key := range keys {
		classes[i].ID = key.IntID()
	}
	data := make([]*ClassCalendarData, len(classes))
	for i, class := range classes {
		data[i] = getCalendarData(c, class)
	}
	return data
}

type Class struct {
	ID              int64  `datastore: "-"`
	Title           string `datastore: ",noindex"`
	LongDescription []byte `datastore: ",noindex"`
	Teacher         *datastore.Key

	Weekday   time.Weekday
	StartTime time.Time     `datastore: ",noindex"`
	Length    time.Duration `datastore: ",noindex"`

	Session int64

	DropInOnly bool
	Capacity   int32 `datastore: ",noindex"`

	// The following fields are deprecated, but exist in legacy data.
	BeginDate     time.Time
	EndDate       time.Time
	SpacesLeft    int32
	Active        bool
	Description   string `datastore: ",noindex"`
	LengthMinutes int32  `datastore: ",noindex"`
	DayOfWeek     string
}

func (c *Class) Before(d *Class) bool {
	switch {
	case c.Weekday != d.Weekday:
		return c.Weekday < d.Weekday

	case c.StartTime != d.StartTime:
		return c.StartTime.Before(d.StartTime)
	}
	return false
}

func GetClass(c appengine.Context, id int64) (*Class, error) {
	k := datastore.NewKey(c, "Class", "", id, nil)
	class := &Class{}
	if err := datastore.Get(c, k, class); err != nil {
		if err != datastore.ErrNoSuchEntity {
			return nil, fmt.Errorf("error looking up class: %s", err)
		}
		return nil, nil
	}
	class.ID = id
	return class, nil
}

func ListClasses(c appengine.Context) []*Class {
	q := datastore.NewQuery("Class")
	classes := []*Class{}
	keys, err := q.GetAll(c, &classes)
	if err != nil {
		c.Errorf("Error listing classes: %s", err)
		return nil
	}
	for i, key := range keys {
		classes[i].ID = key.IntID()
	}
	return classes
}

type ClassCalendarData struct {
	*Class
	*Teacher
	Description string
	EndTime     time.Time
}

type classCalendarDataList []*ClassCalendarData

func (l classCalendarDataList) Len() int      { return len(l) }
func (l classCalendarDataList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l classCalendarDataList) Less(i, j int) bool {
	return l[i].Class.Before(l[j].Class)
}

func getCalendarData(c appengine.Context, class *Class) *ClassCalendarData {
	teacher := &Teacher{}
	if err := datastore.Get(c, class.Teacher, teacher); err != nil {
		c.Errorf("Error looking up teacher for class %d: %s", class.ID, err)
		return nil
	}
	return &ClassCalendarData{
		Class:       class,
		EndTime:     class.StartTime.Add(class.Length),
		Teacher:     teacher,
		Description: string(class.LongDescription),
	}
}

// NextClassTime returns the earliest start time of the class which starts strictly later than the
// given time.
func (c *Class) NextClassTime(after time.Time) time.Time {
	return time.Time{}
}

func (c *Class) key(ctx appengine.Context) *datastore.Key {
	return datastore.NewKey(ctx, "Class", "", c.ID, nil)
}

func (c *Class) Registrations() int32 {
	return c.Capacity - c.SpacesLeft
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(),
		0, 0, 0, 0, t.Location())
}

func dateTime(d, t time.Time) time.Time {
	return time.Date(
		d.Year(), d.Month(), d.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location(),
	)
}

func (c *Class) GetExpirationTime() time.Time {
	return c.GetEndingTime(c.EndDate)
}

func (c *Class) GetEndingTime(date time.Time) time.Time {
	t := c.StartTime.Add(time.Minute * time.Duration(c.LengthMinutes))
	return dateTime(date, t)
}

func (c *Class) ValidDate(date time.Time) bool {
	if c.Weekday != date.Weekday() {
		return false
	}
	if c.DropInOnly {
		return true
	}
	day := dateOnly(date)
	if day.Before(c.BeginDate) || day.After(c.EndDate) {
		return false
	}
	return true
}

func (c *Class) Write(ctx appengine.Context) error {
	k := datastore.NewKey(ctx, "Class", "", c.ID, nil)
	c.Description = ""
	if _, err := datastore.Put(ctx, k, c); err != nil {
		return fmt.Errorf("Error writing class %d: %s", c.ID, err)
	}
	return nil
}

// A Scheduler is responsible for manipulating classes.
type Scheduler interface {
	AddNew(c *Class) error
	GetClass(id int64) *Class
	ListActiveClasses() []*Class
	ListAllClasses() []*Class
	GetCalendarData(class *Class) *ClassCalendarData
	ListCalendarData(classes []*Class) []*ClassCalendarData
	DeleteClass(c *Class) error
	GetTeacherNames(classes []*Class) map[int64]string
	GetTeacher(class *Class) *UserAccount
	ListClassesForTeacher(teacher *Teacher) []*Class
	WriteClass(class *Class) error
	GetTeachers(classes []*Class) []*Teacher
}

type scheduler struct {
	appengine.Context
}

func NewScheduler(c appengine.Context) Scheduler {
	return &scheduler{c}
}

func (s *scheduler) GetClass(id int64) *Class {
	key := datastore.NewKey(s, "Class", "", id, nil)
	class := &Class{}
	if err := datastore.Get(s, key, class); err != nil {
		s.Errorf("Error getting class %d: %s", id, err)
		return nil
	}
	class.ID = id
	class.Description = string(class.LongDescription)
	return class
}

func (s *scheduler) AddNew(c *Class) error {
	key := datastore.NewIncompleteKey(s, "Class", nil)
	if _, err := datastore.Put(s, key, c); err != nil {
		return fmt.Errorf("Error writing class %s: %s", c.Title, err)
	}
	return nil
}

func (s *scheduler) ListActiveClasses() []*Class {
	dropins := []*Class{}
	q := datastore.NewQuery("Class").
		Filter("DropInOnly = ", true)
	keys, err := q.GetAll(s, &dropins)
	if err != nil {
		s.Errorf("Error listing drop in classes: %s", err)
		return nil
	}
	for i, class := range dropins {
		class.ID = keys[i].IntID()
	}

	sessions := []*Class{}
	q = datastore.NewQuery("Class").
		Filter("DropInOnly =", false).
		Filter("EndDate >=", dateOnly(time.Now()))
	keys, err = q.GetAll(s, &sessions)
	if err != nil {
		s.Errorf("Error listing session classes: %s", err)
		return nil
	}
	for i, class := range sessions {
		class.ID = keys[i].IntID()
	}
	openClasses := append(dropins, sessions...)
	return openClasses
}

func (s *scheduler) ListAllClasses() []*Class {
	classes := []*Class{}
	keys, err := datastore.NewQuery("Class").GetAll(s, &classes)
	if err != nil {
		s.Errorf("Error listing drop in classes: %s", err)
		return nil
	}
	for i, class := range classes {
		class.ID = keys[i].IntID()
	}
	return classes
}

func (s *scheduler) GetCalendarData(class *Class) *ClassCalendarData {
	teacher := &Teacher{}
	if err := datastore.Get(s, class.Teacher, teacher); err != nil {
		s.Errorf("Error looking up teacher for class %d: %s", class.ID, err)
		return nil
	}
	return &ClassCalendarData{
		Class:       class,
		EndTime:     class.StartTime.Add(class.Length),
		Teacher:     teacher,
		Description: string(class.LongDescription),
	}
}

func (s *scheduler) ListCalendarData(classes []*Class) []*ClassCalendarData {
	teacherKeys := make([]*datastore.Key, len(classes))
	teachers := make([]*Teacher, len(classes))
	for i, class := range classes {
		teacherKeys[i] = class.Teacher
		teachers[i] = &Teacher{}
	}
	if err := datastore.GetMulti(s, teacherKeys, teachers); err != nil {
		s.Errorf("Error looking up class teachers: %s", err)
		return nil
	}
	data := make([]*ClassCalendarData, len(classes))
	for i, class := range classes {
		data[i] = &ClassCalendarData{
			Class:       class,
			EndTime:     class.StartTime.Add(class.Length),
			Teacher:     teachers[i],
			Description: string(class.LongDescription),
		}
	}
	sort.Sort(classCalendarDataList(data))
	return data
}

func (s *scheduler) ListClassesForTeacher(t *Teacher) []*Class {
	q := datastore.NewQuery("Class").
		Filter("Teacher =", t.key(s))
	classes := []*Class{}
	keys, err := q.GetAll(s, &classes)
	if err != nil {
		s.Errorf("Error listing classes for teacher %s: %s", t.AccountID, err)
		return nil
	}
	for idx, key := range keys {
		classes[idx].ID = key.IntID()
	}
	return classes
}

func (s *scheduler) GetTeacherNames(classes []*Class) map[int64]string {
	keys := make([]*datastore.Key, len(classes))
	teachers := make([]*UserAccount, len(classes))
	for idx, class := range classes {
		keys[idx] = class.Teacher
		teachers[idx] = &UserAccount{}
	}
	if err := datastore.GetMulti(s, keys, teachers); err != nil {
		s.Errorf("Error looking up teacher names: %s", err)
		return nil
	}
	names := map[int64]string{}
	for idx, class := range classes {
		names[class.ID] = teachers[idx].FirstName
	}
	return names
}

func (s *scheduler) GetTeacher(class *Class) *UserAccount {
	t := &UserAccount{}
	if err := datastore.Get(s, class.Teacher, t); err != nil {
		s.Criticalf("Couldn't get teacher for class %d: %s", class.ID, err)
		return nil
	}
	return t
}

func (s *scheduler) DeleteClass(c *Class) error {
	key := c.key(s)
	err := datastore.RunInTransaction(s, func(context appengine.Context) error {
		q := datastore.NewQuery("Registration").
			Ancestor(key).
			KeysOnly().
			Limit(1)
		keys, err := q.GetAll(context, nil)
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			return ErrClassNotEmpty
		}
		if err := datastore.Delete(context, key); err != nil {
			return err
		}
		return nil
	}, nil)
	if err != nil && err != ErrClassNotEmpty {
		return fmt.Errorf("Error deleting class %d: %s", c.ID, err)
	}
	return err
}

func (s *scheduler) WriteClass(c *Class) error {
	key := c.key(s)
	c.Description = ""
	if _, err := datastore.Put(s, key, c); err != nil {
		return fmt.Errorf("Error writing class %d: %s", c.ID, err)
	}
	return nil
}

func (s *scheduler) GetTeachers(classes []*Class) []*Teacher {
	keys := make([]*datastore.Key, len(classes))
	teachers := make([]*Teacher, len(classes))
	for i, class := range classes {
		keys[i] = class.Teacher
		teachers[i] = &Teacher{}
	}
	if err := datastore.GetMulti(s, keys, teachers); err != nil {
		s.Errorf("Error looking up teachers: %s", err)
		return nil
	}
	return teachers
}

// A Registration represents a reserved space in a class, either for the entire session or as a drop
// in.
type Registration struct {
	StudentID string
	ClassID   int64

	// The last date on which this registration is still valid.
	Date   time.Time
	DropIn bool
}

func (r *Registration) key(c appengine.Context) *datastore.Key {
	classKey := datastore.NewKey(c, "Class", "", r.ClassID, nil)
	return datastore.NewKey(c, "Registration", r.StudentID, 0, classKey)
}

type Student struct {
	ClassID int64
	Email   string

	FirstName string
	LastName  string
	Phone     string

	Date   time.Time
	DropIn bool
}

func NewSessionStudent(class *Class, user *UserAccount) *Student {
	return &Student{
		ClassID:   class.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Phone:     user.Phone,
		DropIn:    false,
	}
}

func NewDropInStudent(class *Class, user *UserAccount, date time.Time) *Student {
	return &Student{
		ClassID:   class.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Phone:     user.Phone,
		DropIn:    true,
		Date:      dateOnly(date),
	}
}

type Roster interface {
	LookupStudent(email string) *Student
	ListStudents() []*Student
	AddStudent(student *Student) error
}

type roster struct {
	appengine.Context
	class *Class
}

func NewRoster(c appengine.Context, class *Class) Roster {
	return &roster{c, class}
}

func filterExpiredStudents(students []*Student) []*Student {
	out := []*Student{}
	now := dateOnly(time.Now())
	for _, student := range students {
		if !student.DropIn || !student.Date.Before(now) {
			out = append(out, student)
		}
	}
	return out
}

func (r *roster) LookupStudent(email string) *Student {
	key := datastore.NewKey(r, "Student", email, 0, r.class.key(r))
	student := &Student{}
	if err := datastore.Get(r, key, student); err != nil {
		if err != datastore.ErrNoSuchEntity {
			r.Errorf("Error looking up student %q in class %d: %s", email, r.class.ID, err)
		}
		return nil
	}
	if student.DropIn {
		now := dateOnly(time.Now())
		if student.Date.Before(now) {
			return nil
		}
	}
	return student
}

func (r *roster) ListStudents() []*Student {
	q := datastore.NewQuery("Student").
		Ancestor(r.class.key(r))
	students := []*Student{}
	if _, err := q.GetAll(r, &students); err != nil {
		r.Errorf("Error looking up students for class %d: %s", r.class.ID, err)
		return nil
	}
	return filterExpiredStudents(students)
}

func (r *roster) AddStudent(student *Student) error {
	classKey := datastore.NewKey(r, "Class", "", r.class.ID, nil)
	studentKey := datastore.NewKey(r, "Student", student.Email, 0, classKey)
	err := datastore.RunInTransaction(r, func(ctx appengine.Context) error {
		old := &Student{}
		if err := datastore.Get(ctx, studentKey, old); err != datastore.ErrNoSuchEntity {
			if err != nil {
				return fmt.Errorf("Error looking up student %s: %s", student.Email, err)
			}
			if !old.DropIn {
				return ErrAlreadyRegistered
			}
			now := dateOnly(time.Now())
			if !old.Date.Before(now) {
				return ErrAlreadyRegistered
			}
		}

		class := &Class{}
		if err := datastore.Get(ctx, classKey, class); err != nil {
			return fmt.Errorf("Error looking up class %d: %s", r.class.ID, err)
		}
		class.ID = classKey.IntID()
		q := datastore.NewQuery("Student").
			Ancestor(classKey)
		students := []*Student{}
		if _, err := q.GetAll(r, &students); err != nil {
			return fmt.Errorf("Error counting registration: %s", err)
		}
		students = filterExpiredStudents(students)
		if int32(len(students)) >= class.Capacity {
			return ErrClassFull
		}

		if _, err := datastore.Put(ctx, studentKey, student); err != nil {
			return fmt.Errorf("Error adding student %s to class %d: %s", student.Email, class.ID, err)
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	return nil
}

type Registrar interface {
	ListRegistrations() []*Student
	ListRegisteredClasses([]*Student) []*RegisteredClass
}

type registrar struct {
	appengine.Context
	user *UserAccount
}

func NewRegistrar(c appengine.Context, user *UserAccount) Registrar {
	return &registrar{c, user}
}

func (r *registrar) ListRegistrations() []*Student {
	students := []*Student{}
	q := datastore.NewQuery("Student").
		Filter("Email =", r.user.Email)
	if _, err := q.GetAll(r, &students); err != nil {
		return nil
	}
	return filterExpiredStudents(students)
}

type RegisteredClass struct {
	*Class
	Teacher *Teacher
	*Student
}

type registeredClassList []*RegisteredClass

func (l registeredClassList) Len() int      { return len(l) }
func (l registeredClassList) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l registeredClassList) Less(i, j int) bool {
	a, b := l[i], l[j]
	if !a.Student.DropIn && b.Student.DropIn {
		return true
	}
	if a.Student.DropIn && !b.Student.DropIn {
		return false
	}
	if a.Student.DropIn && b.Student.DropIn {
		return a.Student.Date.Before(b.Student.Date)
	}
	return l[i].Class.Before(l[j].Class)
}

func (r *registrar) ListRegisteredClasses(regs []*Student) []*RegisteredClass {
	classKeys := make([]*datastore.Key, len(regs))
	classes := make([]*Class, len(regs))
	for i, reg := range regs {
		classKeys[i] = datastore.NewKey(r, "Class", "", reg.ClassID, nil)
		classes[i] = &Class{}
	}
	if err := datastore.GetMulti(r, classKeys, classes); err != nil {
		r.Errorf("Error getting registered classes for %s: %s", r.user.AccountID, err)
		return nil
	}
	teacherKeys := make([]*datastore.Key, len(classes))
	teachers := make([]*Teacher, len(classes))
	for i, class := range classes {
		teacherKeys[i] = class.Teacher
		teachers[i] = &Teacher{}
	}
	if err := datastore.GetMulti(r, teacherKeys, teachers); err != nil {
		r.Errorf("Error looking up teachers: %s", err)
		return nil
	}
	registered := make([]*RegisteredClass, len(regs))
	for i, _ := range registered {
		classes[i].ID = classKeys[i].IntID()
		registered[i] = &RegisteredClass{
			Class:   classes[i],
			Student: regs[i],
			Teacher: teachers[i],
		}
	}
	sort.Sort(registeredClassList(registered))
	return registered
}
