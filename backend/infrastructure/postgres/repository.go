package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"scrumpoker/backend/domain"
)

type Repository struct {
	db *sql.DB
}

func Open(connectionURL string) (*Repository, error) {
	config, err := connectionConfig(connectionURL)
	if err != nil {
		return nil, err
	}
	db := stdlib.OpenDB(*config)

	repository := &Repository{db: db}
	if err := repository.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repository, nil
}

func connectionConfig(connectionURL string) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(connectionURL)
	if err != nil {
		return nil, err
	}

	// Transaction poolers can move each query to a different server connection,
	// so connection-local named prepared statements are not safe to cache.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	return config, nil
}

func (r *Repository) Close() error { return r.db.Close() }

func (r *Repository) migrate() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS rooms (
			code TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			host_id TEXT NOT NULL,
			round INTEGER NOT NULL DEFAULT 1,
			revealed BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMPTZ NOT NULL
		);
		CREATE TABLE IF NOT EXISTS participants (
			id TEXT PRIMARY KEY,
			room_code TEXT NOT NULL REFERENCES rooms(code) ON DELETE CASCADE,
			name TEXT NOT NULL,
			joined_at TIMESTAMPTZ NOT NULL
		);
		CREATE UNIQUE INDEX IF NOT EXISTS participants_room_name
			ON participants(room_code, LOWER(name));
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
		_, err = tx.ExecContext(ctx, `INSERT INTO rooms(code, name, host_id, created_at) VALUES ($1, $2, $3, $4)`, code, roomName, id, time.Now().UTC())
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO participants(id, room_code, name, joined_at) VALUES ($1, $2, $3, $4)`, id, code, participantName, time.Now().UTC())
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
	_, err := r.db.ExecContext(ctx, `INSERT INTO participants(id, room_code, name, joined_at) VALUES ($1, $2, $3, $4)`, id, code, participantName, time.Now().UTC())
	if err != nil {
		var postgresErr *pgconn.PgError
		if errors.As(err, &postgresErr) && postgresErr.Code == "23505" {
			return domain.Room{}, "", domain.ValidationError("that name is already in the room")
		}
		return domain.Room{}, "", err
	}
	room, err := r.RoomState(ctx, code, id)
	return room, id, err
}

func (r *Repository) RoomState(ctx context.Context, code, participantID string) (domain.Room, error) {
	var room domain.Room
	if err := r.db.QueryRowContext(ctx, `SELECT code, name, round, revealed, host_id FROM rooms WHERE code = $1`, code).
		Scan(&room.Code, &room.Name, &room.Round, &room.Revealed, &room.HostID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Room{}, domain.NotFoundError("room not found")
		}
		return domain.Room{}, err
	}

	if participantID != "" {
		var exists int
		if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM participants WHERE id = $1 AND room_code = $2`, participantID, code).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Room{}, domain.ForbiddenError("participant session is not valid for this room")
			}
			return domain.Room{}, err
		}
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.name, v.value
		FROM participants p
		LEFT JOIN votes v ON v.participant_id = p.id AND v.room_code = p.room_code AND v.round = $1
		WHERE p.room_code = $2
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
		WHERE r.code = $1 AND p.id = $2`, code, participantID).Scan(&round, &revealed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ForbiddenError("participant session is not valid for this room")
		}
		return err
	}
	if revealed {
		return domain.ValidationError("votes are already revealed")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO votes(room_code, participant_id, round, value) VALUES ($1, $2, $3, $4)
		ON CONFLICT(room_code, participant_id, round) DO UPDATE SET value = excluded.value`, code, participantID, round, value)
	return err
}

func (r *Repository) Reveal(ctx context.Context, code, participantID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE rooms SET revealed = TRUE WHERE code = $1 AND host_id = $2`, code, participantID)
	if err != nil {
		return err
	}
	return requireChanged(result, "only the room host can reveal votes")
}

func (r *Repository) Reset(ctx context.Context, code, participantID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE rooms SET round = round + 1, revealed = FALSE WHERE code = $1 AND host_id = $2`, code, participantID)
	if err != nil {
		return err
	}
	return requireChanged(result, "only the room host can start a new round")
}

func (r *Repository) roomExists(ctx context.Context, code string) bool {
	var exists int
	return r.db.QueryRowContext(ctx, `SELECT 1 FROM rooms WHERE code = $1`, code).Scan(&exists) == nil
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
