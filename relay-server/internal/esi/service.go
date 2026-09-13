package esi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// QueryService exposes public read-only ESI data. No account token is accepted.
type QueryService struct {
	Client  *Client
	BaseURL string
}
type SolarSystem struct {
	ID              int64   `json:"system_id"`
	Name            string  `json:"name"`
	SecurityStatus  float64 `json:"security_status"`
	ConstellationID int64   `json:"constellation_id"`
}
type ItemType struct {
	ID        int64  `json:"type_id"`
	Name      string `json:"name"`
	GroupID   int64  `json:"group_id"`
	Published bool   `json:"published"`
}

func (s QueryService) query(ctx context.Context, path string, target any) (bool, error) {
	base := s.BaseURL
	if base == "" {
		base = "https://esi.evetech.net/latest/"
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false, fmt.Errorf("invalid ESI base URL")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(path, "/")
	q := u.Query()
	q.Set("datasource", "tranquility")
	q.Set("language", "en")
	u.RawQuery = q.Encode()
	client := s.Client
	if client == nil {
		client = &Client{}
	}
	body, cached, err := client.Get(ctx, u.String())
	if err != nil {
		return false, err
	}
	if err = json.Unmarshal(body, target); err != nil {
		return false, fmt.Errorf("decode ESI response: %w", err)
	}
	return cached, nil
}
func (s QueryService) SolarSystem(ctx context.Context, id int64) (SolarSystem, bool, error) {
	var result SolarSystem
	if id <= 0 {
		return result, false, fmt.Errorf("system ID must be positive")
	}
	cached, err := s.query(ctx, "universe/systems/"+strconv.FormatInt(id, 10)+"/", &result)
	if err == nil && (result.ID != id || result.Name == "") {
		err = fmt.Errorf("invalid ESI system response")
	}
	return result, cached, err
}
func (s QueryService) ItemType(ctx context.Context, id int64) (ItemType, bool, error) {
	var result ItemType
	if id <= 0 {
		return result, false, fmt.Errorf("type ID must be positive")
	}
	cached, err := s.query(ctx, "universe/types/"+strconv.FormatInt(id, 10)+"/", &result)
	if err == nil && (result.ID != id || result.Name == "") {
		err = fmt.Errorf("invalid ESI type response")
	}
	return result, cached, err
}
