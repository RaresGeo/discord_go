// Package client: the actual websocket client to the Discord gateway
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"personal/discord_go/src/opcodes"

	"github.com/gorilla/websocket"
)

// Client represents a Discord gateway client with thread-safe operations
type Client struct {
	token  string
	prefix string

	// mu protects concurrent access to client state
	mu sync.RWMutex

	gateway       string
	sessionId     string
	resumeGateway string

	heartbeatInterval      time.Duration
	lastHeartbeatAcked     bool
	lastHeartbeatTimestamp int64
	sequence               int64
	isResuming             bool

	connection     *websocket.Conn
	messageChannel chan []byte
	httpClient     *http.Client
}

// NewBot creates a new Discord bot client with the given token and command prefix.
// It fetches the gateway URL from Discord's API and returns the initialized client.
// Returns an error if the gateway URL cannot be fetched.
func NewBot(token string, prefix string) (*Client, error) {
	// Create HTTP client with reasonable timeouts
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make HTTP request to get gateway URL
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

	// Parse gateway URL from response
	var response GatewayResponse
	err = json.Unmarshal(resBody, &response)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal response body: %w", err)
	}

	return &Client{
		token:              token,
		prefix:             prefix,
		gateway:            response.Url,
		sequence:           -1,
		httpClient:         httpClient,
		lastHeartbeatAcked: true,
	}, nil
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

	c.mu.Lock()
	c.connection = conn
	c.mu.Unlock()

	// First message should be Hello
	_, messageBody, err := c.connection.ReadMessage()
	if err != nil {
		return fmt.Errorf("could not receive hello message: %w", err)
	}

	// Unmarshal hello message
	var message HelloMessage
	err = json.Unmarshal(messageBody, &message)
	if err != nil {
		return fmt.Errorf("could not unmarshal hello message: %w", err)
	}

	if message.Op != opcodes.Hello {
		return fmt.Errorf("invalid handshake: expected Hello (opcode %d), got %d", opcodes.Hello, message.Op)
	}

	c.mu.Lock()
	c.heartbeatInterval = time.Duration(message.D.HeartbeatInterval) * time.Millisecond
	c.mu.Unlock()

	fmt.Printf("ConnectToGateway: Successfully made handshake; heartbeat interval: %v\n", c.heartbeatInterval)

	if err := c.identify(); err != nil {
		return fmt.Errorf("failed to identify: %w", err)
	}

	// Start heartbeat in background
	go c.startHeartbeat(ctx)

	// Start listening for messages (blocks until ctx is done or error)
	return c.startListening(ctx)
}

// identify sends the IDENTIFY payload to Discord to authenticate the bot
func (c *Client) identify() error {
	c.mu.RLock()
	conn := c.connection
	c.mu.RUnlock()

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

// startHeartbeat runs in a goroutine and sends periodic heartbeats to Discord.
// This is the idiomatic Go way using time.Ticker and channels instead of recursive timers.
func (c *Client) startHeartbeat(ctx context.Context) {
	c.mu.RLock()
	interval := c.heartbeatInterval
	c.mu.RUnlock()

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
				// Continue trying - Discord will reconnect us if needed
			}
		}
	}
}

