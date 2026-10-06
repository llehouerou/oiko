{
  lib,
  buildGoModule,
  buildNpmPackage,
  go_1_27,
  nodejs_24,
  # Types of Bridge added to the built-in ones (ADR 0017), by override: the
  # Go package registering each, and the version of its module.
  bridges ? { },
  # A build with other Bridges vendors other modules: `nix build` prints the
  # hash to set.
  vendorHash ?
    if bridges == { } then "sha256-qcWsye790SYsDOqf49D/BdRlPs5hhb2zpm0Y94fOKhU=" else lib.fakeHash,
}:
let
  fs = lib.fileset;
  added = bridges != { };
  web = buildNpmPackage {
    pname = "oiko-web";
    version = "0";
    nodejs = nodejs_24;
    # The type check reads the automation fixtures the Go tests share.
    src = fs.toSource {
      root = ../.;
      fileset = fs.unions [
        ../internal/automation/testdata
        (fs.difference ../web (
          fs.unions [
            (fs.maybeMissing ../web/node_modules)
            (fs.maybeMissing ../web/dist)
          ]
        ))
      ];
    };
    sourceRoot = "source/web";
    npmDepsHash = "sha256-axLlc2cqswLBsN9JkBinlpMzVe6RsdPnx4M3rx8kFts=";
    installPhase = "cp -r dist $out";
  };
in
(buildGoModule.override { go = go_1_27; }) {
  pname = "oiko";
  version = "0";
  src = fs.toSource {
    root = ../.;
    fileset = fs.unions [
      ../go.mod
      ../go.sum
      ../oiko.go
      ../upgrade.go
      ../signinlink.go
      ../bridge
      ../cmd
      ../internal
      ../web/embed.go
    ];
  };
  inherit vendorHash;
  # With other Bridges, go.mod gains their modules while fetching them, which
  # has the network, and again while building, from what was fetched: the
  # module cache is what gets vendored. Getting cmd/oiko with them records in
  # go.sum the modules whose versions they raise.
  proxyVendor = added;
  postPatch = lib.optionalString added ''
    cat > bridges.go <<EOF
    package oiko

    import (
    ${lib.concatMapStrings (p: "\t_ \"${p}\"\n") (lib.attrNames bridges)})
    EOF
  '';
  preBuild = lib.optionalString added "go get ${lib.escapeShellArgs (lib.mapAttrsToList (p: v: "${p}@${v}") bridges)} ./cmd/oiko";
  subPackages = [ "cmd/oiko" ];
  env.CGO_ENABLED = 0;
  postConfigure = "cp -r ${web} web/dist"; # not while fetching the modules
  meta.mainProgram = "oiko";
}
