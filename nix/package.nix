{ lib, buildGoModule }:

buildGoModule {
  pname = "deployment-tracker";
  version = "1.0.0";
  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [ ../go.mod ../go.sum ../cmd ../internal ];
  };
  vendorHash = "sha256-IUyZTlMqht5sx010M3kc96okTBsRrL1e+6m1VVWvK0k=";
  subPackages = [ "cmd/api" ];
  env.CGO_ENABLED = "1";
  ldflags = [ "-s" "-w" ];
  checkPhase = ''
    runHook preCheck
    go test ./...
    runHook postCheck
  '';
  meta = {
    description = "Small Go/SQLite broken-deployment tracker";
    homepage = "https://github.com/zachpmanson/api";
    mainProgram = "api";
    platforms = lib.platforms.unix;
  };
}
