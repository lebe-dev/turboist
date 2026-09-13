package inboxproc

import "time"

// Config is the runtime configuration of the processor, built from env.
type Config struct {
	Enabled    bool
	Interval   time.Duration
	APIURL     string
	APIKey     string
	Model      string
	BatchLimit int
	Timeout    time.Duration
}
