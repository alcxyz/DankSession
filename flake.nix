{
  description = "Application session restoration plugin for DankMaterialShell";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = {
    self,
    nixpkgs,
    flake-utils,
  }:
    {
      homeManagerModules.default = import ./nix/home-manager.nix;
    }
    // flake-utils.lib.eachDefaultSystem (
      system: let
        pkgs = nixpkgs.legacyPackages.${system};
        version = (builtins.fromJSON (builtins.readFile ./plugin.json)).version;
      in {
        packages = rec {
          danksession = pkgs.callPackage ./default.nix {inherit version;};
          default = danksession;
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [go gopls gotools jq shellcheck];
        };
      }
    );
}
