# P4 Programs Directory

This directory contains P4 programs that can be deployed to the BMv2 switch via the API.

## Structure

- `simple_routing.p4` - Simple L3 routing example
- `compile.sh` - Script to compile P4 programs for BMv2

## Compiling P4 Programs

To compile a P4 program for BMv2, use the `compile.sh` script:

```bash
./compile.sh simple_routing.p4
```

This will generate:
- `simple_routing.p4info.txt` - P4Info text format (readable)
- `simple_routing.p4info.bin` - P4Info binary format (for deployment)
- `simple_routing.json` - BMv2 device configuration (JSON format)

## Requirements

- `p4c` - P4 compiler (install from https://github.com/p4lang/p4c)
- BMv2 compiler backend

### Install p4c on Ubuntu/Debian:

```bash
sudo apt-get install p4lang-p4c
```

Or build from source:

```bash
git clone https://github.com/p4lang/p4c.git
cd p4c
./bootstrap.sh
./build/p4c --version
```

## Using Compiled Programs

Once compiled, the driver can load the P4Info binary and device config to deploy to the switch.

### Via API

1. **Deploy a program** (POST):
```bash
curl -X POST http://localhost:8080/api/p4/program \
  -H "Content-Type: application/json" \
  -d '{"program": "<base64-encoded-program>", "dry_run": false}'
```

2. **Verify a program** (POST, no deployment):
```bash
curl -X POST http://localhost:8080/api/p4/verify \
  -H "Content-Type: application/json" \
  -d '{"program": "<base64-encoded-program>", "dry_run": true}'
```

3. **Get current program info** (GET):
```bash
curl -X GET http://localhost:8080/api/p4/program
```

## Example Programs

### Simple L3 Routing
A basic L3 forwarding table that matches on destination MAC/IP and forwards packets.

### L2 Learning Switch
A simple learning bridge that learns MAC addresses on ports.

For more examples, see:
- https://github.com/p4lang/behavioral-model/tree/main/targets/simple_switch_grpc/tests
- https://github.com/p4lang/p4-tutorials

## Notes

- P4Info must be in binary format for deployment
- BMv2 device config must be valid JSON
- Programs must target the BMv2 behavioral model
- Currently, only one program can be active at a time
