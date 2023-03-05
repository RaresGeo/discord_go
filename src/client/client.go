package client

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

const DISCORD_API = "https://discordapp.com/api/v6"

type Client struct {
	gateway           string
	heartbeatInterval int
}

type GatewayResponse struct {
	Url string `json:"url"`
}

func newBot(token string) *Client {
	// make http request to DISCORD_API/gateway
	requestURL := DISCORD_API + "/gateway"
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		fmt.Printf("client: could not create request: %s\n", err)
		os.Exit(1)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("client: error making http request: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("client: got response!\n")
	fmt.Printf("client: status code: %d\n", res.StatusCode)

	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("client: could not read response body: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("client: response body: %s\n", resBody)

	// get .url from resBody
	var response GatewayResponse
	err = json.Unmarshal(resBody, &response)
	if err != nil {
		fmt.Printf("client: could not unmarshal response body: %s\n", err)
		os.Exit(1)
	}

	return &Client{gateway: response.Url}
}

func (c *Client) connectToGateway() {
	if c.gateway == "" {
		fmt.Println("Gateway not set")
		return
	}

	conn, _, err := websocket.DefaultDialer.Dial(c.gateway, http.Header{})
	if err != nil {
		fmt.Printf("client: could not connect to WebSocket: %s\n", err)
		return
	}

	// First message should be hello
	_, messageBody, err := conn.ReadMessage()
	if err != nil {
		fmt.Printf("client: could not receive message from WebSocket: %s\n", err)
		return
	}

	// Unmarshal message body
	var message map[string]interface{}
	err = json.Unmarshal(messageBody, &message)
	if err != nil {
		fmt.Printf("client: could not unmarshal message body: %s\n", err)
		return
	}

	fmt.Printf("client: received message from WebSocket: x %s\n", message)

}

func (c *Client) getGateway() string {
	return c.gateway
}
