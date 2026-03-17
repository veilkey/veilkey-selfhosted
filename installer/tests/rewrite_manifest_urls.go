package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func replaceOnce(text, old, new string) (string, error) {
	if !strings.Contains(text, old) {
		return text, fmt.Errorf("missing expected manifest fragment: %s", old)
	}
	return strings.Replace(text, old, new, 1), nil
}

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr, "usage: %s <manifest> <cli-artifact> <proxy-artifact>\n", filepath.Base(os.Args[0]))
		os.Exit(2)
	}

	manifestPath := os.Args[1]
	cliArtifact, err := filepath.Abs(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve cli artifact: %v\n", err)
		os.Exit(1)
	}
	proxyArtifact, err := filepath.Abs(os.Args[3])
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve proxy artifact: %v\n", err)
		os.Exit(1)
	}

	textBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read manifest: %v\n", err)
		os.Exit(1)
	}
	text := string(textBytes)

	replacements := [][2]string{
		{`ref = "RELEASE_OR_TAG"`, `ref = "local-test"`},
		{`artifact_url = "https://github.com/veilkey/veilkey-selfhosted/releases/download/RELEASE_OR_TAG/veilkey-cli_RELEASE_OR_TAG_linux_amd64.tar.gz"`, fmt.Sprintf(`artifact_url = "file://%s"`, cliArtifact)},
		{`artifact_filename = "veilkey-cli_RELEASE_OR_TAG_linux_amd64.tar.gz"`, `artifact_filename = "veilkey-cli.tar.gz"`},
		{`artifact_url = "https://your-gitlab-host/api/v4/projects/veilkey%2Fveilkey-proxy/repository/archive.tar.gz?sha=670d1e33736adab35149275428ed3aa75b4e787b"`, fmt.Sprintf(`artifact_url = "file://%s"`, proxyArtifact)},
		{`artifact_filename = "veilkey-proxy-670d1e33736adab35149275428ed3aa75b4e787b.tar.gz"`, `artifact_filename = "veilkey-proxy-local.tar.gz"`},
	}

	for _, pair := range replacements {
		text, err = replaceOnce(text, pair[0], pair[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(manifestPath, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write manifest: %v\n", err)
		os.Exit(1)
	}
}
