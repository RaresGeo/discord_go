package client

import (
	"encoding/json"
)

type Snowflake string

type GatewayResponse struct {
	Url string `json:"url"`
}

type HelloMessage struct {
	Op int       `json:"op"`
	D  HelloData `json:"d"`
}

type HelloData struct {
	HeartbeatInterval int `json:"heartbeat_interval"`
}

type IdentifyMessage struct {
	Op int          `json:"op"`
	D  IdentifyData `json:"d"`
}

type IdentifyData struct {
	Token      string `json:"token"`
	Properties struct {
		Os      string `json:"$os"`
		Browser string `json:"$browser"`
		Device  string `json:"$device"`
	} `json:"properties"`
	Compress       bool        `json:"compress"`
	LargeThreshold int         `json:"large_threshold"`
	Shard          []int       `json:"shard"`
	Presence       interface{} `json:"presence"`
	Intents        int         `json:"intents"`
}

type ReadyData struct {
	SessionId string `json:"session_id"`
	ResumeUrl string `json:"resume_gateway_url"`
}

type HeartbeatMessage struct {
	Op int   `json:"op"`
	D  int64 `json:"d"`
}

type ResumeMessage struct {
	Op int        `json:"op"`
	D  ResumeData `json:"d"`
}

type ResumeData struct {
	Token     string `json:"token"`
	SessionID string `json:"session_id"`
	Sequence  int64  `json:"seq"`
}

type Packet struct {
	Op int             `json:"op"`
	T  string          `json:"t"`
	D  json.RawMessage `json:"d"`
	S  int64           `json:"s"`
}
