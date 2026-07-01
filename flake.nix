{
  description = "revdiff - TUI for reviewing diffs, files, and documents with inline annotations";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { self
    , nixpkgs
    , flake-utils
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = self.shortRev or self.dirtyShortRev or "dev";
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "revdiff";
          inherit version;

          src = ./.;

          # The repository vendors all dependencies (vendor/ is committed),
          # so build straight from the vendored tree with no network fetch.
          vendorHash = null;

          subPackages = [ "app" ];

          # Tests need the git working tree, which is absent in the Nix sandbox.
          doCheck = false;

          # The project builds with cgo disabled.
          env.CGO_ENABLED = 0;

          # Mirror the Makefile's ldflags, injecting the revision into
          # `var revision` in package main.
          ldflags = [
            "-s"
            "-w"
            "-X main.revision=${version}"
          ];

          # The main package lives in ./app, so the produced binary is named
          # `app`; rename it to `revdiff`.
          postInstall = ''
            mv $out/bin/app $out/bin/revdiff
          '';

          meta = {
            description = "TUI for reviewing diffs, files, and documents with inline annotations";
            homepage = "https://github.com/umputun/revdiff";
            license = pkgs.lib.licenses.mit;
            mainProgram = "revdiff";
          };
        };

        apps.default = flake-utils.lib.mkApp {
          drv = self.packages.${system}.default;
        };

        devShells.default = pkgs.mkShell {
          packages = [ pkgs.go ];
        };
      }
    );
}
