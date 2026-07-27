package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"scrumpoker/backend/application"
	"scrumpoker/backend/domain"
)

type handler struct {
	service *application.Service
	hub     *hub
	logger  *slog.Logger
}

type createRoomRequest struct {
	RoomName string `json:"roomName"`
	UserName string `json:"userName"`
}

type joinRoomRequest struct {
	UserName string `json:"userName"`
}

type sessionResponse struct {
	Room          roomResponse `json:"room"`
	ParticipantID string       `json:"participantId"`
}

type roomResponse struct {
	Code                 string                `json:"code"`
	Name                 string                `json:"name"`
	Round                int                   `json:"round"`
	Revealed             bool                  `json:"revealed"`
	HostID               string                `json:"hostId"`
	Participants         []participantResponse `json:"participants"`
	OnlineParticipantIDs []string              `json:"onlineParticipantIds"`
}

type participantResponse struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	HasVoted bool    `json:"hasVoted"`
	Vote     *string `json:"vote"`
}

type clientMessage struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

type serverMessage struct {
	Type    string        `json:"type"`
	Room    *roomResponse `json:"room,omitempty"`
	Message string        `json:"message,omitempty"`
}

func NewHandler(service *application.Service, logger *slog.Logger, frontendDir string) http.Handler {
	api := &handler{service: service, hub: newHub(), logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", api.health)
	mux.HandleFunc("POST /api/rooms", api.createRoom)
	mux.HandleFunc("POST /api/rooms/{code}/join", api.joinRoom)
	mux.HandleFunc("GET /api/rooms/{code}", api.getRoom)
	mux.HandleFunc("GET /ws", api.websocket)
	if frontendDir != "" {
		mux.Handle("/", spaHandler(frontendDir))
	}
	return requestLogger(logger, mux)
}

func (h *handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) createRoom(w http.ResponseWriter, r *http.Request) {
	var input createRoomRequest
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	room, participantID, err := h.service.CreateRoom(r.Context(), input.RoomName, input.UserName)
	if err != nil {
		h.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Room: mapRoom(room), ParticipantID: participantID})
}

func (h *handler) joinRoom(w http.ResponseWriter, r *http.Request) {
	var input joinRoomRequest
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	room, participantID, err := h.service.JoinRoom(r.Context(), r.PathValue("code"), input.UserName)
	if err != nil {
		h.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Room: mapRoom(room), ParticipantID: participantID})
	h.broadcastRoom(room.Code)
}

func (h *handler) getRoom(w http.ResponseWriter, r *http.Request) {
	participantID := strings.TrimSpace(r.URL.Query().Get("participantId"))
	room, err := h.service.RoomState(r.Context(), r.PathValue("code"), participantID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mapRoom(room))
}

func (h *handler) websocket(w http.ResponseWriter, r *http.Request) {
	code := domain.NormalizeCode(r.URL.Query().Get("roomCode"))
	participantID := strings.TrimSpace(r.URL.Query().Get("participantId"))
	if _, err := h.service.RoomState(r.Context(), code, participantID); err != nil {
		h.handleError(w, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		h.logger.Warn("accept websocket", "error", err)
		return
	}
	defer conn.CloseNow()

	client := h.hub.add(code, participantID, conn)
	defer func() {
		h.hub.remove(client)
		h.broadcastRoom(code)
	}()
	h.broadcastRoom(code)

	for {
		var message clientMessage
		if err := wsjson.Read(r.Context(), conn, &message); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure && websocket.CloseStatus(err) != websocket.StatusGoingAway {
				h.logger.Debug("websocket read ended", "error", err)
			}
			return
		}

		if err := h.handleMessage(r.Context(), code, participantID, message); err != nil {
			client.send(serverMessage{Type: "error", Message: domain.PublicError(err)})
			continue
		}
		h.broadcastRoom(code)
	}
}

func (h *handler) handleMessage(ctx context.Context, code, participantID string, message clientMessage) error {
	switch message.Type {
	case "vote":
		return h.service.CastVote(ctx, code, participantID, message.Value)
	case "reveal":
		return h.service.Reveal(ctx, code, participantID)
	case "reset":
		return h.service.Reset(ctx, code, participantID)
	default:
		return domain.ValidationError("unknown message type")
	}
}

func (h *handler) broadcastRoom(code string) {
	room, err := h.service.RoomState(context.Background(), code, "")
	if err != nil {
		h.logger.Error("load state for broadcast", "room", code, "error", err)
		return
	}
	room.OnlineParticipantIDs = h.hub.onlineParticipants(code)
	response := mapRoom(room)
	h.hub.broadcast(code, serverMessage{Type: "state", Room: &response})
}

func (h *handler) handleError(w http.ResponseWriter, err error) {
	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		status := http.StatusBadRequest
		switch domainErr.Kind {
		case domain.ErrorNotFound:
			status = http.StatusNotFound
		case domain.ErrorForbidden:
			status = http.StatusForbidden
		}
		writeError(w, status, domainErr.Message)
		return
	}
	h.logger.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
}

func mapRoom(room domain.Room) roomResponse {
	participants := make([]participantResponse, len(room.Participants))
	for i, participant := range room.Participants {
		participants[i] = participantResponse{
			ID: participant.ID, Name: participant.Name,
			HasVoted: participant.HasVoted, Vote: participant.Vote,
		}
	}
	return roomResponse{
		Code: room.Code, Name: room.Name, Round: room.Round,
		Revealed: room.Revealed, HostID: room.HostID,
		Participants: participants, OnlineParticipantIDs: room.OnlineParticipantIDs,
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}

func spaHandler(directory string) http.Handler {
	files := http.FileServer(http.Dir(directory))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(directory, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(directory, "index.html"))
	})
}
