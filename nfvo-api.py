from http import HTTPStatus
import os
import uuid
from ruamel.yaml import YAML
from ruamel.yaml.scalarstring import SingleQuotedScalarString
from ruamel.yaml.parser import ParserError
import random
from subprocess import run

from flask import Flask, request, jsonify

# Test:
# curl -F file=@ping.yml http://localhost:5000/iml/yaml/deploy
# curl -X DELETE http://localhost:5000/iml/yaml/deploy/1

app = Flask(__name__)

app_registry = []
ns_registry = []

interpod_mode = 'br'
#interpod_mode = 'memif'
nf_memif_setup = False
DEFAULT_NAMESPACE = 'desire6g'
DEFAULT_CHART = './graph-chart'
SERVICES_FOLDER = "services"
DEPLOY_FOLDER = "deploys"
UPLOAD_FOLDER = 'files'
IML_SUBNET = '10.100'
IML_SUBNET_PREFIX = '/16'
app.config['UPLOAD_FOLDER'] = UPLOAD_FOLDER

if not os.path.exists(UPLOAD_FOLDER):
  os.makedirs(UPLOAD_FOLDER)
if not os.path.exists(DEPLOY_FOLDER):
  os.makedirs(DEPLOY_FOLDER)

def get_next_deploy_id():
  next_id = 1
  while os.path.exists(os.path.join(DEPLOY_FOLDER, f'values-{next_id}.yaml')):
    next_id += 1
  return next_id

# TODO check on host if available
currentmemifbridge = 4
def getnextmemifbridgeid():
  global currentmemifbridge
  currentmemifbridge += 1
  return currentmemifbridge

memifid = {}
def getnextmemifid(nf):
  if nf not in memifid:
    memifid[nf] = -1
  memifid[nf] += 1
  return memifid[nf]

given_macs = []
def generate_mac():
  global given_macs
  while True:
    next_mac = "02:" + ":".join([f"{random.randint(0, 255):02x}" for x in range(5)])
    if next_mac not in given_macs:
      break
  given_macs.append(next_mac)
  return next_mac

given_ips = []
def generate_ip():
  global given_ips
  while True:
    next_ip = IML_SUBNET + "." + ".".join([f"{random.randint(0, 255)}" for x in range(2)])
    if next_ip not in given_ips:
      break
  given_ips.append(next_ip)
  return next_ip

def addiptoinit(dic, ip, intf, memifid=None, mac=None):
  # note: memifid act as a type param
  if memifid is not None and nf_memif_setup:
    if "cmd" not in dic:
      dic["cmd"] = SingleQuotedScalarString('')
    #if '/usr/bin/vpp ' not in dic["cmd"]:
      dic["cmd"] += SingleQuotedScalarString('/usr/bin/vpp unix { log /var/log/vpp/vpp-app.log full-coredump cli-listen /run/vpp/cli.sock gid vpp } api-trace {on} api-segment {gid vpp} cpu {skip-cores 4} dpdk {no-pci} socksvr {socket-name /run/vpp/api.sock};sleep 5;')
    dic["cmd"] += SingleQuotedScalarString(f'vppctl "create memif socket id {memifid+1} filename $(ls /var/lib/cni/usrspcni/*-{intf}.sock)";')
    dic["cmd"] += SingleQuotedScalarString(f'vppctl "create interface memif id {memifid} socket-id {memifid+1} slave no-zero-copy";')
    dic["cmd"] += SingleQuotedScalarString(f'vppctl "set int state memif{memifid+1}/{memifid} up";')
    dic["cmd"] += SingleQuotedScalarString(f'vppctl "set int mac address memif{memifid+1}/{memifid} {mac}";')
    dic["cmd"] += SingleQuotedScalarString(f'vppctl "set int ip address memif{memifid+1}/{memifid} {ip}/32";')
  elif memifid is None:
    if "initcmd" not in dic:
      dic["initcmd"] = SingleQuotedScalarString('')
    dic["initimage"] = "busybox:1.36"
    dic["initcmd"] += SingleQuotedScalarString(f"ip addr add {ip}/32 dev {intf};")

