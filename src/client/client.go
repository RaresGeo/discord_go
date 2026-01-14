package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"personal/discord_go/src/opcodes"

	"github.com/gorilla/websocket"
)

type Client struct {
	token      string
	httpClient *http.Client
	prefix     string

	mu            sync.RWMutex
	gateway       string
	sessionId     string
	resumeGateway string

	heartbeatInterval      atomic.Int64
	lastHeartbeatAcked     atomic.Bool
	lastHeartbeatTimestamp atomic.Int64
	sequence               atomic.Int64
	isResuming             atomic.Bool
	connection             atomic.Pointer[websocket.Conn]

	messageChannel chan []byte
}

func NewBot(token string, prefix string) (*Client, error) {
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	requestURL := DiscordAPI + "/gateway/bot"

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bot %s", token))

	res, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making http request: %w", err)
	}
	defer res.Body.Close()

	fmt.Printf("client: gateway response status code: %d\n", res.StatusCode)

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read response body: %w", err)
	}

	var response GatewayResponse
	err = json.Unmarshal(resBody, &response)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %w", err)
	}

	client := &Client{
		token:      token,
		prefix:     prefix,
		gateway:    response.Url,
		httpClient: httpClient,
	}

	client.sequence.Store(-1)
	client.lastHeartbeatAcked.Store(true)

	return client, nil
}

// ConnectToGateway establishes a WebSocket connection to Discord's gateway and
// starts the main event loop. It will block until the context is cancelled or
// an error occurs. Returns an error if the connection fails.
func (c *Client) ConnectToGateway(ctx context.Context) error {
	if c.gateway == "" {
		return fmt.Errorf("gateway URL not set")
	}

	conn, _, err := websocket.DefaultDialer.Dial(c.gateway, http.Header{})
	if err != nil {
		return fmt.Errorf("could not connect to WebSocket: %w", err)
	}

	c.connection.Store(conn)

	_, messageBody, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("could not receive hello message: %w", err)
	}

	var message HelloMessage
	err = json.Unmarshal(messageBody, &message)
	if err != nil {
		return fmt.Errorf("could not unmarshal hello message: %w", err)
	}

	if message.Op != opcodes.Hello {
		return fmt.Errorf("invalid handshake: expected Hello (opcode %d), got %d", opcodes.Hello, message.Op)
	}

	interval := time.Duration(message.D.HeartbeatInterval) * time.Millisecond
	c.heartbeatInterval.Store(int64(interval))

	fmt.Printf("ConnectToGateway: Successfully made handshake; heartbeat interval: %v\n", c.heartbeatInterval)

	if err := c.identify(); err != nil {
		return fmt.Errorf("failed to identify: %w", err)
	}

	go c.startHeartbeat(ctx)

	return c.startListening(ctx)
}

func (c *Client) identify() error {
	conn := c.connection.Load()
	if conn == nil {
		return fmt.Errorf("connection is not open")
	}

	identifyMessage := IdentifyMessage{
		Op: opcodes.Identify,
		D: IdentifyData{
			Token: c.token,
			Properties: struct {
				Os      string `json:"$os"`
				Browser string `json:"$browser"`
				Device  string `json:"$device"`
			}{
				Os:      "linux",
				Browser: "discord_go",
				Device:  "discord_go",
			},
			Shard:   []int{0, 1},
			Intents: Intents,
		},
	}

	payload, err := json.Marshal(identifyMessage)
	if err != nil {
		return fmt.Errorf("could not marshal identify message: %w", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		return fmt.Errorf("could not send identify message: %w", err)
	}

	fmt.Println("Identify: sent identify message")
	return nil
}

func (c *Client) startHeartbeat(ctx context.Context) {
	interval := time.Duration(c.heartbeatInterval.Load())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	fmt.Printf("startHeartbeat: starting heartbeat with interval %v\n", interval)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("startHeartbeat: context cancelled, stopping heartbeat")
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(); err != nil {
				fmt.Printf("startHeartbeat: failed to send heartbeat: %v\n", err)
			}
		}
	}
}

