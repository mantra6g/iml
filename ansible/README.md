# Install Ansible
See [docs](https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html).

## Requirements before running
You'll need
* **Two physical nodes**
* **Both of them must have SR-IOV capable Network Interface Cards (NIC)**
* **Enable 1GiB Hugepages inside of both nodes**: For this you should modify /etc/default/grub and set
```bash
GRUB_CMDLINE_LINUX_DEFAULT="default_hugepagesz=1G hugepages=1"
```

## Adjust `inventory.yml`
You'll need to set the following variables under each node:
* `ansible_host`: The IP or url of the host.
* `ansible_user`: The ssh user ansible will be using to set up the node.
* `vf_iface`: The interface name of the SR-IOV enabled NIC.
* `sriov_vendor`: The vendor ID of the SR-IOV card. You can use `sudo lscpi -nn` to get the value.
* `sriov_dev`: The device ID of the SR-IOV card. Use `sudo lscpi -nn`.
* `vf_num`: The amount of SR-IOV Virtual Functions to use. (default is 4)

## Install ansible roles and collections
```bash
ansible-galaxy install -r requirements.yml
```

## Reaching the hosts
```bash
ansible all -m ping
```

# Playbooks
init-k8s-cluster: install and init the cluster on nodes specified in inventory
reset-k8s-cluster: reset the cluster

## Run a playbook
```bash
ansible-playbook <playbook.yml> [-l hostname]
```
