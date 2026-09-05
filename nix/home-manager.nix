{
  config,
  lib,
  pkgs,
  ...
}: let
  cfg = config.services.dankSession;
in {
  options.services.dankSession = {
    enable = lib.mkEnableOption "DankSession desktop restoration";

    package = lib.mkOption {
      type = lib.types.package;
      description = "DankSession package to install and run.";
    };

    autoStart = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Start the capture daemon with the graphical session.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [cfg.package];

    systemd.user.services.danksession = lib.mkIf cfg.autoStart {
      Unit = {
        Description = "Capture and restore the DankSession desktop state";
        PartOf = ["graphical-session.target"];
        After = ["graphical-session.target"];
      };
      Service = {
        ExecStart = "${cfg.package}/bin/danksession daemon";
        Restart = "on-failure";
        RestartSec = 2;
      };
      Install.WantedBy = ["graphical-session.target"];
    };
  };
}
