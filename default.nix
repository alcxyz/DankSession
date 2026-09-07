{
  lib,
  buildGoModule,
  makeWrapper,
  systemd,
  version ? "dev",
}:
buildGoModule {
  pname = "danksession";
  inherit version;

  src = ./.;
  vendorHash = null;
  subPackages = ["cmd/danksession"];
  ldflags = ["-s" "-w" "-X main.version=${version}"];
  nativeBuildInputs = [makeWrapper];
  postInstall = ''
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
