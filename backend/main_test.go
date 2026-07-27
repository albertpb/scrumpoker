package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestRealtimeVotingWorkflow(t *testing.T) {
	st, err := openStore(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.close() })

	app := &application{store: st, hub: newHub(), logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/rooms", app.createRoom)
	mux.HandleFunc("POST /api/rooms/{code}/join", app.joinRoom)
	mux.HandleFunc("GET /ws", app.websocket)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	host := postSession(t, server.URL+"/api/rooms", createRoomRequest{RoomName: "Release planning", UserName: "Ada"})
	guest := postSession(t, server.URL+"/api/rooms/"+host.Room.Code+"/join", joinRoomRequest{UserName: "Linus"})

	hostSocket := dialRoom(t, server.URL, host.Room.Code, host.ParticipantID)
	t.Cleanup(func() { _ = hostSocket.CloseNow() })
	guestSocket := dialRoom(t, server.URL, host.Room.Code, guest.ParticipantID)
	t.Cleanup(func() { _ = guestSocket.CloseNow() })

	writeMessage(t, hostSocket, clientMessage{Type: "vote", Value: "5"})
	writeMessage(t, guestSocket, clientMessage{Type: "vote", Value: "8"})
	readRoomUntil(t, hostSocket, func(room roomState) bool {
		return len(room.Participants) == 2 && room.Participants[0].HasVoted && room.Participants[1].HasVoted
	})
	writeMessage(t, hostSocket, clientMessage{Type: "reveal"})

	revealed := readRoomUntil(t, guestSocket, func(room roomState) bool { return room.Revealed })
	if len(revealed.Participants) != 2 {
		t.Fatalf("got %d participants, want 2", len(revealed.Participants))
	}
	for _, person := range revealed.Participants {
		if person.Vote == nil {
			t.Fatalf("revealed vote missing for %s", person.Name)
		}
	}
}

func postSession(t *testing.T, url string, body any) sessionResponse {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		t.Fatalf("POST %s returned %s", url, response.Status)
	}
	var session sessionResponse
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	return session
}

func dialRoom(t *testing.T, serverURL, roomCode, participantID string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(serverURL, "http") + "/ws?roomCode=" + roomCode + "&participantId=" + participantID
	conn, _, err := websocket.Dial(context.Background(), url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeMessage(t *testing.T, conn *websocket.Conn, message clientMessage) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, conn, message); err != nil {
		t.Fatal(err)
	}
}

func readRoomUntil(t *testing.T, conn *websocket.Conn, matches func(roomState) bool) roomState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		var message serverMessage
		err := wsjson.Read(ctx, conn, &message)
		cancel()
		if err != nil {
			t.Fatalf("read state: %v", err)
		}
		if message.Type == "state" && message.Room != nil && matches(*message.Room) {
			return *message.Room
		}
	}
	t.Fatal("timed out waiting for room state")
	return roomState{}
}
