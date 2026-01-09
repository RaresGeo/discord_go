package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) getChannel(channelId string) (Channel, error) {
	requestURL := DiscordAPI + "/channels/" + channelId

	req, err := http.NewRequest("GET", requestURL, nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bot %s", c.token))
	if err != nil {
		return Channel{}, fmt.Errorf("client: could not create request: %s\n", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return Channel{}, fmt.Errorf("client: error making http request: %s\n", err)
	}

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return Channel{}, fmt.Errorf("client: could not read response body: %s\n", err)
	}

	var channel Channel
	err = json.Unmarshal(resBody, &channel)
	if err != nil {
		return Channel{}, fmt.Errorf("client: could not unmarshal response body: %s\n", err)
	}

	return channel, nil
}
