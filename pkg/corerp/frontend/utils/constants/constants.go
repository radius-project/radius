package constants

import (
	"time"
)

const (
	RetryAfterHeader = "Retry-After"
	ArmPollDuration  = time.Duration(10) * time.Second
)