// sendHeartbeat sends a heartbeat message to Discord
func (c *Client) sendHeartbeat() error {
	c.mu.Lock()
	if !c.lastHeartbeatAcked {
		c.mu.Unlock()
		// TODO: Implement reconnection logic when heartbeat isn't acknowledged
		return fmt.Errorf("last heartbeat was not acknowledged")
	}

	c.lastHeartbeatAcked = false
	c.lastHeartbeatTimestamp = time.Now().UnixNano() / int64(time.Millisecond)
	seq := c.sequence
	conn := c.connection
	c.mu.Unlock()

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

// startListening reads messages from the WebSocket and processes them.
// It runs in the main goroutine and blocks until context is cancelled or an error occurs.
// Uses a buffered channel to prevent blocking the reader goroutine.
func (c *Client) startListening(ctx context.Context) error {
	c.mu.RLock()
	conn := c.connection
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection is not open")
	}

	fmt.Println("startListening: started listening for messages")

	// Buffered channel to prevent reader goroutine from blocking
	c.messageChannel = make(chan []byte, 10)
	errCh := make(chan error, 1)

	// Reader goroutine - reads from WebSocket and sends to channel
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

	// Process messages from channel
	for {
		select {
		case <-ctx.Done():
			fmt.Println("startListening: context cancelled, closing connection")
			conn.Close()
			return ctx.Err()

		case err := <-errCh:
			return err

		case messageBody, ok := <-c.messageChannel:
			if !ok {
				return fmt.Errorf("message channel closed")
			}

			if err := c.handleMessage(messageBody); err != nil {
				fmt.Printf("startListening: error handling message: %v\n", err)
				// Continue processing other messages
			}
		}
	}
}

// handleMessage processes a single message from Discord
func (c *Client) handleMessage(messageBody []byte) error {
	var message Packet
	if err := json.Unmarshal(messageBody, &message); err != nil {
		return fmt.Errorf("could not unmarshal message body: %w (body: %s)", err, string(messageBody))
	}

	// Handle event types
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
		c.lastHeartbeatAcked = true
		c.mu.Unlock()

		// Send initial heartbeat after READY
		if err := c.sendHeartbeat(); err != nil {
			fmt.Printf("handleMessage: failed to send initial heartbeat: %v\n", err)
		}

	case "RESUMED":
		fmt.Println("handleMessage: received RESUMED event")
		c.mu.Lock()
		c.isResuming = false
		c.mu.Unlock()

	case "":
		// No event type - this is normal for some opcodes like HeartbeatACK
	default:
		fmt.Printf("handleMessage: received event type: %s\n", message.T)
	}

	// Handle opcodes
	switch message.Op {
	case opcodes.HeartbeatACK:
		c.acknowledgeHeartbeat()

	case opcodes.Heartbeat:
		// Discord is requesting an immediate heartbeat
		if err := c.sendHeartbeat(); err != nil {
			return fmt.Errorf("failed to send requested heartbeat: %w", err)
		}

	case opcodes.Reconnect:
		return fmt.Errorf("received reconnect opcode - TODO: implement reconnection")

	case opcodes.Dispatch:
		c.mu.Lock()
		c.sequence = message.S
		c.mu.Unlock()
		c.onEvent(message.D)

	case opcodes.InvalidSession:
		return c.handleInvalidSession(message.D)

	default:
		fmt.Printf("handleMessage: received unknown opcode: %d\n", message.Op)
	}

	// Update sequence if message has one
	if message.S > 0 {
		c.mu.Lock()
		if message.S > c.sequence {
			c.sequence = message.S
		}
		c.mu.Unlock()
	}

	return nil
}

// acknowledgeHeartbeat marks the last heartbeat as acknowledged
func (c *Client) acknowledgeHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Println("acknowledgeHeartbeat: received heartbeat ACK")
	c.lastHeartbeatAcked = true
	c.lastHeartbeatTimestamp = time.Now().UnixNano() / int64(time.Millisecond)
}

// onEvent handles Discord dispatch events
// Override this method to implement custom event handling
func (c *Client) onEvent(data json.RawMessage) {
	// TODO: Implement event handlers for MESSAGE_CREATE, etc.
}

// handleInvalidSession handles the InvalidSession opcode from Discord
func (c *Client) handleInvalidSession(data json.RawMessage) error {
	var canResume bool
	if err := json.Unmarshal(data, &canResume); err != nil {
		return fmt.Errorf("could not unmarshal invalid session data: %w", err)
	}

	if canResume {
		return fmt.Errorf("received invalid session (resumable) - TODO: implement resume logic")
	}

	return fmt.Errorf("received invalid session (not resumable) - need to create new connection")
}
