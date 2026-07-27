package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

var allowedVotes = map[string]bool{
	"0": true, "1": true, "2": true, "3": true, "5": true,
	"8": true, "13": true, "21": true, "34": true, "55": true,
	"89": true, "?": true, "coffee": true,
}

type store struct {
	db *sql.DB
}

type storeError struct {
	status  int
	message string
}

func (e *storeError) Error() string { return e.message }

func validationError(message string) error {
	return &storeError{status: http.StatusBadRequest, message: message}
}

func notFoundError(message string) error {
	return &storeError{status: http.StatusNotFound, message: message}
}

func forbiddenError(message string) error {
	return &storeError{status: http.StatusForbidden, message: message}
}

type roomState struct {
	Code                 string        `json:"code"`
	Name                 string        `json:"name"`
	Round                int           `json:"round"`
	Revealed             bool          `json:"revealed"`
	HostID               string        `json:"hostId"`
	Participants         []participant `json:"participants"`
	OnlineParticipantIDs []string      `json:"onlineParticipantIds"`
}

type participant struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	HasVoted bool    `json:"hasVoted"`
	Vote     *string `json:"vote"`
}

func openStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	s := &store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *store) close() error { return s.db.Close() }

func (s *store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS rooms (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			host_id TEXT NOT NULL,
			round INTEGER NOT NULL DEFAULT 1,
			revealed INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS participants (
			id TEXT PRIMARY KEY,
			room_code TEXT NOT NULL REFERENCES rooms(code) ON DELETE CASCADE,
			name TEXT NOT NULL,
			joined_at TEXT NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS participants_room_name
			ON participants(room_code, name COLLATE NOCASE);
		CREATE TABLE IF NOT EXISTS votes (
			room_code TEXT NOT NULL REFERENCES rooms(code) ON DELETE CASCADE,
			participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
			round INTEGER NOT NULL,
			value TEXT NOT NULL,
			PRIMARY KEY (room_code, participant_id, round)
		);
	`)
	return err
}

func (s *store) createRoom(ctx context.Context, roomName, userName string) (roomState, string, error) {
	roomName, err := cleanName(roomName, "Room name", 60)
	if err != nil {
		return roomState{}, "", err
	}
	userName, err = cleanName(userName, "Your name", 30)
	if err != nil {
		return roomState{}, "", err
	}

	for range 5 {
		code, id, err := randomToken(6), randomToken(24), error(nil)
		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return roomState{}, "", txErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO rooms(code, name, host_id, created_at) VALUES (?, ?, ?, ?)`, code, roomName, id, time.Now().UTC().Format(time.RFC3339))
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO participants(id, room_code, name, joined_at) VALUES (?, ?, ?, ?)`, id, code, userName, time.Now().UTC().Format(time.RFC3339))
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			continue
		}
		state, err := s.roomState(ctx, code, id)
		return state, id, err
	}
	return roomState{}, "", errors.New("could not allocate room code")
}

func (s *store) joinRoom(ctx context.Context, code, userName string) (roomState, string, error) {
	code = normalizeCode(code)
	userName, err := cleanName(userName, "Your name", 30)
	if err != nil {
		return roomState{}, "", err
	}
	if !s.roomExists(ctx, code) {
		return roomState{}, "", notFoundError("room not found")
	}

	id := randomToken(24)
	_, err = s.db.ExecContext(ctx, `INSERT INTO participants(id, room_code, name, joined_at) VALUES (?, ?, ?, ?)`, id, code, userName, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return roomState{}, "", validationError("that name is already in the room")
		}
		return roomState{}, "", err
	}
	state, err := s.roomState(ctx, code, id)
	return state, id, err
}

func (s *store) roomState(ctx context.Context, code, participantID string) (roomState, error) {
	code = normalizeCode(code)
	var state roomState
	if err := s.db.QueryRowContext(ctx, `SELECT code, name, round, revealed, host_id FROM rooms WHERE code = ?`, code).
		Scan(&state.Code, &state.Name, &state.Round, &state.Revealed, &state.HostID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return roomState{}, notFoundError("room not found")
		}
		return roomState{}, err
	}

	if participantID != "" {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM participants WHERE id = ? AND room_code = ?`, participantID, code).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return roomState{}, forbiddenError("participant session is not valid for this room")
			}
			return roomState{}, err
		}
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, v.value
		FROM participants p
		LEFT JOIN votes v ON v.participant_id = p.id AND v.room_code = p.room_code AND v.round = ?
		WHERE p.room_code = ?
		ORDER BY p.joined_at, p.name`, state.Round, code)
	if err != nil {
		return roomState{}, err
	}
	defer rows.Close()

	state.Participants = []participant{}
	state.OnlineParticipantIDs = []string{}
	for rows.Next() {
		var person participant
		var vote sql.NullString
		if err := rows.Scan(&person.ID, &person.Name, &vote); err != nil {
			return roomState{}, err
		}
		person.HasVoted = vote.Valid
		if state.Revealed && vote.Valid {
			person.Vote = &vote.String
		}
		state.Participants = append(state.Participants, person)
	}
	return state, rows.Err()
}

func (s *store) castVote(ctx context.Context, code, participantID, value string) error {
	if !allowedVotes[value] {
		return validationError("invalid vote")
	}
	var round int
	var revealed bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT r.round, r.revealed
		FROM rooms r JOIN participants p ON p.room_code = r.code
		WHERE r.code = ? AND p.id = ?`, normalizeCode(code), participantID).Scan(&round, &revealed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return forbiddenError("participant session is not valid for this room")
		}
		return err
	}
	if revealed {
		return validationError("votes are already revealed")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO votes(room_code, participant_id, round, value) VALUES (?, ?, ?, ?)
		ON CONFLICT(room_code, participant_id, round) DO UPDATE SET value = excluded.value`, normalizeCode(code), participantID, round, value)
	return err
}

func (s *store) reveal(ctx context.Context, code, participantID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rooms SET revealed = 1 WHERE code = ? AND host_id = ?`, normalizeCode(code), participantID)
	if err != nil {
		return err
	}
	return requireChanged(result, "only the room host can reveal votes")
}

func (s *store) reset(ctx context.Context, code, participantID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE rooms SET round = round + 1, revealed = 0 WHERE code = ? AND host_id = ?`, normalizeCode(code), participantID)
	if err != nil {
		return err
	}
	return requireChanged(result, "only the room host can start a new round")
}

func (s *store) roomExists(ctx context.Context, code string) bool {
	var exists int
	return s.db.QueryRowContext(ctx, `SELECT 1 FROM rooms WHERE code = ?`, normalizeCode(code)).Scan(&exists) == nil
}

func requireChanged(result sql.Result, message string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return forbiddenError(message)
	}
	return nil
}

func cleanName(value, label string, max int) (string, error) {
	value = strings.Join(strings.Fields(value), " ")
	length := len([]rune(value))
	if length < 2 || length > max {
		return "", validationError(fmt.Sprintf("%s must be between 2 and %d characters", label, max))
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", validationError(label + " contains invalid characters")
		}
	}
	return value, nil
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func randomToken(length int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	for i := range bytes {
		bytes[i] = alphabet[int(bytes[i])%len(alphabet)]
	}
	return string(bytes)
}

func publicError(err error) string {
	var requestErr *storeError
	if errors.As(err, &requestErr) {
		return requestErr.message
	}
	return "something went wrong"
}
