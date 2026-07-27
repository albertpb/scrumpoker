package domain

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

type Room struct {
	Code                 string
	Name                 string
	Round                int
	Revealed             bool
	HostID               string
	Participants         []Participant
	OnlineParticipantIDs []string
}

type Participant struct {
	ID       string
	Name     string
	HasVoted bool
	Vote     *string
}

type ErrorKind string

const (
	ErrorValidation ErrorKind = "validation"
	ErrorNotFound   ErrorKind = "not_found"
	ErrorForbidden  ErrorKind = "forbidden"
)

type Error struct {
	Kind    ErrorKind
	Message string
}

func (e *Error) Error() string { return e.Message }

func ValidationError(message string) error {
	return &Error{Kind: ErrorValidation, Message: message}
}

func NotFoundError(message string) error {
	return &Error{Kind: ErrorNotFound, Message: message}
}

func ForbiddenError(message string) error {
	return &Error{Kind: ErrorForbidden, Message: message}
}

func PublicError(err error) string {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Message
	}
	return "something went wrong"
}

func ValidateRoomName(value string) (string, error) {
	return cleanName(value, "Room name", 60)
}

func ValidateParticipantName(value string) (string, error) {
	return cleanName(value, "Your name", 30)
}

func ValidateVote(value string) error {
	allowed := map[string]bool{
		"0": true, "1": true, "2": true, "3": true, "5": true,
		"8": true, "13": true, "21": true, "34": true, "55": true,
		"89": true, "?": true, "coffee": true,
	}
	if !allowed[value] {
		return ValidationError("invalid vote")
	}
	return nil
}

func NormalizeCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func cleanName(value, label string, max int) (string, error) {
	value = strings.Join(strings.Fields(value), " ")
	length := len([]rune(value))
	if length < 2 || length > max {
		return "", ValidationError(fmt.Sprintf("%s must be between 2 and %d characters", label, max))
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", ValidationError(label + " contains invalid characters")
		}
	}
	return value, nil
}
