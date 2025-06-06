package main

import (
  "fmt"
  "log"

  "github.com/vishvananda/netlink"
)

func createBridge(name string) error {
  // Check if bridge already exists
  if _, err := netlink.LinkByName(name); err == nil {
    fmt.Printf("Bridge %s already exists\n", name)
    return nil
  }

  bridge := &netlink.Bridge{
    LinkAttrs: netlink.LinkAttrs{
      Name: name,
    },
  }

  // Create the bridge
  if err := netlink.LinkAdd(bridge); err != nil {
    return fmt.Errorf("could not add bridge: %w", err)
  }

  // Bring the bridge up
  if err := netlink.LinkSetUp(bridge); err != nil {
    return fmt.Errorf("could not set bridge up: %w", err)
  }

  fmt.Printf("Bridge %s created and set up\n", name)
  return nil
}

func main() {
  if err := createBridge("nfbridge"); err != nil {
    log.Fatalf("Failed to create bridge: %v", err)
  }

  fmt.Println("Sleeping forever...")
  select {} // Block forever
}