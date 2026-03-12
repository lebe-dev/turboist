package scheduler

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
)

// Job is a function executed periodically by the Scheduler.
type Job func(ctx context.Context)

// Scheduler runs a set of named jobs at a fixed interval.
type Scheduler struct {
	interval time.Duration
	jobs     []namedJob
}

type namedJob struct {
	name string
	fn   Job
}

// New creates a Scheduler that runs jobs every interval.
func New(interval time.Duration) *Scheduler {
	return &Scheduler{interval: interval}
}

// Register adds a job to the scheduler.
func (s *Scheduler) Register(name string, fn Job) {
	s.jobs = append(s.jobs, namedJob{name: name, fn: fn})
}

// Start launches all registered jobs in separate goroutines.
// Each goroutine stops when ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	for _, j := range s.jobs {
		log.Info("scheduler: starting job", "job", j.name, "interval", s.interval)
		go func() {
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					log.Info("scheduler: stopping job", "job", j.name)
					return
				case <-ticker.C:
					log.Debug("scheduler: running job", "job", j.name)
					j.fn(ctx)
				}
			}
		}()
	}
}
