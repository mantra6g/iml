package vnfs

import (
	"testing"
	"iml-daemon/internal/testutil"

	"github.com/stretchr/testify/assert"
)

func TestDummy(t *testing.T) {
	// Defining the columns of the table
	var tests = []struct {
		name string
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
