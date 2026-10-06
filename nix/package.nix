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
    if bridges == { } then "sha256-kSU8vaj/m1F8IMe2tX9yzcCEK2q1z5SWxcUJWQ1UxYk=" else lib.fakeHash,
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
    npmDepsHash = "sha256-Wb6sJ8dfSrQrmKf/IzI3WW8WlEfXuqwFKkVCUiGUa28=";
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
      ../bridge
      ../cmd
      ../internal
      ../web/embed.go
    ];
  };
  inherit vendorHash;
  # With other Bridges, go.mod gains their modules while fetching them, which
  # has the network, and again while building, from what was fetched: the
  # module cache is what gets vendored.
  proxyVendor = added;
  postPatch = lib.optionalString added ''
    cat > bridges.go <<EOF
    package oiko

    import (
    ${lib.concatMapStrings (p: "\t_ \"${p}\"\n") (lib.attrNames bridges)})
    EOF
  '';
  preBuild = lib.optionalString added "go get ${lib.escapeShellArgs (lib.mapAttrsToList (p: v: "${p}@${v}") bridges)}";
  subPackages = [ "cmd/oiko" ];
  env.CGO_ENABLED = 0;
  postConfigure = "cp -r ${web} web/dist"; # not while fetching the modules
  meta.mainProgram = "oiko";
}
