package account

import (
	"text/template"
)

var (
	accountConfirmationEmail = template.Must(template.New("accountConfirm").Parse(`Thank you for registering an account with InnerHearthYoga.
You must confirm your account before registering for classes; you can confirm by visiting

http://innerhearthyoga.appspot.com/login/confirm?code={{.ConfirmationCode}}

in your web browser. If you have any questions, please contact us at info@innerhearthyoga.com. Thank you!`))
)
