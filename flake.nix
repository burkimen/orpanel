{
  description = "Orpanel — OmniRoute icin capraz platform kontrol paneli";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
        version = "1.0.0";
        # Prebuilt tarball'dan kurulum (tercih) — eger kaynaktan derlemek istersen buildGoModule kullan
        orpanelBin =
          if system == "x86_64-linux" then pkgs.fetchurl {
            url = "https://github.com/burkimen/orpanel/releases/download/v${version}/orpanel-v${version}-linux-x64.tar.gz";
            sha256 = "REPLACE_LINUX_X64_SHA256";
          } else if system == "aarch64-darwin" then pkgs.fetchurl {
            url = "https://github.com/burkimen/orpanel/releases/download/v${version}/orpanel-v${version}-darwin-arm64.tar.gz";
            sha256 = "REPLACE_DARWIN_ARM64_SHA256";
          } else if system == "x86_64-darwin" then pkgs.fetchurl {
            url = "https://github.com/burkimen/orpanel/releases/download/v${version}/orpanel-v${version}-darwin-x64.tar.gz";
            sha256 = "REPLACE_DARWIN_X64_SHA256";
          } else throw "Desteklenmeyen system: ${system}";
      in {
        packages.default = pkgs.stdenv.mkDerivation {
          pname = "orpanel";
          inherit version;
          src = orpanelBin;
          dontUnpack = false;
          installPhase = ''
            mkdir -p $out/bin $out/share/orpanel
            # tar.gz icinde binary + themes/locales
            cp -r themes locales $out/share/orpanel/ 2>/dev/null || true
            cp app.ico icon.png $out/share/orpanel/ 2>/dev/null || true
            # binary'yi bul ve kur
            BIN=$(find . -maxdepth 3 -name "orpanel" -o -name "orPanel" | head -n1)
            install -m755 "$BIN" $out/bin/orpanel
          '';
          meta = with pkgs.lib; {
            description = "OmniRoute kontrol paneli";
            homepage = "https://github.com/burkimen/orpanel";
            license = licenses.mit;
            platforms = platforms.linux ++ platforms.darwin;
            mainProgram = "orpanel";
          };
        };

        # Kaynaktan derle (alternatif): nix build .#orpanel-from-source
        packages.orpanel-from-source = pkgs.buildGoModule {
          pname = "orpanel";
          inherit version;
          src = ./.;
          vendorHash = null; # go mod tidy sonrası nix hash ekle: nix run nixpkgs#nix-prefetch -- go mod download
          # r_windows.syso sadece windows'ta, nix'te ignore
        };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/orpanel";
        };
      });
}
