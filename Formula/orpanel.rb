class Orpanel < Formula
  desc "OmniRoute icin capraz platform masaustu kontrol paneli — sistem tepsisi, yerlesik terminal"
  homepage "https://github.com/burkimen/orpanel"
  license "MIT"
  version "1.0.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/burkimen/orpanel/releases/download/v1.0.0/orpanel-v1.0.0-darwin-arm64.tar.gz"
      sha256 "REPLACE_DARWIN_ARM64_SHA256"
    else
      url "https://github.com/burkimen/orpanel/releases/download/v1.0.0/orpanel-v1.0.0-darwin-x64.tar.gz"
      sha256 "REPLACE_DARWIN_X64_SHA256"
    end
  end

  on_linux do
    url "https://github.com/burkimen/orpanel/releases/download/v1.0.0/orpanel-v1.0.0-linux-x64.tar.gz"
    sha256 "REPLACE_LINUX_X64_SHA256"
  end

  def install
    bin.install Dir["orpanel", "orPanel"].first => "orpanel"
    # themes/locales yaninda kalsin, share'a da kopyala (XDG uyumlu)
    pkgshare.install "themes", "locales" if File.exist?("themes")
    (pkgshare/"app.ico").install "app.ico" if File.exist?("app.ico")
    (pkgshare/"icon.png").install "icon.png" if File.exist?("icon.png")
  end

  def post_install
    # XDG dirs are created at runtime by orpanel, no need here
    ohai "orpanel kuruldu — calistir: orpanel"
  end

  test do
    assert_match "orpanel", shell_output("#{bin}/orpanel --help 2>&1 || true")
  end
end
