import ipaddress
import urllib.request, json
import p4runtime_sh.shell as sh
from p4runtime_sh.context import P4Type
from typing import List
from pydantic import BaseModel
from ipaddress import IPv6Network, IPv6Address, IPv6Interface
import time
import argparse
import os
import signal
import psutil
import socket

parser = argparse.ArgumentParser(description="P4Runtime Sidecar")
parser.add_argument("--sleep", type=int, default=10, help="Time to sleep before starting the sidecar")
parser.add_argument("--p4info", type=str, required=True, help="Path to the P4Info file")
parser.add_argument("--json", type=str, required=True, help="Path to the P4 binary file")
parser.add_argument("--main-interface", type=str, required=True, help="Main interface of the NF")


def handle_signal(signum, frame):
  print("Received termination signal, shutting down...")
  sh.teardown()
  exit(0)

def getTables(sh):
  """Retrieves a list of tables in the P4Runtime shell.
  """
  tableList = []
  for table in sh.P4Objects(P4Type.table):
    tableList.append(str(table))
  return tableList

def getTable(sh, tableName):
  """Retrieves a table.

  Receives a string (``tableName``) with the name of the table to get.
  - If the table name is found, returns the table object.
  - If the table name is not found, returns None.
  """

  if tableName in sh.P4Objects(P4Type.table):
    return str(sh.P4Objects(P4Type.table)[tableName])
  return None

def get_interface_ipv6s(interface: str) -> List[IPv6Address]:
  """
  Returns IPv6 addresses for a given interface using psutil.
  """
  if interface not in psutil.net_if_addrs():
    raise ValueError(f"Interface '{interface}' not found")

  ipv6s: List[IPv6Address] = []
  for addr in psutil.net_if_addrs()[interface]:
    if addr.family == socket.AF_INET6:
      ip = addr.address.split('%')[0]
      ipv6s.append(IPv6Address(ip))

  return ipv6s

def process_assignments(url, existing_ipv6s):
  print("Fetching assignments from controller...")
  assignments: List[Assignment] = json.loads(url.read().decode())
  for assignment in assignments:
    print(f"Processing assignment: {assignment}")
    sid = ipaddress.IPv6Interface(assignment['sid'])
    sid_ip_str = str(sid.ip)
    # If sid of assignment does not belong here, skip it
    if sid.ip not in existing_ipv6s:
      print(f"Skipping assignment for SID {sid_ip_str} as it does not belong to this interface")
      continue
    # If assignment exists and is up to date, skip it
    if (sid_ip_str in set_assignments and set_assignments[sid_ip_str] == assignment['subfunction_id']):
      print(f"Assignment for SID {sid_ip_str} is already up to date, skipping")
      continue

    print(f"New assignment received: {sid_ip_str} -> Subfunction ID: {assignment['subfunction_id']}")
    set_assignments[sid_ip_str] = assignment['subfunction_id']
    entry = sh.TableEntry("MyIngress.function_id_table")(action="MyIngress.set_function_id")
    entry.match["hdr.outer_ipv6.dst_addr"] = sid_ip_str
    entry.action["func_id"] = str(assignment['subfunction_id'])
    entry.insert()
    print(f"Inserted table entry for SID {sid_ip_str} with Subfunction ID {assignment['subfunction_id']}")

class Assignment(BaseModel):
  subfunction_id: int
  sid: IPv6Interface

set_assignments = {
  # "<sid>": <subfunction_id>
}

if __name__ == '__main__':
  print("Starting P4Runtime Sidecar...")
  # Register signal handlers for graceful shutdown
  signal.signal(signal.SIGINT, handle_signal)   # Ctrl+C
  signal.signal(signal.SIGTERM, handle_signal)  # Termination from Kubernetes, docker, etc.

  # Parse command-line arguments
  args = parser.parse_args()

  # Get the NF_ID from environment variable
  nf_id = os.getenv("NF_ID")
  if nf_id is None:
    raise ValueError("NF_ID environment variable is not set")

  print(f"Found Network Function ID: {nf_id}")
  existing_ipv6s = get_interface_ipv6s(args.main_interface)
  print(f"Existing IPv6 addresses on interface {args.main_interface}: {existing_ipv6s}")

  # Wait until the other container is ready
  time.sleep(10)

  print("Initializing P4Runtime shell...")
  sh.setup(
    device_id=0,
    grpc_addr='localhost:9559',
    config=sh.FwdPipeConfig(args.p4info, args.json),
  )
  print("P4Runtime shell initialized.")

  while True:
    with urllib.request.urlopen(f"http://iml-p4-controller.loom-system.svc.cluster.local/api/v1/p4controller/assignments/{nf_id}") as url:
      process_assignments(url, existing_ipv6s)
    time.sleep(args.sleep)