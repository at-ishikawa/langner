package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// DBRepository serves analytics queries from the langner MySQL schema.
// It joins learning_logs with notes (for the expression) and
// notebook_notes (for the notebook title and scene) so the response
// matches what the YAML repository produces.
type DBRepository struct {
	db *sqlx.DB
}

// NewDBRepository returns an analytics Repository backed by the configured DB.
func NewDBRepository(db *sqlx.DB) *DBRepository {
	return &DBRepository{db: db}
}

// dailyRow is the projection used for one row of the daily summary query.
type dailyRow struct {
	Date          time.Time `db:"date"`
	Total         int       `db:"total"`
	Wrong         int       `db:"wrong"`
	Notebooks     int       `db:"notebooks"`
	QuizTypesCSV  string    `db:"quiz_types"`
}

// DailySummaries returns per-day rollups for the given range and filters.
// rangeDays == 0 means "all time".
func (r *DBRepository) DailySummaries(ctx context.Context, rangeDays int, filters Filters) ([]DailySummary, error) {
	var args []interface{}
	conds := []string{}
	if rangeDays > 0 {
		conds = append(conds, "learned_at >= ?")
		args = append(args, time.Now().UTC().AddDate(0, 0, -rangeDays))
	}
	if filters.NotebookID != "" {
		conds = append(conds, "source_notebook_id = ?")
		args = append(args, filters.NotebookID)
	}
	if filters.QuizType != "" {
		conds = append(conds, "quiz_type = ?")
		args = append(args, filters.QuizType)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	query := `
		SELECT
			DATE(learned_at) AS date,
			COUNT(*) AS total,
			SUM(CASE WHEN status = 'misunderstood' THEN 1 ELSE 0 END) AS wrong,
			COUNT(DISTINCT source_notebook_id) AS notebooks,
			GROUP_CONCAT(DISTINCT quiz_type ORDER BY quiz_type) AS quiz_types
		FROM learning_logs
		` + where + `
		GROUP BY DATE(learned_at)
		ORDER BY DATE(learned_at) DESC
	`
	var rows []dailyRow
	if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("daily summaries: %w", err)
	}
	out := make([]DailySummary, len(rows))
	for i, row := range rows {
		out[i] = DailySummary{
			Date:          row.Date,
			WrongCount:    row.Wrong,
			TotalCount:    row.Total,
			NotebookCount: row.Notebooks,
			QuizTypes:     splitCSV(row.QuizTypesCSV),
		}
	}
	return out, nil
}

// wrongRow is the projection used for one wrong attempt on the requested day.
type wrongRow struct {
	NoteID           int64     `db:"note_id"`
	Expression       string    `db:"expression"`
	NotebookID       string    `db:"notebook_id"`
	NotebookTitle    string    `db:"notebook_title"`
	SceneTitle       string    `db:"scene_title"`
	QuizType         string    `db:"quiz_type"`
	LearnedAt        time.Time `db:"learned_at"`
	Status           string    `db:"status"`
}

// DayDetail returns the wrong words for the day plus adjacent-day pointers.
func (r *DBRepository) DayDetail(ctx context.Context, day time.Time, filters Filters) (DayDetail, error) {
	dayStr := dayKey(day)
	args := []interface{}{dayStr}
	conds := []string{"DATE(ll.learned_at) = ?"}
	if filters.NotebookID != "" {
		conds = append(conds, "ll.source_notebook_id = ?")
		args = append(args, filters.NotebookID)
	}
	if filters.QuizType != "" {
		conds = append(conds, "ll.quiz_type = ?")
		args = append(args, filters.QuizType)
	}
	whereDay := "WHERE " + strings.Join(conds, " AND ")

	// One-row summary for the day.
	summary, err := r.daySummary(ctx, dayStr, filters)
	if err != nil {
		return DayDetail{}, err
	}

	// Wrong attempts on the day.
	wrongQuery := `
		SELECT
			ll.note_id,
			n.usage AS expression,
			COALESCE(nn.notebook_id, ll.source_notebook_id, '') AS notebook_id,
			COALESCE(nn.notebook_id, ll.source_notebook_id, '') AS notebook_title,
			COALESCE(nn.subgroup, '') AS scene_title,
			ll.quiz_type,
			ll.learned_at,
			ll.status
		FROM learning_logs ll
		JOIN notes n ON n.id = ll.note_id
		LEFT JOIN notebook_notes nn
			ON nn.note_id = ll.note_id
			AND (ll.source_notebook_id = '' OR nn.notebook_id = ll.source_notebook_id)
		` + whereDay + `
			AND ll.status = 'misunderstood'
		GROUP BY ll.note_id, ll.quiz_type, ll.learned_at, ll.status, n.usage, nn.notebook_id, nn.subgroup
		ORDER BY n.usage
	`
	var wrongs []wrongRow
	if err := r.db.SelectContext(ctx, &wrongs, wrongQuery, args...); err != nil {
		return DayDetail{}, fmt.Errorf("wrong words: %w", err)
	}

	// For each wrong attempt, fetch the recent attempts (newest first) so streak
	// helpers can compute the pattern and the streaks around the day's attempt.
	words := make([]WrongWord, 0, len(wrongs))
	for _, w := range wrongs {
		attempts, err := r.recentAttempts(ctx, w.NoteID, w.QuizType, w.LearnedAt)
		if err != nil {
			return DayDetail{}, err
		}
		words = append(words, WrongWord{
			NoteID:                w.NoteID,
			Expression:            w.Expression,
			NotebookID:            w.NotebookID,
			NotebookTitle:         w.NotebookTitle,
			SceneTitle:            w.SceneTitle,
			QuizType:              w.QuizType,
			RecentPattern:         RecentPattern(attempts),
			CurrentWrongStreak:    CurrentWrongStreak(attempts),
			PreviousCorrectStreak: PreviousCorrectStreak(attempts),
			CurrentStatus:         w.Status,
		})
	}

	// Adjacent days with activity matching filters.
	prev, next, err := r.adjacentDayDates(ctx, day, filters)
	if err != nil {
		return DayDetail{}, err
	}

	return DayDetail{
		Summary:      summary,
		WrongWords:   words,
		PreviousDate: prev,
		NextDate:     next,
	}, nil
}

