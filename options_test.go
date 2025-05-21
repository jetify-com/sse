package sse

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOptions(t *testing.T) {
	tests := []struct {
		name     string
		option   Option
		expected func(c *config) bool
	}{
		{
			name:   "WithHeartbeatInterval",
			option: WithHeartbeatInterval(30 * time.Second),
			expected: func(c *config) bool {
				return c.heartbeatInterval == 30*time.Second
			},
		},
		{
			name:   "WithHeartbeatInterval_Zero",
			option: WithHeartbeatInterval(0),
			expected: func(c *config) bool {
				return c.heartbeatInterval == 0
			},
		},
		{
			name:   "WithHeaders",
			option: WithHeaders(http.Header{"X-Test": []string{"test-value"}}),
			expected: func(c *config) bool {
				return c.headers.Get("X-Test") == "test-value"
			},
		},
		{
			name:   "WithHeartbeatComment",
			option: WithHeartbeatComment("test-comment"),
			expected: func(c *config) bool {
				return c.heartbeatComment == "test-comment"
			},
		},
		{
			name:   "WithStatus",
			option: WithStatus(http.StatusAccepted),
			expected: func(c *config) bool {
				return c.status == http.StatusAccepted
			},
		},
		{
			name:   "WithRetryDelay",
			option: WithRetryDelay(5 * time.Second),
			expected: func(c *config) bool {
				return c.retryDelay == 5*time.Second
			},
		},
		{
			name:   "WithRetryDelay_Zero",
			option: WithRetryDelay(0),
			expected: func(c *config) bool {
				return c.retryDelay == 0
			},
		},
		{
			name:   "WithWriteTimeout",
			option: WithWriteTimeout(10 * time.Second),
			expected: func(c *config) bool {
				return c.writeTimeout == 10*time.Second
			},
		},
		{
			name: "WithCloseMessage",
			option: WithCloseMessage(&Event{
				Data: "closing connection",
			}),
			expected: func(c *config) bool {
				return c.closeMessage != nil && c.closeMessage.Data == "closing connection"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Start with default config
			c := defaultConfig()

			// Apply the option under test
			tt.option(&c)

			// Verify the option modified the config as expected
			assert.True(t, tt.expected(&c), "Option did not set config as expected")
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	c := defaultConfig()

	// Verify default values
	assert.Equal(t, 15*time.Second, c.heartbeatInterval)
	assert.Equal(t, "keep-alive", c.heartbeatComment)
	assert.Equal(t, http.StatusOK, c.status)
	assert.Equal(t, 3*time.Second, c.retryDelay)
	assert.Equal(t, 5*time.Second, c.writeTimeout)
	assert.Nil(t, c.closeMessage)
	assert.NotNil(t, c.headers)
}
