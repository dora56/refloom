class Refloom < Formula
  desc "Local-first reading support RAG tool for PDF/EPUB"
  homepage "https://github.com/dora56/refloom"
  version "0.2.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/dora56/refloom/releases/download/v#{version}/refloom-v#{version}-darwin-arm64.zip"
      # sha256 will be filled after release
      sha256 "PLACEHOLDER"
    end
  end

  depends_on "uv" => :build
  depends_on :macos

  def install
    bin.install "refloom"

    # Install Python worker alongside the binary
    (libexec/"python"/"refloom_worker").install Dir["python/refloom_worker/*.py"]
    (libexec/"python"/"refloom_worker").install "python/refloom_worker/pyproject.toml" if File.exist?("python/refloom_worker/pyproject.toml")

    # Copy pyproject.toml for uv sync
    (libexec/"python").install "python/pyproject.toml" if File.exist?("python/pyproject.toml")
  end

  def post_install
    # Setup Python venv with uv
    cd libexec/"python" do
      system "uv", "sync"
    end
  end

  def caveats
    <<~EOS
      Refloom requires Ollama for embedding generation:
        brew install ollama
        ollama pull nomic-embed-text

      Run health check:
        refloom doctor

      Configuration: ~/.refloom/config.yaml
      Data: ~/.refloom/refloom.db
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/refloom version")
  end
end
