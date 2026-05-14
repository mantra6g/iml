package proxy

import (
	"fmt"
	"net/netip"
	"os/exec"
	"sync"
)

const (
	ProtocolTCP  = "tcp"
	ProtocolUDP  = "udp"
	ProtocolBoth = "both"
)

type Config struct {
	Port     uint16
	Protocol string
}

type Client interface {
	SetDestination(addr netip.Addr) error
}

type socat struct {
	mu           sync.Mutex
	openCommands []*exec.Cmd
	ownAddr      netip.Addr
	port         uint16
	protocol     string
}

func NewSocat(cfg Config) (Client, error) {
	return &socat{
		port:     cfg.Port,
		protocol: cfg.Protocol,
	}, nil
}

func (s *socat) SetDestination(addr netip.Addr) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	err := s.cleanupOldProcesses()
	if err != nil {
		return fmt.Errorf("failed to cleanup old processes: %w", err)
	}

	cmds, err := s.createCommands(addr)
	if err != nil {
		return fmt.Errorf("could not create proxy command: %w", err)
	}

	// Start in background (non-blocking)
	cmds, err = s.executeCommands(cmds)
	if err != nil {
		return fmt.Errorf("failed to execute proxy commands: %w", err)
	}

	s.openCommands = cmds
	return nil
}

func (s *socat) createCommands(addr netip.Addr) ([]*exec.Cmd, error) {
	var cmds []*exec.Cmd
	switch s.protocol {
	case ProtocolTCP:
		cmds = []*exec.Cmd{s.createTCPCommand(addr)}
	case ProtocolUDP:
		cmds = []*exec.Cmd{s.createUDPCommand(addr)}
	case ProtocolBoth:
		cmds = []*exec.Cmd{s.createTCPCommand(addr), s.createUDPCommand(addr)}
	default:
		return nil, fmt.Errorf("unknown proxy protocol: %s", s.protocol)
	}
	return cmds, nil
}

func (s *socat) createTCPCommand(addr netip.Addr) *exec.Cmd {
	// socat TCP4-LISTEN:<port>,fork,su=nobody,reuseaddr "TCP6:[<addr>]:<port>"
	return exec.Command(
		"socat",
		fmt.Sprintf("TCP4-LISTEN:%d,fork,su=nobody,reuseaddr", s.port),
		fmt.Sprintf("TCP6:[%s]:%d", addr.String(), s.port),
	)
}

func (s *socat) createUDPCommand(addr netip.Addr) *exec.Cmd {
	// socat UDP4-LISTEN:<port>,fork,su=nobody,reuseaddr "UDP6:[<addr>]:<port>"
	return exec.Command(
		"socat",
		fmt.Sprintf("UDP4-LISTEN:%d,fork,su=nobody,reuseaddr", s.port),
		fmt.Sprintf("UDP6:[%s]:%d", addr.String(), s.port),
	)
}

func (s *socat) cleanupOldProcesses() error {
	for _, cmd := range s.openCommands {
		if err := cmd.Process.Kill(); err != nil {
			return err
		}
		// Wait to reap the process and avoid zombies
		_ = cmd.Wait()
	}
	s.openCommands = nil
	return nil
}

func (s *socat) executeCommands(cmds []*exec.Cmd) ([]*exec.Cmd, error) {
	started := make([]*exec.Cmd, 0, len(cmds))
	for i := range cmds {
		if err := cmds[i].Start(); err != nil {
			// Clean up already-started commands on failure
			for _, cmd := range started {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
			return nil, err
		}
		started = append(started, cmds[i])
	}
	return started, nil
}
