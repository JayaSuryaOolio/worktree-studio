// Package httpapi implements app.Workspace and app.AttentionFeed against
// the running worktree-studio server (REST + /ws/attention) — the same
// endpoints web/src/api.ts calls.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"worktree-studio/tui/domain"
)

const (
	defaultAddr    = ":8787"
	reconnectDelay = 3 * time.Second
)

type Client struct {
	base string // http://host:port
	http *http.Client
}

// FromEnv honours WORKTREE_STUDIO_ADDR like the main binary's CLI subcommands do.
func FromEnv() *Client {
	addr := defaultAddr
	if v := os.Getenv("WORKTREE_STUDIO_ADDR"); v != "" {
		addr = v
	}
	host := "localhost"
	if i := strings.LastIndex(addr, ":"); i > 0 {
		host, addr = addr[:i], addr[i:]
	} else if i == -1 {
		addr = ":" + addr
	}
	return &Client{base: "http://" + host + addr, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		body, _ = json.Marshal(in)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("is worktree-studio running at %s? %w", c.base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Repos(ctx context.Context) ([]domain.Repo, error) {
	var raw []struct{ ID, Name, Path string }
	if err := c.do(ctx, "GET", "/api/repos/", nil, &raw); err != nil {
		return nil, err
	}
	repos := make([]domain.Repo, len(raw))
	for i, r := range raw {
		repos[i] = domain.Repo{ID: domain.RepoID(r.ID), Name: r.Name, Path: r.Path}
	}
	return repos, nil
}

func (c *Client) Worktrees(ctx context.Context, repo domain.RepoID) ([]domain.Worktree, error) {
	var raw []struct {
		ID, Name, Branch, Status string
		Pinned                   bool
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/api/repos/%s/worktrees/", repo), nil, &raw); err != nil {
		return nil, err
	}
	out := make([]domain.Worktree, len(raw))
	for i, w := range raw {
		out[i] = domain.Worktree{
			ID: domain.WorktreeID(w.ID), RepoID: repo, Name: w.Name, Branch: w.Branch,
			Pinned: w.Pinned, Active: w.Status == "active",
		}
	}
	return out, nil
}

func (c *Client) GitStatus(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID) (domain.GitStatus, error) {
	var raw struct {
		Dirty       bool `json:"dirty"`
		HasUpstream bool `json:"has_upstream"`
		Ahead       int  `json:"ahead"`
		Behind      int  `json:"behind"`
	}
	err := c.do(ctx, "GET", fmt.Sprintf("/api/repos/%s/worktrees/%s/status", repo, wt), nil, &raw)
	return domain.GitStatus{Dirty: raw.Dirty, HasUpstream: raw.HasUpstream, Ahead: raw.Ahead, Behind: raw.Behind}, err
}

func (c *Client) ClearAttention(ctx context.Context, repo domain.RepoID, wt domain.WorktreeID) error {
	return c.do(ctx, "POST", fmt.Sprintf("/api/repos/%s/worktrees/%s/attention/clear", repo, wt), nil, nil)
}

// Subscribe streams /ws/attention, reconnecting until ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context) <-chan domain.AttentionEvent {
	out := make(chan domain.AttentionEvent, 16)
	url := "ws" + strings.TrimPrefix(c.base, "http") + "/ws/attention"
	go func() {
		defer close(out)
		for ctx.Err() == nil {
			c.readAttention(ctx, url, out)
			select {
			case <-ctx.Done():
			case <-time.After(reconnectDelay):
			}
		}
	}()
	return out
}

func (c *Client) readAttention(ctx context.Context, url string, out chan<- domain.AttentionEvent) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	go func() { <-ctx.Done(); conn.Close() }()
	for {
		var m struct {
			Type       string          `json:"type"`
			Pending    json.RawMessage `json:"pending"`
			WorktreeID string          `json:"worktree_id"`
			Message    string          `json:"message"`
		}
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		var ev domain.AttentionEvent
		switch m.Type {
		case "snapshot":
			ev.Snapshot, ev.Pending = true, domain.Attention{}
			var snap map[string]string
			_ = json.Unmarshal(m.Pending, &snap)
			for k, v := range snap {
				ev.Pending[domain.WorktreeID(k)] = v
			}
		case "update":
			ev.Worktree, ev.Message = domain.WorktreeID(m.WorktreeID), m.Message
			_ = json.Unmarshal(m.Pending, &ev.IsPending)
		default:
			continue
		}
		select {
		case out <- ev:
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) NameSuggestion(ctx context.Context, repo domain.RepoID) (string, error) {
	var r struct{ Name string }
	err := c.do(ctx, "GET", fmt.Sprintf("/api/repos/%s/worktrees/new-name-suggestion", repo), nil, &r)
	return r.Name, err
}

func (c *Client) Branches(ctx context.Context, repo domain.RepoID) (domain.BranchChoices, error) {
	var r struct {
		Branches []string `json:"branches"`
		Default  string   `json:"default"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("/api/repos/%s/branches", repo), nil, &r); err != nil {
		return domain.BranchChoices{}, err
	}
	return domain.NewBranchChoices(r.Branches, r.Default), nil
}

func (c *Client) CreateWorktree(ctx context.Context, repo domain.RepoID, name, sourceBranch string) (domain.Worktree, error) {
	var r struct{ ID, Name, Branch string }
	in := map[string]string{"name": name, "source_branch": sourceBranch}
	if err := c.do(ctx, "POST", fmt.Sprintf("/api/repos/%s/worktrees/", repo), in, &r); err != nil {
		return domain.Worktree{}, err
	}
	return domain.Worktree{ID: domain.WorktreeID(r.ID), RepoID: repo, Name: r.Name, Branch: r.Branch, Active: true}, nil
}
