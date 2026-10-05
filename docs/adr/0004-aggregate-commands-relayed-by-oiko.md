# Aggregate Commands relayed by Oiko, not by Bridge Groups

A Command on an Aggregate is relayed by Oiko as one Command per member, all sent together, rather than delegated to a Bridge Group (a Zigbee group transmitted in one frame). Aggregates must span Bridges once Oiko integrates platforms other than zigbee2mqtt, and their definition must live in Oiko, not in each Bridge's own configuration. Home Assistant light groups already work this way (one service call per member, run concurrently) on the setup Oiko replaces, which has no zigbee2mqtt groups, and the members switching one after another has not been noticeable.

## Considered Options

- **Bridge Groups**: a single radio transmission, no staggered switching, keeps working without Oiko. Rejected as the mechanism because it cannot cover members on different Bridges and requires configuration outside Oiko. If staggered switching ever becomes noticeable, Oiko can create and maintain a zigbee2mqtt group itself (`bridge/request/group/add`, `bridge/request/group/members/add`) for an Aggregate whose members all belong to that Bridge, so the configuration still lives only in Oiko.
