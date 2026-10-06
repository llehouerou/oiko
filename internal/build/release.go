package build

// release is Oiko's version as the commit of a release stamps it
// (scripts/release, ADR 0018), for a build where Go records none, such as
// Nix's. On main it is empty: the line must stay as is for the script.
const release = "v0.6.0"
