package client

import (
	"context"
	"net/http"
	"time"

	"github.com/n0madic/go-vot/pkg/secure"
	"github.com/n0madic/go-vot/pkg/yaproto"
)

// getSession returns a valid cached session for the module or creates a new one.
func (c *Client) getSession(ctx context.Context, module string) (secure.Session, error) {
	now := time.Now().Unix()

	c.mu.Lock()
	s, ok := c.sessions[module]
	c.mu.Unlock()
	if ok && s.Valid(now) {
		return s, nil
	}

	s, err := c.createSession(ctx, module)
	if err != nil {
		return secure.Session{}, err
	}
	s.Timestamp = now

	c.mu.Lock()
	c.sessions[module] = s
	c.mu.Unlock()
	return s, nil
}

// createSession performs POST /session/create and returns the new session.
func (c *Client) createSession(ctx context.Context, module string) (secure.Session, error) {
	uuid := secure.UUID()
	body := (&yaproto.YandexSessionRequest{UUID: uuid, Module: module}).Marshal()

	data, ok, err := c.request(ctx, paths.session, body, map[string]string{
		"Vtrans-Signature": secure.Signature(body),
	}, http.MethodPost)
	if err != nil {
		return secure.Session{}, err
	}
	if !ok {
		return secure.Session{}, &VOTError{Msg: "failed to request create session", Data: string(data)}
	}

	var resp yaproto.YandexSessionResponse
	if err := resp.Unmarshal(data); err != nil {
		return secure.Session{}, err
	}
	return secure.Session{SecretKey: resp.SecretKey, Expires: resp.Expires, UUID: uuid}, nil
}
