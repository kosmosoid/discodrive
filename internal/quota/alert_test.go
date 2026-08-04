package quota

import "testing"

func TestFreePercent(t *testing.T) {
	cases := []struct {
		name        string
		free, total int64
		want        int
	}{
		{"half", 500, 1000, 50},
		{"rounds down", 199, 1000, 19},
		{"full", 0, 1000, 0},
		// A usage that overshot the limit must not read as "plenty of room left".
		{"overshot", -5, 1000, 0},
		// No limit configured — not an alarm, see LevelFor.
		{"no limit", 0, 0, -1},
	}
	for _, c := range cases {
		if got := FreePercent(c.free, c.total); got != c.want {
			t.Errorf("%s: FreePercent(%d, %d) = %d, want %d", c.name, c.free, c.total, got, c.want)
		}
	}
}

func TestLevelFor(t *testing.T) {
	cases := []struct {
		percent int
		want    Level
	}{
		{100, LevelOK},
		{21, LevelOK},
		{20, LevelWarn}, // the thresholds themselves already count as reached
		{11, LevelWarn},
		{10, LevelCritical},
		{0, LevelCritical},
		{-1, LevelOK}, // unknown limit
	}
	for _, c := range cases {
		if got := LevelFor(c.percent); got != c.want {
			t.Errorf("LevelFor(%d) = %v, want %v", c.percent, got, c.want)
		}
	}
}

// Freeing a few bytes right at a threshold must not clear the alert — otherwise a
// server hovering on 20% flips level on every tick and mails the admins each time it
// crosses back up.
func TestSettledLevel_Hysteresis(t *testing.T) {
	cases := []struct {
		name    string
		percent int
		was     Level
		want    Level
	}{
		{"escalates immediately", 9, LevelWarn, LevelCritical},
		{"holds just above the threshold", 21, LevelWarn, LevelWarn},
		{"clears once well clear", 22, LevelWarn, LevelOK},
		{"critical holds at 11", 11, LevelCritical, LevelCritical},
		{"critical steps down to warn at 12", 12, LevelCritical, LevelWarn},
		{"quiet stays quiet", 80, LevelOK, LevelOK},
	}
	for _, c := range cases {
		if got := SettledLevel(c.percent, c.was); got != c.want {
			t.Errorf("%s: SettledLevel(%d, %v) = %v, want %v", c.name, c.percent, c.was, got, c.want)
		}
	}
}
