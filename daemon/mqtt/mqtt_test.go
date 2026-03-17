package mqtt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMQTTServiceCreationFailsWhenDependenciesAreNil(t *testing.T) {
	t.Run("Error should be thrown when context is nil", func(t *testing.T) {
		t.Parallel()

		client, err := NewClient(nil)
		assert.Nil(t, client)
		assert.Error(t, err)
	})
}
