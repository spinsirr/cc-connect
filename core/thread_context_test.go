package core

import (
	"context"
	"strings"
	"testing"
)

// stubTCPlatform implements Platform plus ThreadContextProvider.
type stubTCPlatform struct{ ctx string }

func (stubTCPlatform) Name() string                                   { return "stub" }
func (stubTCPlatform) Start(MessageHandler) error                     { return nil }
func (stubTCPlatform) Reply(context.Context, any, string) error       { return nil }
func (stubTCPlatform) Send(context.Context, any, string) error        { return nil }
func (stubTCPlatform) Stop() error                                    { return nil }
func (s stubTCPlatform) ThreadContext(any) string                     { return s.ctx }

// stubPlainPlatform implements Platform only (no ThreadContextProvider).
type stubPlainPlatform struct{}

func (stubPlainPlatform) Name() string                             { return "plain" }
func (stubPlainPlatform) Start(MessageHandler) error               { return nil }
func (stubPlainPlatform) Reply(context.Context, any, string) error { return nil }
func (stubPlainPlatform) Send(context.Context, any, string) error  { return nil }
func (stubPlainPlatform) Stop() error                              { return nil }

func TestApplyThreadContext_PrependsWhenProviderReturnsContext(t *testing.T) {
	p := stubTCPlatform{ctx: "[Thread context]\nalice: hi"}
	got := applyThreadContext(p, nil, "[cc-connect sender_id=U1]\nhello")
	if !strings.HasPrefix(got, "[Thread context]\nalice: hi") {
		t.Errorf("expected thread context prepended, got:\n%s", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("original prompt must be preserved, got:\n%s", got)
	}
}

func TestApplyThreadContext_UnchangedWhenNotProvider(t *testing.T) {
	got := applyThreadContext(stubPlainPlatform{}, nil, "hello")
	if got != "hello" {
		t.Errorf("expected prompt unchanged for non-provider, got:\n%s", got)
	}
}

func TestApplyThreadContext_UnchangedWhenEmptyContext(t *testing.T) {
	got := applyThreadContext(stubTCPlatform{ctx: ""}, nil, "hello")
	if got != "hello" {
		t.Errorf("expected prompt unchanged when context empty, got:\n%s", got)
	}
}
