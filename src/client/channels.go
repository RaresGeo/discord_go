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
	if err != nil {
		return Channel{}, fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bot %s", c.token))

	res, err := c.httpClient.Do(req)
	if err != nil {
		return Channel{}, fmt.Errorf("error making http request: %w", err)
	}
	defer res.Body.Close()

	resBody, err := io.ReadAll(res.Body)
	if err != nil {
		return Channel{}, fmt.Errorf("could not read response body: %w", err)
	}

	var channel Channel
	if err := json.Unmarshal(resBody, &channel); err != nil {
		return Channel{}, fmt.Errorf("could not unmarshal response body: %w", err)
	}

	return channel, nil
}
