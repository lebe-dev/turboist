package scheduler

import "github.com/lebe-dev/turboist/internal/todoist"

func task(id string, labels ...string) *todoist.Task {
	if labels == nil {
		labels = []string{}
	}
	return &todoist.Task{ID: id, Content: "task " + id, Labels: labels}
}
