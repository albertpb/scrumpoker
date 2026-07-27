package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"time"

	"scrumpoker/backend/domain"

	_ "modernc.org/sqlite"
)

type Repository struct {
	db *sql.DB
}

func Open(path string) (*Repository, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	repository := &Repository{db: db}
	if err := repository.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repository, nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) migrate() error {
	_, err := r.db.Exec(`
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

func (r *Repository) CreateRoom(ctx context.Context, roomName, participantName string) (domain.Room, string, error) {
	for range 5 {
		code, id := randomToken(6), randomToken(24)
		tx, err := r.db.BeginTx(ctx, nil)
		if err != nil {
			return domain.Room{}, "", err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO rooms(code, name, host_id, created_at) VALUES (?, ?, ?, ?)`, code, roomName, id, time.Now().UTC().Format(time.RFC3339))
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO participants(id, room_code, name, joined_at) VALUES (?, ?, ?, ?)`, id, code, participantName, time.Now().UTC().Format(time.RFC3339))
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if err != nil {
			continue
		}
		room, err := r.RoomState(ctx, code, id)
		return room, id, err
	}
	return domain.Room{}, "", errors.New("could not allocate room code")
}

func (r *Repository) JoinRoom(ctx context.Context, code, participantName string) (domain.Room, string, error) {
	if !r.roomExists(ctx, code) {
		return domain.Room{}, "", domain.NotFoundError("room not found")
	}

	id := randomToken(24)
	_, err := r.db.ExecContext(ctx, `INSERT INTO participants(id, room_code, name, joined_at) VALUES (?, ?, ?, ?)`, id, code, participantName, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return domain.Room{}, "", domain.ValidationError("that name is already in the room")
		}
		return domain.Room{}, "", err
	}
	room, err := r.RoomState(ctx, code, id)
	return room, id, err
}

func (r *Repository) RoomState(ctx context.Context, code, participantID string) (domain.Room, error) {
	var room domain.Room
	if err := r.db.QueryRowContext(ctx, `SELECT code, name, round, revealed, host_id FROM rooms WHERE code = ?`, code).
		Scan(&room.Code, &room.Name, &room.Round, &room.Revealed, &room.HostID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Room{}, domain.NotFoundError("room not found")
		}
		return domain.Room{}, err
	}

	if participantID != "" {
		var exists int
		if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM participants WHERE id = ? AND room_code = ?`, participantID, code).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Room{}, domain.ForbiddenError("participant session is not valid for this room")
			}
			return domain.Room{}, err
		}
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.name, v.value
		FROM participants p
		LEFT JOIN votes v ON v.participant_id = p.id AND v.room_code = p.room_code AND v.round = ?
		WHERE p.room_code = ?
		ORDER BY p.joined_at, p.name`, room.Round, code)
	if err != nil {
		return domain.Room{}, err
	}
	defer rows.Close()

	room.Participants = []domain.Participant{}
	room.OnlineParticipantIDs = []string{}
	for rows.Next() {
		var participant domain.Participant
		var vote sql.NullString
		if err := rows.Scan(&participant.ID, &participant.Name, &vote); err != nil {
			return domain.Room{}, err
		}
		participant.HasVoted = vote.Valid
		if room.Revealed && vote.Valid {
			participant.Vote = &vote.String
		}
		room.Participants = append(room.Participants, participant)
	}
	return room, rows.Err()
}

func (r *Repository) CastVote(ctx context.Context, code, participantID, value string) error {
	var round int
	var revealed bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT r.round, r.revealed
		FROM rooms r JOIN participants p ON p.room_code = r.code
		WHERE r.code = ? AND p.id = ?`, code, participantID).Scan(&round, &revealed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ForbiddenError("participant session is not valid for this room")
		}
		return err
	}
	if revealed {
		return domain.ValidationError("votes are already revealed")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO votes(room_code, participant_id, round, value) VALUES (?, ?, ?, ?)
		ON CONFLICT(room_code, participant_id, round) DO UPDATE SET value = excluded.value`, code, participantID, round, value)
	return err
}

func (r *Repository) Reveal(ctx context.Context, code, participantID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE rooms SET revealed = 1 WHERE code = ? AND host_id = ?`, code, participantID)
	if err != nil {
		return err
	}
	return requireChanged(result, "only the room host can reveal votes")
}

func (r *Repository) Reset(ctx context.Context, code, participantID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE rooms SET round = round + 1, revealed = 0 WHERE code = ? AND host_id = ?`, code, participantID)
	if err != nil {
		return err
	}
	return requireChanged(result, "only the room host can start a new round")
}

func (r *Repository) roomExists(ctx context.Context, code string) bool {
	var exists int
	return r.db.QueryRowContext(ctx, `SELECT 1 FROM rooms WHERE code = ?`, code).Scan(&exists) == nil
}

func requireChanged(result sql.Result, message string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return domain.ForbiddenError(message)
	}
	return nil
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