// daySummary builds a single-day rollup (one row) used by DayDetail.
func (r *DBRepository) daySummary(ctx context.Context, dayStr string, filters Filters) (DailySummary, error) {
	args := []interface{}{dayStr}
	conds := []string{"DATE(learned_at) = ?"}
	if filters.NotebookID != "" {
		conds = append(conds, "source_notebook_id = ?")
		args = append(args, filters.NotebookID)
	}
	if filters.QuizType != "" {
		conds = append(conds, "quiz_type = ?")
		args = append(args, filters.QuizType)
	}
	where := "WHERE " + strings.Join(conds, " AND ")
	query := `
		SELECT
			DATE(learned_at) AS date,
			COUNT(*) AS total,
			SUM(CASE WHEN status = 'misunderstood' THEN 1 ELSE 0 END) AS wrong,
			COUNT(DISTINCT source_notebook_id) AS notebooks,
			GROUP_CONCAT(DISTINCT quiz_type ORDER BY quiz_type) AS quiz_types
		FROM learning_logs
		` + where + `
		GROUP BY DATE(learned_at)
	`
	var row dailyRow
	err := r.db.GetContext(ctx, &row, query, args...)
	if err == sql.ErrNoRows {
		d, _ := time.Parse("2006-01-02", dayStr)
		return DailySummary{Date: d}, nil
	}
	if err != nil {
		return DailySummary{}, fmt.Errorf("day summary: %w", err)
	}
	return DailySummary{
		Date:          row.Date,
		WrongCount:    row.Wrong,
		TotalCount:    row.Total,
		NotebookCount: row.Notebooks,
		QuizTypes:     splitCSV(row.QuizTypesCSV),
	}, nil
}

// recentAttempts returns up to RecentPatternLength most recent attempts for the
// given (note, quiz_type) on or before upTo (inclusive of upTo).
func (r *DBRepository) recentAttempts(ctx context.Context, noteID int64, quizType string, upTo time.Time) ([]Attempt, error) {
	query := `
		SELECT status, learned_at, quality, quiz_type
		FROM learning_logs
		WHERE note_id = ? AND quiz_type = ? AND learned_at <= ?
		ORDER BY learned_at DESC
		LIMIT ?
	`
	var rows []struct {
		Status    string    `db:"status"`
		LearnedAt time.Time `db:"learned_at"`
		Quality   int       `db:"quality"`
		QuizType  string    `db:"quiz_type"`
	}
	if err := r.db.SelectContext(ctx, &rows, query, noteID, quizType, upTo, RecentPatternLength); err != nil {
		return nil, fmt.Errorf("recent attempts: %w", err)
	}
	out := make([]Attempt, len(rows))
	for i, row := range rows {
		out[i] = Attempt{
			LearnedAt: row.LearnedAt,
			QuizType:  row.QuizType,
			IsWrong:   row.Status == "misunderstood",
			Quality:   row.Quality,
			Status:    row.Status,
		}
	}
	return out, nil
}

