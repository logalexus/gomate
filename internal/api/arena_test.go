package api

import (
	"context"
	"encoding/base64"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"gomate/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestArenaUsesAuthenticationAndChatIdentity(t *testing.T) {
	s := &Server{Config: config.Config{Login: "test", Password: "secret"}}
	server := httptest.NewServer(s)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v2/chat/arena"
	c, response, err := websocket.Dial(ctx, url, nil)
	if c != nil {
		c.CloseNow()
	}
	if err == nil || response.StatusCode != 401 {
		t.Fatal("arena allowed anonymous HTTP access")
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v2/chat", nil)
	name := s.chatAuthor(recorder, request)
	cookie := recorder.Result().Cookies()[0]
	headers := http.Header{"Authorization": []string{"Basic " + base64.StdEncoding.EncodeToString([]byte("test:secret"))}, "Cookie": []string{cookie.Name + "=" + cookie.Value}}
	c, _, err = websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	var welcome any
	if err = wsjson.Read(ctx, c, &welcome); err != nil {
		t.Fatal(err)
	}
	var state struct{ Players []struct{ Name string } }
	if err = wsjson.Read(ctx, c, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Players) != 1 || state.Players[0].Name != name {
		t.Fatal("chat identity not preserved")
	}
}
