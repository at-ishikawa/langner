package analytics

import "testing"

func mkAttempts(results ...bool) []Attempt {
	out := make([]Attempt, len(results))
	for i, wrong := range results {
		out[i] = Attempt{IsWrong: wrong}
	}
	return out
}

func TestRecentPattern_FewerThanFiveAttempts(t *testing.T) {
	got := RecentPattern(mkAttempts(true, false))
	want := []string{PatternNone, PatternNone, PatternNone, PatternCorrect, PatternWrong}
	for i, g := range got {
		if g != want[i] {
			t.Fatalf("index %d: got %s, want %s (full=%v)", i, g, want[i], got)
		}
	}
}

func TestRecentPattern_MoreThanFiveAttempts(t *testing.T) {
	// Newest -> oldest: w, w, c, w, c, w, c (only the first five count)
	got := RecentPattern(mkAttempts(true, true, false, true, false, true, false))
	want := []string{PatternCorrect, PatternWrong, PatternCorrect, PatternWrong, PatternWrong}
	for i, g := range got {
		if g != want[i] {
			t.Fatalf("index %d: got %s, want %s (full=%v)", i, g, want[i], got)
		}
	}
}

func TestRecentPattern_Empty(t *testing.T) {
	got := RecentPattern(nil)
	for _, g := range got {
		if g != PatternNone {
			t.Fatalf("expected all none, got %v", got)
		}
	}
}

func TestCurrentWrongStreak(t *testing.T) {
	cases := []struct {
		name     string
		attempts []Attempt
		want     int
	}{
		{"empty", nil, 0},
		{"newest correct", mkAttempts(false, true, true), 0},
		{"one wrong", mkAttempts(true, false), 1},
		{"three wrong in a row", mkAttempts(true, true, true, false, false), 3},
		{"all wrong", mkAttempts(true, true, true), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CurrentWrongStreak(tc.attempts); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPreviousCorrectStreak(t *testing.T) {
	cases := []struct {
		name     string
		attempts []Attempt
		want     int
	}{
		{"empty", nil, 0},
		{"all wrong", mkAttempts(true, true, true), 0},
		{"one wrong after two correct", mkAttempts(true, false, false, true), 2},
		{"three wrong after one correct", mkAttempts(true, true, true, false, true), 1},
		{"newest correct returns 0", mkAttempts(false, true, true), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PreviousCorrectStreak(tc.attempts); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestStreakBeforeAttempt(t *testing.T) {
	attempts := mkAttempts(true, true, false, false, true)
	// index 0 (newest, wrong): previous was wrong, streak=1
	w, c := StreakBeforeAttempt(attempts, 0)
	if w != 1 || c != 0 {
		t.Fatalf("index 0: got w=%d c=%d, want w=1 c=0", w, c)
	}
	// index 1 (wrong): previous was correct, streak=2 (two correct in a row)
	w, c = StreakBeforeAttempt(attempts, 1)
	if w != 0 || c != 2 {
		t.Fatalf("index 1: got w=%d c=%d, want w=0 c=2", w, c)
	}
	// index 2 (correct): previous was correct, streak=1
	w, c = StreakBeforeAttempt(attempts, 2)
	if w != 0 || c != 1 {
		t.Fatalf("index 2: got w=%d c=%d, want w=0 c=1", w, c)
	}
	// index 3 (correct): previous was wrong, streak=1
	w, c = StreakBeforeAttempt(attempts, 3)
	if w != 1 || c != 0 {
		t.Fatalf("index 3: got w=%d c=%d, want w=1 c=0", w, c)
	}
	// index 4 (oldest): no previous attempt
	w, c = StreakBeforeAttempt(attempts, 4)
	if w != 0 || c != 0 {
		t.Fatalf("index 4: got w=%d c=%d, want 0/0", w, c)
	}
}
