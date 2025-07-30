/**
 * Standalone signaling server for the Nextcloud Spreed app.
 * Copyright (C) 2017 struktur AG
 *
 * @author Joachim Bauch <bauch@struktur.de>
 *
 * @license GNU AGPL version 3 or any later version
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */
package signaling

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testBackendSecret  = []byte("secret")
	testInternalSecret = []byte("internal-secret")

	ErrNoMessageReceived = fmt.Errorf("no message was received by the server")

	testClientDialer = websocket.Dialer{
		WriteBufferPool: &sync.Pool{},
	}
)

type TestBackendClientAuthParams struct {
	UserId string `json:"userid"`
}

func getWebsocketUrl(url string) string {
	if strings.HasPrefix(url, "http://") {
		return "ws://" + url[7:] + "/spreed"
	} else if strings.HasPrefix(url, "https://") {
		return "wss://" + url[8:] + "/spreed"
	} else {
		panic("Unsupported URL: " + url)
	}
}

func getPubliceSessionIdData(h *Hub, publicId string) *SessionIdData {
	decodedPublic := h.decodePublicSessionId(publicId)
	if decodedPublic == nil {
		panic("invalid public session id")
	}
	return decodedPublic
}

func checkUnexpectedClose(err error) error {
	if err != nil && websocket.IsUnexpectedCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived) {
		return fmt.Errorf("Connection was closed with unexpected error: %s", err)
	}

	return nil
}

func checkMessageType(message *ServerMessage, expectedType string) error {
	if message == nil {
		return ErrNoMessageReceived
	}

	if message.Type != expectedType {
		return fmt.Errorf("Expected \"%s\" message, got %+v", expectedType, message)
	}
	switch message.Type {
	case "hello":
		if message.Hello == nil {
			return fmt.Errorf("Expected \"%s\" message, got %+v", expectedType, message)
		}
	case "message":
		if message.Message == nil {
			return fmt.Errorf("Expected \"%s\" message, got %+v", expectedType, message)
		} else if len(message.Message.Data) == 0 {
			return fmt.Errorf("Received message without data")
		}
	case "room":
		if message.Room == nil {
			return fmt.Errorf("Expected \"%s\" message, got %+v", expectedType, message)
		}
	case "event":
		if message.Event == nil {
			return fmt.Errorf("Expected \"%s\" message, got %+v", expectedType, message)
		}
	case "transient":
		if message.TransientData == nil {
			return fmt.Errorf("Expected \"%s\" message, got %+v", expectedType, message)
		}
	}

	return nil
}

func checkMessageSender(hub *Hub, sender *MessageServerMessageSender, senderType string, hello *HelloServerMessage) error {
	if sender.Type != senderType {
		return fmt.Errorf("Expected sender type %s, got %s", senderType, sender.Type)
	} else if sender.SessionId != hello.SessionId {
		return fmt.Errorf("Expected session id %+v, got %+v",
			getPubliceSessionIdData(hub, hello.SessionId), getPubliceSessionIdData(hub, sender.SessionId))
	} else if sender.UserId != hello.UserId {
		return fmt.Errorf("Expected user id %s, got %s", hello.UserId, sender.UserId)
	}

	return nil
}

func checkReceiveClientMessageWithSenderAndRecipient(ctx context.Context, client *TestClient, senderType string, hello *HelloServerMessage, payload any, sender **MessageServerMessageSender, recipient **MessageClientMessageRecipient) error {
	message, err := client.RunUntilMessage(ctx)
	if err := checkUnexpectedClose(err); err != nil {
		return err
	} else if err := checkMessageType(message, "message"); err != nil {
		return err
	} else if err := checkMessageSender(client.hub, message.Message.Sender, senderType, hello); err != nil {
		return err
	} else {
		if err := json.Unmarshal(message.Message.Data, payload); err != nil {
			return err
		}
	}
	if sender != nil {
		*sender = message.Message.Sender
	}
	if recipient != nil {
		*recipient = message.Message.Recipient
	}
	return nil
}

func checkReceiveClientMessageWithSender(ctx context.Context, client *TestClient, senderType string, hello *HelloServerMessage, payload any, sender **MessageServerMessageSender) error {
	return checkReceiveClientMessageWithSenderAndRecipient(ctx, client, senderType, hello, payload, sender, nil)
}

