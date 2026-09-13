package store

import "context"

type Message struct{ ID, Body string }
type Store interface {
	PutMessage(context.Context, Message) error
	HasMessage(context.Context, string) (bool, error)
	Ack(context.Context, string) error
	AddPair(context.Context, PairCode) error
	ConsumePair(context.Context, string) (PairCode, bool)
	AddDevice(Device) error
	VerifyToken(string) bool
	RevokeDevice(string) bool
}
