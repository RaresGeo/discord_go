// Package client: the actual websocket client to the Discord gateway
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"personal/discord_go/src/opcodes"

	"github.com/gorilla/websocket"
)

type Client struct {
	token  string
	prefix string

	gateway       string
	sessionId     string
	resumeGateway string

	heartbeatTimer         *time.Timer
	heartbeatInterval      int
	lastHeartbeatAcked     bool
	lastHeartbeatTimestamp int64
	sequence               int64
	isResuming             bool

	connection     *websocket.Conn
	messageChannel chan []byte
}

func NewBot(token string, prefix string) *Client {
	// make http request to DISCORD_API/gateway
	requestURL := DiscordAPI + "/gateway/bot"

	req, err := http.NewRequest("GET", requestURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bot %s", token))
	if err != nil {
		fmt.Printf("client: could not create request: %s\n", err)
		os.Exit(1)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("client: error making http request: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("client: status code: %d\n", res.StatusCode)

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("client: could not read response body: %s\n", err)
		os.Exit(1)
	}

	// get .url from resBody
	var response GatewayResponse
	err = json.Unmarshal(resBody, &response)
	if err != nil {
		fmt.Printf("client: could not unmarshal response body: %s\n", err)
		os.Exit(1)
	}

	return &Client{token: token, gateway: response.Url, sequence: -1}
}

func (c *Client) ConnectToGateway() {
	if c.gateway == "" {
		fmt.Println("ConnectToGateway: Gateway not set")
		return
	}

	conn, _, err := websocket.DefaultDialer.Dial(c.gateway, http.Header{})
	if err != nil {
		fmt.Printf("ConnectToGateway: could not connect to WebSocket: %s\n", err)
		return
	}

	c.connection = conn

	// First message should be hello
	_, messageBody, err := c.connection.ReadMessage()
	if err != nil {
		fmt.Printf("ConnectToGateway: could not receive message from WebSocket: %s\n", err)
		return
	}

	// Unmarshal message body
	var message HelloMessage
	err = json.Unmarshal(messageBody, &message)
	if err != nil {
		fmt.Printf("ConnectToGateway: could not unmarshal message body: %s\n", err)
		return
	}

	if message.Op != opcodes.Hello {
		fmt.Printf("ConnectToGateway: invalid handshake, expected Hello, got %d\n", message.Op)
		return
	}

	c.heartbeatInterval = message.D.HeartbeatInterval
	c.setHeartbeatInterval(c.heartbeatInterval)
	fmt.Printf("ConnectToGateway: Successfully made handshake; heartbeat interval: %d\n", c.heartbeatInterval)

	c.identify()
}

func (c *Client) identify() {
	if c.connection == nil {
		fmt.Println("Identify: connection is not open")
		return
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
		fmt.Printf("Identify: could not marshal identify message: %s\n", err)
		return
	}

	err = c.connection.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		fmt.Printf("Identify: could not send identify message: %s\n", err)
		return
	}

	fmt.Println("Identify: sent identify message")

	c.startListening()
}

func (c *Client) sendHeartbeat() {
	if !c.lastHeartbeatAcked {
		// TODO: handle this

		fmt.Println("SendHeartbeat: Last heartbeat was not acknowledged, reconnecting")
		return
	}

	c.lastHeartbeatAcked = false
	c.lastHeartbeatTimestamp = time.Now().UnixNano() / int64(time.Millisecond)

	heartbeatMessage := HeartbeatMessage{
		Op: opcodes.Heartbeat,
		D:  c.sequence,
	}

	payload, err := json.Marshal(heartbeatMessage)
	if err != nil {
		fmt.Printf("SendHeartbeat: could not marshal heartbeat message: %s\n", err)
		return
	}

	err = c.connection.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		fmt.Printf("SendHeartbeat: could not send heartbeat message: %s\n", err)
		return
	}

	fmt.Println("SendHeartbeat: sent heartbeat message")
}

func (c *Client) setHeartbeatInterval(timeToWait int) {
	if timeToWait == -1 {
		// Stop timer
		c.heartbeatTimer.Stop()
		return
	}

	fmt.Printf("setHeartbeatInterval: setting heartbeat timer to %d milliseconds \n", timeToWait)
	c.heartbeatTimer = time.AfterFunc(time.Duration(timeToWait)*time.Millisecond, func() {
		c.sendHeartbeat()
		c.setHeartbeatInterval(timeToWait)
	})
}