func checkReceiveClientMessage(ctx context.Context, client *TestClient, senderType string, hello *HelloServerMessage, payload any) error {
	return checkReceiveClientMessageWithSenderAndRecipient(ctx, client, senderType, hello, payload, nil, nil)
}

func checkReceiveClientControlWithSenderAndRecipient(ctx context.Context, client *TestClient, senderType string, hello *HelloServerMessage, payload any, sender **MessageServerMessageSender, recipient **MessageClientMessageRecipient) error {
	message, err := client.RunUntilMessage(ctx)
	if err := checkUnexpectedClose(err); err != nil {
		return err
	} else if err := checkMessageType(message, "control"); err != nil {
		return err
	} else if err := checkMessageSender(client.hub, message.Control.Sender, senderType, hello); err != nil {
		return err
	} else {
		if err := json.Unmarshal(message.Control.Data, payload); err != nil {
			return err
		}
	}
	if sender != nil {
		*sender = message.Control.Sender
	}
	if recipient != nil {
		*recipient = message.Control.Recipient
	}
	return nil
}

func checkReceiveClientControlWithSender(ctx context.Context, client *TestClient, senderType string, hello *HelloServerMessage, payload any, sender **MessageServerMessageSender) error { // nolint
	return checkReceiveClientControlWithSenderAndRecipient(ctx, client, senderType, hello, payload, sender, nil)
}

func checkReceiveClientControl(ctx context.Context, client *TestClient, senderType string, hello *HelloServerMessage, payload any) error {
	return checkReceiveClientControlWithSenderAndRecipient(ctx, client, senderType, hello, payload, nil, nil)
}

func checkReceiveClientEvent(ctx context.Context, client *TestClient, eventType string, msg **EventServerMessage) error {
	message, err := client.RunUntilMessage(ctx)
	if err := checkUnexpectedClose(err); err != nil {
		return err
	} else if err := checkMessageType(message, "event"); err != nil {
		return err
	} else if message.Event.Type != eventType {
		return fmt.Errorf("Expected \"%s\" event type, got \"%s\"", eventType, message.Event.Type)
	} else {
		if msg != nil {
			*msg = message.Event
		}
	}
	return nil
}

type TestClient struct {
	t      *testing.T
	hub    *Hub
	server *httptest.Server

	mu        sync.Mutex
	conn      *websocket.Conn
	localAddr net.Addr

	messageChan   chan []byte
	readErrorChan chan error

	publicId string
}

func NewTestClientContext(ctx context.Context, t *testing.T, server *httptest.Server, hub *Hub) *TestClient {
	// Reference "hub" to prevent compiler error.
	conn, _, err := testClientDialer.DialContext(ctx, getWebsocketUrl(server.URL), nil)
	require.NoError(t, err)

	messageChan := make(chan []byte)
	readErrorChan := make(chan error, 1)

	go func() {
		for {
			messageType, data, err := conn.ReadMessage()
			if err != nil {
				readErrorChan <- err
				return
			} else if !assert.Equal(t, websocket.TextMessage, messageType) {
				return
			}

			messageChan <- data
		}
	}()

	return &TestClient{
		t:      t,
		hub:    hub,
		server: server,

		conn:      conn,
		localAddr: conn.LocalAddr(),

		messageChan:   messageChan,
		readErrorChan: readErrorChan,
	}
}

func NewTestClient(t *testing.T, server *httptest.Server, hub *Hub) *TestClient {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	client := NewTestClientContext(ctx, t, server, hub)
	msg, err := client.RunUntilMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "welcome", msg.Type)
	return client
}

func (c *TestClient) CloseWithBye() {
	c.SendBye() // nolint
	c.Close()
}

func (c *TestClient) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err == websocket.ErrCloseSent {
		// Already closed
		return
	}

	// Wait a bit for close message to be processed.
	time.Sleep(100 * time.Millisecond)
	c.conn.Close()

	// Drain any entries in the channels to terminate the read goroutine.
loop:
	for {
		select {
		case <-c.readErrorChan:
		case <-c.messageChan:
		default:
			break loop
		}
	}
}

