package ctl

import (
	"context"
	"errors"
)

type contextKey struct{}

// ErrClientMissing signals that no control client is attached to the context.
var ErrClientMissing = errors.New("ctl: client not initialised")

// WithClient stores the control client on the provided context.
func WithClient(ctx context.Context, client *Client) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, client)
}

// FromContext retrieves the control client associated with the context.
func FromContext(ctx context.Context) (*Client, error) {
	if ctx == nil {
		return nil, ErrClientMissing
	}
	client, ok := ctx.Value(contextKey{}).(*Client)
	if !ok || client == nil {
		return nil, ErrClientMissing
	}
	return client, nil
}