def addroutetoinit(srcnf, dstnf, dstintf, srcintf):
  s = next(i for i in srcnf['interfaces'] if i['interface'] == srcintf)
  d = next(i for i in dstnf['interfaces'] if i['interface'] == dstintf)
  if 'memifid' in s and nf_memif_setup:
    srcnf["cmd"] += f"vppctl \"set ip neighbor memif{s['memifid']+1}/{s['memifid']} {d['ip']} {d['mac']}\";"
    srcnf["cmd"] += f"vppctl \"ip route add {d['ip']}/32 via memif{s['memifid']+1}/{s['memifid']}\";"
    srcnf["cmd"] = SingleQuotedScalarString(srcnf["cmd"])
  elif 'memifid' not in s:
    srcnf["initcmd"] += f"arp -i {srcintf} -s {d['ip']} {d['mac']};ip route add {d['ip']}/32 dev {srcintf};"
    srcnf["initcmd"] = SingleQuotedScalarString(srcnf["initcmd"])

def addif(dic, name, type, ifindex='1'):
  if f"{name}-{type}-{ifindex}" in dic:
    return
  n = {}
  n["type"] = type
  if type == "sriov":
    n["mac"] = SingleQuotedScalarString(generate_mac())
    n["vf"] = SingleQuotedScalarString("nvidia.com/cx6dx_vf")
  if type == "memif":
    n["bridgedomain"] = getnextmemifbridgeid()

  dic[f"{name}-{type}-{ifindex}"] = n

def getif(nf, intf):
  x = next(i for i in nf['interfaces'] if i['interface'] == intf)
  return x

def getifindex(nf, intf):
  x = next(i for i, dic in enumerate(nf['interfaces']) if dic['interface'] == intf)
  return x

def addtonf(nf, name, intf, mac=None, ip=None, memifid=None):
  if 'interfaces' not in nf:
    nf['interfaces'] = []
  x = next((i for i in nf['interfaces'] if i['interface'] == intf), None)
  if x != None:
    return

  i = {}
  i['interface'] = intf
  i['name'] = name
  if memifid is not None:
    i['memifid'] = memifid
  if mac:
    i['mac'] = SingleQuotedScalarString(mac)
  if ip:
    i['ip'] = ip
    addiptoinit(nf, ip, intf, memifid, mac)
  nf['interfaces'].append(i)

def addroutetonfr(nfrsrc, nfrdst, srcnode, dstnf, dstintfname):
  # TODO handle duplicate
  nfrdstport = getifindex(nfrdst, dstintfname)
  dstintf = getif(dstnf, dstintfname)
  nfrdst['files']['ipv4rules.cfg'] += f"R{dstintf['ip']}/32 {nfrdstport}\n"
  if srcnode != dstnf['node']:
    port = getifindex(nfrsrc, f"{srcnode}-{dstnf['node']}-1")
    nfrsrc['files']['ipv4rules.cfg'] += f"R{dstintf['ip']}/32 {port}\n"

def addnfr(services, node):
  if f"nfr-{node}" in services:
    return
  d = {}
  d['name'] = 'nfrouter'
  d['node'] = node
  d['files'] = {}
  d['files']['ipv6rules.cfg'] = SingleQuotedScalarString('R::/128 0')
  d['files']['ipv4rules.cfg'] = ''
  services[f"nfr-{node}"] = d