func (c *TestClient) WaitForClientRemoved(ctx context.Context) error {
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	for {
		found := false
		for _, client := range c.hub.clients {
			if cc, ok := client.(*Client); ok {
				cc.mu.Lock()
				conn := cc.conn
				cc.mu.Unlock()
				if conn != nil && conn.RemoteAddr().String() == c.localAddr.String() {
					found = true
					break
				}
			}
		}
		if !found {
			break
		}

		c.hub.mu.Unlock()
		select {
		case <-ctx.Done():
			c.hub.mu.Lock()
			return ctx.Err()
		default:
			time.Sleep(time.Millisecond)
		}
		c.hub.mu.Lock()
	}
	return nil
}

func (c *TestClient) WaitForSessionRemoved(ctx context.Context, sessionId string) error {
	data := c.hub.decodePublicSessionId(sessionId)
	if data == nil {
		return fmt.Errorf("Invalid session id passed")
	}

	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	for {
		_, found := c.hub.sessions[data.Sid]
		if !found {
			break
		}

		c.hub.mu.Unlock()
		select {
		case <-ctx.Done():
			c.hub.mu.Lock()
			return ctx.Err()
		default:
			time.Sleep(time.Millisecond)
		}
		c.hub.mu.Lock()
	}
	return nil
}

func (c *TestClient) WriteJSON(data any) error {
	if !strings.Contains(c.t.Name(), "HelloUnsupportedVersion") {
		if msg, ok := data.(*ClientMessage); ok {
			if err := msg.CheckValid(); err != nil {
				return err
			}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(data)
}

func (c *TestClient) EnsuerWriteJSON(data any) {
	require.NoError(c.t, c.WriteJSON(data), "Could not write JSON %+v", data)
}

func (c *TestClient) SendHello(userid string) error {
	return c.SendHelloV1(userid)
}

func (c *TestClient) SendHelloV1(userid string) error {
	params := TestBackendClientAuthParams{
		UserId: userid,
	}
	return c.SendHelloParams(c.server.URL, HelloVersionV1, "", nil, params)
}

func (c *TestClient) SendHelloV2(userid string) error {
	return c.SendHelloV2WithFeatures(userid, nil)
}

func (c *TestClient) SendHelloV2WithFeatures(userid string, features []string) error {
	now := time.Now()
	return c.SendHelloV2WithTimesAndFeatures(userid, now, now.Add(time.Minute), features)
}

func (c *TestClient) CreateHelloV2TokenWithUserdata(userid string, issuedAt time.Time, expiresAt time.Time, userdata StringMap) (string, error) {
	data, err := json.Marshal(userdata)
	if err != nil {
		return "", err
	}

	claims := &HelloV2TokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:  c.server.URL,
			Subject: userid,
		},
		UserData: data,
	}
	if !issuedAt.IsZero() {
		claims.IssuedAt = jwt.NewNumericDate(issuedAt)
	}
	if !expiresAt.IsZero() {
		claims.ExpiresAt = jwt.NewNumericDate(expiresAt)
	}

	var token *jwt.Token
	if strings.Contains(c.t.Name(), "ECDSA") {
		token = jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	} else if strings.Contains(c.t.Name(), "Ed25519") {
		token = jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	} else {
		token = jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	}
	private := getPrivateAuthToken(c.t)
	return token.SignedString(private)
}

func (c *TestClient) CreateHelloV2Token(userid string, issuedAt time.Time, expiresAt time.Time) (string, error) {
	userdata := StringMap{
		"displayname": "Displayname " + userid,
	}

	return c.CreateHelloV2TokenWithUserdata(userid, issuedAt, expiresAt, userdata)
}

func (c *TestClient) SendHelloV2WithTimes(userid string, issuedAt time.Time, expiresAt time.Time) error {
	return c.SendHelloV2WithTimesAndFeatures(userid, issuedAt, expiresAt, nil)
}

func (c *TestClient) SendHelloV2WithTimesAndFeatures(userid string, issuedAt time.Time, expiresAt time.Time, features []string) error {
	tokenString, err := c.CreateHelloV2Token(userid, issuedAt, expiresAt)
	require.NoError(c.t, err)

	params := HelloV2AuthParams{
		Token: tokenString,
	}
	return c.SendHelloParams(c.server.URL, HelloVersionV2, "", features, params)
}

func (c *TestClient) SendHelloResume(resumeId string) error {
	hello := &ClientMessage{
		Id:   "1234",
		Type: "hello",
		Hello: &HelloClientMessage{
			Version:  HelloVersionV1,
			ResumeId: resumeId,
		},
	}
	return c.WriteJSON(hello)
}

func (c *TestClient) SendHelloClient(userid string) error {
	return c.SendHelloClientWithFeatures(userid, nil)
}

