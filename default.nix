{ lib, buildGoModule, python3, makeWrapper, systemd
, version ? (builtins.fromJSON (builtins.readFile ./plugin.json)).version
, revision ? null, release ? false }:
let
  metadata = import ./build-metadata.nix { inherit lib version revision release; };
in
buildGoModule {
  pname = "danksession";
  version = metadata.version;
  src = metadata.source;
  vendorHash = null;
  subPackages = [ "cmd/danksession" ];
  ldflags = [ "-s" "-w" "-X main.version=${metadata.version}" ];
  nativeBuildInputs = [ python3 makeWrapper ];
  postInstall = ''
    python3 scripts/package.py --stage-only --output "$out" \
      --revision ${lib.escapeShellArg metadata.revision} ${lib.optionalString release "--release"}
    wrapProgram "$out/bin/danksession" --suffix PATH : ${lib.makeBinPath [systemd]}
  '';
  meta = {
    description = "DankMaterialShell session restoration backend";
    homepage = "https://github.com/alcxyz/DankSession";
    license = lib.licenses.mit;
    mainProgram = "danksession";
    platforms = lib.platforms.linux;
  };
}
