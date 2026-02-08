package noop

import "context"

// NotificationAdapter is a no-op implementation for testing.
type NotificationAdapter struct{}

// NewNotificationAdapter creates a no-op notification adapter.
func NewNotificationAdapter() *NotificationAdapter {
	return &NotificationAdapter{}
}

// Send does nothing and returns nil.
func (n *NotificationAdapter) Send(ctx context.Context, title, message, sound string) error {
	return nil
}
