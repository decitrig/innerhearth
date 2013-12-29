package scheduling

import (
	"time"

	"github.com/decitrig/innerhearth/auth"
)

var (
	stafferSmith = &Staff{"0x1", auth.UserInfo{"staffer", "smith", "foo@foo.com", ""}}
)

func unix(seconds int64) time.Time {
	return time.Unix(seconds, 0)
}
