{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    wayland
    xorg.libX11
    xorg.libxcb
  ];

  # Make libraries available at runtime for purego
  shellHook = ''
    export LD_LIBRARY_PATH=${pkgs.lib.makeLibraryPath [
      pkgs.wayland
      pkgs.xorg.libX11
      pkgs.xorg.libxcb
    ]}:$LD_LIBRARY_PATH

    echo "NixOS environment ready!"
    echo "Build with: go build"
    echo "Run with: go run ."
  '';
}
