package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"personal/discord_go/src/opcodes"
	"time"

	"github.com/gorilla/websocket"
)

const DISCORD_API = "https://discordapp.com/api/v6"

func NewBot() *Client {
	// make http request to DISCORD_API/gateway
	requestURL := DISCORD_API + "/gateway"

	req, err := http.NewRequest("GET", requestURL, nil)
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

	resBody, err := ioutil.ReadAll(res.Body)
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

	return &Client{gateway: response.Url, sequence: -1}
}

func (c *Client) ConnectToGateway(token string) {
	if c.gateway == "" {
		fmt.Println("Gateway not set")
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

	c.Identify(token)
}

func (c *Client) Identify(token string) {
	if c.connection == nil {
		fmt.Println("Identify: connection is not open")
		return
	}

	identifyMessage := IdentifyMessage{
		Op: opcodes.Identify,
		D: IdentifyData{
			Token: token,
			Properties: struct {
				Os      string `json:"$os"`
				Browser string `json:"$browser"`
				Device  string `json:"$device"`
			}{
				Os:      "linux",
				Browser: "discord_go",
				Device:  "discord_go",
			},
			Shard: []int{0, 1},
		},
	}

	identifyMessageBody, err := json.Marshal(identifyMessage)

	if err != nil {
		fmt.Printf("Identify: could not marshal identify message: %s\n", err)
		return
	}

	err = c.connection.WriteMessage(websocket.TextMessage, identifyMessageBody)

	if err != nil {
		fmt.Printf("Identify: could not send identify message: %s\n", err)
		return
	}

	fmt.Println("Identify: sent identify message")

	c.StartListening()
}

func (c *Client) SendHeartbeat() {
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

	heartbeatMessageBody, err := json.Marshal(heartbeatMessage)

	if err != nil {
		fmt.Printf("SendHeartbeat: could not marshal heartbeat message: %s\n", err)
		return
	}

	err = c.connection.WriteMessage(websocket.TextMessage, heartbeatMessageBody)

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
		c.SendHeartbeat()
		c.setHeartbeatInterval(timeToWait)
	})
}

func (c *Client) StartListening() {
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

	for {
		select {
		case messageBody := <-c.messageChannel:
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

				c.sessionId = ReadyData.SessionId
				c.resumeUrl = ReadyData.ResumeUrl
				c.lastHeartbeatAcked = true
				c.SendHeartbeat()
			default:
				// TODO: resumed event
			}

			switch message.Op {
			case opcodes.HeartbeatACK:
				c.AcknowledgeHeartbeat()
			case opcodes.Heartbeat:
				c.SendHeartbeat()
			default:
				fmt.Printf("StartListening: received unknown message type: %d\n", message.Op)
			}

			if message.S > c.sequence {
				c.sequence = message.S
			}
		}
	}
}

func (c *Client) AcknowledgeHeartbeat() {
	fmt.Println("AcknowledgeHeartbeat: received heartbeat ACK")
	c.lastHeartbeatAcked = true
	c.lastHeartbeatTimestamp = time.Now().UnixNano() / int64(time.Millisecond)
}

func (c *Client) GetGateway() string {
	return c.gateway
}
