self: {
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.services.kena;
in {
  options.services.kena = {
    enable = lib.mkEnableOption "kena";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      defaultText = lib.literalExpression "kena.packages.\${system}.default";
    };

    publicUrl = lib.mkOption {
      type = lib.types.str;
      example = "https://kena.example.org";
      description = "Public base URL kena is reachable at (KENA_PUBLIC_URL).";
    };

    listen = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1:8080";
      description = "Address kena listens on (KENA_LISTEN).";
    };

    oidc = {
      issuer = lib.mkOption {
        type = lib.types.nullOr lib.types.str;
        default = null;
        example = "https://id.example.org/oauth2/openid/kena";
        description = "OIDC issuer for single sign-on (KENA_OIDC_ISSUER); null keeps SSO off. The client secret comes as KENA_OIDC_CLIENT_SECRET through environmentFiles.";
      };

      clientId = lib.mkOption {
        type = lib.types.str;
        default = "kena";
        description = "OIDC client id (KENA_OIDC_CLIENT_ID).";
      };

      name = lib.mkOption {
        type = lib.types.str;
        default = "SSO";
        description = "Shown on the login button as \"Log in with <name>\" (KENA_OIDC_NAME).";
      };
    };

    environmentFiles = lib.mkOption {
      type = lib.types.listOf lib.types.path;
      default = [];
      description = "systemd EnvironmentFile entries, e.g. one holding KENA_OIDC_CLIENT_SECRET.";
    };

    database = {
      createLocally = lib.mkOption {
        type = lib.types.bool;
        default = true;
        description = "Create a local Postgres database and role for kena.";
      };

      url = lib.mkOption {
        type = lib.types.str;
        default = "postgresql:///kena?host=/run/postgresql";
        description = "KENA_DATABASE_URL. Defaults to the local-socket URL used by database.createLocally.";
      };
    };
  };

  config = lib.mkIf cfg.enable {
    users.users.kena = {
      isSystemUser = true;
      group = "kena";
    };
    users.groups.kena = {};

    services.postgresql = lib.mkIf cfg.database.createLocally {
      enable = true;
      ensureDatabases = ["kena"];
      ensureUsers = [
        {
          name = "kena";
          ensureDBOwnership = true;
        }
      ];
    };

    systemd.services.kena = {
      description = "kena";
      wantedBy = ["multi-user.target"];
      after = ["network.target"] ++ lib.optional cfg.database.createLocally "postgresql.target";
      requires = lib.optional cfg.database.createLocally "postgresql.target";

      environment =
        {
          KENA_DATABASE_URL = cfg.database.url;
          KENA_LISTEN = cfg.listen;
          KENA_PUBLIC_URL = cfg.publicUrl;
        }
        // lib.optionalAttrs (cfg.oidc.issuer != null) {
          KENA_OIDC_ISSUER = cfg.oidc.issuer;
          KENA_OIDC_CLIENT_ID = cfg.oidc.clientId;
          KENA_OIDC_NAME = cfg.oidc.name;
        };

      serviceConfig = {
        ExecStart = "${cfg.package}/bin/kena serve";
        EnvironmentFile = cfg.environmentFiles;
        User = "kena";
        Group = "kena";
        Restart = "on-failure";
        RestartSec = 5;

        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = ["AF_UNIX" "AF_INET" "AF_INET6"];
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        RestrictNamespaces = true;
        SystemCallArchitectures = "native";
        CapabilityBoundingSet = "";
      };
    };

    environment.systemPackages = [
      (pkgs.writeShellScriptBin "kena-admin" ''
        set -euo pipefail
        if [ "$(id -u)" = 0 ]; then
          exec ${pkgs.util-linux}/bin/runuser -u kena -- \
            ${pkgs.coreutils}/bin/env KENA_DATABASE_URL=${lib.escapeShellArg cfg.database.url} \
                KENA_LISTEN=${lib.escapeShellArg cfg.listen} \
                KENA_PUBLIC_URL=${lib.escapeShellArg cfg.publicUrl} \
                ${cfg.package}/bin/kena "$@"
        else
          exec sudo -u kena \
            ${pkgs.coreutils}/bin/env KENA_DATABASE_URL=${lib.escapeShellArg cfg.database.url} \
                KENA_LISTEN=${lib.escapeShellArg cfg.listen} \
                KENA_PUBLIC_URL=${lib.escapeShellArg cfg.publicUrl} \
                ${cfg.package}/bin/kena "$@"
        fi
      '')
    ];
  };
}
