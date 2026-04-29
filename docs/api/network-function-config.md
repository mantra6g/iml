# Network Function Config (WIP)

## Examples
Here is an example of a NetworkFunctionConfig that defines two table entries. The first table entry matches on 
source Ethernet and destination IPv4 addresses and drops the packet if there is a match. 
The second table matches on source IP address and applies the `forward` action with `port=1` if there is a match.
```yaml
apiVersion: core.loom.io/v1alpha1
kind: NetworkFunctionConfig
metadata:
  name: example-network-function-config
spec:
  tables:
    example-table-1:
      entries:
      - matchFields:
        - name: hdr.ethernet.srcAddr
          type: Exact
          exact:
            macAddress: 00:11:22:33:44:55
        - name: hdr.inner_ipv4.dstAddr
          type: Range
          range:
            low:
              ipv4Address: "10.123.0.0"
            high:
              ipv4Address: "10.123.0.122"
        action:
          name: MyIngress.drop
    example-table-2:
      entries:
      - matchFields:
        - name: hdr.inner_ipv4.srcAddr
          type: Ternary
          ternary:
            value: 
              ipv4Address: "10.123.0.0"
            mask: "0xFFFF0FF0"
        action:
          name: MyIngress.forward
          parameters:
            - name: port
              int: "1"
```

## Spec fields
`tables`: A list of tables that define the matching and actions for the network function. Each table has a name 
and a list of entries. Each entry consists of match fields and an action. Match fields specify the criteria for 
matching packets, while the action defines what to do with the matched packets.
  - `entries`: A list of entries in the table. Each entry has match fields and an action.
    - `matchFields`: A list of fields to match on. Each field has a name, value, and type.
      - `name`: The full name of the field to match.
      - `type`: The type of the match (Exact, LPM, Range, Ternary, Optional).
      - `exact`: An exact match value for the field, if the type is Exact.
      - `range`: A range of values for the field, if the type is Range.
        - `low`: The lower bound of the range.
        - `high`: The upper bound of the range.
      - `ternary`: A value and mask for the field, if the type is Ternary.
        - `value`: The value to match.
        - `mask`: The mask to apply to the value for matching.
      - `lpm`: A value and prefix length for the field, if the type is LPM (Longest Prefix Match).
        - `value`: The value to match.
        - `prefixLength`: The prefix length for the LPM match.
      - `optional`: An optional value for the field, if the type is Optional. If the field is not present in the packet, it will be treated as a wildcard match.
         - `value`: The exact value to match if the field is present. If the field is not present, it will be treated as a match.
    - `action`: The action to perform on matched packets. It has a name and optional parameters.
      - `name`: The full name of the action.
      - `parameters`: A list of parameters for the action. Each parameter has a name, value, and type.
        - `name`: The name of the parameter (e.g., port).
        - `value`: The value of the parameter.

On the other hand, the value fields for match fields and action parameters can be of different types, such as `int`, `string`, `macAddress`, `ipv4Address`, etc., depending on the specific field and action being defined.
The following field types are defined:
- `int`: An integer value.
- `rawHex`: A raw hex string.
- `macAddress`: A MAC address value.
- `ipv4Address`: An IPv4 address value.
- `ipv6Address`: An IPv6 address value.

## Status fields
This resource does not have any status fields.
