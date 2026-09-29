// Module identity rules resolve names and slots without inferring physical hardware IDs.
package modules

import (
	"regexp"
)

var RackPrefixPattern = regexp.MustCompile(`^A[0-9]{1,6}$`)

var RackSlotPattern = regexp.MustCompile(`^(A[0-9]{1,6})-([0-9]{2,4})$`)