def addcmdtonfr(nfr, services, interfaces):
  nfr['cmd'] = './l3fwd-static -l 0-1 -n 4 --no-pci'
  i = 0
  for intf in nfr['interfaces']:
    if interpod_mode=='memif' and interfaces[intf['name']]['type'] == 'memif':
      nfr['cmd'] += f" --vdev=net_memif{i},role=client,socket-abstract=no,socket=$(ls /var/lib/cni/usrspcni/*{intf['interface']}.sock)"
    else:
      nfr['cmd'] += f" --vdev=net_tap{i},iface=dtap{i},remote={intf['interface']}"
    i += 1
  nfr['cmd'] += f" -- -p 0x{int('1'*len(nfr['interfaces']), 2):x}"
  nfr['cmd'] += ' --config="'
  nfr['cmd'] += ','.join(f'({i},0,0),({i},1,1)' for i in range(len(nfr['interfaces'])))
  nfr['cmd'] += '" --mode=poll -P'
  i = 0
  for intf in nfr['interfaces']:
    servicewithtype, ifindex = intf['name'].rsplit('-', 1)
    service, type = servicewithtype.rsplit('-', 1)
    if type == 'br' or type == 'memif':
      mac = getif(services[service], intf['name'])['mac']
    elif type == 'sriov':
      _if = intf['interface'].rsplit('-', 1)[0]
      mac = interfaces[f"{_if.rsplit('-', 1)[1]}-sriov-1"]['mac']
    nfr['cmd'] += f" --eth-dest={i},{mac}"
    i += 1
  nfr['cmd'] += ' --rule_ipv4="/opt/nfconfig/ipv4rules.cfg" --rule_ipv6="/opt/nfconfig/ipv6rules.cfg"'
  if interpod_mode == 'memif':
    nfr['cmd'] += ' --relax-rx-offload --parse-ptype'

  nfr['cmd'] = SingleQuotedScalarString(nfr['cmd'])

def cleanintf(services):
  for s in services:
    for i in services[s]['interfaces']:
      if 'ip' in i:
        del i['ip']
      if 'memifid' in i:
        del i['mac']
        del i['memifid']

