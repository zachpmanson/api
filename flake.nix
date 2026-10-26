{
  description = "Small Go/SQLite deployment tracker";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let pkgs = nixpkgs.legacyPackages.${system};
      in {
        packages.default = pkgs.callPackage ./nix/package.nix {};
        checks = {
          package = self.packages.${system}.default;
        } // pkgs.lib.optionalAttrs pkgs.stdenv.isLinux {
          module = import ./nix/module-check.nix { inherit pkgs nixpkgs self; };
        };
        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gcc pkgs.sqlite pkgs.pkg-config ];
          CGO_ENABLED = "1";
        };
      }) // {
        nixosModules.default = import ./nix/module.nix self;
      };
}