func (c *TestClient) SendHelloClientWithFeatures(userid string, features []string) error {
	params := TestBackendClientAuthParams{
		UserId: userid,
	}
	return c.SendHelloParams(c.server.URL, HelloVersionV1, "client", features, params)
}

func (c *TestClient) SendHelloInternal() error {
	return c.SendHelloInternalWithFeatures(nil)
}

func (c *TestClient) SendHelloInternalWithFeatures(features []string) error {
	random := newRandomString(48)
	mac := hmac.New(sha256.New, testInternalSecret)
	mac.Write([]byte(random)) // nolint
	token := hex.EncodeToString(mac.Sum(nil))
	backend := c.server.URL

	params := ClientTypeInternalAuthParams{
		Random:  random,
		Token:   token,
		Backend: backend,
	}
	return c.SendHelloParams("", HelloVersionV1, "internal", features, params)
}

func (c *TestClient) SendHelloParams(url string, version string, clientType string, features []string, params any) error {
	data, err := json.Marshal(params)
	require.NoError(c.t, err)

	hello := &ClientMessage{
		Id:   "1234",
		Type: "hello",
		Hello: &HelloClientMessage{
			Version:  version,
			Features: features,
			Auth: &HelloClientMessageAuth{
				Type:   clientType,
				Url:    url,
				Params: data,
			},
		},
	}
	return c.WriteJSON(hello)
}

func (c *TestClient) SendBye() error {
	hello := &ClientMessage{
		Id:   "9876",
		Type: "bye",
		Bye:  &ByeClientMessage{},
	}
	return c.WriteJSON(hello)
}

