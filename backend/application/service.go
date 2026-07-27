package application

import (
	"context"

	"scrumpoker/backend/domain"
)

type RoomRepository interface {
	CreateRoom(ctx context.Context, roomName, participantName string) (domain.Room, string, error)
	JoinRoom(ctx context.Context, code, participantName string) (domain.Room, string, error)
	RoomState(ctx context.Context, code, participantID string) (domain.Room, error)
	CastVote(ctx context.Context, code, participantID, value string) error
	Reveal(ctx context.Context, code, participantID string) error
	Reset(ctx context.Context, code, participantID string) error
}

type Service struct {
	rooms RoomRepository
}

func NewService(rooms RoomRepository) *Service {
	return &Service{rooms: rooms}
}

func (s *Service) CreateRoom(ctx context.Context, roomName, participantName string) (domain.Room, string, error) {
	roomName, err := domain.ValidateRoomName(roomName)
	if err != nil {
		return domain.Room{}, "", err
	}
	participantName, err = domain.ValidateParticipantName(participantName)
	if err != nil {
		return domain.Room{}, "", err
	}
	return s.rooms.CreateRoom(ctx, roomName, participantName)
}

func (s *Service) JoinRoom(ctx context.Context, code, participantName string) (domain.Room, string, error) {
	participantName, err := domain.ValidateParticipantName(participantName)
	if err != nil {
		return domain.Room{}, "", err
	}
	return s.rooms.JoinRoom(ctx, domain.NormalizeCode(code), participantName)
}

func (s *Service) RoomState(ctx context.Context, code, participantID string) (domain.Room, error) {
	return s.rooms.RoomState(ctx, domain.NormalizeCode(code), participantID)
}

func (s *Service) CastVote(ctx context.Context, code, participantID, value string) error {
	if err := domain.ValidateVote(value); err != nil {
		return err
	}
	return s.rooms.CastVote(ctx, domain.NormalizeCode(code), participantID, value)
}

func (s *Service) Reveal(ctx context.Context, code, participantID string) error {
	return s.rooms.Reveal(ctx, domain.NormalizeCode(code), participantID)
}

func (s *Service) Reset(ctx context.Context, code, participantID string) error {
	return s.rooms.Reset(ctx, domain.NormalizeCode(code), participantID)
}
