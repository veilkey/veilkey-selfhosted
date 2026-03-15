package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

type manifest struct {
	Release    releaseConfig              `toml:"release"`
	Components map[string]componentConfig `toml:"components"`
	Profiles   map[string]profileConfig   `toml:"profiles"`
}

type releaseConfig struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
	Channel string `toml:"channel"`
}

type componentConfig struct {
	Source            string   `toml:"source"`
	Project           string   `toml:"project"`
	Ref               string   `toml:"ref"`
	Type              string   `toml:"type"`
	InstallOrder      int      `toml:"install_order"`
	ArtifactURL       string   `toml:"artifact_url"`
	ArtifactFilename  string   `toml:"artifact_filename"`
	SHA256            string   `toml:"sha256"`
	StageAssets       []string `toml:"stage_assets"`
	PostInstallVerify []string `toml:"post_install_verify"`
}

type profileConfig struct {
	Description string   `toml:"description"`
	Components  []string `toml:"components"`
}

func loadManifest(path string) (*manifest, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("manifest not found: %s", path)
		}
		return nil, err
	}

	var data manifest
	if _, err := toml.DecodeFile(path, &data); err != nil {
		return nil, err
	}
	return &data, nil
}

func validateManifest(data *manifest) error {
	if data.Release.Name == "" || data.Release.Version == "" || data.Release.Channel == "" {
		return errors.New("release missing required fields")
	}
	if len(data.Components) == 0 {
		return errors.New("missing [components]")
	}
	for name, component := range data.Components {
		if component.Source == "" || component.Project == "" || component.Ref == "" || component.Type == "" || component.InstallOrder == 0 {
			return fmt.Errorf("component %s missing required fields", name)
		}
	}
	for profileName, profile := range data.Profiles {
		if len(profile.Components) == 0 {
			return fmt.Errorf("profile %s missing components list", profileName)
		}
		for _, componentName := range profile.Components {
			if _, ok := data.Components[componentName]; !ok {
				return fmt.Errorf("profile %s references unknown component: %s", profileName, componentName)
			}
		}
	}
	return nil
}

type namedComponent struct {
	Name      string
	Component componentConfig
}

func sortedComponents(data *manifest, selected []string) ([]namedComponent, error) {
	names := selected
	if len(names) == 0 {
		names = make([]string, 0, len(data.Components))
		for name := range data.Components {
			names = append(names, name)
		}
	}

	result := make([]namedComponent, 0, len(names))
	for _, name := range names {
		component, ok := data.Components[name]
		if !ok {
			return nil, fmt.Errorf("unknown component: %s", name)
		}
		result = append(result, namedComponent{Name: name, Component: component})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Component.InstallOrder == result[j].Component.InstallOrder {
			return result[i].Name < result[j].Name
		}
		return result[i].Component.InstallOrder < result[j].Component.InstallOrder
	})
	return result, nil
}

func artifactFilenameFor(name string, component componentConfig) string {
	if component.ArtifactFilename != "" {
		return component.ArtifactFilename
	}
	artifactURL := component.ArtifactURL
	if idx := strings.IndexByte(artifactURL, '?'); idx >= 0 {
		artifactURL = artifactURL[:idx]
	}
	if artifactURL != "" {
		parts := strings.Split(artifactURL, "/")
		candidate := parts[len(parts)-1]
		if candidate != "" {
			return candidate
		}
	}
	return name + ".artifact"
}

func encodeListField(values []string) string {
	return strings.Join(values, ",")
}

func printPlan(data *manifest, profile string) error {
	cfg, ok := data.Profiles[profile]
	if !ok {
		return fmt.Errorf("unknown profile: %s", profile)
	}
	components, err := sortedComponents(data, cfg.Components)
	if err != nil {
		return err
	}
	fmt.Printf("[profile] %s\n", profile)
	for _, item := range components {
		fmt.Printf("%3d %-12s %s@%s\n", item.Component.InstallOrder, item.Name, item.Component.Project, item.Component.Ref)
	}
	return nil
}

