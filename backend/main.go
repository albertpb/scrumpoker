package main

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
)

type application struct {
	store  *store
	hub    *hub
	logger *slog.Logger
}

type createRoomRequest struct {
	RoomName string `json:"roomName"`
	UserName string `json:"userName"`
}

type joinRoomRequest struct {
	UserName string `json:"userName"`
}

type sessionResponse struct {
	Room          roomState `json:"room"`
	ParticipantID string    `json:"participantId"`
}

type clientMessage struct {
	Type  string `json:"type"`
	Value string `json:"value,omitempty"`
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	databasePath := envOrDefault("DATABASE_PATH", "poker.db")
	st, err := openStore(databasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer st.close()

	app := &application{store: st, hub: newHub(), logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", app.health)
	mux.HandleFunc("POST /api/rooms", app.createRoom)
	mux.HandleFunc("POST /api/rooms/{code}/join", app.joinRoom)
	mux.HandleFunc("GET /api/rooms/{code}", app.getRoom)
	mux.HandleFunc("GET /ws", app.websocket)

	if frontendDir := os.Getenv("FRONTEND_DIR"); frontendDir != "" {
		mux.Handle("/", spaHandler(frontendDir))
	}

	server := &http.Server{
		Addr:              envOrDefault("ADDR", ":8080"),
		Handler:           requestLogger(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("server listening", "address", server.Addr, "database", databasePath)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func (app *application) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (app *application) createRoom(w http.ResponseWriter, r *http.Request) {
	var input createRoomRequest
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	room, participantID, err := app.store.createRoom(r.Context(), input.RoomName, input.UserName)
	if err != nil {
		app.handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Room: room, ParticipantID: participantID})
}

func (app *application) joinRoom(w http.ResponseWriter, r *http.Request) {
	var input joinRoomRequest
	if err := readJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	room, participantID, err := app.store.joinRoom(r.Context(), r.PathValue("code"), input.UserName)
	if err != nil {
		app.handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sessionResponse{Room: room, ParticipantID: participantID})
	app.broadcastRoom(room.Code)
}

func (app *application) getRoom(w http.ResponseWriter, r *http.Request) {
	participantID := strings.TrimSpace(r.URL.Query().Get("participantId"))
	room, err := app.store.roomState(r.Context(), r.PathValue("code"), participantID)
	if err != nil {
		app.handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, room)
}

func (app *application) websocket(w http.ResponseWriter, r *http.Request) {
	code := normalizeCode(r.URL.Query().Get("roomCode"))
	participantID := strings.TrimSpace(r.URL.Query().Get("participantId"))
	if _, err := app.store.roomState(r.Context(), code, participantID); err != nil {
		app.handleStoreError(w, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost:*", "127.0.0.1:*"},
	})
	if err != nil {
		app.logger.Warn("accept websocket", "error", err)
		return
	}
	defer conn.CloseNow()

	client := app.hub.add(code, participantID, conn)
	defer func() {
		app.hub.remove(client)
		app.broadcastRoom(code)
	}()
	app.broadcastRoom(code)

	for {
		var message clientMessage
		if err := wsjson.Read(r.Context(), conn, &message); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure && websocket.CloseStatus(err) != websocket.StatusGoingAway {
				app.logger.Debug("websocket read ended", "error", err)
			}
			return
		}

		if err := app.handleMessage(r.Context(), code, participantID, message); err != nil {
			client.send(serverMessage{Type: "error", Message: publicError(err)})
			continue
		}
		app.broadcastRoom(code)
	}
}

func (app *application) handleMessage(ctx context.Context, code, participantID string, message clientMessage) error {
	switch message.Type {
	case "vote":
		return app.store.castVote(ctx, code, participantID, message.Value)
	case "reveal":
		return app.store.reveal(ctx, code, participantID)
	case "reset":
		return app.store.reset(ctx, code, participantID)
	default:
		return validationError("unknown message type")
	}
}

func (app *application) broadcastRoom(code string) {
	state, err := app.store.roomState(context.Background(), code, "")
	if err != nil {
		app.logger.Error("load state for broadcast", "room", code, "error", err)
		return
	}
	state.OnlineParticipantIDs = app.hub.onlineParticipants(code)
	app.hub.broadcast(code, serverMessage{Type: "state", Room: &state})
}

func (app *application) handleStoreError(w http.ResponseWriter, err error) {
	var requestErr *storeError
	if errors.As(err, &requestErr) {
		writeError(w, requestErr.status, requestErr.message)
		return
	}
	app.logger.Error("request failed", "error", err)
	writeError(w, http.StatusInternalServerError, "something went wrong")
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

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
