# Smart HID machine contracts

`smart-hid-v1.json` is the machine-readable mirror of the deployed V1 contract.
It does not alter protocol semantics. Human explanation remains in
`../ble/PROVISIONING_V1.md`; firmware and ControlHub sources listed in its
`source` object remain implementation references.

Consumers must pin an exact commit and verify the SHA-256 of the canonical JSON
bytes. Never obtain protocol meaning from a moving branch tip.
