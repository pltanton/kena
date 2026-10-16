{
  lib,
  buildGoModule,
  postgresql,
  version ? "dev",
}:
buildGoModule {
  pname = "kena";
  inherit version;

  src = lib.fileset.toSource {
    root = ../.;
    fileset = lib.fileset.unions [
      ../go.mod
      ../go.sum
      ../cmd
      ../internal
    ];
  };

  vendorHash = "sha256-MdFVeLEzkO+JGkYhJUX2oHq4hom6Bd5HRjMyu3/d2iY=";

  subPackages = ["cmd/kena"];

  nativeCheckInputs = [postgresql];

  # subPackages restricts the default checkPhase to cmd/kena; run the full suite instead.
  checkPhase = ''
    runHook preCheck
    export GOFLAGS=''${GOFLAGS//-trimpath/}
    go test ./...
    runHook postCheck
  '';

  ldflags = ["-s" "-w" "-X main.version=${version}"];

  meta = {
    description = "kena: self-hosted web app backed by Postgres";
    mainProgram = "kena";
  };
}