// adjacentDayDates returns prev/next dates with activity matching filters.
func (r *DBRepository) adjacentDayDates(ctx context.Context, day time.Time, filters Filters) (time.Time, time.Time, error) {
	dayStr := dayKey(day)
	condsBase := []string{}
	var argsBase []interface{}
	if filters.NotebookID != "" {
		condsBase = append(condsBase, "source_notebook_id = ?")
		argsBase = append(argsBase, filters.NotebookID)
	}
	if filters.QuizType != "" {
		condsBase = append(condsBase, "quiz_type = ?")
		argsBase = append(argsBase, filters.QuizType)
	}
	tail := ""
	if len(condsBase) > 0 {
		tail = " AND " + strings.Join(condsBase, " AND ")
	}

	var prev sql.NullTime
	prevArgs := append([]interface{}{dayStr}, argsBase...)
	if err := r.db.GetContext(ctx, &prev,
		"SELECT MAX(learned_at) FROM learning_logs WHERE DATE(learned_at) < ?"+tail,
		prevArgs...); err != nil && err != sql.ErrNoRows {
		return time.Time{}, time.Time{}, fmt.Errorf("prev day: %w", err)
	}
	var next sql.NullTime
	nextArgs := append([]interface{}{dayStr}, argsBase...)
	if err := r.db.GetContext(ctx, &next,
		"SELECT MIN(learned_at) FROM learning_logs WHERE DATE(learned_at) > ?"+tail,
		nextArgs...); err != nil && err != sql.ErrNoRows {
		return time.Time{}, time.Time{}, fmt.Errorf("next day: %w", err)
	}
	var prevT, nextT time.Time
	if prev.Valid {
		prevT = truncToDay(prev.Time)
	}
	if next.Valid {
		nextT = truncToDay(next.Time)
	}
	return prevT, nextT, nil
}

// WordHistory returns every attempt for one (note_id, quiz_type) pair.
func (r *DBRepository) WordHistory(ctx context.Context, ref WordRef) (WordHistory, error) {
	if ref.NoteID == 0 {
		// Fall back to lookup by notebook_id + expression.
		if err := r.db.GetContext(ctx, &ref.NoteID, `
			SELECT n.id FROM notes n
			JOIN notebook_notes nn ON nn.note_id = n.id
			WHERE n.usage = ? AND nn.notebook_id = ?
			LIMIT 1`, ref.Expression, ref.NotebookID); err != nil {
			if err == sql.ErrNoRows {
				return WordHistory{Expression: ref.Expression, NotebookID: ref.NotebookID}, nil
			}
			return WordHistory{}, fmt.Errorf("resolve note: %w", err)
		}
	}

	query := `
		SELECT status, learned_at, quality, quiz_type
		FROM learning_logs
		WHERE note_id = ? AND quiz_type = ?
		ORDER BY learned_at DESC
	`
	var rows []struct {
		Status    string    `db:"status"`
		LearnedAt time.Time `db:"learned_at"`
		Quality   int       `db:"quality"`
		QuizType  string    `db:"quiz_type"`
	}
	if err := r.db.SelectContext(ctx, &rows, query, ref.NoteID, ref.QuizType); err != nil {
		return WordHistory{}, fmt.Errorf("word history: %w", err)
	}
	flat := make([]Attempt, len(rows))
	for i, row := range rows {
		flat[i] = Attempt{
			LearnedAt: row.LearnedAt,
			QuizType:  row.QuizType,
			IsWrong:   row.Status == "misunderstood",
			Quality:   row.Quality,
			Status:    row.Status,
		}
	}
	entries := make([]AttemptEntry, len(rows))
	for i, row := range rows {
		streakWrong, streakCorrect := StreakBeforeAttempt(flat, i)
		result := PatternCorrect
		if flat[i].IsWrong {
			result = PatternWrong
		}
		entries[i] = AttemptEntry{
			Date:                row.LearnedAt,
			QuizType:            row.QuizType,
			Result:              result,
			Quality:             row.Quality,
			StreakBeforeWrong:   streakWrong,
			StreakBeforeCorrect: streakCorrect,
		}
	}

	currentStatus := ""
	if len(rows) > 0 {
		currentStatus = rows[0].Status
	}
	// Try to resolve a notebook title (best-effort).
	var notebookTitle string
	_ = r.db.GetContext(ctx, &notebookTitle, `
		SELECT notebook_id FROM notebook_notes
		WHERE note_id = ?
		ORDER BY id LIMIT 1`, ref.NoteID)
	if ref.NotebookID == "" {
		ref.NotebookID = notebookTitle
	}
	return WordHistory{
		Expression:         ref.Expression,
		NotebookID:         ref.NotebookID,
		NotebookTitle:      notebookTitle,
		CurrentStatus:      currentStatus,
		CurrentWrongStreak: CurrentWrongStreak(flat),
		Attempts:           entries,
	}, nil
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
