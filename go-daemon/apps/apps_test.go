package apps

import (
	"iml-daemon/internal/testutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppInstanceFactoryCreationFailsWhenDependenciesAreNil(t *testing.T) {
	t.Run("Error should be thrown when repo is nil", func(t *testing.T) {
		t.Parallel()
		bus := new(testutil.MockEventBus)
		dataplane := new(testutil.MockDataplaneManager)
		imlClient := new(testutil.MockIMLClient)

		factory, err := NewInstanceFactory(nil, bus, dataplane, imlClient)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
	t.Run("Error should be thrown when bus is nil", func(t *testing.T) {
		t.Parallel()
		repo := new(testutil.MockRepo)
		dataplane := new(testutil.MockDataplaneManager)
		imlClient := new(testutil.MockIMLClient)

		factory, err := NewInstanceFactory(repo, nil, dataplane, imlClient)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
	t.Run("Error should be thrown when dataplane is nil", func(t *testing.T) {
		t.Parallel()
		repo := new(testutil.MockRepo)
		bus := new(testutil.MockEventBus)
		imlClient := new(testutil.MockIMLClient)

		factory, err := NewInstanceFactory(repo, bus, nil, imlClient)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
	t.Run("Error should be thrown when imlClient is nil", func(t *testing.T) {
		t.Parallel()
		repo := new(testutil.MockRepo)
		bus := new(testutil.MockEventBus)
		dataplane := new(testutil.MockDataplaneManager)

		factory, err := NewInstanceFactory(repo, bus, dataplane, nil)
		assert.Nil(t, factory)
		assert.Error(t, err)
	})
}