def generate_values(nsd, path):
  with open(path, 'w') as f:
    data = {}
    data['services'] = {}
    data['interfaces'] = {}
    for i in nsd['lnsd']['ns']['application-functions']:
      found_app = None
      for af in app_registry:
        if af['id'] == i['af-id']:
          found_app = af
          break
      if found_app is None:
        print(f"Application function {i['af-id']} not found in app registry.")
        return

      s = {}
      s['name'] = i['af-id']
      s['node'] = found_app['node']
      data['services'][i['af-instance-id']] = s

    # services:
    #   pkt-src:
    #     name: trex
    #     node: epyc1
    #   pkt-dst:
    #     name: trex
    #     node: epyc1


    for g in nsd['lnsd']['ns']['forwarding_graphs']:
      for l in g['links']:
        for p in l['connection-points']:
          if p['member-connection-point-index'] == 1:
            srcid, srcifindex = p['member-if-id-ref'].split(':')
          if p['member-connection-point-index'] == 2:
            dstid, dstifindex = p['member-if-id-ref'].split(':')

        srcnf = data['services'][srcid]
        dstnf = data['services'][dstid]

        # Creates interfaces (ONLY, so no IP configs) for the src and dst containers
        addif(data['interfaces'], srcid, interpod_mode, srcifindex)
        addif(data['interfaces'], dstid, interpod_mode, dstifindex)
        srcintf = f'{srcid}-{interpod_mode}-{srcifindex}'
        dstintf = f'{dstid}-{interpod_mode}-{dstifindex}'

        # interfaces:
        #   pkt-src-br-1:
        #     type: br
        #   pkt-src-br-2:
        #     type: br

        addtonf(srcnf, srcintf, srcintf, generate_mac(), generate_ip(), getnextmemifid(srcid) if interpod_mode == 'memif' else None)
        addtonf(dstnf, dstintf, dstintf, generate_mac(), generate_ip(), getnextmemifid(dstid) if interpod_mode == 'memif' else None)

        # services:
        #   pkt-src:
        #     name: trex
        #     node: epyc1
        #     interfaces:
        #     - interface: pkt-src-br-1
        #       name: pkt-src-br-1
        #       mac: '02:68:0f:d0:01:aa'
        #     - interface: pkt-src-br-2
        #       name: pkt-src-br-2
        #       mac: '02:22:c4:d7:ac:84'
        #     initcmd: 'ip addr add 10.196.188.182/32 dev pkt-src-br-1;ip addr add 10.14.80.122/32 dev pkt-src-br-2;arp -i pkt-src-br-1 -s 10.14.80.122 02:22:c4:d7:ac:84;ip route add 10.14.80.122/32 dev pkt-src-br-1;arp -i pkt-src-br-2 -s 10.196.188.182 02:68:0f:d0:01:aa;ip route add 10.196.188.182/32 dev pkt-src-br-2;'
        #     initimage: busybox:1.36

        addnfr(data['services'], srcnf['node'])
        addnfr(data['services'], dstnf['node'])
        nfrsrc = data['services'][f"nfr-{srcnf['node']}"]
        nfrdst = data['services'][f"nfr-{dstnf['node']}"]

        # services:
        #  nfr-epyc1:
        #    name: nfrouter
        #    node: epyc1
        #    files:
        #      ipv6rules.cfg: 'R::/128 0'
        #      ipv4rules.cfg: ""

        if interpod_mode == 'memif':
          srcnf['hostpath'] = {'name': 'shared-dir', 'hostpath': f"/run/vpp/{srcid}", 'path': "/var/lib/cni/usrspcni"}
          dstnf['hostpath'] = {'name': 'shared-dir', 'hostpath': f"/run/vpp/{dstid}", 'path': "/var/lib/cni/usrspcni"}
          nfrsrc['hostpath'] = {'name': 'shared-dir', 'hostpath': f"/run/vpp/nfr-{srcnf['node']}", 'path': "/var/lib/cni/usrspcni"}
          nfrdst['hostpath'] = {'name': 'shared-dir', 'hostpath': f"/run/vpp/nfr-{dstnf['node']}", 'path': "/var/lib/cni/usrspcni"}

        addtonf(nfrsrc, srcintf, srcintf)
        addtonf(nfrdst, dstintf, dstintf)

        # services:
        #  nfr-epyc1:
        #    name: nfrouter
        #    node: epyc1
        #    files:
        #      ipv6rules.cfg: 'R::/128 0'
        #      ipv4rules.cfg: ""
        #    interfaces:
        #    - interface: pkt-src-br-1
        #      name: pkt-src-br-1
        #    - interface: pkt-src-br-2
        #      name: pkt-src-br-2

        if srcnf['node'] != dstnf['node']:
          addif(data['interfaces'], srcnf['node'], "sriov")
          addif(data['interfaces'], dstnf['node'], "sriov")
          addtonf(nfrsrc, f"{srcnf['node']}-sriov-1", f"{srcnf['node']}-{dstnf['node']}-1")

        addroutetoinit(srcnf, dstnf, dstintf, srcintf)

        

        addroutetonfr(nfrsrc, nfrdst, srcnf['node'], dstnf, dstintf)



    for n in data['services'].values():
      if n['name'] == 'nfrouter':
        addcmdtonfr(n, data['services'], data['interfaces'])
      elif nf_memif_setup:
        n['cmd'] += 'sleep infinity;'

    cleanintf(data['services'])
    yaml=YAML()
    yaml.width = 4096
    yaml.default_flow_style = False
    yaml.dump(data, f)

def get_app(app_id):
  for af in app_registry:
    if af['id'] == app_id:
      return af
  return None

def get_app_by_name(app_name):
  global app_registry
  for af in app_registry:
    if af['instance-id'] == app_name:
      return af
  return None