func printDownloadPlan(data *manifest, profile string) error {
	cfg, ok := data.Profiles[profile]
	if !ok {
		return fmt.Errorf("unknown profile: %s", profile)
	}
	components, err := sortedComponents(data, cfg.Components)
	if err != nil {
		return err
	}
	fmt.Printf("[profile] %s\n", profile)
	for _, item := range components {
		if item.Component.ArtifactURL == "" {
			return fmt.Errorf("component %s missing artifact_url for download plan", item.Name)
		}
		sha256 := item.Component.SHA256
		if sha256 == "" {
			sha256 = "none"
		}
		fmt.Printf("%3d %-12s %s %s %s\n",
			item.Component.InstallOrder,
			item.Name,
			artifactFilenameFor(item.Name, item.Component),
			item.Component.ArtifactURL,
			sha256,
		)
	}
	return nil
}

func printStagePlan(data *manifest, profile string) error {
	cfg, ok := data.Profiles[profile]
	if !ok {
		return fmt.Errorf("unknown profile: %s", profile)
	}
	components, err := sortedComponents(data, cfg.Components)
	if err != nil {
		return err
	}
	fmt.Printf("release_name=%s\n", data.Release.Name)
	fmt.Printf("release_version=%s\n", data.Release.Version)
	fmt.Printf("release_channel=%s\n", data.Release.Channel)
	fmt.Printf("profile=%s\n", profile)
	for _, item := range components {
		fmt.Printf("component=%s;project=%s;ref=%s;type=%s;install_order=%d;artifact_url=%s;artifact_filename=%s;sha256=%s;stage_assets=%s;post_install_verify=%s\n",
			item.Name,
			item.Component.Project,
			item.Component.Ref,
			item.Component.Type,
			item.Component.InstallOrder,
			item.Component.ArtifactURL,
			artifactFilenameFor(item.Name, item.Component),
			item.Component.SHA256,
			encodeListField(item.Component.StageAssets),
			encodeListField(item.Component.PostInstallVerify),
		)
	}
	return nil
}

func printComponents(data *manifest) error {
	components, err := sortedComponents(data, nil)
	if err != nil {
		return err
	}
	for _, item := range components {
		fmt.Printf("%s\t%s\t%s\t%s\t%d\n", item.Name, item.Component.Project, item.Component.Ref, item.Component.Type, item.Component.InstallOrder)
	}
	return nil
}

func printProfiles(data *manifest) {
	names := make([]string, 0, len(data.Profiles))
	for name := range data.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%s\t%s\n", name, data.Profiles[name].Description)
	}
}

func lintLegacyLayout(manifestPath string) {
	root := filepath.Dir(manifestPath)
	legacy := make([]string, 0, 2)
	for _, name := range []string{"veilkey-keycenter", "veilkey-localvault"} {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			legacy = append(legacy, name)
		}
	}
	if len(legacy) == 0 {
		fmt.Println("no legacy component directories found")
		return
	}
	fmt.Println("legacy component directories still present:")
	for _, name := range legacy {
		fmt.Printf("- %s\n", name)
	}
}

func main() {
	manifestPath := flag.String("manifest", "", "manifest path")
	flag.Parse()

	if *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "Error: --manifest is required")
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Error: command is required")
		os.Exit(1)
	}

	data, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := validateManifest(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "validate":
		fmt.Println("ok")
	case "list-components":
		if err := printComponents(data); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "list-profiles":
		printProfiles(data)
	case "lint-legacy-layout":
		lintLegacyLayout(*manifestPath)
	case "plan":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: profile is required")
			os.Exit(1)
		}
		if err := printPlan(data, args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "plan-download":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: profile is required")
			os.Exit(1)
		}
		if err := printDownloadPlan(data, args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "plan-stage":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Error: profile is required")
			os.Exit(1)
		}
		if err := printStagePlan(data, args[1]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "print-json":
		out, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", args[0])
		os.Exit(1)
	}
}
