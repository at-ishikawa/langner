package database

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// Per-request database usage accounting, to find which endpoint pulls the most
// data from Postgres. For each HTTP request that runs a
// query, QueryStatsMiddleware logs ONE "db usage" line: query count, rows,
// bytes received from the server over the wire, DB time, and the heaviest query
// shapes. Only SQL text (with $n placeholders) is logged — never query
// arguments or result data.

// topQueryShapes is how many of a request's heaviest query shapes are logged.
const topQueryShapes = 5

// maxSQLShapeLen truncates logged SQL so a line stays readable.
const maxSQLShapeLen = 160

// QueryStats accumulates the database usage of one request.
type QueryStats struct {
	mu       sync.Mutex
	queries  int
	rows     int64
	bytesIn  int64
	duration time.Duration
	// inferred counts queries made WITHOUT the request context (e.g. with
	// context.Background()) that were attributed to this request because it was
	// the only one in flight.
	inferred int
	shapes   map[string]*queryShape
}

type queryShape struct {
	sql      string
	calls    int
	rows     int64
	bytesIn  int64
	duration time.Duration
}

func (q queryShape) String() string {
	return fmt.Sprintf("bytes_in=%d calls=%d rows=%d db_ms=%d sql=%q", q.bytesIn, q.calls, q.rows, q.duration.Milliseconds(), q.sql)
}

func (s *QueryStats) add(sql string, rows, bytesIn int64, d time.Duration, inferred bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries++
	s.rows += rows
	s.bytesIn += bytesIn
	s.duration += d
	if inferred {
		s.inferred++
	}
	if s.shapes == nil {
		s.shapes = make(map[string]*queryShape)
	}
	key := sqlShape(sql)
	shape, ok := s.shapes[key]
	if !ok {
		shape = &queryShape{sql: key}
		s.shapes[key] = shape
	}
	shape.calls++
	shape.rows += rows
	shape.bytesIn += bytesIn
	shape.duration += d
}

// topShapes returns the heaviest query shapes by bytes received, then calls,
// formatted for logging.
func (s *QueryStats) topShapes() []string {
	shapes := make([]queryShape, 0, len(s.shapes))
	for _, shape := range s.shapes {
		shapes = append(shapes, *shape)
	}
	sort.Slice(shapes, func(i, j int) bool {
		if shapes[i].bytesIn != shapes[j].bytesIn {
			return shapes[i].bytesIn > shapes[j].bytesIn
		}
		return shapes[i].calls > shapes[j].calls
	})
	if len(shapes) > topQueryShapes {
		shapes = shapes[:topQueryShapes]
	}
	top := make([]string, len(shapes))
	for i, shape := range shapes {
		top[i] = shape.String()
	}
	return top
}

type queryStatsKey struct{}

// logUnattributed enables a log line for each query that can't be attributed
// to a request. Off by default so CLI tools (imports, seeds) stay quiet; the
// server turns it on at startup to also surface boot-time queries.
var logUnattributed atomic.Bool

// LogUnattributedQueries turns on logging of queries made outside any request.
func LogUnattributedQueries() {
	logUnattributed.Store(true)
}

// inFlight tracks the requests currently being served, so a query made without
// the request context can still be attributed when exactly one is running.
var inFlight = struct {
	sync.Mutex
	stats map[*QueryStats]struct{}
}{stats: map[*QueryStats]struct{}{}}

// statsFor returns the stats a query should be recorded on, and whether that
// was inferred (no stats in ctx, but a single request in flight).
func statsFor(ctx context.Context) (*QueryStats, bool) {
	if stats, ok := ctx.Value(queryStatsKey{}).(*QueryStats); ok {
		return stats, false
	}
	inFlight.Lock()
	defer inFlight.Unlock()
	if len(inFlight.stats) != 1 {
		return nil, false
	}
	for stats := range inFlight.stats {
		return stats, true
	}
	return nil, false
}

// QueryStatsMiddleware records the database usage of each request and logs one
// "db usage" line for every request that ran at least one query.
func QueryStatsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stats := &QueryStats{}
		inFlight.Lock()
		inFlight.stats[stats] = struct{}{}
		inFlight.Unlock()

		start := time.Now()
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), queryStatsKey{}, stats)))
		elapsed := time.Since(start)

		inFlight.Lock()
		delete(inFlight.stats, stats)
		inFlight.Unlock()

		stats.mu.Lock()
		defer stats.mu.Unlock()
		if stats.queries == 0 {
			return
		}
		slog.Info("db usage",
			"endpoint", r.URL.Path,
			"queries", stats.queries,
			"rows", stats.rows,
			"bytes_in", stats.bytesIn,
			"db_ms", stats.duration.Milliseconds(),
			"request_ms", elapsed.Milliseconds(),
			"inferred_queries", stats.inferred,
			"top", stats.topShapes(),
		)
	})
}

// queryTracer is the pgx tracer that feeds QueryStats. A query whose request
// can't be determined is logged on its own line instead (see
// LogUnattributedQueries).
type queryTracer struct{}

type queryStartKey struct{}

type queryStart struct {
	sql     string
	started time.Time
	bytesIn int64
}

func (queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryStartKey{}, queryStart{sql: data.SQL, started: time.Now(), bytesIn: bytesRead(conn)})
}

func (queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryStartKey{}).(queryStart)
	if !ok {
		return
	}
	rows := data.CommandTag.RowsAffected()
	bytesIn := bytesRead(conn) - start.bytesIn
	elapsed := time.Since(start.started)

	if stats, inferred := statsFor(ctx); stats != nil {
		stats.add(start.sql, rows, bytesIn, elapsed, inferred)
		return
	}
	if !logUnattributed.Load() {
		return
	}
	slog.Info("db usage (unattributed query)",
		"sql", sqlShape(start.sql),
		"rows", rows,
		"bytes_in", bytesIn,
		"db_ms", elapsed.Milliseconds(),
	)
}

// countingConn counts the bytes read from the database server, i.e. the
// network traffic from Postgres (including TLS/protocol overhead).
type countingConn struct {
	net.Conn
	bytesIn atomic.Int64
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.bytesIn.Add(int64(n))
	return n, err
}

// bytesRead returns the bytes read so far on conn's underlying network
// connection, unwrapping TLS. 0 when it isn't a countingConn.
func bytesRead(conn *pgx.Conn) int64 {
	c := conn.PgConn().Conn()
	for c != nil {
		if counting, ok := c.(*countingConn); ok {
			return counting.bytesIn.Load()
		}
		wrapped, ok := c.(interface{ NetConn() net.Conn })
		if !ok {
			return 0
		}
		c = wrapped.NetConn()
	}
	return 0
}

// sqlShape collapses whitespace and truncates SQL for logging. SQL text holds
// only $n placeholders, never argument values.
func sqlShape(sql string) string {
	shape := strings.Join(strings.Fields(sql), " ")
	if len(shape) > maxSQLShapeLen {
		shape = shape[:maxSQLShapeLen] + "…"
	}
	return shape
}