def generate_values2(ns, path):
  values = {}
  values["nfrouters"] = []

  for graph in ns['forwarding_graphs']:
    # Get the source and destination application functions from the graph
    # The source and destination are in the format "app_name:intf_number"
    # e.g., "app1:1", "app1:2", "app2:1", etc.
    src_app_name, src_intf_number = graph['source'].split(':')
    dst_app_name, dst_intf_number = graph['target'].split(':')

    # Find the source and destination application functions in the app registry
    src_app = get_app_by_name(src_app_name)
    dst_app = get_app_by_name(dst_app_name)
    
    if not src_app:
      print(f"Source application function {src_app_name} not found in app registry.")
      return
    if not dst_app:
      print(f"Target application function {dst_app_name} not found in app registry.")
      return

    nfrouter_data = {}

    # Generate a "unique" ID for the nfrouter. 
    # TODO: check if this ID is already used in the registry
    nfrouter_id = random.randint(0x0000, 0xFFFF)

    # Assign a name to the nfrouter like "nfrouter-89ab"
    nfrouter_data["name"] = f"nfrouter-{nfrouter_id:04x}"

    # Assign the node where the nfrouter will be deployed.
    # For now, it is assumed that all application functions are deployed on the same node.
    # As a result, the node the nfrouter will be deployed will be on the first node 
    # of the first application function.
    # In a real scenario, this should be calculated based on the forwarding graph.
    nfrouter_data["node"] = src_app['node']

    # Generate the interfaces for the nfrouter from the 
    # application functions in the forwarding graph
    nfrouter_data["interfaces"] = []
    for app in [src_app, dst_app]:
      # Get the interface name and MAC address from the application function
      interface_data = {
        'name': app['peer_name'],
        'peer_mac': SingleQuotedScalarString(app['mac']), # SingleQuotedScalarString needed because yaml does not like colons in MAC addresses
        'peer_ip': app['ip'][:-3], # Remove the last 3 characters (the /16 part)
      }
      nfrouter_data["interfaces"].append(interface_data)
    
    # Add the nfrouter data to the values dictionary
    # This will be used to generate the values.yaml file for the Helm chart
    values["nfrouters"].append(nfrouter_data)

  with open(path, 'w') as f:
    yaml=YAML()
    yaml.width = 4096
    yaml.default_flow_style = False
    yaml.dump(values, f)

def register_app_instance(app_data):
  app_instance = {
    'id': app_data['af-id'],
    'instance-id': app_data['af-instance-id'],
    'af-version': app_data['af-version'],
    'node': None,  # This will be updated later
    'ip': None,  # This will be generated later
    'mac': None,  # This will be generated later
  }
  app_registry.append(app_instance)

def update_app_instance(app_id, host_id):
  app.logger.info(f"Updating app instance {app_id} on host {host_id}")
  global app_registry
  ip = None; src_mac = None; gateway_mac = None; peer_name = None

  for af in app_registry:
    if af['id'] == app_id:
      af['node'] = host_id
      # Generate a new IP and MAC address for the app instance
      ip = generate_ip() + IML_SUBNET_PREFIX
      src_mac = generate_mac()
      gateway_mac = generate_mac()
      peer_name = f"nfr-{hex(random.randint(0x0000, 0xFFFF))[2:]}"
      # Update the app instance with the new IP and MAC
      af['ip'] = ip
      af['mac'] = src_mac
      af['gateway_mac'] = gateway_mac
      af['peer_name'] = peer_name

  notify_app_instance_update()
  return ip, src_mac, gateway_mac, peer_name

def notify_app_instance_update():
  for ns in ns_registry:
    required_app_ids = ns['required_app_instances']
    missing_apps = len(required_app_ids)
    for af in app_registry:
      if af['id'] in required_app_ids and af['node'] is not None:
        missing_apps -= 1
    app.logger.info(f"Checking deployment for network service {ns['id']}: {missing_apps} missing apps")
    if missing_apps == 0:
      # All required apps are available, trigger NS deployment
      deploy_ns(ns)

def deploy_ns(ns):
  deploy_id = ns['id']
  values_path = os.path.join(DEPLOY_FOLDER, f'values-{deploy_id}.yaml')
  generate_values2(ns, values_path)

  result = run([
    'helm', 'install', 
    '--namespace', DEFAULT_NAMESPACE, '--create-namespace', 
    '-f', values_path, 
    f'nfrouter-{deploy_id}', 
    DEFAULT_CHART
  ], capture_output = True, text = True)

  if result.stderr:
    app.logger.error(f"Failed to deploy: {result.stderr}")
    return
  
  set_ns_deployed(deploy_id)
  app.logger.info(f"Deployed: network service {deploy_id}")

