# NixOS module for Oiko.
#
# Owns the deployment shape: the service, its user and state directory, and
# what the HomeKit controller needs from the network. Placement (listen
# address, broker, the home's location) comes from the host configuration.
self:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.oiko;
  json = pkgs.formats.json { };
  # Each Bridge keeps its state in /var/lib/oiko/<its name>.
  bridges = {
    zigbee2mqtt.broker = cfg.mqtt;
    homekit = { }; # follows what `oiko homekit pair` paired, if anything
  };
  configFile = json.generate "oiko-config.json" (
    cfg.settings
    // {
      bridges = bridges // (cfg.settings.bridges or { });
    }
    // lib.optionalAttrs (cfg.publicUrl != null) { inherit (cfg) publicUrl; }
  );
in
{
  options.services.oiko = {
    enable = lib.mkEnableOption "Oiko";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "oiko.packages.\${system}.default";
    };

    listen = lib.mkOption {
      type = lib.types.str;
      default = ":8080";
      description = "HTTP listen address. There is no authentication: keep it off untrusted networks.";
    };

    publicUrl = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "https://oiko.example.org";
      description = ''
        The Public URL, where users reach Oiko: an HTTPS origin,
        `https://host[:port]`, with no path. Sign-in needs it.
      '';
    };

    mqtt = lib.mkOption {
      type = lib.types.str;
      example = "mqtt://localhost:1883";
      description = "URL of the broker zigbee2mqtt publishes to.";
    };

    settings = lib.mkOption {
      inherit (json) type;
      default = { };
      example = {
        location = {
          latitude = 48.86;
          longitude = 2.35;
        };
        telegram = {
          tokenFile = "/run/credentials/oiko.service/telegram_token";
          chatIdFile = "/run/credentials/oiko.service/telegram_chat_id";
        };
      };
      description = ''
        Oiko's configuration (config.json): the home's location for the sun
        triggers, Telegram, and each Bridge by name, zigbee2mqtt and homekit
        being there already. A secret goes in {option}`credentials`, and the
        configuration names the file the service sees it as.
      '';
    };

    credentials = lib.mkOption {
      type = lib.types.attrsOf lib.types.str; # strings, so no secret lands in the store
      default = { };
      example = {
        telegram_token = "/run/secrets/telegram-token";
        telegram_chat_id = "/run/secrets/telegram-chat-id";
      };
      description = ''
        Files holding secrets, by name, that the service reads as
        /run/credentials/oiko.service/<name>, through systemd's LoadCredential:
        readable by the service only, whoever owns the files.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    # A fixed user, so `sudo -u oiko oiko -data /var/lib/oiko -config
    # /etc/oiko/config.json homekit pair <code>` writes the pairing where the
    # service reads it.
    environment.etc."oiko/config.json".source = configFile;
    users.users.oiko = {
      isSystemUser = true;
      group = "oiko";
      home = "/var/lib/oiko";
    };
    users.groups.oiko = { };

    environment.systemPackages = [ cfg.package ];

    # The HomeKit controller finds accessories over mDNS: it listens on 5353.
    networking.firewall.allowedUDPPorts = [ 5353 ];

    systemd.services.oiko = {
      description = "Oiko home automation";
      wantedBy = [ "multi-user.target" ];
      wants = [ "network-online.target" ];
      after = [ "network-online.target" ];
      environment.OIKO_INSTALL = "nixos"; # the update banner tells how to apply a Release here
      serviceConfig = {
        ExecStart = lib.escapeShellArgs [
          (lib.getExe cfg.package)
          "-listen"
          cfg.listen
          "-data"
          "/var/lib/oiko"
          "-config"
          configFile
        ];
        User = "oiko";
        Group = "oiko";
        StateDirectory = "oiko";
        StateDirectoryMode = "0700"; # it holds secrets
        Restart = "on-failure";
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        NoNewPrivileges = true;
        LoadCredential = lib.mapAttrsToList (name: file: "${name}:${file}") cfg.credentials;
      };
    };
  };
}
