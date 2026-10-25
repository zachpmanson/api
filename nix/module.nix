self:
{ config, lib, pkgs, ... }:
let
  cfg = config.services.deployment-tracker;
in {
  options.services.deployment-tracker = {
    enable = lib.mkEnableOption "the Go/SQLite deployment tracker";
    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
      description = "Deployment tracker package.";
    };
    listenAddress = lib.mkOption {
      type = lib.types.str;
      default = "127.0.0.1:3000";
      description = "HTTP address, reachable only from localhost by the service sandbox.";
    };
    environmentFile = lib.mkOption {
      type = lib.types.str;
      description = ''
        Absolute runtime path to a root-owned systemd EnvironmentFile containing
        PROD_BROKE_KEY. Use an agenix secret path (not a Nix path literal or
        builtins.readFile). The file must not be in the Nix store.
      '';
      example = "/run/agenix/api-prod-broke";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [{
      assertion = lib.hasPrefix "/" cfg.environmentFile && !(lib.hasPrefix builtins.storeDir cfg.environmentFile);
      message = "deployment-tracker.environmentFile must be an absolute runtime path outside /nix/store";
    }];
    users.groups.deployment-tracker = {};
    users.users.deployment-tracker = {
      isSystemUser = true;
      group = "deployment-tracker";
      home = "/var/lib/deployment-tracker";
    };
    systemd.services.deployment-tracker = {
      description = "Go/SQLite deployment tracker";
      wantedBy = [ "multi-user.target" ];
      after = [ "network.target" ];
      environment = {
        API_LISTEN = cfg.listenAddress;
        API_DB_PATH = "/var/lib/deployment-tracker/history.sqlite";
      };
      serviceConfig = {
        ExecStart = "${cfg.package}/bin/api serve";
        User = "deployment-tracker";
        Group = "deployment-tracker";
        StateDirectory = "deployment-tracker";
        StateDirectoryMode = "0700";
        WorkingDirectory = "/var/lib/deployment-tracker";
        EnvironmentFile = cfg.environmentFile;
        UMask = "0077";
        Restart = "on-failure";
        RestartSec = "5s";
        TimeoutStopSec = "15s";
        NoNewPrivileges = true;
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;
        ProtectClock = true;
        ProtectHostname = true;
        ProtectProc = "invisible";
        ProcSubset = "pid";
        RestrictAddressFamilies = [ "AF_UNIX" "AF_INET" "AF_INET6" ];
        IPAddressDeny = "any";
        IPAddressAllow = "localhost";
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        CapabilityBoundingSet = "";
        SystemCallArchitectures = "native";
        SystemCallFilter = [ "@system-service" ];
      };
    };
  };
}
