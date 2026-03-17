package vnfs

import (
	"iml-daemon/internal/testutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVNFInstanceFactoryCreationFailsWhenDependenciesAreNil(t *testing.T) {
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

func TestInstanceRegistration(t *testing.T) {
	// Defining the columns of the table
	var tests = []struct {
		name  string
		input *RegistrationRequest
		want  *InstanceRegistrationResponse
	}{
		// the table itself
		{"9 should be Foo",
			&RegistrationRequest{},
			&InstanceRegistrationResponse{}},
		{"3 should be Foo",
			&RegistrationRequest{},
			&InstanceRegistrationResponse{}},
		{"1 is not Foo",
			&RegistrationRequest{},
			&InstanceRegistrationResponse{}},
		{"0 should be Foo",
			&RegistrationRequest{},
			&InstanceRegistrationResponse{}},
	}

	// Create the mock dependencies
	repo := new(testutil.MockRepo)
	bus := new(testutil.MockEventBus)
	dataplane := new(testutil.MockDataplaneManager)
	imlClient := new(testutil.MockIMLClient)

	// Set up mock expectations
	// repo.On("SaveInstance", mock.Anything).Return(nil)

	// Set up the factory
	factory, err := NewInstanceFactory(repo, bus, dataplane, imlClient)
	assert.NoError(t, err)

	// The execution loop
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // Run test in parallel
			response, err := factory.NewLocalInstance(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, response)
		})
	}
}
