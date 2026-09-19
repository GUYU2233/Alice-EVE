package esidata

import (
	"context"
	"errors"
)

var (
	ErrNotFound     = errors.New("esi data not found")
	ErrInvalidInput = errors.New("invalid esi data input")
)

// Repository stores character-scoped normalized snapshots and public cache
// entries. Character methods always require account identity to prevent
// cross-account reads even when characters happen to share an identifier.
type Repository interface {
	UpsertCharacterSnapshot(context.Context, CharacterSnapshot) error
	GetCharacterSnapshot(context.Context, string, int64, string) (CharacterSnapshot, error)
	ListAccountCharacters(context.Context, string) ([]CharacterSnapshot, error)
	ListCharacterDomains(context.Context, string, int64) ([]CharacterSnapshot, error)
	UpsertPublicData(context.Context, PublicData) error
	GetPublicData(context.Context, string, string) (PublicData, error)
}
