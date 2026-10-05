# Area Aggregates derived from the Areas, never stored

Every Area gets one Aggregate per aggregated kind among its Functions (lights, occupancy to begin with), with a rule fixed per kind in the code. It is a real Aggregate: a Target with its own key, commandable, with a History, usable by an Automation. Its identity is derived from its Area and its kind, and its members are computed from the Areas assigned to Devices and Functions, never stored. It exists while it has at least one member; with none left it is gone, as any deleted Target is, and it comes back under the same identity and with the same History when a member is assigned again. Its Name is derived from its Area's and its kind, and it has no settings of its own.

## Considered Options

- **A dashboard-only view**: the web client combines the Area's Functions itself. No backend work, but a second way of aggregating next to the Aggregate, invisible to Automations and with no History.
- **Aggregates materialized by Oiko**: Oiko creates ordinary Aggregates and keeps their members in step with the Areas. Editable by hand, but then the stored members and the Areas drift apart, and each edit raises which one wins.

## Consequences

- Moving a Device to another Area changes the members of two Area Aggregates and nothing else: there is nothing to keep in step.
- The rules are not settable per Area. A kind is added to the aggregated kinds by adding its rule to the table.
- A Function cannot leave its Area's Aggregate except by being assigned another Area.
- Deleting an Area deletes its Area Aggregates and their History.
