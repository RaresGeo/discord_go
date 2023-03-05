package opcodes

// 0	Dispatch	Receive	An event was dispatched.
// 1	Heartbeat	Send/Receive	Fired periodically by the client to keep the connection alive.
// 2	Identify	Send	Starts a new session during the initial handshake.
// 3	Presence Update	Send	Update the client's presence.
// 4	Voice State Update	Send	Used to join/leave or move between voice channels.
// 6	Resume	Send	Resume a previous session that was disconnected.
// 7	Reconnect	Receive	You should attempt to reconnect and resume immediately.
// 8	Request Guild Members	Send	Request information about offline guild members in a large guild.
// 9	Invalid Session	Receive	The session has been invalidated. You should reconnect and identify/resume accordingly.
// 10	Hello	Receive	Sent immediately after connecting, contains the heartbeat_interval to use.
// 11	Heartbeat ACK	Receive	Sent in response to receiving a heartbeat to acknowledge that it has been received.

const (
	Dispatch = iota
	Heartbeat
	Identify
	PresenceUpdate
	VoiceStateUpdate
	Resume
	Reconnect
	RequestGuildMembers
	InvalidSession
	Hello
	HeartbeatACK
)