func (c *Client) sendHeartbeat() error {
	if !c.lastHeartbeatAcked.Load() {
		return fmt.Errorf("last heartbeat was not acknowledged")
	}

	c.lastHeartbeatAcked.Store(false)
	c.lastHeartbeatTimestamp.Store(time.Now().UnixNano() / int64(time.Millisecond))

	seq := c.sequence.Load()
	conn := c.connection.Load()

	if conn == nil {
		return fmt.Errorf("connection is nil")
	}

	heartbeatMessage := HeartbeatMessage{
		Op: opcodes.Heartbeat,
		D:  seq,
	}

	payload, err := json.Marshal(heartbeatMessage)
	if err != nil {
		return fmt.Errorf("could not marshal heartbeat message: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return fmt.Errorf("could not send heartbeat message: %w", err)
	}

	fmt.Println("sendHeartbeat: sent heartbeat message")
	return nil
}

func (c *Client) startListening(ctx context.Context) error {
	conn := c.connection.Load()
	if conn == nil {
		return fmt.Errorf("connection is not open")
	}

	fmt.Println("startListening: started listening for messages")

	c.messageChannel = make(chan []byte, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(c.messageChannel)
		for {
			_, messageBody, err := conn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("could not receive message from WebSocket: %w", err)
				return
			}

			select {
			case c.messageChannel <- messageBody:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("startListening: context cancelled, closing connection")
			if conn := c.connection.Load(); conn != nil {
				conn.Close()
			}
			return ctx.Err()

		case err := <-errCh:
			return err

		case messageBody, ok := <-c.messageChannel:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			if err := c.handleMessage(messageBody); err != nil {
				fmt.Printf("startListening: error handling message: %v\n", err)
			}
		}
	}
}

func (c *Client) handleMessage(messageBody []byte) error {
	var message Packet
	if err := json.Unmarshal(messageBody, &message); err != nil {
		return fmt.Errorf("could not unmarshal message body: %w (body: %s)", err, string(messageBody))
	}

	switch message.T {
	case "READY":
		fmt.Println("handleMessage: received READY event")
		var data ReadyData
		if err := json.Unmarshal(message.D, &data); err != nil {
			return fmt.Errorf("could not unmarshal READY event data: %w", err)
		}

		c.mu.Lock()
		c.sessionId = data.SessionId
		c.resumeGateway = data.ResumeUrl
		c.mu.Unlock()

		c.lastHeartbeatAcked.Store(true)

		if err := c.sendHeartbeat(); err != nil {
			fmt.Printf("handleMessage: failed to send initial heartbeat: %v\n", err)
		}

	case "RESUMED":
		fmt.Println("handleMessage: received RESUMED event")
		c.isResuming.Store(false)

	case "":
	default:
		fmt.Printf("handleMessage: received event type: %s\n", message.T)
	}

	switch message.Op {
	case opcodes.HeartbeatACK:
		c.acknowledgeHeartbeat()

	case opcodes.Heartbeat:
		if err := c.sendHeartbeat(); err != nil {
			return fmt.Errorf("failed to send requested heartbeat: %w", err)
		}

	case opcodes.Reconnect:
		return fmt.Errorf("received reconnect opcode - TODO: implement reconnection")

	case opcodes.Dispatch:
		c.sequence.Store(message.S)
		c.onEvent(message.D)

	case opcodes.InvalidSession:
		return c.handleInvalidSession(message.D)

	default:
		fmt.Printf("handleMessage: received unknown opcode: %d\n", message.Op)
	}

	// Update sequence if message has one (lock-free compare-and-swap pattern)
	if message.S > 0 {
		for {
			oldSeq := c.sequence.Load()
			if message.S <= oldSeq {
				break
			}
			if c.sequence.CompareAndSwap(oldSeq, message.S) {
				break
			}
		}
	}

	return nil
}

func (c *Client) acknowledgeHeartbeat() {
	fmt.Println("acknowledgeHeartbeat: received heartbeat ACK")
	c.lastHeartbeatAcked.Store(true)
	c.lastHeartbeatTimestamp.Store(time.Now().UnixNano() / int64(time.Millisecond))
}

func (c *Client) onEvent(data json.RawMessage) {
}

func (c *Client) handleInvalidSession(data json.RawMessage) error {
	var canResume bool
	if err := json.Unmarshal(data, &canResume); err != nil {
		return fmt.Errorf("could not unmarshal invalid session data: %w", err)
	}

	if canResume {
		return fmt.Errorf("received invalid session (resumable)")
	}

	return fmt.Errorf("received invalid session (not resumable)")
}

func (c *Client) Disconnect() error {
	conn := c.connection.Load()
	if conn == nil {
		return nil
	}

	fmt.Println("Disconnect: closing WebSocket connection")

	err := conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)
	if err != nil {
		fmt.Printf("Disconnect: failed to send close message: %v\n", err)
	}

	if err := conn.Close(); err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}

	c.connection.Store(nil)

	fmt.Println("Disconnect: connection closed successfully")
	return nil
}
