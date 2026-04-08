package netns

import (
	"fmt"
	"imlcni/logger"
	"os"
	"runtime"

	"github.com/vishvananda/netns"
)

type FunctionExecutionError struct {
	Err error
}

func (err *FunctionExecutionError) Error() string {
	return "found an error while executing function inside network namespace: " + err.Err.Error()
}
func (err *FunctionExecutionError) Unwrap() error {
	return err.Err
}

type RestoreNetNsError struct {
	Err error
}

func (err *RestoreNetNsError) Error() string {
	return "failed to restore network namespace: " + err.Err.Error()
}
func (err *RestoreNetNsError) Unwrap() error {
	return err.Err
}

// ExecInsideNetworkNamespace executes an arbitrary function inside a network namespace.
//
// It opens a connection to the specified network namespace, switches to it, executes the provided function,
// and then switches back to the original namespace.
//
// Returns a FunctionExecutionError if an error is found while executing the function in the network namespace,
// and a critical RestoreNetNsError if an error is found while switching back to the original namespace.
func ExecInsideNetworkNamespace(namespacePath string, function func() error) (retErr error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	originalNetNamespace, err := netns.Get()
	if err != nil {
		return fmt.Errorf("failed to get handle of current network namespace: %w", err)
	}
	defer func() {
		if closeErr := originalNetNamespace.Close(); closeErr != nil {
			logger.DebugLogger().Printf("WARN: failed to close original network namespace handle: %v", closeErr)
		}
	}()

	targetNetNamespace, err := netns.GetFromPath(namespacePath)
	if err != nil {
		return fmt.Errorf("failed to get handle of target network namespace %s: %w", namespacePath, err)
	}
	defer func() {
		if closeErr := targetNetNamespace.Close(); closeErr != nil {
			logger.DebugLogger().Printf("WARN: failed to close target network namespace handle: %v", closeErr)
		}
	}()

	err = netns.Set(targetNetNamespace)
	if err != nil {
		return fmt.Errorf("failed to set current network namespace to target's namespace %s: %w", namespacePath, err)
	}

	funcErr := function()
	if funcErr != nil {
		funcErr = &FunctionExecutionError{Err: funcErr}
	}

	if err = netns.Set(originalNetNamespace); err != nil {
		return &RestoreNetNsError{Err: err}
	}

	return funcErr
}

func EnableSRv6InNamespace(namespacePath string) error {
	return ExecInsideNetworkNamespace(namespacePath, func() error {
		// Set up ip forwarding and SRv6
		if err := os.WriteFile("/proc/sys/net/ipv6/conf/all/forwarding", []byte("1"), 0644); err != nil {
			return fmt.Errorf("failed to enable IPv6 forwarding: %w", err)
		}
		if err := os.WriteFile("/proc/sys/net/ipv6/conf/all/seg6_enabled", []byte("1"), 0644); err != nil {
			return fmt.Errorf("failed to enable SRv6: %w", err)
		}
		return nil
	})
}
