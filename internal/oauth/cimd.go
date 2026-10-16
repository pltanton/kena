package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pltanton/kena/internal/store"
)

const cimdMaxBytes = 64 << 10

var errUnknownClient = errors.New("unknown client")

func (s *Server) fetchClientMetadata(ctx context.Context, clientID string) (store.OAuthClient, error) {
	c, err := s.fetchClientDoc(ctx, clientID)
	if err != nil {
		return store.OAuthClient{}, fmt.Errorf("%w: %w", errUnknownClient, err)
	}
	if err := s.Store.SaveClient(ctx, c); err != nil {
		return store.OAuthClient{}, err
	}
	return c, nil
}

func (s *Server) fetchClientDoc(ctx context.Context, clientID string) (store.OAuthClient, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, nil)
	if err != nil {
		return store.OAuthClient{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return store.OAuthClient{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return store.OAuthClient{}, fmt.Errorf("client metadata: status %d", resp.StatusCode)
	}
	var doc struct {
		ClientID     string   `json:"client_id"`
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, cimdMaxBytes)).Decode(&doc); err != nil {
		return store.OAuthClient{}, fmt.Errorf("client metadata: %w", err)
	}
	if doc.ClientID != clientID {
		return store.OAuthClient{}, fmt.Errorf("client metadata: client_id %q does not match URL", doc.ClientID)
	}
	if err := validateRedirectURIs(doc.RedirectURIs); err != nil {
		return store.OAuthClient{}, fmt.Errorf("client metadata: %w", err)
	}
	return store.OAuthClient{ClientID: clientID, RedirectURIs: doc.RedirectURIs, ClientName: doc.ClientName}, nil
}

func (s *Server) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 5 * time.Second}
}
