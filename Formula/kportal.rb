class Kportal < Formula
  desc "Modern Kubernetes port-forward manager with interactive TUI"
  homepage "https://lukaszraczylo.github.io/kportal"
  license "MIT"

  # Version will be dynamically set by bump-homebrew-formula-action
  # This is a template - actual releases will have specific version and URLs
  version "0.1.5"

  on_macos do
    on_arm do
      url "https://github.com/lukaszraczylo/kportal/releases/download/v#{version}/kportal-#{version}-darwin-arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_ARM64"
    end

    on_intel do
      url "https://github.com/lukaszraczylo/kportal/releases/download/v#{version}/kportal-#{version}-darwin-amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/lukaszraczylo/kportal/releases/download/v#{version}/kportal-#{version}-linux-arm64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_ARM64"
    end

    on_intel do
      url "https://github.com/lukaszraczylo/kportal/releases/download/v#{version}/kportal-#{version}-linux-amd64.tar.gz"
      sha256 "PLACEHOLDER_SHA256_LINUX_AMD64"
    end
  end

  # Optional dependency - kubectl is commonly already installed
  # but kportal requires it to function
  depends_on "kubernetes-cli" => :optional

  def install
    bin.install "kportal"

    # Generate shell completions if the binary supports it
    # This will be implemented in future versions
    # generate_completions_from_executable(bin/"kportal", "completion")
  end

  def caveats
    <<~EOS
      kportal requires:
        • kubectl installed and configured
        • Access to a Kubernetes cluster
        • A valid kubeconfig file (~/.kube/config)

      Quick start:
        1. Create a configuration file: .kportal.yaml
        2. Add your port-forward definitions
        3. Run: kportal

      For configuration examples and full documentation:
        https://lukaszraczylo.github.io/kportal

      To validate your configuration:
        kportal --check
    EOS
  end

  test do
    # Test that binary runs and reports correct version
    assert_match version.to_s, shell_output("#{bin}/kportal --version")

    # Test that binary can validate an empty config (should fail gracefully)
    (testpath/".kportal.yaml").write <<~YAML
      contexts:
        test:
          namespaces:
            default:
              - resource: test-pod
                port: 8080
                local_port: 8080
    YAML

    # Should be able to validate config even without kube access
    system bin/"kportal", "--check", "-c", testpath/".kportal.yaml"
  end
end
