package history

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "history.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func mustRecord(t *testing.T, s *Store, events ...Event) {
	t.Helper()
	for _, e := range events {
		if err := s.Record(e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPortFights(t *testing.T) {
	now := time.Now()
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	kill := func(port, pid int, at time.Time) Event {
		return Event{At: at, Port: port, PID: pid, Process: "node", Project: "app", KilledViaPortfind: true}
	}
	exit := func(port, pid int, at time.Time) Event {
		return Event{At: at, Port: port, PID: pid, Process: "node", KilledViaPortfind: false}
	}

	tests := []struct {
		name   string
		events []Event
		want   map[int]int // port -> kills
	}{
		{"no events", nil, map[int]int{}},
		{"three kills in window", []Event{
			kill(3000, 1, ago(10*time.Minute)), kill(3000, 2, ago(5*time.Minute)), kill(3000, 3, ago(time.Minute)),
		}, map[int]int{3000: 3}},
		{"two kills is not a fight", []Event{
			kill(3000, 1, ago(2*time.Minute)), kill(3000, 2, ago(time.Minute)),
		}, map[int]int{}},
		{"kills outside the window don't count", []Event{
			kill(3000, 1, ago(20*time.Minute)), kill(3000, 2, ago(5*time.Minute)), kill(3000, 3, ago(time.Minute)),
		}, map[int]int{}},
		{"exits not via portfind don't count", []Event{
			exit(3000, 1, ago(3*time.Minute)), kill(3000, 2, ago(2*time.Minute)), kill(3000, 3, ago(time.Minute)),
		}, map[int]int{}},
		{"ports are counted separately", []Event{
			kill(3000, 1, ago(3*time.Minute)), kill(8080, 2, ago(3*time.Minute)),
			kill(3000, 3, ago(2*time.Minute)), kill(8080, 4, ago(2*time.Minute)),
			kill(3000, 5, ago(time.Minute)),
		}, map[int]int{3000: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := openTemp(t)
			mustRecord(t, s, tt.events...)
			fights, err := s.PortFights(now.Add(-FightWindow), FightThreshold)
			if err != nil {
				t.Fatal(err)
			}
			got := map[int]int{}
			for _, f := range fights {
				got[f.Port] = f.Kills
			}
			if len(got) != len(tt.want) {
				t.Fatalf("fights = %v, want %v", got, tt.want)
			}
			for port, n := range tt.want {
				if got[port] != n {
					t.Fatalf("fights = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestFightDescribesLatestKill(t *testing.T) {
	s, _ := openTemp(t)
	now := time.Now()
	mustRecord(t, s,
		Event{At: now.Add(-3 * time.Minute), Port: 3000, PID: 1, Process: "node", Project: "old", KilledViaPortfind: true},
		Event{At: now.Add(-2 * time.Minute), Port: 3000, PID: 1, Process: "node", Project: "old", KilledViaPortfind: true},
		Event{At: now.Add(-1 * time.Minute), Port: 3000, PID: 2, Process: "python", Project: "new", KilledViaPortfind: true},
	)
	fights, err := s.PortFights(now.Add(-FightWindow), FightThreshold)
	if err != nil || len(fights) != 1 {
		t.Fatalf("fights=%v err=%v", fights, err)
	}
	f := fights[0]
	if f.Process != "python" || f.Project != "new" || f.Kills != 3 {
		t.Fatalf("fight = %+v, want 3 kills described by the latest (python/new)", f)
	}
}

func TestRecentNewestFirstAndPersists(t *testing.T) {
	s, path := openTemp(t)
	base := time.Now().Truncate(time.Millisecond)
	mustRecord(t, s,
		Event{At: base.Add(-2 * time.Second), Port: 1, PID: 1, Process: "a"},
		Event{At: base, Port: 3, PID: 3, Process: "c", Project: "proj", KilledViaPortfind: true},
		Event{At: base.Add(-time.Second), Port: 2, PID: 2, Process: "b"},
	)
	s.Close()

	s2, err := Open(path) // reopen: schema already migrated, data kept
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.Recent(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Port != 3 || got[1].Port != 2 {
		t.Fatalf("Recent(2) = %+v, want ports 3 then 2", got)
	}
	want := Event{At: base, Port: 3, PID: 3, Process: "c", Project: "proj", KilledViaPortfind: true}
	if !got[0].At.Equal(want.At) || got[0].Project != "proj" || !got[0].KilledViaPortfind {
		t.Fatalf("round trip = %+v, want %+v", got[0], want)
	}
}
