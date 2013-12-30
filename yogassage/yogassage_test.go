package yogassage

import (
	"sort"
	"testing"
	"time"

	"appengine/aetest"

	"github.com/decitrig/innerhearth/classes"
)

func TestYinYogassage(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	yins := []*YinYogassage{
		New(time.Unix(1000, 0), "a"),
		New(time.Unix(2000, 0), "b"),
		New(time.Unix(3000, 0), "c"),
	}
	for i, y := range yins {
		if err := y.Insert(c); err != nil {
			t.Fatalf("Failed to insert yin %d: %s", i, err)
		}
		switch got, err := WithID(c, y.ID); {
		case err != nil:
			t.Fatalf("Didn't find yogassage class %d: %s", y.ID, err)
		case !yinsEqual(got, y):
			t.Errorf("Wrong yogassage for %d: %v vs %v", y.ID, got, y)
		}
	}
	got, err := Classes(c, time.Unix(1500, 0))
	if err != nil {
		t.Fatalf("Failed to get yogassage classes: %s", err)
	}
	sort.Sort(ByDate(got))
	for i, want := range yins[1:] {
		if got := got[i]; !yinsEqual(got, want) {
			t.Errorf("Wrong class at %d: %v vs %v", i, got, want)
		}
	}
	y := yins[0]
	if err := y.Delete(c); err != nil {
		t.Fatalf("Failed to delete class: %s", err)
	}
	if _, err := WithID(c, y.ID); err != classes.ErrClassNotFound {
		t.Errorf("Shouldn't have found class %d", y.ID)
	}
}

func yinsEqual(a, b *YinYogassage) bool {
	switch {
	case a == nil || b == nil:
		return a == b
	case a.ID != b.ID:
		return false
	case !a.Date.Equal(b.Date):
		return false
	case a.SignupLink != b.SignupLink:
		return false
	}
	return true
}
