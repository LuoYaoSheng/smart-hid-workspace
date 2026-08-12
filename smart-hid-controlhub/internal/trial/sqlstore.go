package trial

import (
	"database/sql"
	"fmt"
)

// sqlTrialStore 是 trialStore 的 SQLite 实现。
type sqlTrialStore struct {
	db *sql.DB
}

// NewSQLStore 包装 *sql.DB 为 trialStore。
func NewSQLStore(db *sql.DB) trialStore {
	return &sqlTrialStore{db: db}
}

func (s *sqlTrialStore) GetUsage(deviceID, anchor string) (float64, int, error) {
	var used float64
	var count int
	err := s.db.QueryRow(
		`SELECT used_seconds, session_count FROM trial_usage WHERE device_id = ? AND machine_anchor = ?`,
		deviceID, anchor,
	).Scan(&used, &count)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("get trial_usage: %w", err)
	}
	return used, count, nil
}

// UpsertUsage 把 addSec 累加到现有 used_seconds（不存在则新建，初始 0+addSec）。
// sessionCountDelta 同时累加到 session_count。
func (s *sqlTrialStore) UpsertUsage(deviceID, anchor string, addSec float64, sessionCountDelta int) error {
	_, err := s.db.Exec(
		`INSERT INTO trial_usage(device_id, machine_anchor, used_seconds, session_count, last_session_at)
		 VALUES(?, ?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(device_id, machine_anchor) DO UPDATE SET
		   used_seconds = used_seconds + excluded.used_seconds,
		   session_count = session_count + excluded.session_count,
		   last_session_at = excluded.last_session_at`,
		deviceID, anchor, addSec, sessionCountDelta,
	)
	if err != nil {
		return fmt.Errorf("upsert trial_usage: %w", err)
	}
	return nil
}

func (s *sqlTrialStore) InsertSession(sessionID, deviceID, anchor string, startedAt int64) error {
	_, err := s.db.Exec(
		`INSERT INTO trial_sessions(session_id, device_id, machine_anchor, started_at, accumulated_seconds)
		 VALUES(?, ?, ?, ?, 0)`,
		sessionID, deviceID, anchor, startedAt,
	)
	if err != nil {
		return fmt.Errorf("insert trial_session: %w", err)
	}
	return nil
}

func (s *sqlTrialStore) UpdateSession(sessionID string, endedAt int64, accumulated float64) error {
	_, err := s.db.Exec(
		`UPDATE trial_sessions SET ended_at = ?, accumulated_seconds = ? WHERE session_id = ?`,
		endedAt, accumulated, sessionID,
	)
	return err
}
