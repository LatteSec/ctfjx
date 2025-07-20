package socket

// The header byte denoting the type of the message
type Action uint8

const (
	ActionInvalid Action = iota
	ActionAck            // Generic ACK
	ActionError          // Error message

	// Control & lifecycle
	ActionPing    // Keepalive & healthcheck
	ActionPong    // Response to ping
	ActionHello   // Initial handshake (agent info)
	ActionGoodbye // Disconnect notification

	// Config management
	ActionRequestConfig // Agent asking for config
	ActionPushConfig    // Daemon pushing config

	// File transfers
	ActionSendFile      // Agent uploads file (e.g., flag, log)
	ActionRequestLogs   // Daemon requests logs from agent
	ActionSendFileChunk // (Optional) Chunked file part

	// Status and logs
	ActionPushStatus    // Agent pushes status update
	ActionRequestStatus // Server requests current status
)