func (c *TestClient) SendMessage(recipient MessageClientMessageRecipient, data any) error {
	payload, err := json.Marshal(data)
	require.NoError(c.t, err)

	message := &ClientMessage{
		Id:   "abcd",
		Type: "message",
		Message: &MessageClientMessage{
			Recipient: recipient,
			Data:      payload,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) SendControl(recipient MessageClientMessageRecipient, data any) error {
	payload, err := json.Marshal(data)
	require.NoError(c.t, err)

	message := &ClientMessage{
		Id:   "abcd",
		Type: "control",
		Control: &ControlClientMessage{
			MessageClientMessage: MessageClientMessage{
				Recipient: recipient,
				Data:      payload,
			},
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) SendInternalAddSession(msg *AddSessionInternalClientMessage) error {
	message := &ClientMessage{
		Id:   "abcd",
		Type: "internal",
		Internal: &InternalClientMessage{
			Type:       "addsession",
			AddSession: msg,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) SendInternalUpdateSession(msg *UpdateSessionInternalClientMessage) error {
	message := &ClientMessage{
		Id:   "abcd",
		Type: "internal",
		Internal: &InternalClientMessage{
			Type:          "updatesession",
			UpdateSession: msg,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) SendInternalRemoveSession(msg *RemoveSessionInternalClientMessage) error {
	message := &ClientMessage{
		Id:   "abcd",
		Type: "internal",
		Internal: &InternalClientMessage{
			Type:          "removesession",
			RemoveSession: msg,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) SendInternalDialout(msg *DialoutInternalClientMessage) error {
	message := &ClientMessage{
		Id:   "abcd",
		Type: "internal",
		Internal: &InternalClientMessage{
			Type:    "dialout",
			Dialout: msg,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) SetTransientData(key string, value any, ttl time.Duration) error {
	payload, err := json.Marshal(value)
	require.NoError(c.t, err)

	message := &ClientMessage{
		Id:   "efgh",
		Type: "transient",
		TransientData: &TransientDataClientMessage{
			Type:  "set",
			Key:   key,
			Value: payload,
			TTL:   ttl,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) RemoveTransientData(key string) error {
	message := &ClientMessage{
		Id:   "ijkl",
		Type: "transient",
		TransientData: &TransientDataClientMessage{
			Type: "remove",
			Key:  key,
		},
	}
	return c.WriteJSON(message)
}

func (c *TestClient) DrainMessages(ctx context.Context) error {
	select {
	case err := <-c.readErrorChan:
		return err
	case <-c.messageChan:
		n := len(c.messageChan)
		for i := 0; i < n; i++ {
			<-c.messageChan
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (c *TestClient) GetPendingMessages(ctx context.Context) ([]*ServerMessage, error) {
	var result []*ServerMessage
	select {
	case err := <-c.readErrorChan:
		return nil, err
	case msg := <-c.messageChan:
		var m ServerMessage
		if err := json.Unmarshal(msg, &m); err != nil {
			return nil, err
		}
		result = append(result, &m)
		n := len(c.messageChan)
		for i := 0; i < n; i++ {
			var m ServerMessage
			msg = <-c.messageChan
			if err := json.Unmarshal(msg, &m); err != nil {
				return nil, err
			}
			result = append(result, &m)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return result, nil
}

func (c *TestClient) RunUntilMessage(ctx context.Context) (message *ServerMessage, err error) {
	select {
	case err = <-c.readErrorChan:
	case msg := <-c.messageChan:
		var m ServerMessage
		if err = json.Unmarshal(msg, &m); err == nil {
			message = &m
		}
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (c *TestClient) RunUntilError(ctx context.Context, code string) (*Error, error) {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkUnexpectedClose(err); err != nil {
		return nil, err
	}
	if err := checkMessageType(message, "error"); err != nil {
		return nil, err
	}
	if message.Error.Code != code {
		return nil, fmt.Errorf("expected error %s, got %s", code, message.Error.Code)
	}
	return message.Error, nil
}

func (c *TestClient) RunUntilHello(ctx context.Context) (message *ServerMessage, err error) {
	if message, err = c.RunUntilMessage(ctx); err != nil {
		return nil, err
	}
	if err := checkUnexpectedClose(err); err != nil {
		return nil, err
	}
	if err := checkMessageType(message, "hello"); err != nil {
		return nil, err
	}
	c.publicId = message.Hello.SessionId
	return message, nil
}

func (c *TestClient) JoinRoom(ctx context.Context, roomId string) (message *ServerMessage, err error) {
	return c.JoinRoomWithRoomSession(ctx, roomId, roomId+"-"+c.publicId)
}

func (c *TestClient) JoinRoomWithRoomSession(ctx context.Context, roomId string, roomSessionId string) (message *ServerMessage, err error) {
	msg := &ClientMessage{
		Id:   "ABCD",
		Type: "room",
		Room: &RoomClientMessage{
			RoomId:    roomId,
			SessionId: roomSessionId,
		},
	}
	if err := c.WriteJSON(msg); err != nil {
		return nil, err
	}

	if message, err = c.RunUntilMessage(ctx); err != nil {
		return nil, err
	}
	if err := checkUnexpectedClose(err); err != nil {
		return nil, err
	}
	if err := checkMessageType(message, "room"); err != nil {
		return nil, err
	}
	if message.Id != msg.Id {
		return nil, fmt.Errorf("expected message id %s, got %s", msg.Id, message.Id)
	}
	return message, nil
}

func checkMessageRoomId(message *ServerMessage, roomId string) error {
	if err := checkMessageType(message, "room"); err != nil {
		return err
	}
	if message.Room.RoomId != roomId {
		return fmt.Errorf("Expected room id %s, got %+v", roomId, message.Room)
	}
	return nil
}

func (c *TestClient) RunUntilRoom(ctx context.Context, roomId string) error {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return err
	}
	if err := checkUnexpectedClose(err); err != nil {
		return err
	}
	return checkMessageRoomId(message, roomId)
}

func (c *TestClient) checkMessageJoined(message *ServerMessage, hello *HelloServerMessage) error {
	return c.checkMessageJoinedSession(message, hello.SessionId, hello.UserId)
}

func (c *TestClient) checkSingleMessageJoined(message *ServerMessage) error {
	if err := checkMessageType(message, "event"); err != nil {
		return err
	} else if message.Event.Target != "room" {
		return fmt.Errorf("Expected event target room, got %+v", message.Event)
	} else if message.Event.Type != "join" {
		return fmt.Errorf("Expected event type join, got %+v", message.Event)
	} else if len(message.Event.Join) != 1 {
		return fmt.Errorf("Expected one join event entry, got %+v", message.Event)
	}
	return nil
}

func (c *TestClient) checkMessageJoinedSession(message *ServerMessage, sessionId string, userId string) error {
	if err := c.checkSingleMessageJoined(message); err != nil {
		return err
	}

	evt := message.Event.Join[0]
	if sessionId != "" && evt.SessionId != sessionId {
		return fmt.Errorf("Expected join session id %+v, got %+v",
			getPubliceSessionIdData(c.hub, sessionId), getPubliceSessionIdData(c.hub, evt.SessionId))
	}
	if evt.UserId != userId {
		return fmt.Errorf("Expected join user id %s, got %+v", userId, evt)
	}
	return nil
}

func (c *TestClient) RunUntilJoinedAndReturn(ctx context.Context, hello ...*HelloServerMessage) ([]*EventServerMessageSessionEntry, []*ServerMessage, error) {
	received := make([]*EventServerMessageSessionEntry, len(hello))
	var ignored []*ServerMessage
	hellos := make(map[*HelloServerMessage]int, len(hello))
	for idx, h := range hello {
		hellos[h] = idx
	}
	for len(hellos) > 0 {
		message, err := c.RunUntilMessage(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("got error while waiting for %+v: %w", hellos, err)
		}

		if err := checkMessageType(message, "event"); err != nil {
			ignored = append(ignored, message)
			continue
		} else if message.Event.Target != "room" || message.Event.Type != "join" {
			ignored = append(ignored, message)
			continue
		}

		for len(message.Event.Join) > 0 {
			found := false
		loop:
			for h, idx := range hellos {
				for idx2, evt := range message.Event.Join {
					if evt.SessionId == h.SessionId && evt.UserId == h.UserId {
						received[idx] = evt
						delete(hellos, h)
						message.Event.Join = append(message.Event.Join[:idx2], message.Event.Join[idx2+1:]...)
						found = true
						break loop
					}
				}
			}
			if !found {
				return nil, nil, fmt.Errorf("expected one of the passed hello sessions, got %+v", message.Event.Join[0])
			}
		}
	}
	return received, ignored, nil
}

func (c *TestClient) RunUntilJoined(ctx context.Context, hello ...*HelloServerMessage) error {
	_, unexpected, err := c.RunUntilJoinedAndReturn(ctx, hello...)
	if err != nil {
		return err
	}
	if len(unexpected) > 0 {
		return fmt.Errorf("Received unexpected messages: %+v", unexpected)
	}
	return nil
}

func (c *TestClient) checkMessageRoomLeave(message *ServerMessage, hello *HelloServerMessage) error {
	return c.checkMessageRoomLeaveSession(message, hello.SessionId)
}

func (c *TestClient) checkMessageRoomLeaveSession(message *ServerMessage, sessionId string) error {
	if err := checkMessageType(message, "event"); err != nil {
		return err
	} else if message.Event.Target != "room" {
		return fmt.Errorf("Expected event target room, got %+v", message.Event)
	} else if message.Event.Type != "leave" {
		return fmt.Errorf("Expected event type leave, got %+v", message.Event)
	} else if len(message.Event.Leave) != 1 {
		return fmt.Errorf("Expected one leave event entry, got %+v", message.Event)
	} else if message.Event.Leave[0] != sessionId {
		return fmt.Errorf("Expected leave session id %+v, got %+v",
			getPubliceSessionIdData(c.hub, sessionId), getPubliceSessionIdData(c.hub, message.Event.Leave[0]))
	}
	return nil
}

func (c *TestClient) RunUntilLeft(ctx context.Context, hello *HelloServerMessage) error {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return err
	}

	return c.checkMessageRoomLeave(message, hello)
}

func checkMessageRoomlistUpdate(message *ServerMessage) (*RoomEventServerMessage, error) {
	if err := checkMessageType(message, "event"); err != nil {
		return nil, err
	} else if message.Event.Target != "roomlist" {
		return nil, fmt.Errorf("Expected event target room, got %+v", message.Event)
	} else if message.Event.Type != "update" || message.Event.Update == nil {
		return nil, fmt.Errorf("Expected event type update, got %+v", message.Event)
	} else {
		return message.Event.Update, nil
	}
}

func (c *TestClient) RunUntilRoomlistUpdate(ctx context.Context) (*RoomEventServerMessage, error) {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return nil, err
	}

	return checkMessageRoomlistUpdate(message)
}

func checkMessageRoomlistDisinvite(message *ServerMessage) (*RoomDisinviteEventServerMessage, error) {
	if err := checkMessageType(message, "event"); err != nil {
		return nil, err
	} else if message.Event.Target != "roomlist" {
		return nil, fmt.Errorf("Expected event target room, got %+v", message.Event)
	} else if message.Event.Type != "disinvite" || message.Event.Disinvite == nil {
		return nil, fmt.Errorf("Expected event type disinvite, got %+v", message.Event)
	}

	return message.Event.Disinvite, nil
}

func (c *TestClient) RunUntilRoomlistDisinvite(ctx context.Context) (*RoomDisinviteEventServerMessage, error) {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return nil, err
	}

	return checkMessageRoomlistDisinvite(message)
}

func checkMessageParticipantsInCall(message *ServerMessage) (*RoomEventServerMessage, error) {
	if err := checkMessageType(message, "event"); err != nil {
		return nil, err
	} else if message.Event.Target != "participants" {
		return nil, fmt.Errorf("Expected event target participants, got %+v", message.Event)
	} else if message.Event.Type != "update" || message.Event.Update == nil {
		return nil, fmt.Errorf("Expected event type update, got %+v", message.Event)
	}

	return message.Event.Update, nil
}

func checkMessageParticipantFlags(message *ServerMessage) (*RoomFlagsServerMessage, error) {
	if err := checkMessageType(message, "event"); err != nil {
		return nil, err
	} else if message.Event.Target != "participants" {
		return nil, fmt.Errorf("Expected event target room, got %+v", message.Event)
	} else if message.Event.Type != "flags" || message.Event.Flags == nil {
		return nil, fmt.Errorf("Expected event type flags, got %+v", message.Event)
	}

	return message.Event.Flags, nil
}

func checkMessageRoomMessage(message *ServerMessage) (*RoomEventMessage, error) {
	if err := checkMessageType(message, "event"); err != nil {
		return nil, err
	} else if message.Event.Target != "room" {
		return nil, fmt.Errorf("Expected event target room, got %+v", message.Event)
	} else if message.Event.Type != "message" || message.Event.Message == nil {
		return nil, fmt.Errorf("Expected event type message, got %+v", message.Event)
	}

	return message.Event.Message, nil
}

func (c *TestClient) RunUntilRoomMessage(ctx context.Context) (*RoomEventMessage, error) {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return nil, err
	}

	return checkMessageRoomMessage(message)
}

func checkMessageError(message *ServerMessage, msgid string) error {
	if err := checkMessageType(message, "error"); err != nil {
		return err
	} else if message.Error.Code != msgid {
		return fmt.Errorf("Expected error \"%s\", got \"%s\" (%+v)", msgid, message.Error.Code, message.Error)
	}

	return nil
}

func (c *TestClient) RunUntilOffer(ctx context.Context, offer string) error {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return err
	}
	if err := checkUnexpectedClose(err); err != nil {
		return err
	} else if err := checkMessageType(message, "message"); err != nil {
		return err
	}

	var data StringMap
	if err := json.Unmarshal(message.Message.Data, &data); err != nil {
		return err
	}

	if dt, ok := GetStringMapEntry[string](data, "type"); !ok || dt != "offer" {
		return fmt.Errorf("expected data type offer, got %+v", data)
	}

	payload, ok := ConvertStringMap(data["payload"])
	if !ok {
		return fmt.Errorf("expected string map, got %+v", data["payload"])
	}
	if pt, ok := GetStringMapEntry[string](payload, "type"); !ok || pt != "offer" {
		return fmt.Errorf("expected payload type offer, got %+v", payload)
	}
	if sdp, ok := GetStringMapEntry[string](payload, "sdp"); !ok || sdp != offer {
		return fmt.Errorf("expected payload offer %s, got %+v", offer, payload)
	}

	return nil
}

func (c *TestClient) RunUntilAnswer(ctx context.Context, answer string) error {
	return c.RunUntilAnswerFromSender(ctx, answer, nil)
}

func (c *TestClient) RunUntilAnswerFromSender(ctx context.Context, answer string, sender *MessageServerMessageSender) error {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return err
	}
	if err := checkUnexpectedClose(err); err != nil {
		return err
	} else if err := checkMessageType(message, "message"); err != nil {
		return err
	}

	if sender != nil {
		if err := checkMessageSender(c.hub, message.Message.Sender, sender.Type, &HelloServerMessage{
			SessionId: sender.SessionId,
			UserId:    sender.UserId,
		}); err != nil {
			return err
		}
	}

	var data StringMap
	if err := json.Unmarshal(message.Message.Data, &data); err != nil {
		return err
	}

	if dt, ok := GetStringMapEntry[string](data, "type"); !ok || dt != "answer" {
		return fmt.Errorf("expected data type answer, got %+v", data)
	}

	payload, ok := ConvertStringMap(data["payload"])
	if !ok {
		return fmt.Errorf("expected string map, got %+v", payload)
	}
	if pt, ok := GetStringMapEntry[string](payload, "type"); !ok || pt != "answer" {
		return fmt.Errorf("expected payload type answer, got %+v", payload)
	}
	if sdp, ok := GetStringMapEntry[string](payload, "sdp"); !ok || sdp != answer {
		return fmt.Errorf("expected payload answer %s, got %+v", answer, payload)
	}

	return nil
}

func checkMessageTransientSet(t *testing.T, message *ServerMessage, key string, value any, oldValue any) error {
	if err := checkMessageType(message, "transient"); err != nil {
		return err
	}

	assert := assert.New(t)
	assert.Equal("set", message.TransientData.Type, "invalid message type")
	assert.Equal(key, message.TransientData.Key, "invalid key")
	assert.EqualValues(value, message.TransientData.Value, "invalid value")
	assert.EqualValues(oldValue, message.TransientData.OldValue, "invalid old value")
	return nil
}

func checkMessageTransientRemove(t *testing.T, message *ServerMessage, key string, oldValue any) error {
	if err := checkMessageType(message, "transient"); err != nil {
		return err
	}

	assert := assert.New(t)
	assert.Equal("remove", message.TransientData.Type, "invalid message type")
	assert.Equal(key, message.TransientData.Key, "invalid key")
	assert.EqualValues(oldValue, message.TransientData.OldValue, "invalid old value")
	return nil
}

func checkMessageTransientInitial(t *testing.T, message *ServerMessage, data StringMap) error {
	if err := checkMessageType(message, "transient"); err != nil {
		return err
	}

	assert := assert.New(t)
	assert.Equal("initial", message.TransientData.Type, "invalid message type")
	assert.EqualValues(data, message.TransientData.Data, "invalid initial data")
	return nil
}

func checkMessageInCallAll(message *ServerMessage, roomId string, inCall int) error {
	if err := checkMessageType(message, "event"); err != nil {
		return err
	} else if message.Event.Type != "update" {
		return fmt.Errorf("Expected update event, got %+v", message.Event)
	} else if message.Event.Target != "participants" {
		return fmt.Errorf("Expected participants update event, got %+v", message.Event)
	} else if message.Event.Update.RoomId != roomId {
		return fmt.Errorf("Expected participants update event for room %s, got %+v", roomId, message.Event.Update)
	} else if !message.Event.Update.All {
		return fmt.Errorf("Expected participants update event for all, got %+v", message.Event.Update)
	} else if !bytes.Equal(message.Event.Update.InCall, []byte(strconv.FormatInt(int64(inCall), 10))) {
		return fmt.Errorf("Expected incall flags %d, got %+v", inCall, message.Event.Update)
	}
	return nil
}

func checkMessageSwitchTo(message *ServerMessage, roomId string, details json.RawMessage) (*EventServerMessageSwitchTo, error) {
	if err := checkMessageType(message, "event"); err != nil {
		return nil, err
	} else if message.Event.Type != "switchto" {
		return nil, fmt.Errorf("Expected switchto event, got %+v", message.Event)
	} else if message.Event.Target != "room" {
		return nil, fmt.Errorf("Expected room switchto event, got %+v", message.Event)
	} else if message.Event.SwitchTo.RoomId != roomId {
		return nil, fmt.Errorf("Expected room switchto event for room %s, got %+v", roomId, message.Event)
	}
	if details != nil {
		if message.Event.SwitchTo.Details == nil || !bytes.Equal(details, message.Event.SwitchTo.Details) {
			return nil, fmt.Errorf("Expected details %s, got %+v", string(details), message.Event)
		}
	} else if message.Event.SwitchTo.Details != nil {
		return nil, fmt.Errorf("Expected no details, got %+v", message.Event)
	}
	return message.Event.SwitchTo, nil
}

func (c *TestClient) RunUntilSwitchTo(ctx context.Context, roomId string, details json.RawMessage) (*EventServerMessageSwitchTo, error) {
	message, err := c.RunUntilMessage(ctx)
	if err != nil {
		return nil, err
	}

	return checkMessageSwitchTo(message, roomId, details)
}
