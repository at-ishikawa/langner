package analytics

// RecentPattern returns the last RecentPatternLength attempts as glyph
// strings, oldest first. attempts must be ordered newest-first (matching
// how LearningHistoryExpression stores its logs).
//
// Example with 3 attempts (newest=wrong, then correct, then wrong) and
// RecentPatternLength=5 the result is:
//
//	{none, none, wrong, correct, wrong}
//
// so the caller can render the row left-to-right with the newest attempt
// on the right.
func RecentPattern(attempts []Attempt) []string {
	out := make([]string, RecentPatternLength)
	for i := range out {
		out[i] = PatternNone
	}
	// Take the most recent N (newest-first input).
	n := len(attempts)
	if n > RecentPatternLength {
		n = RecentPatternLength
	}
	for i := 0; i < n; i++ {
		glyph := PatternCorrect
		if attempts[i].IsWrong {
			glyph = PatternWrong
		}
		// Place newest at the end of the slice.
		out[RecentPatternLength-1-i] = glyph
	}
	return out
}

// CurrentWrongStreak counts consecutive wrong attempts ending at the
// newest attempt. Returns 0 when the newest attempt is correct or when
// attempts is empty. attempts must be newest-first.
func CurrentWrongStreak(attempts []Attempt) int {
	n := 0
	for _, a := range attempts {
		if !a.IsWrong {
			break
		}
		n++
	}
	return n
}

// PreviousCorrectStreak counts consecutive correct attempts that come
// immediately before the current wrong streak. Returns 0 when there is
// no current wrong streak (the newest attempt was correct or the slice
// is empty) — the metric is only meaningful when describing a wrong
// attempt, so a correct newest attempt has no "before". attempts must
// be newest-first.
func PreviousCorrectStreak(attempts []Attempt) int {
	skip := CurrentWrongStreak(attempts)
	if skip == 0 {
		return 0
	}
	n := 0
	for _, a := range attempts[skip:] {
		if a.IsWrong {
			break
		}
		n++
	}
	return n
}

// StreakBeforeAttempt returns the consecutive-run length immediately
// preceding the attempt at index i in a newest-first slice. The run is
// scanned starting at i+1 (the previous attempt) and continues while
// each attempt matches the result kind of attempts[i+1].
//
// streakWrong is non-zero when the previous attempt was wrong;
// streakCorrect is non-zero when the previous attempt was correct.
// Both zero means attempts[i] was the very first attempt.
func StreakBeforeAttempt(attempts []Attempt, i int) (streakWrong, streakCorrect int) {
	if i+1 >= len(attempts) {
		return 0, 0
	}
	prevWrong := attempts[i+1].IsWrong
	n := 0
	for _, a := range attempts[i+1:] {
		if a.IsWrong != prevWrong {
			break
		}
		n++
	}
	if prevWrong {
		return n, 0
	}
	return 0, n
}
