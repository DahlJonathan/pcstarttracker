package status

import (
	"testing"
	"time"

	"pc-tracker-server/internal/models"
)

func TestHeartbeatTimeout(t *testing.T) {
	now := time.Now().UTC()
	for _, test := range []struct {
		name  string
		age   time.Duration
		event string
		want  models.Status
	}{
		{"fresh", 15 * time.Second, "boot", models.StatusOnline},
		{"boundary", 45 * time.Second, "boot", models.StatusOnline},
		{"expired", 45*time.Second + time.Nanosecond, "boot", models.StatusOffline},
		{"shutdown", time.Second, "shutdown", models.StatusOffline},
	} {
		t.Run(test.name, func(t *testing.T) {
			beat := now.Add(-test.age)
			if got := Evaluate(test.event, &beat, now); got != test.want {
				t.Fatalf("status=%s want=%s", got, test.want)
			}
		})
	}
	if got := Evaluate("", nil, now); got != models.StatusOffline {
		t.Fatalf("unseen device status=%s", got)
	}
}
