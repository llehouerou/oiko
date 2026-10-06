{
  description = "Oiko: local home automation platform";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs, ... }:
    let
      forAllSystems =
        f:
        nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" ] (
          system: f nixpkgs.legacyPackages.${system}
        );
    in
    {
      packages = forAllSystems (pkgs: {
        default = pkgs.callPackage ./nix/package.nix { };
      });

      # Deployment shape, consumed by the infrastructure repo: the service, its
      # user, state directory and flags. Placement (listen address, broker,
      # location) is set there.
      nixosModules.default = import ./nix/module.nix self;

      # The module's options render into config.json.
      checks = forAllSystems (pkgs: {
        module =
          let
            nixos = nixpkgs.lib.nixosSystem {
              inherit (pkgs.stdenv.hostPlatform) system;
              modules = [
                self.nixosModules.default
                {
                  services.oiko = {
                    enable = true;
                    mqtt = "mqtt://localhost:1883";
                    publicUrl = "https://oiko.example.org";
                  };
                  system.stateVersion = "25.11";
                }
              ];
            };
            configFile = nixos.config.environment.etc."oiko/config.json".source;
          in
          pkgs.runCommand "oiko-module" { nativeBuildInputs = [ pkgs.jq ]; } ''
            jq -e '.publicUrl == "https://oiko.example.org"' ${configFile}
            touch $out
          '';
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [
            pkgs.go_1_27
            pkgs.nodejs_24
            pkgs.mosquitto
            pkgs.gnumake
            pkgs.imagemagick # make icons
          ];
          # The browser test drives it (web/e2e).
          CHROMIUM = pkgs.lib.getExe pkgs.chromium;
        };
      });
    };
}
