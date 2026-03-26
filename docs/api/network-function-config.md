# Network Function Config (WIP)

## Examples
Here is an example of a NetworkFunctionConfig that defines two table entries. The first table entry matches on 
source and destination Ethernet addresses and drops the packet if there is a match. 
The second table matches on source IP address and applies the `forward` action with `port=1` if there is a match.
```yaml
apiVersion: core.loom.io/v1alpha1
kind: NetworkFunctionConfig
metadata:
  name: example-network-function-config
spec:
  tables:
    - name: example-table-1
      entries:
        - match_fields:
            - name: src_eth
              value: "00:11:22:33:44:55"
              type: ethernet
            - name: dst_eth
              value: "66:77:88:99:AA:BB"
              type: ethernet
          action:
            name: drop
    - name: example-table-2
      entries:
        - match_fields:
            name: src_ip
            value: "10.0.0.1"
            type: ip
          action:
            name: forward
            parameters:
              - name: port
                value: 1
                type: int
```

## Spec fields
`tables`: A list of tables that define the matching and actions for the network function. Each table has a name 
and a list of entries. Each entry consists of match fields and an action. Match fields specify the criteria for 
matching packets, while the action defines what to do with the matched packets.
  - `name`: The name of the table.
  - `entries`: A list of entries in the table. Each entry has match fields and an action.
    - `match_fields`: A list of fields to match on. Each field has a name, value, and type.
      - `name`: The name of the field to match (e.g., src_ip, dst_eth).
      - `value`: The value to match against (e.g., "10.0.0.1", "00:11:22:33:44:55").
      - `type`: The type of the field (e.g., ip, ethernet, int, hex).
    - `action`: The action to perform on matched packets. It has a name and optional parameters.
      - `name`: The name of the action (e.g., drop, forward).
      - `parameters`: A list of parameters for the action. Each parameter has a name, value, and type.
        - `name`: The name of the parameter (e.g., port).
        - `value`: The value of the parameter (e.g., 1).
        - `type`: The type of the parameter (e.g., ip, ethernet, int, hex).

## Status fields
This resource does not have any status fields.
