{ pkgs, nixpkgs, self }:
let
  makeConfig = environmentFile: (nixpkgs.lib.nixosSystem {
    system = pkgs.stdenv.hostPlatform.system;
    modules = [
      self.nixosModules.default
      {
        system.stateVersion = "26.05";
        boot.isContainer = true;
        services.deployment-tracker = {
          enable = true;
          inherit environmentFile;
        };
      }
    ];
  }).config;
  config = makeConfig "/run/agenix/api-prod-broke";
  service = config.systemd.services.deployment-tracker;
  validAssertions = c: builtins.all (a: a.assertion) c.assertions;
in
assert validAssertions config;
assert !(validAssertions (makeConfig "/nix/store/not-a-secret"));
assert !(validAssertions (makeConfig "relative.env"));
assert config.users.users.deployment-tracker.isSystemUser;
assert service.environment.API_LISTEN == "127.0.0.1:3000";
assert service.environment.API_DB_PATH == "/var/lib/deployment-tracker/history.sqlite";
assert service.serviceConfig.StateDirectory == "deployment-tracker";
assert service.serviceConfig.EnvironmentFile == "/run/agenix/api-prod-broke";
assert service.serviceConfig.ProtectSystem == "strict";
assert service.serviceConfig.User == "deployment-tracker";
pkgs.runCommand "deployment-tracker-module-check" {} ''
  touch "$out"
''
