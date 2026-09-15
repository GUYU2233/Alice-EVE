package store

import (
	"context"
	"time"
)

// Message is a durable item in the server backlog. Cursor is monotonically
// increasing within a store and is used for incremental synchronization.
type Message struct {
	ID             string
	AccountID      string
	Body           string
	OwnerDeviceID  string
	Cursor         int64
	AcknowledgedAt *time.Time
}

type Store interface {
	PutMessage(context.Context, Message) error
	// HasMessage is tenant-scoped; message IDs are only idempotent within an account.
	HasMessage(context.Context, string, string) (bool, error)
	// Ack is retained solely for old internal callers and must not be used by HTTP APIs.
	Ack(context.Context, string) error
	AckMessage(context.Context, string, string, string) (bool, error)
	ListMessages(context.Context, string, string, int64, int) ([]Message, error)
	DeviceIDForToken(string) (string, bool)
	DeviceType(string) (string, bool)
	DeviceAccountID(string) (string, bool)
	DeviceExists(string) bool
	AddPair(context.Context, PairCode) error
	ConsumePair(context.Context, string) (PairCode, bool)
	AddDevice(Device) error
	VerifyToken(string) bool
	VerifyTokenType(string, string) bool
	OwnsToken(string, string) bool
	RevokeDevice(string) bool
	RevokeDeviceForAccount(string, string) bool
	ListDevices(string) []Device
	GetDevice(string) (Device, bool)
	GetDeviceForAccount(string, string) (Device, bool)
}
