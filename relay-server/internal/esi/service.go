package esi

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// Gateway exposes typed public and authenticated ESI operations.
type Gateway struct{ Client *Client }

func NewGateway(client *Client) (*Gateway, error) {
	if client == nil {
		return nil, errors.New("ESI client is required")
	}
	if strings.TrimSpace(client.UserAgent) == "" {
		return nil, errors.New("ESI User-Agent is required")
	}
	if _, err := client.baseURL(); err != nil {
		return nil, err
	}
	return &Gateway{Client: client}, nil
}

type UniverseName struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

func (g *Gateway) UniverseNames(ctx context.Context, ids []int64) ([]UniverseName, Response, error) {
	if g == nil || g.Client == nil {
		return nil, Response{}, errors.New("ESI client is required")
	}
	if len(ids) == 0 || len(ids) > 1000 {
		return nil, Response{}, errors.New("IDs must contain 1 to 1000 entries")
	}
	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, Response{}, errors.New("ID must be positive")
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			normalized = append(normalized, id)
		}
	}
	var out []UniverseName
	meta, err := g.Client.postJSON(ctx, "universe/names/", nil, normalized, &out)
	return out, meta, err
}

func (g *Gateway) get(ctx context.Context, path string, query url.Values, token string, target any) (Response, error) {
	if g == nil || g.Client == nil {
		return Response{}, errors.New("ESI client is required")
	}
	return g.Client.getJSON(ctx, path, query, token, target)
}

func authenticated(characterID int64, token string) error {
	if characterID <= 0 {
		return errors.New("character ID must be positive")
	}
	if strings.TrimSpace(token) == "" {
		return errors.New("access token must not be empty")
	}
	return nil
}

func positive(name string, id int64) error {
	if id <= 0 {
		return errors.New(name + " must be positive")
	}
	return nil
}

func idPath(prefix string, id int64, suffix string) string {
	return prefix + strconv.FormatInt(id, 10) + suffix
}

func getAllPages[T any](ctx context.Context, g *Gateway, path string, query url.Values, token string) ([]T, Response, error) {
	if query == nil {
		query = make(url.Values)
	} else {
		query = cloneValues(query)
	}
	query.Set("page", "1")
	var all []T
	var page []T
	meta, err := g.get(ctx, path, query, token, &page)
	if err != nil {
		return nil, meta, err
	}
	all = append(all, page...)
	pages := meta.Pages
	for p := 2; p <= pages; p++ {
		query.Set("page", strconv.Itoa(p))
		page = nil
		current, err := g.get(ctx, path, query, token, &page)
		if err != nil {
			return nil, current, err
		}
		all = append(all, page...)
		if current.Pages > pages {
			pages = current.Pages
		}
	}
	meta.Pages = pages
	return all, meta, nil
}

func cloneValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for key, values := range in {
		out[key] = append([]string(nil), values...)
	}
	return out
}
