package router

import (
	"iml-daemon/internal/testutil"
	"testing"

	"github.com/stretchr/testify/assert"
)


func TestRouterCreationFailsWhenDependenciesAreNil(t *testing.T) {
	t.Run("Error should be thrown when repo is nil", func(t *testing.T) {
		t.Parallel()
		bus := new(testutil.MockEventBus)
		dataplane := new(testutil.MockDataplaneManager)

		factory, err := New(nil, bus, dataplane)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
	t.Run("Error should be thrown when bus is nil", func(t *testing.T) {
		t.Parallel()
		repo := new(testutil.MockRepo)
		dataplane := new(testutil.MockDataplaneManager)

		factory, err := New(repo, nil, dataplane)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
	t.Run("Error should be thrown when dataplane is nil", func(t *testing.T) {
		t.Parallel()
		repo := new(testutil.MockRepo)
		bus := new(testutil.MockEventBus)

		factory, err := New(repo, bus, nil)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
}