def set_ns_deployed(ns_id):
  global ns_registry
  for ns in ns_registry:
    if ns['id'] == ns_id:
      ns['deployed'] = True
      return
  print(f"Network service {ns_id} not found in registry.")

@app.route("/iml/yaml/deploy/<id>", methods=["DELETE"])
def deleteDeployment(id):
  ns = find_ns_by_id(id)
  if ns is None: return jsonify({
    "response": "network service does not exist"
  }), HTTPStatus.NO_CONTENT

  if ns['deployed']:
    result = run(['helm', 'uninstall', '--namespace', DEFAULT_NAMESPACE, f'nfrouter-{id}'], capture_output = True, text = True)

    if result.stderr: return jsonify({
      "response": result.stderr
    }), HTTPStatus.INTERNAL_SERVER_ERROR
  
  remove_ns(id)

  return jsonify({
    "response": f"Succesful deletion of the deployment with id: {id}"
  }), HTTPStatus.NO_CONTENT

@app.route("/iml/yaml/deploy", methods=["POST"])
def deploy_yaml():
  file = request.files['file']

  try:
    yaml=YAML(typ='safe')
    yaml_data = yaml.load(file.stream)
  except ParserError:
    return jsonify({
      "message": "Failed to parse yaml file"
    }), HTTPStatus.BAD_REQUEST

  # found_ns = find_ns_by_id(yaml_data['lnsd']['ns']['id'])
  # if found_ns is not None: return jsonify({
  #   "message": "a network service with that ID already exists"
  # }), HTTPStatus.CONFLICT

  ns = {
    'id': uuid.uuid4().hex,
    'forwarding_graphs': yaml_data['lnsd']['ns']['forwarding_graphs'],
    'required_app_instances': [],
    'deployed': False,
  }

  for af in yaml_data['lnsd']['ns']['application-functions']:
    register_app_instance(af)
    ns['required_app_instances'].append(af['af-id'])

  ns_registry.append(ns)
  
  return jsonify(ns), HTTPStatus.OK

@app.route("/iml/cni/register", methods=["POST"])
def handle_cni_register():
  data = request.get_json()
  # Validate the received data
  if 'application_id' not in data or 'host_id' not in data:
    return jsonify({"error": "Invalid request data"}), HTTPStatus.BAD_REQUEST

  # Extract app information from the received data
  application_id = data.get("application_id")
  host_id = data.get("host_id")

  # Register app and obtain its IP and MAC address
  ip, mac, gateway_mac, peer_name = update_app_instance(application_id, host_id)
  if ip is None or mac is None: return jsonify({
    "error": "Failed to update app: instance not found"
  }), HTTPStatus.NOT_FOUND

  # Return the app information in the response
  return jsonify({
    "ip": ip, 
    "mac_address": mac,
    "peer_name": peer_name,
    "route": {
      "destination": IML_SUBNET + ".0.0" + IML_SUBNET_PREFIX,
      "gateway_ip": IML_SUBNET + ".0.1",
      "gateway_mac": gateway_mac
    }
  }), HTTPStatus.OK

def find_ns_by_id(ns_id):
  global ns_registry
  for ns in ns_registry:
    if ns['id'] == ns_id: return ns
  return None

def remove_app_instance(app_id):
  global app_registry
  app_registry = [af for af in app_registry if af['id'] != app_id]

def remove_ns(ns_id):
  global ns_registry
  ns_registry = [ns for ns in ns_registry if ns['id'] != ns_id]

@app.route("/iml/cni/teardown", methods=["POST"])
def handle_cni_teardown():
  data = request.get_json()
  # Validate the received data
  if 'application_id' not in data: return jsonify({
    "error": "Invalid request data"
  }), HTTPStatus.BAD_REQUEST

  # Extract app information from the received data
  application_id = data.get("application_id")

  remove_app_instance(application_id)

  # Return the app information in the response
  return jsonify({
    "message": "Application removed successfully"
  }), HTTPStatus.NO_CONTENT


if __name__ == "__main__":
  app.run(host='0.0.0.0', debug=False)
