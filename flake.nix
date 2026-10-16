{
  description = "kena: self-hosted web app backed by Postgres";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = ["aarch64-darwin" "x86_64-darwin" "aarch64-linux" "x86_64-linux"];
    forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    kena = pkgs: pkgs.callPackage ./nix/package.nix {version = "0.1.0-${self.shortRev or "dirty"}";};
  in {
    packages = forAllSystems (pkgs: {
      default = kena pkgs;
      kena = kena pkgs;
    });

    overlays.default = final: _: {kena = kena final;};

    nixosModules.default = import ./nix/module.nix self;

    devShells = forAllSystems (pkgs: {
      default = pkgs.mkShell {
        inputsFrom = [(kena pkgs)];
        packages = with pkgs; [go gopls gotools postgresql];
      };
    });

    # A full NixOS VM test for the module is too heavy for `nix flake check`;
    # building the package stands in for it here.
    checks = nixpkgs.lib.genAttrs ["x86_64-linux" "aarch64-linux"] (system: {
      module = kena nixpkgs.legacyPackages.${system};
    });

    formatter = forAllSystems (pkgs: pkgs.alejandra);
  };
}