func (c *Client) startListening() {
	if c.connection == nil {
		fmt.Println("StartListening: connection is not open")
		return
	}

	fmt.Println("StartListening: started listening for messages")

	c.messageChannel = make(chan []byte)

	go func() {
		for {
			_, messageBody, err := c.connection.ReadMessage()
			if err != nil {
				fmt.Printf("StartListening: could not receive message from WebSocket: %s\n", err)
				close(c.messageChannel)
				return
			}

			c.messageChannel <- messageBody
		}
	}()

	for messageBody := range c.messageChannel {
		var message Packet

		err := json.Unmarshal(messageBody, &message)
		if err != nil {
			fmt.Printf("StartListening: could not unmarshal message body: %s\n", err)
			fmt.Println(string(messageBody))
			return
		}

		switch message.T {
		case "READY":
			fmt.Println("StartListening: received READY event")
			var data ReadyData
			err := json.Unmarshal(message.D, &data)
			if err != nil {
				fmt.Printf("StartListening: could not unmarshal READY event data: %s\n", err)
				return
			}

			c.sessionId = data.SessionId
			c.resumeGateway = data.ResumeUrl
			c.lastHeartbeatAcked = true
			c.sendHeartbeat()
		case "RESUMED":
			// finished resuming, all subsequent events are new events.
			c.isResuming = false
		default:
			fmt.Printf("Received uknown message type %s", message.T)
		}

		switch message.Op {
		case opcodes.HeartbeatACK:
			c.acknowledgeHeartbeat()
		case opcodes.Heartbeat:
			c.sendHeartbeat()
		case opcodes.Reconnect:
			c.resumeOldConnection()
		case opcodes.Dispatch:
			c.sequence = message.S
			c.onEvent(message.D)
		case opcodes.InvalidSession:
			// failed to connect to gateway for some reason
			c.handleInvalidSession(message.D)

		default:
			fmt.Printf("StartListening: received unknown message type: %d\n", message.Op)
		}

		if message.S > c.sequence {
			c.sequence = message.S
		}

	}
}

func (c *Client) acknowledgeHeartbeat() {
	fmt.Println("AcknowledgeHeartbeat: received heartbeat ACK")
	c.lastHeartbeatAcked = true
	c.lastHeartbeatTimestamp = time.Now().UnixNano() / int64(time.Millisecond)
}

func (c *Client) GetGateway() string {
	return c.gateway
}

func (c *Client) resumeOldConnection() {
	if c.resumeGateway == "" {
		fmt.Println("resumeOldConnection: Resume gateway not set")
		return
	}

	c.isResuming = true
	conn, _, err := websocket.DefaultDialer.Dial(c.resumeGateway, http.Header{})
	if err != nil {
		fmt.Printf("resumeOldConnection: could not connect to WebSocket: %s\n", err)
		return
	}

	c.connection = conn

	// Have to send gateway resume event
	resumeMessage := ResumeMessage{
		Op: opcodes.Resume,
		D: ResumeData{
			Token:     c.token,
			SessionID: c.sessionId,
			Sequence:  c.sequence,
		},
	}

	payload, err := json.Marshal(resumeMessage)
	if err != nil {
		fmt.Printf("resumeOldConnection: could not marshal heartbeat message: %s\n", err)
		return
	}

	err = c.connection.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		fmt.Printf("resumeOldConnection: could not send resume message: %s\n", err)
		return
	}

	fmt.Println("resumeOldConnection: sent resume message")
}

func (c *Client) onEvent(data json.RawMessage) {
}

func (c *Client) handleInvalidSession(data json.RawMessage) {
	var canResume bool
	err := json.Unmarshal(data, &canResume)
	if err != nil {
		fmt.Printf("client: could not unmarshal response body: %s\n", err)
		os.Exit(1)
	}

	if canResume {
		c.resumeOldConnection()
	} else {
		newClient := NewBot(c.token)
		newClient.ConnectToGateway()

		c = newClient
	}
}
